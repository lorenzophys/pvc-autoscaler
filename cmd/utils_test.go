package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestIsStorageClassExpandable(t *testing.T) {
	ctx := context.TODO()

	tests := []struct {
		name             string
		mockStorageClass *storagev1.StorageClass
		mockError        error
		expectedResult   bool
		expectedError    error
	}{
		{
			name:             "StorageClass is expandable",
			mockStorageClass: &storagev1.StorageClass{AllowVolumeExpansion: boolPtr(true)},
			mockError:        nil,
			expectedResult:   true,
			expectedError:    nil,
		},
		{
			name:             "StorageClass is not expandable",
			mockStorageClass: &storagev1.StorageClass{AllowVolumeExpansion: boolPtr(false)},
			mockError:        nil,
			expectedResult:   false,
			expectedError:    nil,
		},
		{
			name:             "StorageClass AllowVolumeExpansion property is nil",
			mockStorageClass: &storagev1.StorageClass{AllowVolumeExpansion: nil},
			mockError:        nil,
			expectedResult:   false,
			expectedError:    nil,
		},
		{
			name:             "Error fetching StorageClass",
			mockStorageClass: nil,
			mockError:        errors.New("mock error"),
			expectedResult:   false,
			expectedError:    errors.New("mock error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()

			if tt.mockStorageClass != nil {
				client.StorageV1().StorageClasses().Create(ctx, tt.mockStorageClass, metav1.CreateOptions{})
			}

			client.PrependReactor("get", "storageclasses", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, tt.mockStorageClass, tt.mockError
			})

			result, err := isStorageClassExpandable(ctx, client, "mock-sc")

			assert.Equal(t, tt.expectedResult, result)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestGetAnnotatedPVCs(t *testing.T) {
	ctx := context.TODO()

	tests := []struct {
		name              string
		mockPVCs          []corev1.PersistentVolumeClaim
		mockError         error
		expectedPVCsCount int
		expectedError     error
	}{
		{
			name:              "No PVCs available",
			mockPVCs:          []corev1.PersistentVolumeClaim{},
			mockError:         nil,
			expectedPVCsCount: 0,
			expectedError:     nil,
		},
		{
			name: "PVCs without annotation",
			mockPVCs: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2"}},
			},
			mockError:         nil,
			expectedPVCsCount: 0,
			expectedError:     nil,
		},
		{
			name: "Some PVCs with annotation",
			mockPVCs: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "pvc-1",
						Annotations: map[string]string{PVCAutoscalerEnabledAnnotation: "true"},
					},
				},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2"}},
			},
			mockError:         nil,
			expectedPVCsCount: 1,
			expectedError:     nil,
		},
		{
			name:              "Error fetching PVCs",
			mockPVCs:          nil,
			mockError:         errors.New("mock error"),
			expectedPVCsCount: 0,
			expectedError:     errors.New("mock error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()

			for _, pvc := range tt.mockPVCs {
				client.CoreV1().PersistentVolumeClaims("").Create(ctx, &pvc, metav1.CreateOptions{})
			}

			client.PrependReactor("list", "persistentvolumeclaims", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, &corev1.PersistentVolumeClaimList{Items: tt.mockPVCs}, tt.mockError
			})

			result, err := getAnnotatedPVCs(ctx, client)

			assert.Equal(t, tt.expectedError, err)
			if err == nil {
				assert.Equal(t, tt.expectedPVCsCount, len(result.Items))
			}
		})
	}
}

func TestGetPVCStorageCeiling(t *testing.T) {
	tests := []struct {
		name           string
		pvc            *corev1.PersistentVolumeClaim
		expectedResult resource.Quantity
		expectedError  error
	}{
		{
			name: "PVC with valid ceiling annotation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						PVCAutoscalerCeilingAnnotation: "10Gi",
					},
				},
			},
			expectedResult: resource.MustParse("10Gi"),
			expectedError:  nil,
		},
		{
			name: "PVC with invalid ceiling annotation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						PVCAutoscalerCeilingAnnotation: "invalid",
					},
				},
			},
			expectedResult: resource.Quantity{},
			expectedError:  resource.ErrFormatWrong,
		},
		{
			name: "PVC without ceiling annotation but with storage limit",
			pvc: &corev1.PersistentVolumeClaim{
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("5Gi"),
						},
					},
				},
			},
			expectedResult: resource.MustParse("5Gi"),
			expectedError:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getPVCStorageCeiling(tt.pvc)

			assert.Equal(t, tt.expectedResult, result)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func TestConvertPercentageToBytes(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		capacity     int64
		defaultValue string
		expectedRes  int64
		expectedErr  error
	}{
		{
			name:         "Valid percentage",
			value:        "50%",
			capacity:     200,
			defaultValue: "25%",
			expectedRes:  100,
			expectedErr:  nil,
		},
		{
			name:         "Value exceeds 100%",
			value:        "150%",
			capacity:     200,
			defaultValue: "25%",
			expectedRes:  0,
			expectedErr:  fmt.Errorf("annotation value is 150%%, but should between 0%% and 100%%"),
		},
		{
			name:         "Negative value",
			value:        "-50%",
			capacity:     200,
			defaultValue: "25%",
			expectedRes:  0,
			expectedErr:  fmt.Errorf("annotation value is -50%%, but should between 0%% and 100%%"),
		},
		{
			name:         "Invalid value",
			value:        "abc%",
			capacity:     200,
			defaultValue: "25%",
			expectedRes:  0,
			expectedErr:  &strconv.NumError{Func: "ParseFloat", Num: "abc", Err: errors.New("invalid syntax")},
		},
		{
			name:         "Not a percentage",
			value:        "50",
			capacity:     200,
			defaultValue: "25%",
			expectedRes:  0,
			expectedErr:  errors.New("annotation value should be a percentage"),
		},
		{
			name:         "Empty value, valid default",
			value:        "",
			capacity:     400,
			defaultValue: "25%",
			expectedRes:  100,
			expectedErr:  nil,
		},
		{
			name:         "Empty value, invalid default",
			value:        "",
			capacity:     400,
			defaultValue: "150%",
			expectedRes:  0,
			expectedErr:  fmt.Errorf("annotation value is 150%%, but should between 0%% and 100%%"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertPercentageToBytes(tt.value, tt.capacity, tt.defaultValue)

			assert.Equal(t, tt.expectedRes, result)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}

func TestIsPVCResizable(t *testing.T) {
	tests := []struct {
		name        string
		pvc         *corev1.PersistentVolumeClaim
		expectedErr error
	}{
		{
			name: "Valid resizable PVC",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						PVCAutoscalerCeilingAnnotation: "10Gi",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeMode: func() *corev1.PersistentVolumeMode {
						vm := corev1.PersistentVolumeFilesystem
						return &vm
					}(),
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimBound,
				},
			},
			expectedErr: nil,
		},
		{
			name: "Error in ceiling annotation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						PVCAutoscalerCeilingAnnotation: "invalid",
					},
				},
			},
			expectedErr: fmt.Errorf("invalid storage ceiling in the annotation: %w", resource.ErrFormatWrong),
		},
		{
			name: "Ceiling is zero",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						PVCAutoscalerCeilingAnnotation: "0Gi",
					},
				},
			},
			expectedErr: errors.New("the storage ceiling is zero"),
		},
		{
			name: "Non-filesystem volume mode",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						PVCAutoscalerCeilingAnnotation: "10Gi",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeMode: func() *corev1.PersistentVolumeMode {
						vm := corev1.PersistentVolumeBlock
						return &vm
					}(),
				},
			},
			expectedErr: errors.New("the associated volume must be formatted with a filesystem"),
		},
		{
			name: "PVC not bound",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						PVCAutoscalerCeilingAnnotation: "10Gi",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeMode: func() *corev1.PersistentVolumeMode {
						vm := corev1.PersistentVolumeFilesystem
						return &vm
					}(),
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimLost,
				},
			},
			expectedErr: errors.New("not bound to any pod"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := isPVCResizable(tt.pvc)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}
