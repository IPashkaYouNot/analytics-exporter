package prometheus

import (
	"diploma/analytics-exporter/internal/database"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"sync"
)

type AnalyticsCollector struct {
	logger   zap.Logger
	metrics  map[string]*prometheus.Desc
	mutex    sync.Mutex
	database database.Database
	domain   string
}

func NewAnalyticsCollector(constLabels map[string]string, logger *zap.Logger, db database.Database, domain string) *AnalyticsCollector {
	return &AnalyticsCollector{
		logger: *logger,
		metrics: map[string]*prometheus.Desc{
			"unique_visitors_total": prometheus.NewDesc("unique_visitors_total", "Total number of unique visitors", nil, constLabels),
			"visits_total":          prometheus.NewDesc("visits_total", "Total number of visitors", nil, constLabels),
			"total_page_views":      prometheus.NewDesc("page_views", "Total number of page views", nil, constLabels),
			"current_visitors":      prometheus.NewDesc("current_visitors", "Current visitors", nil, constLabels),
			"bounce_rate":           prometheus.NewDesc("bounce_rate", "Bounce rate in %", nil, constLabels),
			"page_rate":             prometheus.NewDesc("page_rate", "Rating of page", []string{"page"}, constLabels),
			"source_rate":           prometheus.NewDesc("source_rate", "Rating of source", []string{"source"}, constLabels),
			"os_rate":               prometheus.NewDesc("os_rate", "Rating of OS", []string{"os"}, constLabels),
			"browser_rate":          prometheus.NewDesc("browser_rate", "Rating of browser", []string{"browser"}, constLabels),
			"device_rate":           prometheus.NewDesc("device_rate", "Rating of device", []string{"device"}, constLabels),
			"entry_pages_rate":      prometheus.NewDesc("entry_pages_rate", "Rating of entry pages", []string{"page"}, constLabels),
			"exit_pages_rate":       prometheus.NewDesc("exit_pages_rate", "Rating of exit pages", []string{"page"}, constLabels),
			// TODO: add 404 error pages tracking (https://plausible.io/docs/error-pages-tracking-404)
		},
		mutex:    sync.Mutex{},
		database: db,
		domain:   domain,
	}
}

func (c *AnalyticsCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range c.metrics {
		ch <- m
	}
}

func (c *AnalyticsCollector) Collect(ch chan<- prometheus.Metric) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	stats, err := GetAnalyticsStats(c.database, c.domain)
	if err != nil {
		c.logger.Fatal("Error getting stats", zap.Error(err))
		return
	}

	ch <- prometheus.MustNewConstMetric(c.metrics["unique_visitors_total"],
		prometheus.CounterValue, float64(stats.UniqueVisitors))
	ch <- prometheus.MustNewConstMetric(c.metrics["visits_total"],
		prometheus.CounterValue, float64(stats.TotalVisits))
	ch <- prometheus.MustNewConstMetric(c.metrics["total_page_views"],
		prometheus.CounterValue, float64(stats.TotalPageViews))
	ch <- prometheus.MustNewConstMetric(c.metrics["current_visitors"],
		prometheus.CounterValue, float64(stats.CurrentVisitors))
	ch <- prometheus.MustNewConstMetric(c.metrics["bounce_rate"],
		prometheus.GaugeValue, stats.BounceRate)

	// Collect the rating of pages
	if stats.PagesRate != nil {
		for page, rate := range stats.PagesRate {
			ch <- prometheus.MustNewConstMetric(c.metrics["page_rate"],
				prometheus.GaugeValue, float64(rate), page)
		}
	}

	// Collect the rating of sources
	if stats.SourcesRate != nil {
		for source, rate := range stats.SourcesRate {
			ch <- prometheus.MustNewConstMetric(c.metrics["source_rate"],
				prometheus.GaugeValue, float64(rate), source)
		}
	}

	// Collect the rating of devices
	if stats.DevicesRate != nil {
		for device, rate := range stats.DevicesRate {
			ch <- prometheus.MustNewConstMetric(c.metrics["device_rate"],
				prometheus.GaugeValue, float64(rate), device)
		}
	}

	// Collect the rating of oss
	if stats.OSsRate != nil {
		for os, rate := range stats.OSsRate {
			ch <- prometheus.MustNewConstMetric(c.metrics["os_rate"],
				prometheus.GaugeValue, float64(rate), os)
		}
	}

	// Collect the rating of browsers
	if stats.BrowsersRate != nil {
		for browser, rate := range stats.BrowsersRate {
			ch <- prometheus.MustNewConstMetric(c.metrics["browser_rate"],
				prometheus.GaugeValue, float64(rate), browser)
		}
	}

	// Collect the rating of entry pages
	if stats.EntryPagesRate != nil {
		for page, rate := range stats.EntryPagesRate {
			ch <- prometheus.MustNewConstMetric(c.metrics["entry_pages_rate"],
				prometheus.GaugeValue, float64(rate), page)
		}
	}

	// Collect the rating of exit pages
	if stats.ExitPagesRate != nil {
		for page, rate := range stats.ExitPagesRate {
			ch <- prometheus.MustNewConstMetric(c.metrics["exit_pages_rate"],
				prometheus.GaugeValue, float64(rate), page)
		}
	}
}
