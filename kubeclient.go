package main

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func newKubeClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}
