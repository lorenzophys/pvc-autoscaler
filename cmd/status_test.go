package main

import (
	"io"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInitPVCAnnotations(t *testing.T) {
	t.Run("annotations not initialized", func(t *testing.T) {
		// Prepare test PVC without AutoscalerStatusAnnotation
		testPVC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: make(map[string]string),
			},
		}

		logger := log.New()
		logger.SetOutput(io.Discard)

		autoscaler := &PVCAutoscaler{
			logger: logger,
		}

		// Call the function under test
		err := autoscaler.initPVCAnnotations(testPVC)
		assert.Nil(t, err)

		// Check if AutoscalerStatusAnnotation has been set
		statusStr, ok := testPVC.Annotations[PVCAutoscalerStatusAnnotation]
		assert.True(t, ok)

		// Check if AutoscalerStatusAnnotation is the marshalled version of the default PVCAutoscalerStatus
		expectedStatus := &PVCAutoscalerStatus{
			LastScaleTime: time.Time{},
		}

		expectedStatusStr, err := expectedStatus.MarshalToAnnotation()
		assert.Nil(t, err)
		assert.Equal(t, expectedStatusStr, statusStr)
	})

	t.Run("annotations already initialized", func(t *testing.T) {
		// Prepare test PVC with AutoscalerStatusAnnotation
		status := &PVCAutoscalerStatus{
			LastScaleTime: time.Now(),
		}

		statusStr, err := status.MarshalToAnnotation()
		assert.Nil(t, err)

		testPVC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					PVCAutoscalerStatusAnnotation: statusStr,
				},
			},
		}

		logger := log.New()
		logger.SetOutput(io.Discard)

		autoscaler := &PVCAutoscaler{
			logger: logger,
		}

		// Call the function under test
		err = autoscaler.initPVCAnnotations(testPVC)
		assert.Nil(t, err)

		// Check if AutoscalerStatusAnnotation remains the same
		newStatusStr, ok := testPVC.Annotations[PVCAutoscalerStatusAnnotation]
		assert.True(t, ok)
		assert.Equal(t, statusStr, newStatusStr)
	})
}

func TestRemovePVCAnnotations(t *testing.T) {
	// Initialize test PVC with AutoscalerStatusAnnotation
	status := &PVCAutoscalerStatus{
		LastScaleTime: time.Now(),
	}

	statusStr, err := status.MarshalToAnnotation()
	assert.Nil(t, err)

	testPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				PVCAutoscalerStatusAnnotation: statusStr,
			},
		},
	}

	logger := log.New()
	logger.SetOutput(io.Discard)

	autoscaler := &PVCAutoscaler{
		logger: logger,
	}

	// Call the function under test
	autoscaler.removePVCAnnotations(testPVC)

	// Check if AutoscalerStatusAnnotation has been removed
	_, ok := testPVC.Annotations[PVCAutoscalerStatusAnnotation]
	assert.False(t, ok)
}
