package prometheus

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	clients "github.com/lorenzophys/pvc-autoscaler/internal/metrics_clients/clients"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	prometheusmodel "github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/types"
)

type MockPrometheusAPI struct {
}

func TestGetMetricValues(t *testing.T) {
	t.Run("server not found", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(http.NotFound))
		defer ts.Close()

		// If 404 the client should be created
		client, err := NewPrometheusClient(ts.URL)
		assert.NoError(t, err)

		// but the metrics obviously cannot be fetched
		_, err = client.FetchPVCsMetrics(context.TODO(), time.Time{})
		assert.Error(t, err)

	})

	t.Run("good query", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAPI := NewMockAPI(ctrl)

		client := &PrometheusClient{
			prometheusAPI: mockAPI,
		}

		mockReturn := prometheusmodel.Vector{
			&prometheusmodel.Sample{
				Metric:    prometheusmodel.Metric{"namespace": "default", "persistentvolumeclaim": "mypvc"},
				Value:     100,
				Timestamp: prometheusmodel.TimeFromUnix(123),
			},
		}
		expectedResult := map[types.NamespacedName]int64{
			{Namespace: "default", Name: "mypvc"}: 100,
		}

		mockAPI.
			EXPECT().
			Query(context.TODO(), "good_query", time.Time{}).
			Return(mockReturn, nil, nil).
			AnyTimes()

		result, err := client.getMetricValues(context.TODO(), "good_query", time.Time{})

		assert.NoError(t, err)
		assert.Equal(t, expectedResult, result)
	})

	t.Run("bad query", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAPI := NewMockAPI(ctrl)

		client := &PrometheusClient{
			prometheusAPI: mockAPI,
		}

		mockAPI.
			EXPECT().
			Query(context.TODO(), "bad_query", time.Time{}).
			Return(nil, nil, errors.New("generic error")).
			AnyTimes()

		_, err := client.getMetricValues(context.TODO(), "bad_query", time.Time{})

		assert.Error(t, err)

	})
}

func TestFetchPVCsMetrics(t *testing.T) {
	t.Run("everything fine", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAPI := NewMockAPI(ctrl)

		client := &PrometheusClient{
			prometheusAPI: mockAPI,
		}

		mockUsedBytesQuery := prometheusmodel.Vector{
			&prometheusmodel.Sample{
				Metric:    prometheusmodel.Metric{"namespace": "default", "persistentvolumeclaim": "mypvc"},
				Value:     80,
				Timestamp: prometheusmodel.TimeFromUnix(123),
			},
		}

		mockCapacityBytesQuery := prometheusmodel.Vector{
			&prometheusmodel.Sample{
				Metric:    prometheusmodel.Metric{"namespace": "default", "persistentvolumeclaim": "mypvc"},
				Value:     100,
				Timestamp: prometheusmodel.TimeFromUnix(123),
			},
		}

		expectedPVCMetric := &clients.PVCMetrics{
			VolumeUsedBytes:     80,
			VolumeCapacityBytes: 100,
		}

		expectedResult := map[types.NamespacedName]*clients.PVCMetrics{
			{Namespace: "default", Name: "mypvc"}: expectedPVCMetric,
		}

		mockAPI.
			EXPECT().
			Query(context.TODO(), gomock.Any(), time.Time{}).
			DoAndReturn(func(ctx context.Context, query string, time time.Time, args ...any) (prometheusmodel.Value, prometheusv1.Warnings, error) {
				if query == usedBytesQuery {
					return mockUsedBytesQuery, nil, nil
				} else {
					return mockCapacityBytesQuery, nil, nil
				}
			}).Times(2)

		result, err := client.FetchPVCsMetrics(context.TODO(), time.Time{})

		assert.NoError(t, err)
		assert.Equal(t, expectedResult, result)
	})

}
