package main

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

func (a *PVCAutoscaler) startPVCInformer() {
	factory := informers.NewSharedInformerFactory(a.kubeClient, 0)
	pvcInformer := factory.Core().V1().PersistentVolumeClaims().Informer()

	pvcInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			addedPVC := obj.(*corev1.PersistentVolumeClaim)
			if _, ok := addedPVC.Annotations[PVCAutoscalerAnnotation]; ok {
				key := fmt.Sprintf("%s/%s", addedPVC.Namespace, addedPVC.Name)
				a.pvcsToWatch.Store(key, addedPVC)
				err := a.initPVCAnnotations(addedPVC)
				if err != nil {
					a.logger.Errorf("failed to write status annotation to %s: %s", key, err.Error())
				}
			}
		},
		DeleteFunc: func(obj any) {
			deletedPVC := obj.(*corev1.PersistentVolumeClaim)
			if _, ok := deletedPVC.Annotations[PVCAutoscalerAnnotation]; ok {
				key := fmt.Sprintf("%s/%s", deletedPVC.Namespace, deletedPVC.Name)
				a.pvcsToWatch.Delete(key)
				a.resizingPVCs.Delete(key)
				a.pvcsQueue.Forget(deletedPVC)
				a.removePVCAnnotations(deletedPVC)
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
					a.pvcsToWatch.Delete(oldKey)
					a.pvcsToWatch.Store(newKey, newPVC)
					a.logger.Infof("start watching %s/%s", newPVC.Namespace, newPVC.Name)
				}
			}
			if oldOk && oldValue == "enabled" { // annotation removed
				if !newOk || newValue != "enabled" {
					a.pvcsToWatch.Delete(oldKey)
					a.logger.Infof("stop watching %s/%s", newPVC.Namespace, newPVC.Name)
				}
			}
			if oldOk && oldValue == "enabled" { // annotation remains, but name changes
				if newOk && newValue == "enabled" {
					if oldPVC.Name != newPVC.Name {
						a.pvcsToWatch.Delete(oldKey)
						a.pvcsToWatch.Store(newKey, newPVC)
						a.logger.Infof("start watching %s/%s", newPVC.Namespace, newPVC.Name)
					}
				}
			}
		},
	})

	factory.Start(wait.NeverStop)
	factory.WaitForCacheSync(wait.NeverStop)
}
