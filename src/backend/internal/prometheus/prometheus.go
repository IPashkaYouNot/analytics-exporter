package prometheus

import (
	"context"
	"diploma/analytics-exporter/internal/database"
	"errors"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"net/http"
)

type Prometheus struct {
	db database.Database

	HTTPServer *http.Server
}

func (p *Prometheus) Shutdown() error {
	zap.L().Info("Shutting down metrics server...")
	return p.HTTPServer.Shutdown(context.Background())
}

func NewPrometheus(db database.Database, addr string, domains []string) (*Prometheus, error) {
	if db == nil {
		return nil, errors.New("database.Database instance is nil")
	}
	if domains == nil {
		return nil, errors.New("domains is nil")
	}
	if len(domains) == 0 {
		return nil, errors.New("the domain list is empty")
	}
	for _, d := range domains {
		labels := make(map[string]string)
		labels["domain"] = d
		prometheus.MustRegister(NewAnalyticsCollector(labels, zap.L(), db, d))
	}

	promHandler := func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
		promhttp.Handler().ServeHTTP(w, r)
	}

	router := runtime.NewServeMux()
	err := router.HandlePath("GET", "/metrics", promHandler)
	if err != nil {
		return nil, err
	}
	httpServer := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	return &Prometheus{
		db:         db,
		HTTPServer: httpServer,
	}, nil
}
