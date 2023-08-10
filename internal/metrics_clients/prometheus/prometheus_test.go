package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
		_, err = client.FetchPVCsMetrics(context.TODO())
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
}
