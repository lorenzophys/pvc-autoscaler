package main

import (
	"context"
	"io"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestFetchPVCsToWatch(t *testing.T) {
	pvcs := []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc1",
				Namespace: "default",
				Annotations: map[string]string{
					PVCAutoscalerAnnotation: "enabled",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc2",
				Namespace: "default",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc3",
				Namespace: "test",
				Annotations: map[string]string{
					PVCAutoscalerAnnotation: "enabled",
				},
			},
		},
	}

	fakeClient := fake.NewSimpleClientset()

	for _, pvc := range pvcs {
		_, err := fakeClient.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), &pvc, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	logger := log.New()
	logger.SetOutput(io.Discard)

	pvcAutoscaler := PVCAutoscaler{
		kubeClient:   fakeClient,
		logger:       logger,
		pvcsToWatch:  &sync.Map{},
		resizingPVCs: &sync.Map{},
		pvcsQueue:    workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}

	err := pvcAutoscaler.fetchPVCsToWatch()
	assert.NoError(t, err)

	pvcAutoscaler.pvcsToWatch.Range(func(key, value any) bool {
		pvc, ok := value.(*corev1.PersistentVolumeClaim)
		assert.True(t, ok)
		assert.Contains(t, []string{"default/test-pvc1", "test/test-pvc3"}, key)
		assert.Equal(t, "enabled", pvc.Annotations[PVCAutoscalerAnnotation])
		return true
	})
}

func TestUpdatePVCWithNewStorageSize(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "test-namespace",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}

	fakeClient := fake.NewSimpleClientset(pvc)

	logger := log.New()
	logger.SetOutput(io.Discard)

	pvcAutoscaler := PVCAutoscaler{
		kubeClient:   fakeClient,
		logger:       logger,
		pvcsToWatch:  &sync.Map{},
		resizingPVCs: &sync.Map{},
		pvcsQueue:    workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}

	err := pvcAutoscaler.updatePVCWithNewStorageSize(pvc)
	assert.NoError(t, err)

	updatedPvc, err := fakeClient.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.Background(), pvc.Name, metav1.GetOptions{})
	assert.NoError(t, err)

	expectedSize := int64(12884901888)
	updatedSize := updatedPvc.Spec.Resources.Requests[corev1.ResourceStorage]
	assert.Equal(t, expectedSize, updatedSize.Value())
}
