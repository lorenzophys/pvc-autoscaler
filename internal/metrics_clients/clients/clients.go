package providers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

type PVCMetrics struct {
	VolumeUsedBytes     int64
	VolumeCapacityBytes int64
}

type MetricsClient interface {
	FetchPVCsMetrics(context.Context, time.Time) (map[types.NamespacedName]*PVCMetrics, error)
}
