package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	prometheusApi "github.com/prometheus/client_golang/api"
)

const (
	apiPrefix = "/api/v1"
)

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

type PrometheusPVCMetric struct {
	PVCName           string
	PVCNamespace      string
	PVCPercentageUsed float64
}

func newPrometheusClient() prometheusApi.Client {
	client, err := prometheusApi.NewClient(prometheusApi.Config{
		Address: "http://prometheus-server.monitoring.svc.cluster.local",
	})
	if err != nil {
		log.Fatalf("Error creating client: %v\n", err)
	}

	return client
}

func queryPrometheusPVCUtilization(client prometheusApi.Client, pvcName, namespace string) (PrometheusResponse, error) {
	u := client.URL(apiPrefix, nil)
	q := u.Query()

	promQuery := fmt.Sprintf(
		`kubelet_volume_stats_used_bytes{persistentvolumeclaim="%[1]s",namespace="%[2]s"}`+
			`/kubelet_volume_stats_capacity_bytes{persistentvolumeclaim="%[1]s",namespace="%[2]s"}`,
		pvcName,
		namespace,
	)
	q.Set("query", promQuery)
	encodedArgs := q.Encode()

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/query", u.String()), strings.NewReader(encodedArgs))
	if err != nil {
		log.Fatalf("Error creating new request: %v", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header["Idempotency-Key"] = nil

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, body, err := client.Do(ctx, req)
	if err != nil {
		log.Fatalf("Error executing request: %v", err)
	}
	defer resp.Body.Close()

	var promResp PrometheusResponse

	err = json.Unmarshal(body, &promResp)
	if err != nil {
		log.Fatal(err)
	}

	return promResp, err
}

func parsePrometheusResponse(response PrometheusResponse) (PrometheusPVCMetric, error) {
	// TODO: What happens when Prometheus API returns warnings?
	switch response.Status {
	case "error":
		{
			return PrometheusPVCMetric{}, fmt.Errorf("error %s in the query: %s", response.ErrorType, response.Error)
		}
	case "success":
		{
			if len(response.Data.Result) == 0 {
				return PrometheusPVCMetric{}, errors.New("prometheus api returned no metrics for this PVC")
			} else {
				absoluteUsedStr, ok := response.Data.Result[0].Value[1].(string)
				if !ok {
					break
				}
				absoluteUsed, err := strconv.ParseFloat(absoluteUsedStr, 64)
				if err != nil {
					break
				}
				return PrometheusPVCMetric{
					PVCName:           response.Data.Result[0].Metric["persistentvolumeclaim"],
					PVCNamespace:      response.Data.Result[0].Metric["namespace"],
					PVCPercentageUsed: absoluteUsed * 100,
				}, nil
			}
		}
	}
	errorMessage := `received an unexpected response from the Prometheus api,
please consider opening an issue: https://github.com/lorenzophys/pvc-autoscaler/issues`
	return PrometheusPVCMetric{}, errors.New(errorMessage)
}
