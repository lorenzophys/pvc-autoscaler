package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lorenzophys/pvc-autoscaler/internal/metrics_providers/providers"
	prometheusApi "github.com/prometheus/client_golang/api"
	corev1 "k8s.io/api/core/v1"
)

const (
	apiPrefix = "/api/v1"
)

type PrometheusProvider struct {
}

type PrometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"`
		} `json:"result"`
	} `json:"data"`
	ErrorType string   `json:"errorType"`
	Error     string   `json:"error"`
	Warnings  []string `json:"warnings"`
}

func (p *PrometheusProvider) newPrometheusClient() (prometheusApi.Client, error) {
	client, err := prometheusApi.NewClient(prometheusApi.Config{
		Address: "http://prometheus-server.monitoring.svc.cluster.local", // TODO: Make it configurable
	})
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (p *PrometheusProvider) QueryPVCUsage(pvc *corev1.PersistentVolumeClaim) (providers.PVCMetricData, error) {
	client, err := p.newPrometheusClient()
	if err != nil {
		return providers.PVCMetricData{}, err
	}
	u := client.URL(apiPrefix, nil)
	q := u.Query()

	promQuery := fmt.Sprintf(
		`kubelet_volume_stats_used_bytes{persistentvolumeclaim="%[1]s",namespace="%[2]s"}`+
			`/kubelet_volume_stats_capacity_bytes{persistentvolumeclaim="%[1]s",namespace="%[2]s"}`,
		pvc.Name,
		pvc.Namespace,
	)
	q.Set("query", promQuery)
	encodedArgs := q.Encode()

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/query", u.String()), strings.NewReader(encodedArgs))
	if err != nil {
		return providers.PVCMetricData{}, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header["Idempotency-Key"] = nil

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, body, err := client.Do(ctx, req)
	if err != nil {
		return providers.PVCMetricData{}, nil
	}
	defer resp.Body.Close()

	var promResp PrometheusResponse

	err = json.Unmarshal(body, &promResp)
	if err != nil {
		return providers.PVCMetricData{}, err
	}

	metric, err := p.parsePrometheusResponse(promResp, pvc)
	if err != nil {
		return providers.PVCMetricData{}, err
	}

	return metric, nil
}

func (p *PrometheusProvider) parsePrometheusResponse(response PrometheusResponse, pvc *corev1.PersistentVolumeClaim) (providers.PVCMetricData, error) {
	// TODO: What happens when Prometheus API returns warnings?
	switch response.Status {
	case "error":
		{
			return providers.PVCMetricData{}, fmt.Errorf("error %s in the query: %s", response.ErrorType, response.Error)
		}
	case "success":
		{
			if len(response.Data.Result) == 0 {
				return providers.PVCMetricData{}, fmt.Errorf("prometheus api returned no metrics for %s/%s", pvc.Namespace, pvc.Name)
			} else {
				absoluteUsedStr, ok := response.Data.Result[0].Value[1].(string)
				if !ok {
					break
				}
				absoluteUsed, err := strconv.ParseFloat(absoluteUsedStr, 64)
				if err != nil {
					break
				}
				return providers.PVCMetricData{
					PVCName:           response.Data.Result[0].Metric["persistentvolumeclaim"],
					PVCNamespace:      response.Data.Result[0].Metric["namespace"],
					PVCPercentageUsed: absoluteUsed * 100,
				}, nil
			}
		}
	}
	errorMessage := `received an unexpected response from the Prometheus api,
please consider opening an issue: https://github.com/lorenzophys/pvc-autoscaler/issues`
	return providers.PVCMetricData{}, errors.New(errorMessage)
}
