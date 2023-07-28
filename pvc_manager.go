package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *PVCAutoscaler) fetchPVCsToWatch() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pvcs, err := a.kubeClient.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	pvcCount := 0
	for _, pvc := range pvcs.Items {
		if value, ok := pvc.Annotations[PVCAutoscalerAnnotation]; ok && value == "enabled" {
			key := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
			a.pvcsToWatch.Store(key, &pvc)
			a.logger.Infof("watching PVC: %s", key)
			pvcCount++
		}
	}
	a.logger.Infof("there are %d PersistentVolumeClaims with autoscaling enabled in the cluster", pvcCount)

	return nil
}

func (a *PVCAutoscaler) updatePVCWithNewStorageSize(pvcToResize *corev1.PersistentVolumeClaim) error {
	pvcId := fmt.Sprintf("%s/%s", pvcToResize.Namespace, pvcToResize.Name)

	currentStorage := pvcToResize.Spec.Resources.Requests[corev1.ResourceStorage]
	newStorage := int64(float64(currentStorage.Value()) * (1 + a.config.expansion))
	newQuantity := resource.NewQuantity(newStorage, resource.BinarySI)

	pvcToResize.Spec.Resources.Requests[corev1.ResourceStorage] = *newQuantity

	ctxUpdate, cancelUpdate := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelUpdate()

	a.logger.Infof("start updating pvc: %s", pvcId)
	_, err := a.kubeClient.CoreV1().PersistentVolumeClaims(pvcToResize.Namespace).Update(ctxUpdate, pvcToResize, metav1.UpdateOptions{})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("timed out while trying to update PVC %s: %s", pvcId, err)
		} else {
			return fmt.Errorf("failed to update PVC %s: %s", pvcId, err)
		}
	}

	a.logger.Infof("update successful for %s, now waiting for the pvc to accept the resize", pvcId)

	return nil
}

func (a *PVCAutoscaler) processPVCs() {
	for a.processNextItem() {
	}
}

func (a *PVCAutoscaler) processNextItem() bool {
	// Wait until there's a PVC in the queue
	item, quit := a.pvcsQueue.Get()
	if quit {
		return false
	}
	pvc := item.(*corev1.PersistentVolumeClaim)
	pvcId := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)

	a.logger.Infof("pvc %s is pulled from the resizing queue", pvcId)

	a.pvcsQueue.Done(item)

	// Check if the PVC is already being processed
	_, alreadyResizing := a.resizingPVCs.LoadOrStore(pvcId, true)
	if alreadyResizing {
		a.logger.Infof("pvc %s is already being resized", pvcId)
		return true
	}

	// Check the status annotation
	status, err := UnmarshalStatusFromAnnotation(pvc)
	if err != nil {
		a.logger.Errorf("impossible to unmarshal the status of pvc %s", pvcId)
		a.resizingPVCs.Delete(pvcId)
		a.pvcsQueue.Add(pvc)
		return true
	}

	islastFailedAttempt := status.LastFailedAttempt.Equal(time.Time{})
	isUpdateNotRetryable := time.Now().Before(status.LastFailedAttempt.Add(a.config.retryAfter))

	if !islastFailedAttempt && isUpdateNotRetryable {
		a.resizingPVCs.Delete(pvcId)
		a.pvcsQueue.Add(pvc)
		return true
	}

	// Resize the PVC and handle errors
	err = a.updatePVCWithNewStorageSize(pvc)
	if err != nil {
		a.logger.Infof("pvc %s could not be resized: %s", pvcId, err)
		a.resizingPVCs.Delete(pvcId)
		status := &PVCAutoscalerStatus{
			LastFailedAttempt: time.Now(),
		}
		statusStr, err := status.MarshalToAnnotation()
		if err != nil {
			a.logger.Errorf("impossible to marshal the status of pvc %s", pvcId)
			a.resizingPVCs.Delete(pvcId)
			a.pvcsQueue.Add(pvc)
			return true
		}

		pvc.Annotations[PVCAutoscalerStatusAnnotation] = statusStr
		a.pvcsQueue.Add(pvc)
		return true
	}

	scaledStatus := &PVCAutoscalerStatus{
		LastScaleTime: time.Now(),
	}
	_, err = scaledStatus.MarshalToAnnotation()
	if err != nil {
		a.logger.Errorf("impossible to write the status of pvc %s", pvcId)
	}

	// After the PVC has been processed, remove it from the map
	a.resizingPVCs.Delete(pvcId)
	a.pvcsQueue.Forget(pvc)
	a.pvcsToWatch.Delete(pvcId)

	a.logger.Infof("pvc %s has been resized correctly, stop watching it", pvcId)

	return true
}
