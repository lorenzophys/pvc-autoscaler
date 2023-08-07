package main

import (
	"context"
	"os"
	"time"

	providers "github.com/lorenzophys/pvc-autoscaler/internal/metrics_clients/clients"
	"github.com/sirupsen/logrus"
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

	PVCMetricsProvider = "prometheus"

	DefaultThreshold = "80%"
	DefaultIncrease  = "20%"

	DefaultReconcileTimeOut = 1 * time.Minute

	LogLevel = logrus.InfoLevel
)

type PVCAutoscaler struct {
	kubeClient      kubernetes.Interface
	metricsClient   providers.MetricsClient
	logger          *log.Logger
	pollingInterval time.Duration
}

func main() {
	var (
		logger = &logrus.Logger{
			Out:       os.Stderr,
			Formatter: new(logrus.TextFormatter),
			Hooks:     make(logrus.LevelHooks),
			Level:     LogLevel,
		}
	)

	kubeClient, err := newKubeClient()
	if err != nil {
		logger.Fatalf("an error occurred while creating the Kubernetes client: %s", err)
	}
	logger.Info("new kubernetes client created")

	metricsClient, err := MetricsClientFactory(PVCMetricsProvider)
	if err != nil {
		logger.Fatalf("metrics client error: %s", err)
	}

	logger.Info("new metrics client created")

	pvcAutoscaler := &PVCAutoscaler{
		kubeClient:      kubeClient,
		metricsClient:   metricsClient,
		logger:          logger,
		pollingInterval: 30 * time.Second,
	}

	ticker := time.NewTicker(pvcAutoscaler.pollingInterval)
	defer ticker.Stop()

	for range ticker.C {
		pvcAutoscaler.logger.Debug("tick")
		ctx, cancel := context.WithTimeout(context.Background(), DefaultReconcileTimeOut)
		defer cancel()

		err := pvcAutoscaler.reconcile(ctx)
		if err != nil {
			pvcAutoscaler.logger.Errorf("failed to reconcile: %v", err)
		}
	}
}
