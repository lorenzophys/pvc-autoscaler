package main

import (
	"fmt"

	"github.com/lorenzophys/pvc-autoscaler/internal/metrics_providers/prometheus"
	"github.com/lorenzophys/pvc-autoscaler/internal/metrics_providers/providers"
)

func MetricsProviderFactory(providerName string) (providers.MetricsProvider, error) {
	switch providerName {
	case "prometheus":
		return &prometheus.PrometheusProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown metrics provider: %s", providerName)
	}
}
