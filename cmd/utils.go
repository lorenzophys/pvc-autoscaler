package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (a *PVCAutoscaler) isStorageClassExpandable(ctx context.Context, scName string) (bool, error) {
	sc, err := a.kubeClient.StorageV1().StorageClasses().Get(ctx, scName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	isExpandable := sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion
	return isExpandable, nil
}

func (a *PVCAutoscaler) getAnnotatedPVCs(ctx context.Context) (*corev1.PersistentVolumeClaimList, error) {
	autoscalerEnabledSelector := labels.Set{PVCAutoscalerEnabledAnnotation: "true"}.AsSelector()

	pvcl, err := a.kubeClient.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{
		LabelSelector: autoscalerEnabledSelector.String(),
	})
	if err != nil {
		return nil, err
	}

	return pvcl, nil
}

func getPVCStorageCeiling(pvc *corev1.PersistentVolumeClaim) (resource.Quantity, error) {
	if annotation, ok := pvc.Annotations[PVCAutoscalerCeilingAnnotation]; ok && annotation != "" {
		return resource.ParseQuantity(annotation)
	}

	return *pvc.Spec.Resources.Limits.Storage(), nil
}

func convertPercentageToBytes(value string, capacity int64, defaultValue string) (int64, error) {
	if len(value) == 0 {
		value = defaultValue
	}

	if strings.HasSuffix(value, "%") {
		perc, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err != nil {
			return 0, err
		}
		if perc < 0 || perc > 100 {
			return 0, fmt.Errorf("annotation value %q should between 0%% and 100%%", value)
		}

		res := int64(float64(capacity) * perc / 100.0)
		return res, nil
	} else {
		return 0, errors.New("annotation value should be a percentage")
	}
}

func isPVCResizable(pvc *corev1.PersistentVolumeClaim) error {
	// Ceiling
	quantity, err := getPVCStorageCeiling(pvc)
	if err != nil {
		return fmt.Errorf("invalid storage ceiling in the annotation: %w", err)
	}
	if quantity.IsZero() {
		return errors.New("the storage ceiling is zero")
	}

	// Specs
	if pvc.Spec.VolumeMode != nil && *pvc.Spec.VolumeMode != corev1.PersistentVolumeFilesystem {
		return errors.New("the associated volume must be formatted with a filesystem")
	}
	if pvc.Status.Phase != corev1.ClaimBound {
		return errors.New("not bound to any pod")
	}

	return nil
}
