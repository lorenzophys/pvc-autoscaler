package main

import (
	"fmt"

	clients "github.com/lorenzophys/pvc-autoscaler/internal/metrics_clients/clients"
	"github.com/lorenzophys/pvc-autoscaler/internal/metrics_clients/prometheus"
)

func MetricsClientFactory(clientName, clientUrl string) (clients.MetricsClient, error) {
	switch clientName {
	case "prometheus":
		prometheusClient, err := prometheus.NewPrometheusClient(clientUrl)
		if err != nil {
			return nil, err
		}
		return prometheusClient, nil
	default:
		return nil, fmt.Errorf("unknown metrics client: %s", clientName)
	}
}
