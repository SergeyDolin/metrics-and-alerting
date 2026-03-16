package main

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/SergeyDolin/metrics-and-alerting/internal/proto"
)

// GRPCClient handles gRPC communication with the server
type GRPCClient struct {
	conn   *grpc.ClientConn
	client proto.MetricsClient
	addr   string
}

// NewGRPCClient creates a new gRPC client
func NewGRPCClient(addr string) (*GRPCClient, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	return &GRPCClient{
		conn:   conn,
		client: proto.NewMetricsClient(conn),
		addr:   addr,
	}, nil
}

// Close closes the gRPC connection
func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

// SendMetricsBatch sends a batch of metrics via gRPC
// Используем тип Metrics из текущего пакета (main)
func (c *GRPCClient) SendMetricsBatch(metricsList []Metrics) error {
	if len(metricsList) == 0 {
		return nil
	}

	// Convert internal Metrics to proto Metrics
	protoMetrics := make([]*proto.Metric, 0, len(metricsList))
	for _, m := range metricsList {
		protoMetric := &proto.Metric{
			Id: m.ID,
		}

		switch m.MType {
		case "gauge":
			protoMetric.Type = proto.Metric_GAUGE
			if m.Value != nil {
				protoMetric.Value = *m.Value
			}
		case "counter":
			protoMetric.Type = proto.Metric_COUNTER
			if m.Delta != nil {
				protoMetric.Delta = *m.Delta
			}
		}

		protoMetrics = append(protoMetrics, protoMetric)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Add IP address to metadata
	agentIP := getHostIP()
	if agentIP != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-real-ip", agentIP)
	}

	// Send request
	req := &proto.UpdateMetricsRequest{
		Metrics: protoMetrics,
	}

	_, err := c.client.UpdateMetrics(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send metrics via gRPC: %w", err)
	}

	return nil
}
