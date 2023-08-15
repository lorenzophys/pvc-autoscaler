package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"testing"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
	ktesting "k8s.io/client-go/testing"
)

func TestUpdatePVCWithNewStorageSize(t *testing.T) {
	tests := []struct {
		name                    string
		pvc                     *corev1.PersistentVolumeClaim
		ExpectedNewStorageBytes *resource.Quantity
		kubeClient              *fake.Clientset
		withErr                 bool
		expectedErr             error
	}{
		{
			name: "Successfully update PVC",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pvc",
					Namespace:   "test-ns",
					Annotations: map[string]string{PVCAutoscalerEnabledAnnotation: "true"},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("10Gi"),
						},
					},
				},
			},
			ExpectedNewStorageBytes: quantityPtr(resource.MustParse("12Gi")),
			kubeClient:              fake.NewSimpleClientset(),
			withErr:                 false,
			expectedErr:             nil,
		},
		{
			name: "Fail to update PVC (force fail with reactor)",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pvc",
					Namespace:   "test-ns",
					Annotations: map[string]string{PVCAutoscalerEnabledAnnotation: "true"},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("10Gi"),
						},
					},
				},
			},
			ExpectedNewStorageBytes: new(resource.Quantity),
			kubeClient:              fake.NewSimpleClientset(),
			withErr:                 true,
			expectedErr:             fmt.Errorf("failed to update PVC test-ns/test-pvc: %w", errors.New("failed to update PVC")),
		},
	}

	// Add a reactor to simulate a failure when updating PVCs
	tests[1].kubeClient.PrependReactor("update", "persistentvolumeclaims", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("failed to update PVC")
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &log.Logger{
				Out: io.Discard,
			}

			autoscaler := &PVCAutoscaler{
				kubeClient: tt.kubeClient,
				logger:     logger,
			}

			tt.kubeClient.CoreV1().PersistentVolumeClaims("test-ns").Create(context.TODO(), tt.pvc, metav1.CreateOptions{})

			err := autoscaler.updatePVCWithNewStorageSize(context.TODO(), tt.pvc, 1000, tt.ExpectedNewStorageBytes)
			assert.Equal(t, tt.withErr, err != nil)
			assert.Equal(t, tt.expectedErr, err)

			if !tt.withErr {
				updatedPVC, _ := tt.kubeClient.CoreV1().PersistentVolumeClaims(tt.pvc.Namespace).Get(context.TODO(), tt.pvc.Name, metav1.GetOptions{})
				newStorage := updatedPVC.Spec.Resources.Requests[corev1.ResourceStorage]
				assert.Equal(t, tt.ExpectedNewStorageBytes, &newStorage)

				assert.Equal(t, updatedPVC.Annotations[PVCAutoscalerPreviousCapacityAnnotation], strconv.FormatInt(1000, 10))
			}
		})
	}
}

func quantityPtr(q resource.Quantity) *resource.Quantity {
	return &q
}
