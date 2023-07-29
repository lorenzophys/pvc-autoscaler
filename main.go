package main

import (
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
)

const (
	PVCAutoscalerAnnotation       = "pvc-autoscaler.lorenzophys.io"
	PVCAutoscalerStatusAnnotation = "pvc-autoscaler.lorenzophys.io/status"
)

type Config struct {
	thresholdPercentage float64
	expansion           float64
	pollingInterval     time.Duration
	retryAfter          time.Duration
}

type PVCAutoscaler struct {
	kubeClient   kubernetes.Interface
	config       Config
	logger       *log.Logger
	pvcsToWatch  *sync.Map
	resizingPVCs *sync.Map
	pvcsQueue    workqueue.RateLimitingInterface
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

	prometheusClient, err := newPrometheusClient()
	if err != nil {
		logger.Fatalf("an error occurred while creating the Prometheus client: %s", err)
	}
	logger.Info("new prometheus client created")

	config := Config{
		thresholdPercentage: 80,
		expansion:           0.2,
		pollingInterval:     10 * time.Second,
		retryAfter:          time.Minute,
	}

	pvcAutoscaler := &PVCAutoscaler{
		kubeClient:   kubeClient,
		config:       config,
		logger:       logger,
		pvcsToWatch:  &sync.Map{},
		resizingPVCs: &sync.Map{},
		pvcsQueue:    workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{}),
	}

	err = pvcAutoscaler.fetchPVCsToWatch()
	if err != nil {
		logger.Fatalf("failed to fetch PersistentVolumeClaims: %s", err.Error())
	}

	pvcAutoscaler.startPVCInformer()
	logger.Info("new informer started watching PersistentVolumeClaims resources")

	go pvcAutoscaler.processPVCs()

	ticker := time.NewTicker(pvcAutoscaler.config.pollingInterval)
	defer ticker.Stop()

	for range ticker.C {
		pvcAutoscaler.pvcsToWatch.Range(func(key, value any) bool {
			pvc := value.(*corev1.PersistentVolumeClaim)
			pvcId := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)

			metric, err := queryPrometheusPVCUtilization(prometheusClient, pvc)
			if err != nil {
				logger.Error(err)
			} else {
				logger.Infof("utilization of %s: %.2f%%", pvcId, metric.PVCPercentageUsed)
			}

			if metric.PVCPercentageUsed >= pvcAutoscaler.config.thresholdPercentage {
				pvcAutoscaler.pvcsQueue.Add(pvc)
				logger.Infof("pvc %s queued for resizing", pvcId)
			}
			return true
		})
	}
}
