package prometheus

import (
	"context"
	"fmt"
	"time"

	clients "github.com/lorenzophys/pvc-autoscaler/internal/metrics_clients/clients"
	prometheusApi "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/types"
)

const (
	usedBytesQuery     = "kubelet_volume_stats_used_bytes"
	capacityBytesQuery = "kubelet_volume_stats_capacity_bytes"
)

type PrometheusClient struct {
	prometheusAPI prometheusv1.API
}

func NewPrometheusClient(url string) (clients.MetricsClient, error) {
	client, err := prometheusApi.NewClient(prometheusApi.Config{
		Address: url,
	})
	if err != nil {
		return nil, err
	}
	v1api := prometheusv1.NewAPI(client)

	return &PrometheusClient{
		prometheusAPI: v1api,
	}, nil
}

func (c *PrometheusClient) FetchPVCsMetrics(ctx context.Context) (map[types.NamespacedName]*clients.PVCMetrics, error) {
	volumeStats := make(map[types.NamespacedName]*clients.PVCMetrics)

	usedBytes, err := c.getMetricValues(ctx, usedBytesQuery, time.Now())
	if err != nil {
		return nil, err
	}

	capacityBytes, err := c.getMetricValues(ctx, capacityBytesQuery, time.Now())
	if err != nil {
		return nil, err
	}

	for key, val := range usedBytes {
		pvcMetrics := &clients.PVCMetrics{VolumeUsedBytes: val}
		if cb, ok := capacityBytes[key]; ok {
			pvcMetrics.VolumeCapacityBytes = cb
		} else {
			continue
		}

		volumeStats[key] = pvcMetrics
	}

	return volumeStats, nil
}

func (c *PrometheusClient) getMetricValues(ctx context.Context, query string, time time.Time) (map[types.NamespacedName]int64, error) {
	res, _, err := c.prometheusAPI.Query(ctx, query, time)
	if err != nil {
		return nil, err
	}

	if res.Type() != model.ValVector {
		return nil, fmt.Errorf("unknown response type: %s", res.Type().String())
	}
	resultMap := make(map[types.NamespacedName]int64)
	vec := res.(model.Vector)
	for _, val := range vec {
		nn := types.NamespacedName{
			Namespace: string(val.Metric["namespace"]),
			Name:      string(val.Metric["persistentvolumeclaim"]),
		}
		resultMap[nn] = int64(val.Value)
	}
	return resultMap, nil
}
