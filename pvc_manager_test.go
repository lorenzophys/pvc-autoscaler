package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
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
		pvcsQueue:    workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{}),
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

	config := Config{
		thresholdPercentage: 80,
		expansion:           0.2,
		pollingInterval:     10 * time.Second,
		retryAfter:          time.Minute,
	}

	pvcAutoscaler := PVCAutoscaler{
		kubeClient:   fakeClient,
		config:       config,
		logger:       logger,
		pvcsToWatch:  &sync.Map{},
		resizingPVCs: &sync.Map{},
		pvcsQueue:    workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{}),
	}

	err := pvcAutoscaler.updatePVCWithNewStorageSize(pvc)
	assert.NoError(t, err)

	updatedPvc, err := fakeClient.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.Background(), pvc.Name, metav1.GetOptions{})
	assert.NoError(t, err)

	expectedSize := int64(12884901888)
	updatedSize := updatedPvc.Spec.Resources.Requests[corev1.ResourceStorage]
	assert.Equal(t, expectedSize, updatedSize.Value())
}

func TestProcessNextItem(t *testing.T) {
	t.Run("update successful", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
				Annotations: map[string]string{
					PVCAutoscalerAnnotation: "enabled",
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("100Gi"),
					},
				},
			},
		}

		fakeClient := fake.NewSimpleClientset(pvc)

		queue := workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{})
		queue.Add(pvc)

		logger := log.New()
		logger.SetOutput(io.Discard)

		autoscaler := &PVCAutoscaler{
			kubeClient:   fakeClient,
			logger:       logger,
			pvcsToWatch:  &sync.Map{},
			resizingPVCs: &sync.Map{},
			pvcsQueue:    queue,
		}

		autoscaler.processNextItem()

		pvcId := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)

		assert.False(t, queue.Len() > 0, "work queue should be empty")

		_, ok := autoscaler.pvcsToWatch.Load(pvcId)
		assert.False(t, ok, "PVC should be deleted from pvcsToWatch map")

		_, ok = autoscaler.resizingPVCs.Load(pvcId)
		assert.False(t, ok, "PVC should be deleted from resizingPVCs map")

	})

	t.Run("update not successful", func(t *testing.T) {
		statusAnnotation := fmt.Sprintf("{\"lastScaleTime\": \"%v\", \"lastFailedAttempt\": \"%v\"}", time.Time{}.Format(time.RFC3339), time.Time{}.Format(time.RFC3339))
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "test-ns",
				Annotations: map[string]string{
					PVCAutoscalerAnnotation:       "enabled",
					PVCAutoscalerStatusAnnotation: statusAnnotation,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("100Gi"),
					},
				},
			},
		}

		// Initialize a new fake queue and add the PVC to it
		queue := workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{})
		queue.Add(pvc)

		// Define the fake client and its behavior
		fakeClient := fake.NewSimpleClientset(pvc)

		// Mock the Update method to return an error
		fakeClient.PrependReactor("update", "persistentvolumeclaims", func(action ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("forced update error")
		})

		// Return the test PVC when the "get" action is called
		fakeClient.PrependReactor("get", "persistentvolumeclaims", func(action ktesting.Action) (bool, runtime.Object, error) {
			return true, pvc, nil
		})

		buf := new(bytes.Buffer)
		logger := log.New()
		logger.SetOutput(buf)

		autoscaler := &PVCAutoscaler{
			kubeClient:   fakeClient,
			logger:       logger,
			pvcsToWatch:  &sync.Map{},
			resizingPVCs: &sync.Map{},
			pvcsQueue:    queue,
		}

		// Process the PVC
		autoscaler.processNextItem()

		// Check that the error was logged
		assert.Contains(t, buf.String(), "forced update error")

		// Check the PVC annotations for the last failed attempt
		pvc, _ = fakeClient.CoreV1().PersistentVolumeClaims("test-ns").Get(context.Background(), "test-pvc", metav1.GetOptions{})
		assert.Contains(t, pvc.Annotations, PVCAutoscalerStatusAnnotation)

		status, _ := UnmarshalStatusFromAnnotation(pvc)

		assert.NotEqual(t, status.LastFailedAttempt, time.Time{})
		assert.Equal(t, status.LastScaleTime, time.Time{})
	})

	t.Run("update not successful retry too soon", func(t *testing.T) {
		statusAnnotation := fmt.Sprintf("{\"lastScaleTime\": \"%v\", \"lastFailedAttempt\": \"%v\"}", time.Time{}.Format(time.RFC3339), time.Now().Format(time.RFC3339))
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "test-ns",
				Annotations: map[string]string{
					PVCAutoscalerAnnotation:       "enabled",
					PVCAutoscalerStatusAnnotation: statusAnnotation,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("100Gi"),
					},
				},
			},
		}

		// Initialize a new fake queue and add the PVC to it
		queue := workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{})
		queue.Add(pvc)

		// Define the fake client and its behavior
		fakeClient := fake.NewSimpleClientset(pvc)

		logger := log.New()
		logger.SetOutput(io.Discard)

		config := Config{
			thresholdPercentage: 80,
			expansion:           0.2,
			pollingInterval:     10 * time.Second,
			retryAfter:          time.Hour,
		}

		autoscaler := &PVCAutoscaler{
			kubeClient:   fakeClient,
			config:       config,
			logger:       logger,
			pvcsToWatch:  &sync.Map{},
			resizingPVCs: &sync.Map{},
			pvcsQueue:    queue,
		}

		// Process the PVC
		autoscaler.processNextItem()

		pvc, _ = fakeClient.CoreV1().PersistentVolumeClaims("test-ns").Get(context.Background(), "test-pvc", metav1.GetOptions{})
		assert.Contains(t, pvc.Annotations, PVCAutoscalerStatusAnnotation)

		status, _ := UnmarshalStatusFromAnnotation(pvc)

		assert.NotEqual(t, status.LastFailedAttempt, time.Time{})
		assert.Equal(t, status.LastScaleTime, time.Time{})

		pvcId := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)

		_, ok := autoscaler.resizingPVCs.Load(pvcId)
		assert.False(t, ok, "PVC should be deleted from resizingPVCs map")

		assert.True(t, queue.Len() > 0, "work queue should not be empty")

		_, ok = autoscaler.pvcsToWatch.Load(pvcId)
		assert.False(t, ok, "PVC should not be deleted from pvcsToWatch map")
	})

}
