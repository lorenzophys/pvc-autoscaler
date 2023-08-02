package main

import (
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
)

type PVCAutoscalerStatus struct {
	LastScaleTime     time.Time `json:"lastScaleTime"`
	LastFailedAttempt time.Time `json:"lastFailedAttempt"`
}

func (status *PVCAutoscalerStatus) MarshalToAnnotation() (string, error) {
	bytes, err := json.Marshal(status)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func UnmarshalStatusFromAnnotation(pvc *corev1.PersistentVolumeClaim) (*PVCAutoscalerStatus, error) {
	statusStr, ok := pvc.Annotations[PVCAutoscalerStatusAnnotation]
	if !ok {
		// If the annotation isn't found, return a default status.
		return &PVCAutoscalerStatus{}, nil
	}

	var status PVCAutoscalerStatus
	if err := json.Unmarshal([]byte(statusStr), &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (a *PVCAutoscaler) initPVCAnnotations(pvc *corev1.PersistentVolumeClaim) error {
	// Check if the annotation is already initialized
	if _, ok := pvc.Annotations[PVCAutoscalerStatusAnnotation]; ok {
		return nil
	}

	// If not, initialize it with default values
	defaultStatus := &PVCAutoscalerStatus{
		LastScaleTime: time.Time{},
	}

	statusStr, err := defaultStatus.MarshalToAnnotation()
	if err != nil {
		return fmt.Errorf("failed to marshal default autoscaler status: %w", err)
	}

	if pvc.Annotations == nil {
		pvc.Annotations = make(map[string]string)
	}

	pvc.Annotations[PVCAutoscalerStatusAnnotation] = statusStr

	return nil
}

func (a *PVCAutoscaler) removePVCAnnotations(pvc *corev1.PersistentVolumeClaim) {
	delete(pvc.Annotations, PVCAutoscalerStatusAnnotation)
}
