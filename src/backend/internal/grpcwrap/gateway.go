package grpcwrap

import (
	"context"
	"diploma/analytics-exporter/pkg/api/analytics"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"net/http"
)

// NewGatewayServer returns new grpc.ClientConn instance.
func NewGatewayServer(gwAddr string, grpcAddr string, tlsEnabled bool) (*http.Server, error) {
	// Create gRPC client connection
	conn, err := NewClientConn(grpcAddr, tlsEnabled)
	if err != nil {
		return nil, err
	}

	// Register gRPC server endpoint
	mux := runtime.NewServeMux()
	if err = analytics.RegisterAnalyticsHandler(context.Background(), mux, conn); err != nil {
		return nil, err
	}
	return &http.Server{
		Addr:    gwAddr,
		Handler: mux,
	}, nil
}
