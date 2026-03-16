package main

import (
	"context"
	"net"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/SergeyDolin/metrics-and-alerting/internal/proto"
	"github.com/SergeyDolin/metrics-and-alerting/internal/storage"
)

// GRPCServer implements the proto.MetricsServer interface
type GRPCServer struct {
	proto.UnimplementedMetricsServer
	store          storage.Storage
	saveFunc       func()
	auditPublisher *Publisher
	trustedSubnet  string
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(store storage.Storage, saveFunc func(), auditPublisher *Publisher, trustedSubnet string) *GRPCServer {
	return &GRPCServer{
		store:          store,
		saveFunc:       saveFunc,
		auditPublisher: auditPublisher,
		trustedSubnet:  trustedSubnet,
	}
}

// UpdateMetrics handles batch metric updates via gRPC
func (s *GRPCServer) UpdateMetrics(ctx context.Context, req *proto.UpdateMetricsRequest) (*proto.UpdateMetricsResponse, error) {
	// Check trusted subnet if configured
	if s.trustedSubnet != "" {
		if err := s.checkTrustedSubnet(ctx); err != nil {
			return nil, err
		}
	}

	if len(req.Metrics) == 0 {
		return &proto.UpdateMetricsResponse{}, nil
	}

	// Process each metric
	metricNames := make([]string, 0, len(req.Metrics))
	for _, m := range req.Metrics {
		var err error
		switch m.Type {
		case proto.Metric_GAUGE:
			err = s.store.UpdateGauge(ctx, m.Id, m.Value)
		case proto.Metric_COUNTER:
			err = s.store.UpdateCounter(ctx, m.Id, m.Delta)
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unknown metric type: %v", m.Type)
		}

		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to update metric %s: %v", m.Id, err)
		}
		metricNames = append(metricNames, m.Id)
	}

	// Save to disk if needed
	if s.saveFunc != nil {
		s.saveFunc()
	}

	// Log audit event if publisher is configured
	if s.auditPublisher != nil {
		ipAddress := s.getClientIP(ctx)
		event := AuditEvent{
			Timestamp: time.Now().Unix(),
			Metrics:   metricNames,
			IPAddress: ipAddress,
		}
		s.auditPublisher.Notify(event)
	}

	return &proto.UpdateMetricsResponse{}, nil
}

// checkTrustedSubnet validates that the client IP is in the trusted subnet
func (s *GRPCServer) checkTrustedSubnet(ctx context.Context) error {
	clientIP := s.getClientIP(ctx)
	if clientIP == "" {
		return status.Error(codes.PermissionDenied, "client IP not found in metadata")
	}

	_, ipNet, err := net.ParseCIDR(s.trustedSubnet)
	if err != nil {
		return status.Error(codes.Internal, "invalid trusted subnet configuration")
	}

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return status.Error(codes.InvalidArgument, "invalid IP address format")
	}

	if !ipNet.Contains(ip) {
		return status.Error(codes.PermissionDenied, "IP address not in trusted subnet")
	}

	return nil
}

// getClientIP extracts client IP from gRPC metadata
func (s *GRPCServer) getClientIP(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	// Check for x-real-ip header
	if values := md.Get("x-real-ip"); len(values) > 0 {
		return values[0]
	}

	return ""
}
