package database

import (
	"context"
	"diploma/analytics-exporter/pkg/api/analytics"
	"fmt"
	"github.com/hashicorp/go-memdb"
	"go.uber.org/zap"
)

const tableEvents = "events"

// schemaAnalytics defines in-memory database schema.
var schemaAnalytics = &memdb.DBSchema{
	Tables: map[string]*memdb.TableSchema{
		tableEvents: {
			Name: tableEvents,
			Indexes: map[string]*memdb.IndexSchema{
				"id": {
					Name:         "id",
					AllowMissing: false,
					Unique:       true,
					Indexer:      &memdb.StringFieldIndex{Field: "ID"},
				},
				"domain": {
					Name:         "domain",
					AllowMissing: false,
					Unique:       false,
					Indexer:      &memdb.StringFieldIndex{Field: "Domain"},
				},
			},
		},
	},
}

// inMem describes in-memory database connection.
type inMem struct {
	db *memdb.MemDB
}

// newInMem returns new inMem instance.
func newInMem() (*inMem, error) {
	db, err := memdb.NewMemDB(schemaAnalytics)
	if err != nil {
		return nil, err
	}
	return &inMem{
		db: db,
	}, nil
}

// Insert inserts new or updates existing record.
//
// error is returned on any non-functional error.
func (d *inMem) Insert(_ context.Context, msg *analytics.Event) error {
	// Create write transaction
	txn := d.db.Txn(true)
	defer txn.Abort()

	// Insert value
	err := txn.Insert(tableEvents, msg)
	zap.L().Named("memdb").Debug("insert "+msg.ID, zap.Bool("success", err == nil))
	if err != nil {
		return err
	}

	// Commit the transaction
	txn.Commit()

	return nil
}

// List returns all records found in database by the domain value.
//
// # If no records present - an empty slice is returned
//
// error is returned on any non-functional error.
func (d *inMem) List(_ context.Context, domain string) (*analytics.Events, error) {
	// Create read-only transaction
	txn := d.db.Txn(false)
	defer txn.Abort()

	// List all the instances
	it, err := txn.Get(tableEvents, "domain", domain)
	if err != nil {
		return nil, err
	}

	c := make([]*analytics.Event, 0)
	for obj := it.Next(); obj != nil; obj = it.Next() {
		switch record := obj.(type) {
		case *analytics.Event:
			c = append(c, record)
		default:
			return nil, fmt.Errorf("unsupported value type %s", record)
		}
	}

	return &analytics.Events{
		Events: c,
	}, nil
}
