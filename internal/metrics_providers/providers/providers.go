package providers

import (
	corev1 "k8s.io/api/core/v1"
)

type PVCMetricData struct {
	PVCName           string
	PVCNamespace      string
	PVCPercentageUsed float64
}

type MetricsProvider interface {
	QueryPVCUsage(*corev1.PersistentVolumeClaim) (PVCMetricData, error)
}
