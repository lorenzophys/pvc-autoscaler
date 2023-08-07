package providers

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
)

type PVCMetrics struct {
	VolumeUsedBytes     int64
	VolumeCapacityBytes int64
}

type MetricsClient interface {
	FetchPVCsMetrics(context.Context) (map[types.NamespacedName]*PVCMetrics, error)
}
