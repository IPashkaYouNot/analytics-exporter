// Package analytics implements analytics.AnalyticsServer.
package analytics

import (
	"crypto/sha256"
	"diploma/analytics-exporter/internal/database"
	"diploma/analytics-exporter/pkg/api/analytics"
	"errors"
	"google.golang.org/grpc"
	"hash"
)

type analyticsServer struct {
	analytics.UnimplementedAnalyticsServer
	h  hash.Hash
	db database.Database
}

// New registers provisioner.ProvisionerServer instance
func New(g *grpc.Server, db database.Database) error {
	if g == nil {
		return errors.New("grpc.Server instance is nil")
	}
	if db == nil {
		return errors.New("database.Database instance is nil")
	}
	h := sha256.New()
	analytics.RegisterAnalyticsServer(g, &analyticsServer{
		db: db,
		h:  h,
	})
	return nil
}
