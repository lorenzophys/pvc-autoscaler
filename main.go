package main

import (
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	PVCAutoscalerAnnotation = "lorenzophys.io/pvc-autoscaler"
	thresholdPercentage     = 80
	expansion               = 0.2
	pollingInterval         = 5 * time.Second
)

func main() {
	kubeClient := newKubeClient()
	log.Print("new kubernetes client created")
	prometheusClient := newPrometheusClient()
	log.Print("new prometheus client created")

	pvcsToWatch := fetchPVCs(kubeClient)

	for {
		var pvcsToResize []*corev1.PersistentVolumeClaim
		for _, pvc := range pvcsToWatch {
			response, _ := queryPrometheusPVCUtilization(prometheusClient, pvc.Name, pvc.Namespace)
			metric, err := parsePrometheusResponse(response)
			if err != nil {
				log.Print(err)
			}
			log.Printf("utilization of %s/%s: %.2f%%", metric.PVCNamespace, metric.PVCName, metric.PVCPercentageUsed)

			if metric.PVCPercentageUsed >= thresholdPercentage {
				pvcsToResize = append(pvcsToResize, &pvc)
			}
		}

		if len(pvcsToResize) != 0 {
			log.Printf("resizing the followings: %v", pvcsToResize)
			err := managePVCs(kubeClient, pvcsToResize)
			if err != nil {
				log.Print(err.Error())
			}
		}

		time.Sleep(pollingInterval)
	}
}
