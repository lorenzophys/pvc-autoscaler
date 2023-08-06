package main

import (
	"context"
	"time"

	providers "github.com/lorenzophys/pvc-autoscaler/internal/metrics_clients/clients"
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
)

type PVCAutoscaler struct {
	kubeClient      kubernetes.Interface
	metricsClient   providers.MetricsClient
	logger          *log.Logger
	pollingInterval time.Duration
}

func main() {
	var (
		logger = log.New()
	)

	kubeClient, err := newKubeClient()
	if err != nil {
		logger.Fatalf("an error occurred while creating the Kubernetes client: %s", err)
	}
	logger.Info("new kubernetes client created")

	metricsClient, err := MetricsClientFactory(PVCMetricsProvider)
	if err != nil {
		logger.Fatalf("metrics provider error: %s", err)
	}

	logger.Info("new metrics provider created")

	pvcAutoscaler := &PVCAutoscaler{
		kubeClient:      kubeClient,
		metricsClient:   metricsClient,
		logger:          logger,
		pollingInterval: 10 * time.Second,
	}

	ticker := time.NewTicker(pvcAutoscaler.pollingInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultReconcileTimeOut)
		defer cancel()

		err := pvcAutoscaler.reconcile(ctx)
		if err != nil {
			pvcAutoscaler.logger.Errorf("failed to reconcile: %v", err)
		}
	}
}
