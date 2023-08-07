package main

import (
	"context"
	"fmt"
	"math"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (a *PVCAutoscaler) reconcile(ctx context.Context) error {
	pvcl, err := a.getAnnotatedPVCs(ctx)
	if err != nil {
		return fmt.Errorf("could not get PersistentVolumeClaims: %w", err)
	}
	a.logger.Debug("fetched annotated pvcs")

	pvcsMetrics, err := a.metricsClient.FetchPVCsMetrics(ctx)
	if err != nil {
		a.logger.Errorf("could not fetch the PersistentVolumeClaims metrics: %v", err)
		return nil
	}
	a.logger.Debug("fetched metrics")

	for _, pvc := range pvcl.Items {
		pvcId := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
		a.logger.Debugf("processing pvc %s", pvcId)

		// Determine if the StorageClass allows volume expansion
		storageClassName := *pvc.Spec.StorageClassName
		isExpandable, err := a.isStorageClassExpandable(ctx, storageClassName)
		if err != nil {
			a.logger.Errorf("could not get StorageClass %s for %s: %v", storageClassName, pvcId, err)
			continue
		}
		if !isExpandable {
			a.logger.Errorf("the StorageClass %s of %s does not allow volume expansion", storageClassName, pvcId)
			continue
		}
		a.logger.Debugf("storageclass for %s ok", pvcId)

		// Determine if pvc the meets the condition for resize
		err = isPVCResizable(&pvc)
		if err != nil {
			a.logger.Errorf("the PersistentVolumeClaim %s is not resizable: %v", pvcId, err)
			continue
		}
		a.logger.Debugf("pvc %s resizable", pvcId)

		namespacedName := types.NamespacedName{
			Namespace: pvc.Namespace,
			Name:      pvc.Name,
		}
		if _, ok := pvcsMetrics[namespacedName]; !ok {
			a.logger.Errorf("could not fetch the metrics for %s", pvcId)
			continue
		}
		a.logger.Debugf("metrics for %s ok", pvcId)

		pvcCurrentCapacityBytes := pvcsMetrics[namespacedName].VolumeCapacityBytes

		threshold, err := convertPercentageToBytes(pvc.Annotations[PVCAutoscalerThresholdAnnotation], pvcCurrentCapacityBytes, DefaultThreshold)
		if err != nil {
			a.logger.Errorf("failed to convert threshold annotation for %s: %v", pvcId, err)
			continue
		}

		capacity, exists := pvc.Status.Capacity[corev1.ResourceStorage]
		if !exists {
			a.logger.Infof("skip %s because its capacity is not set yet", pvcId)
			continue
		}
		if capacity.Value() == 0 {
			a.logger.Infof("skip %s because its capacity is zero", pvcId)
			continue
		}

		increase, err := convertPercentageToBytes(pvc.Annotations[PVCAutoscalerIncreaseAnnotation], capacity.Value(), DefaultIncrease)
		if err != nil {
			a.logger.Errorf("failed to convert increase annotation for %s: %v", pvcId, err)
			continue
		}

		previousCapacity, exist := pvc.Annotations[PVCAutoscalerPreviousCapacityAnnotation]
		if exist {
			parsedPreviousCapacity, err := strconv.ParseInt(previousCapacity, 10, 64)
			if err != nil {
				a.logger.Errorf("failed to parse \"previous_capacity\" annotation: %v", err)
				continue
			}
			if parsedPreviousCapacity == pvcCurrentCapacityBytes {
				a.logger.Infof("pvc %s is still waiting to accept the resize", pvcId)
				continue
			} else if parsedPreviousCapacity < pvcCurrentCapacityBytes {
				a.logger.Infof("pvc %s accepted the resize", pvcId)
				continue
			}
		}

		ceiling, err := getPVCStorageCeiling(&pvc)
		if err != nil {
			a.logger.Errorf("failed to fetch storage ceiling for %s: %v", pvcId, err)
			continue
		}
		if capacity.Cmp(ceiling) >= 0 {
			a.logger.Infof("volume storage limit reached for %s", pvcId)
			continue
		}

		currentUsedBytes := pvcsMetrics[namespacedName].VolumeUsedBytes
		if currentUsedBytes >= threshold {
			a.logger.Debugf("pvc %s usege bigger than threshold", pvcId)

			// 1<<30 is a bit shift operation that represents 2^30, i.e. 1Gi
			newStorageBytes := int64(math.Ceil(float64(capacity.Value()+increase)/(1<<30))) << 30
			newStorage := resource.NewQuantity(newStorageBytes, resource.BinarySI)
			if newStorage.Cmp(ceiling) > 0 {
				newStorage = &ceiling
			}

			err := a.updatePVCWithNewStorageSize(ctx, &pvc, pvcCurrentCapacityBytes, newStorage)
			if err != nil {
				a.logger.Errorf("failed to resize pvc %s: %v", pvcId, err)
			}

			a.logger.Infof("pvc %s resized from %d to %d ", pvcId, capacity.Value(), newStorage.Value())
		}
	}

	return nil
}

func (a *PVCAutoscaler) updatePVCWithNewStorageSize(ctx context.Context, pvcToResize *corev1.PersistentVolumeClaim, capacityBytes int64, newStorageBytes *resource.Quantity) error {
	pvcId := fmt.Sprintf("%s/%s", pvcToResize.Namespace, pvcToResize.Name)

	a.logger.Debugf("update storage for %s ok", pvcId)

	pvcToResize.Spec.Resources.Requests[corev1.ResourceStorage] = *newStorageBytes

	pvcToResize.Annotations[PVCAutoscalerPreviousCapacityAnnotation] = strconv.FormatInt(capacityBytes, 10)
	_, err := a.kubeClient.CoreV1().PersistentVolumeClaims(pvcToResize.Namespace).Update(ctx, pvcToResize, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update PVC %s: %w", pvcId, err)
	}

	return nil
}
