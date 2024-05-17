package main

import (
	"context"
	"crypto/sha256"
	"diploma/analytics-exporter/internal/analytics"
	"diploma/analytics-exporter/internal/database"
	"diploma/analytics-exporter/internal/grpcwrap"
	"diploma/analytics-exporter/internal/prometheus"
	analyticsApi "diploma/analytics-exporter/pkg/api/analytics"
	"encoding/hex"
	"fmt"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// Configuration keys as constants
const (
	configKeyBindAddr       string = "bind-addr"
	configKeyDebug          string = "debug"
	configKeyMock           string = "mock"
	configKeyGRPCPort       string = "rpc-port"
	configKeyGWPort         string = "gw-port"
	configKeyMPort          string = "metric-port"
	configKeyUseMemDB       string = "use-memdb"
	configKeyMetricsTimeout string = "metric-timeout"
	configKeyDomains        string = "domains"
)

type cli struct {
	cfg struct {
		debug    bool
		bindAddr string
		grpcPort uint16
		gwPort   uint16
		mPort    uint16
	}
	metricsTimeout int64
	useMemDB       bool
	domains        []string
	mockData       bool
}

// run is the actual work function that configures and starts all components.
func (c *cli) run(_ *cobra.Command, _ []string) error {
	// TODO: add auto cleaning of the records that are stored longer then 24h
	// Setup logger
	loggerConfig := zap.NewProductionConfig()
	if c.cfg.debug {
		loggerConfig = zap.NewDevelopmentConfig()
	}
	l, err := loggerConfig.Build()
	if err != nil {
		panic(err)
	}
	zap.ReplaceGlobals(l)

	// Log build info and command line arguments
	l.Info("Runtime info", zap.Int("pid", os.Getpid()), zap.Strings("args", os.Args))
	l.Info("Configuration info", zap.Any("config", c.cfg))

	// Initialise database
	db, err := database.NewDatabase(c.useMemDB)
	if err != nil {
		l.Fatal("cannot create db client", zap.Error(err))
	}

	// Mock the data
	if c.mockData {
		go func() {
			initialTime := time.Now()
			for {
				for _, md := range GetMockData(initialTime) {
					time.Sleep(time.Duration(rand.Intn(15)) * time.Second)
					if err = db.Insert(context.Background(), md); err != nil {
						l.Fatal("failed to mock the data", zap.Error(err))
					}
				}
				initialTime = initialTime.Add(time.Duration(3)*time.Hour + time.Duration(20)*time.Minute)
				l.Info("data is mocked")
				time.Sleep(time.Duration(rand.Intn(5)) * time.Minute)
			}
		}()
	}

	// Initialise gRPC server wrapper
	g, err := grpcwrap.NewServer()
	if err != nil {
		return fmt.Errorf("cannot create grpcwrap instance: %w", err)
	}
	defer func() {
		g.Shutdown()
	}()

	// Initialise analytics
	if err = analytics.New(g.GRPCServer, db); err != nil {
		return fmt.Errorf("cannot create catalog instance: %w", err)
	}
	// Start gRPC server
	var lis net.Listener
	bindGRPCAddr := c.cfg.bindAddr + ":" + strconv.Itoa(int(c.cfg.grpcPort))
	lis, err = net.Listen("tcp", bindGRPCAddr)
	if err != nil {
		return fmt.Errorf("cannot create grpc network listener: %w", err)
	}
	go func() {
		if err = g.GRPCServer.Serve(lis); err != nil {
			l.Fatal("Cannot serve incoming connections on the listener", zap.Error(err))
		}
	}()
	l.Info("gRPC server started", zap.String("address", bindGRPCAddr))

	// Initialize gRPC HTTP Gateway server
	bindGWAddr := c.cfg.bindAddr + ":" + strconv.Itoa(int(c.cfg.gwPort))
	gwServer, err := grpcwrap.NewGatewayServer(bindGWAddr, bindGRPCAddr, false)
	if err != nil {
		l.Fatal("Cannot create the gateway server", zap.Error(err))
	}
	defer func() {
		if err = gwServer.Shutdown(context.Background()); err != nil {
			l.Fatal("Cannot shutdown the http server", zap.Error(err))
		}
	}()

	// Start gRPC HTTP Gateway server
	go func() {
		if err = gwServer.ListenAndServe(); err != nil {
			l.Fatal("Cannot serve incoming traffic to the gateway", zap.Error(err))
		}
	}()
	l.Info("Gateway server started", zap.String("address", bindGWAddr))

	// Initialize prometheus server with its metrics
	bindMAddr := c.cfg.bindAddr + ":" + strconv.Itoa(int(c.cfg.mPort))
	prom, err := prometheus.NewPrometheus(db, bindMAddr, c.domains)
	if err != nil {
		l.Fatal("Cannot create the prometheus instance", zap.Error(err))
	}
	defer func() {
		if err = prom.HTTPServer.Shutdown(context.Background()); err != nil {
			l.Fatal("Cannot shutdown the http server", zap.Error(err))
		}
	}()

	// Run prometheus metrics HTTP server
	go func() {
		if err = prom.HTTPServer.ListenAndServe(); err != nil {
			l.Fatal("Cannot serve metrics endpoint", zap.Error(err))
		}
	}()
	l.Info("Metrics server started", zap.String("address", bindMAddr))

	// Wait for the shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	// Shutdown
	l.Info("Shutting down: "+sig.String(), zap.Int("signal", int(sig.(syscall.Signal))))

	return nil
}

func (c *cli) setupConfig(_ *cobra.Command, _ []string) {
	c.cfg.bindAddr = viper.GetString(configKeyBindAddr)
	c.cfg.debug = viper.GetBool(configKeyDebug)
	c.cfg.grpcPort = viper.GetUint16(configKeyGRPCPort)
	c.cfg.gwPort = viper.GetUint16(configKeyGWPort)
	c.cfg.mPort = viper.GetUint16(configKeyMPort)
	c.useMemDB = viper.GetBool(configKeyUseMemDB)
	c.mockData = viper.GetBool(configKeyMock)
	c.metricsTimeout = viper.GetInt64(configKeyMetricsTimeout)
	c.domains = viper.GetStringSlice(configKeyDomains)
}

var (
	Domain    = "web"
	EventType = "pageview"
	URL       = "mockData.example.com"
	OSs       = []string{
		"Windows",
		"Windows Phone",
		"Android",
		"macOS",
		"iOS",
		"Linux",
		"FreeBSD",
		"ChromeOS",
		"BlackBerry",
	}
	Browsers = []string{
		"Opera",
		"Opera Mini",
		"Opera Touch",
		"Chrome",
		"Headless Chrome",
		"Firefox",
		"Internet Explorer",
		"Safari",
		"Edge",
		"Vivaldi",
	}
	Referrers = []string{
		"",
		"https://www.google.com",
		"https://ua.linkedin.com",
		"https://www.bing.com",
		"https://yahoo.com",
	}
	Paths = []string{
		"/",
		"/foo",
		"/bar",
		"/foo/bar",
		"/bar/foo",
	}
	Devices = []*analyticsApi.Device{
		{
			Device: &analyticsApi.Device_Mobile{
				Mobile: true,
			},
		},
		{
			Device: &analyticsApi.Device_Tablet{
				Tablet: true,
			},
		},
		{
			Device: &analyticsApi.Device_Desktop{
				Desktop: true,
			},
		},
		{
			Device: &analyticsApi.Device_Bot{
				Bot: true,
			},
		},
	}
	H = sha256.New()
)

func GetMockData(initialTime time.Time) []*analyticsApi.Event {
	mockData := make([]*analyticsApi.Event, 0)

	type visitor struct {
		IP      string
		Browser string
		OS      string
		Device  *analyticsApi.Device
	}
	ipCidr := "132.14.53."
	// generate from 1 to 10 visitors
	totalVisitors := rand.Intn(10) + 1
	visitors := make([]visitor, totalVisitors)
	for i := 0; i < totalVisitors; i++ {
		visitors[i] = visitor{
			IP:      ipCidr + strconv.Itoa(rand.Intn(255)+1),
			Browser: Browsers[rand.Intn(len(Browsers))],
			OS:      OSs[rand.Intn(len(OSs))],
			Device:  Devices[rand.Intn(len(Devices))],
		}

		pageViewsAmount := rand.Intn(5) + 1

		timeShift := 0
		for j := 0; j < pageViewsAmount; j++ {
			H.Write([]byte(visitors[i].IP + visitors[i].Browser + visitors[i].Device.String()))
			visitHashValue := H.Sum(nil)

			var referrer string
			if rand.Intn(2) == 1 {
				referrer = URL
			} else {
				referrer = Referrers[rand.Intn(len(Referrers))]
			}

			mockData = append(mockData, &analyticsApi.Event{
				ID:          uuid.New().String(),
				Type:        EventType,
				URL:         URL + Paths[rand.Intn(len(Paths))],
				Domain:      Domain,
				Referrer:    referrer,
				Browser:     visitors[i].Browser,
				OS:          visitors[i].OS,
				Device:      visitors[i].Device,
				HashedVisit: hex.EncodeToString(visitHashValue),
				Timestamp:   timestamppb.New(initialTime.Add(time.Minute * time.Duration(timeShift))),
			})

			timeShift += rand.Intn(40)
			H.Reset()
		}
	}

	return mockData
}

func main() {

	now := time.Now()
	eightDaysAgo := now.AddDate(0, 0, -6)
	timestamp := time.Date(eightDaysAgo.Year(), eightDaysAgo.Month(), eightDaysAgo.Day(), 10, 0, 0, 0, eightDaysAgo.Location())
	fmt.Println(timestamp)

	// Get binary name and initialize the Use field
	cmdFull, err := os.Executable()
	if err != nil {
		panic(err)
	}

	// Set variables and start the server
	c := cli{}
	rootCmd := &cobra.Command{
		Use:    filepath.Base(cmdFull),
		PreRun: c.setupConfig,
		RunE:   c.run,
	}

	// Setup persistent flags
	rootCmd.PersistentFlags().StringVar(&c.cfg.bindAddr, configKeyBindAddr, "", "Address to bind.")
	if err := viper.BindPFlag(configKeyBindAddr, rootCmd.PersistentFlags().Lookup(configKeyBindAddr)); err != nil {
		panic(err)
	}

	rootCmd.PersistentFlags().BoolVar(&c.cfg.debug, configKeyDebug, false, "Toggle debugging")
	if err := viper.BindPFlag(configKeyDebug, rootCmd.PersistentFlags().Lookup(configKeyDebug)); err != nil {
		panic(err)
	}

	rootCmd.PersistentFlags().Uint16Var(&c.cfg.grpcPort, configKeyGRPCPort, 9090, "Port for gRPC client connections.")
	if err := viper.BindPFlag(configKeyGRPCPort, rootCmd.PersistentFlags().Lookup(configKeyGRPCPort)); err != nil {
		panic(err)
	}

	rootCmd.PersistentFlags().Uint16Var(&c.cfg.gwPort, configKeyGWPort, 8080, "Port for gateway client connections.")
	if err := viper.BindPFlag(configKeyGWPort, rootCmd.PersistentFlags().Lookup(configKeyGWPort)); err != nil {
		panic(err)
	}

	rootCmd.PersistentFlags().Uint16Var(&c.cfg.mPort, configKeyMPort, 9113, "Port for metrics exposure.")
	if err := viper.BindPFlag(configKeyMPort, rootCmd.PersistentFlags().Lookup(configKeyMPort)); err != nil {
		panic(err)
	}

	rootCmd.PersistentFlags().BoolVar(&c.useMemDB, configKeyUseMemDB, true, "Use MemDB (not for production use)")
	if err := viper.BindPFlag(configKeyUseMemDB, rootCmd.PersistentFlags().Lookup(configKeyUseMemDB)); err != nil {
		panic(err)
	}

	rootCmd.PersistentFlags().BoolVar(&c.mockData, configKeyMock, false, "Use mocking of the data")
	if err := viper.BindPFlag(configKeyMock, rootCmd.PersistentFlags().Lookup(configKeyMock)); err != nil {
		panic(err)
	}

	rootCmd.PersistentFlags().Int64Var(&c.metricsTimeout, configKeyMetricsTimeout, 5, "Time (in seconds) to wait before metrics recalculation")
	if err := viper.BindPFlag(configKeyMetricsTimeout, rootCmd.PersistentFlags().Lookup(configKeyMetricsTimeout)); err != nil {
		panic(err)
	}

	rootCmd.PersistentFlags().StringSliceVar(&c.domains, configKeyDomains, nil, "List of domains to get the analytic from")
	if err := viper.BindPFlag(configKeyDomains, rootCmd.PersistentFlags().Lookup(configKeyDomains)); err != nil {
		panic(err)
	}

	if err := viper.BindPFlags(rootCmd.Flags()); err != nil {
		panic(err)
	}

	// Start the ball
	cobra.CheckErr(rootCmd.Execute())
}
