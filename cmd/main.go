package main

import (
	"context"
	"flag"
	"os"
	"strings"
	"time"

	clients "github.com/lorenzophys/pvc-autoscaler/internal/metrics_clients/clients"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

const (
	PVCAutoscalerAnnotationPrefix           = "pvc-autoscaler.lorenzophys.io/"
	PVCAutoscalerEnabledAnnotation          = PVCAutoscalerAnnotationPrefix + "enabled"
	PVCAutoscalerThresholdAnnotation        = PVCAutoscalerAnnotationPrefix + "threshold"
	PVCAutoscalerCeilingAnnotation          = PVCAutoscalerAnnotationPrefix + "ceiling"
	PVCAutoscalerIncreaseAnnotation         = PVCAutoscalerAnnotationPrefix + "increase"
	PVCAutoscalerPreviousCapacityAnnotation = PVCAutoscalerAnnotationPrefix + "previous_capacity"

	DefaultThreshold = "80%"
	DefaultIncrease  = "20%"

	DefaultReconcileTimeOut = 1 * time.Minute
	DefaultPollingInterval  = 30 * time.Second
	DefaultLogLevel         = "INFO"
	DefaultMetricsProvider  = "prometheus"
)

type PVCAutoscaler struct {
	kubeClient      kubernetes.Interface
	metricsClient   clients.MetricsClient
	logger          *log.Logger
	pollingInterval time.Duration
}

func main() {
	metricsClient := flag.String("metrics-client", DefaultMetricsProvider, "specify the metrics client to use to query volume stats")
	metricsClientURL := flag.String("metrics-client-url", "", "Specify the metrics client URL to use to query volume stats")
	pollingInterval := flag.Duration("polling-interval", DefaultPollingInterval, "specify how often to check pvc stats")
	reconcileTimeout := flag.Duration("reconcile-timeout", DefaultReconcileTimeOut, "specify the time after which the reconciliation is considered failed")
	logLevel := flag.String("log-level", DefaultLogLevel, "specify the log level")

	flag.Parse()

	var loggerLevel log.Level
	switch strings.ToLower(*logLevel) {
	case "INFO":
		loggerLevel = log.InfoLevel
	case "DEBUG":
		loggerLevel = log.DebugLevel
	default:
		loggerLevel = log.InfoLevel
	}

	logger := &log.Logger{
		Out:       os.Stderr,
		Formatter: new(log.JSONFormatter),
		Hooks:     make(log.LevelHooks),
		Level:     loggerLevel,
	}

	kubeClient, err := newKubeClient()
	if err != nil {
		logger.Fatalf("an error occurred while creating the Kubernetes client: %s", err)
	}
	logger.Info("kubernetes client ready")

	PVCMetricsClient, err := MetricsClientFactory(*metricsClient, *metricsClientURL)
	if err != nil {
		logger.Fatalf("metrics client error: %s", err)
	}

	logger.Infof("metrics client (%s) ready at address %s", *metricsClient, *metricsClientURL)

	pvcAutoscaler := &PVCAutoscaler{
		kubeClient:      kubeClient,
		metricsClient:   PVCMetricsClient,
		logger:          logger,
		pollingInterval: *pollingInterval,
	}

	logger.Info("pvc-autoscaler ready")

	ticker := time.NewTicker(pvcAutoscaler.pollingInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), *reconcileTimeout)

		err := pvcAutoscaler.reconcile(ctx)
		if err != nil {
			pvcAutoscaler.logger.Errorf("failed to reconcile: %v", err)
		}

		cancel()
	}
}
