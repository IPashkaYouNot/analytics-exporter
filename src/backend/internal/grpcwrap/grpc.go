// Package grpcwrap wraps grpc.Server, providing gRPC health.Server and zap.Logger.
package grpcwrap

import (
	grpcMiddleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpcZap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpcCtxTags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthPb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"time"
)

// Server holds gRPC servers and logger.
type Server struct {
	GRPCServer *grpc.Server

	grpcHealthServer *health.Server
}

// NewServer returns new Server instance.
//
// This includes logging (with request duration time), tracing in case of errors, and a health check gRPC endpoint.
func NewServer() (*Server, error) {
	grpcSrv, hgSrv := setupGRPCServer()
	return &Server{
		GRPCServer:       grpcSrv,
		grpcHealthServer: hgSrv,
	}, nil
}

// Shutdown ensures Server graceful stop.
func (s *Server) Shutdown() {
	zap.L().Info("Shutting down gRPC server...")
	s.grpcHealthServer.SetServingStatus("", healthPb.HealthCheckResponse_NOT_SERVING)
	s.GRPCServer.GracefulStop()
}

// setupGRPCServer sets up gRPC options, health check, tracing and logging.
func setupGRPCServer() (*grpc.Server, *health.Server) {
	logger := zap.L()
	// Make sure that log statements internal to gRPC library are logged using the logger as well.
	grpcZap.ReplaceGrpcLoggerV2(logger)
	// Don't log gRPC calls if it was a call to healthcheck and no error was raised
	zapDecider := func(fullMethodName string, err error) bool {
		if err == nil && fullMethodName == healthPb.Health_Check_FullMethodName {
			return false
		}

		return true
	}

	zapOpts := []grpcZap.Option{
		grpcZap.WithDecider(zapDecider),
		grpcZap.WithDurationField(
			func(duration time.Duration) zapcore.Field {
				return zap.Float64("grpc.time_sec", duration.Seconds())
			},
		),
	}

	grpcOpts := []grpc.ServerOption{
		grpc.StreamInterceptor(
			grpcMiddleware.ChainStreamServer(
				grpcCtxTags.StreamServerInterceptor(grpcCtxTags.WithFieldExtractor(grpcCtxTags.CodeGenRequestFieldExtractor)),
				grpcZap.StreamServerInterceptor(logger, zapOpts...),
			),
		),
		grpc.UnaryInterceptor(
			grpcMiddleware.ChainUnaryServer(
				grpcCtxTags.UnaryServerInterceptor(grpcCtxTags.WithFieldExtractor(grpcCtxTags.CodeGenRequestFieldExtractor)),
				grpcZap.UnaryServerInterceptor(logger, zapOpts...),
			),
		),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             30 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.MaxConcurrentStreams(50),
	}
	grpcSrv := grpc.NewServer(grpcOpts...)

	grpcHealthSrv := health.NewServer()
	grpcHealthSrv.SetServingStatus("", healthPb.HealthCheckResponse_SERVING)
	healthPb.RegisterHealthServer(grpcSrv, grpcHealthSrv)

	return grpcSrv, grpcHealthSrv
}
