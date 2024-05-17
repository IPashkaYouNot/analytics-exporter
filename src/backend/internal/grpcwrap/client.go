package grpcwrap

import (
	"crypto/tls"
	grpcRetry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"time"
)

// NewClientConn returns new grpc.ClientConn instance.
func NewClientConn(addr string, tlsEnabled bool) (*grpc.ClientConn, error) {
	var newCredentials credentials.TransportCredentials
	if tlsEnabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
		}
		newCredentials = credentials.NewTLS(tlsConfig)
	} else {
		newCredentials = insecure.NewCredentials()
	}

	// Retry on well known gRPC codes that should be retryable,
	// use a 100ms exponential backoff with 10% jitter and maximum of 5 retries.
	opts := []grpcRetry.CallOption{
		grpcRetry.WithBackoff(grpcRetry.BackoffExponentialWithJitter(100*time.Millisecond, 0.1)),
		grpcRetry.WithMax(5),
		grpcRetry.WithCodes(grpcRetry.DefaultRetriableCodes...),
	}

	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(newCredentials),
		grpc.WithStreamInterceptor(grpcRetry.StreamClientInterceptor(opts...)),
		grpc.WithUnaryInterceptor(grpcRetry.UnaryClientInterceptor(opts...)),
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
