package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PVCStatus struct {
	PVC *corev1.PersistentVolumeClaim
	Err error
}

func updatePVCWithNewStorageSize(kubeClient *kubernetes.Clientset, pvcToResize *corev1.PersistentVolumeClaim) error {
	pvcId := fmt.Sprintf("%s/%s", pvcToResize.Namespace, pvcToResize.Name)

	currenStorage := pvcToResize.Spec.Resources.Requests[corev1.ResourceStorage]
	newStorage := float64(currenStorage.Value()) * (1 + expansion)
	newQuantity := resource.NewQuantity(int64(newStorage), resource.DecimalSI)

	pvcToResize.Spec.Resources.Requests[corev1.ResourceStorage] = *newQuantity

	ctxUpdate, cancelUpdate := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelUpdate()

	logger.Infof("start updating pvc: %s", pvcId)
	_, err := kubeClient.CoreV1().PersistentVolumeClaims(pvcToResize.Namespace).Update(ctxUpdate, pvcToResize, metav1.UpdateOptions{})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("timed out while trying to update PVC %s: %s", pvcId, err)
		} else {
			return fmt.Errorf("failed to update PVC %s: %s", pvcId, err)
		}
	}

	logger.Infof("update successful for %s, now waiting for the pvc to accept the resize", pvcId)

	return nil
}

func processPVCs(kubeClient *kubernetes.Clientset) {
	for {
		// Wait until there's a PVC in the queue
		item, quit := pvcsQueue.Get()
		if quit {
			break
		}
		pvc := item.(*corev1.PersistentVolumeClaim)
		pvcId := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)

		logger.Infof("pvc %s is pulled from the resizing queue", pvcId)

		// Process the PVC
		go func(pvc *corev1.PersistentVolumeClaim) {
			pvcId := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)

			// Check if the PVC is already being processed
			_, alreadyResizing := resizingPVCs.LoadOrStore(pvcId, true)
			if alreadyResizing {
				logger.Infof("pvc %s is already being resized", pvcId)
				return
			}

			// Resize the PVC and handle errors
			err := updatePVCWithNewStorageSize(kubeClient, pvc)
			if err != nil {
				logger.Infof("pvc %s could not be resized, stop watching it: %s", pvcId, err)
				pvcsToWatch.Delete(pvcId)
				return
			}

			// After the PVC has been processed, remove it from the map
			resizingPVCs.Delete(pvcId)
			pvcsQueue.Forget(pvc)
			pvcsToWatch.Delete(pvcId)

			logger.Infof("pvc %s has been resized correctly, stop watching it", pvcId)
		}(pvc)

		pvcsQueue.Done(item)
	}
}
