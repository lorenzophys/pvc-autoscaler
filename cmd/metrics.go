package main

import (
	"fmt"

	clients "github.com/lorenzophys/pvc-autoscaler/internal/metrics_clients/clients"
	"github.com/lorenzophys/pvc-autoscaler/internal/metrics_clients/prometheus"
)

func MetricsClientFactory(clientName string) (clients.MetricsClient, error) {
	switch clientName {
	case "prometheus":
		return &prometheus.PrometheusClient{}, nil
	default:
		return nil, fmt.Errorf("unknown metrics client: %s", clientName)
	}
}
