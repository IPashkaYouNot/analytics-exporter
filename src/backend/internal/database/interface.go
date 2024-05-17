package database

import (
	"context"
	"diploma/analytics-exporter/pkg/api/analytics"
	"go.uber.org/zap"
)

type Database interface {
	List(ctx context.Context, domain string) (*analytics.Events, error)
	Insert(ctx context.Context, msg *analytics.Event) error
}

// NewDatabase returns Database implementation
func NewDatabase(useMemDB bool) (Database, error) {
	if useMemDB {
		zap.L().Info("Initialising memdb")
		return newInMem()
	}
	return nil, nil
}
