package main

import (
	"context"
	"time"

	"github.com/SergeyDolin/metrics-and-alerting/internal/proto"
	"github.com/SergeyDolin/metrics-and-alerting/internal/storage"
	"github.com/SergeyDolin/metrics-and-alerting/internal/subnet"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GRPCServer struct {
	proto.UnimplementedMetricsServer
	store          storage.Storage
	saveFunc       func()
	auditPublisher *Publisher
	validator      *subnet.Validator
}

func NewGRPCServer(store storage.Storage, saveFunc func(), auditPublisher *Publisher, validator *subnet.Validator) *GRPCServer {
	return &GRPCServer{
		store:          store,
		saveFunc:       saveFunc,
		auditPublisher: auditPublisher,
		validator:      validator,
	}
}

func (s *GRPCServer) UpdateMetrics(ctx context.Context, req *proto.UpdateMetricsRequest) (*proto.UpdateMetricsResponse, error) {
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
		ipAddress := subnet.ExtractIPFromGRPCContext(ctx)
		event := AuditEvent{
			Timestamp: time.Now().Unix(),
			Metrics:   metricNames,
			IPAddress: ipAddress,
		}
		s.auditPublisher.Notify(event)
	}

	return &proto.UpdateMetricsResponse{}, nil
}
