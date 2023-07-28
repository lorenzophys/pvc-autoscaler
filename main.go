package main

import (
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	PVCAutoscalerAnnotation = "pvc-autoscaler.lorenzophys.io"
	thresholdPercentage     = 80
	expansion               = 0.2
	pollingInterval         = 10 * time.Second
)

type PVCAutoscaler struct {
	kubeClient   kubernetes.Interface
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

	pvcAutoscaler := &PVCAutoscaler{
		kubeClient:   kubeClient,
		logger:       logger,
		pvcsToWatch:  &sync.Map{},
		resizingPVCs: &sync.Map{},
		pvcsQueue:    workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{}),
	}

	err = pvcAutoscaler.fetchPVCsToWatch()
	if err != nil {
		logger.Fatalf("failed to fetch PersistentVolumeClaims: %s", err.Error())
	}

	factory := informers.NewSharedInformerFactory(kubeClient, 0)
	pvcInformer := factory.Core().V1().PersistentVolumeClaims().Informer()

	pvcInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			addedPVC := obj.(*corev1.PersistentVolumeClaim)
			if _, ok := addedPVC.Annotations[PVCAutoscalerAnnotation]; ok {
				key := fmt.Sprintf("%s/%s", addedPVC.Namespace, addedPVC.Name)
				pvcAutoscaler.pvcsToWatch.Store(key, addedPVC)
			}
		},
		DeleteFunc: func(obj any) {
			deletedPVC := obj.(*corev1.PersistentVolumeClaim)
			if _, ok := deletedPVC.Annotations[PVCAutoscalerAnnotation]; ok {
				key := fmt.Sprintf("%s/%s", deletedPVC.Namespace, deletedPVC.Name)
				pvcAutoscaler.pvcsToWatch.Delete(key)
			}
		},
		UpdateFunc: func(oldObj any, newObj any) {
			newPVC := newObj.(*corev1.PersistentVolumeClaim)
			oldPVC := oldObj.(*corev1.PersistentVolumeClaim)

			// this happens if name or annotations are changed

			oldValue, oldOk := oldPVC.Annotations[PVCAutoscalerAnnotation]
			newValue, newOk := newPVC.Annotations[PVCAutoscalerAnnotation]

			newKey := fmt.Sprintf("%s/%s", newPVC.Namespace, newPVC.Name)
			oldKey := fmt.Sprintf("%s/%s", oldPVC.Namespace, oldPVC.Name)

			if !oldOk || oldValue != "enabled" { // annotation added
				if newOk && newValue == "enabled" {
					pvcAutoscaler.pvcsToWatch.Delete(oldKey)
					pvcAutoscaler.pvcsToWatch.Store(newKey, newPVC)
					logger.Infof("start watching %s/%s", newPVC.Namespace, newPVC.Name)
				}
			}
			if oldOk && oldValue == "enabled" { // annotation removed
				if !newOk || newValue != "enabled" {
					pvcAutoscaler.pvcsToWatch.Delete(oldKey)
					logger.Infof("stop watching %s/%s", newPVC.Namespace, newPVC.Name)
				}
			}
			if oldOk && oldValue == "enabled" { // annotation remains, but name changes
				if newOk && newValue == "enabled" {
					if oldPVC.Name != newPVC.Name {
						pvcAutoscaler.pvcsToWatch.Delete(oldKey)
						pvcAutoscaler.pvcsToWatch.Store(newKey, newPVC)
						logger.Infof("start watching %s/%s", newPVC.Namespace, newPVC.Name)
					}
				}
			}
		},
	})

	factory.Start(wait.NeverStop)
	factory.WaitForCacheSync(wait.NeverStop)
	logger.Info("new informer started watching PersistentVolumeClaims resources")

	go pvcAutoscaler.processPVCs()

	ticker := time.NewTicker(pollingInterval)
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

			if metric.PVCPercentageUsed >= thresholdPercentage {
				pvcAutoscaler.pvcsQueue.Add(pvc)
				logger.Infof("pvc %s queued for resizing", pvcId)
			}
			return true
		})
	}
}
