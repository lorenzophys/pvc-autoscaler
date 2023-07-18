package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PVCStatus struct {
	PVC *corev1.PersistentVolumeClaim
	Err error
}

func fetchPVCs(kubeClient *kubernetes.Clientset) []corev1.PersistentVolumeClaim {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pvcs, err := kubeClient.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("failed to fetch PersistentVolumeClaims: %s", err.Error())
	}

	var watchedPVCs []corev1.PersistentVolumeClaim
	for _, pvc := range pvcs.Items {
		annotations := pvc.GetAnnotations()
		if value, ok := annotations[PVCAutoscalerAnnotation]; ok && value == "enabled" {
			watchedPVCs = append(watchedPVCs, pvc)
		}
	}
	log.Printf("there are %d PersistentVolumeClaims with autoscaling enabled in the cluster", len(watchedPVCs))

	var pvcString []string
	for _, pvc := range watchedPVCs {
		s := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
		pvcString = append(pvcString, s)
	}
	log.Printf("watching the following PVCs: %v", strings.Join(pvcString, ", "))

	return watchedPVCs
}

func managePVCs(kubeClient *kubernetes.Clientset, pvcsToResize []*corev1.PersistentVolumeClaim) error {
	statusChannel := make(chan PVCStatus)

	for _, pvcToResize := range pvcsToResize {
		log.Printf("spawning gorutine for pvc: %s", pvcToResize.Name)
		go func(pvc *corev1.PersistentVolumeClaim) {
			err := resizePVC(kubeClient, pvc)
			statusChannel <- PVCStatus{PVC: pvc, Err: err}
		}(pvcToResize)
	}

	for i := 0; i < len(pvcsToResize); i++ {
		pvcStatus := <-statusChannel
		if pvcStatus.Err != nil {
			return fmt.Errorf("error resizing PVC %s/%s: %v", pvcStatus.PVC.Name, pvcStatus.PVC.Namespace, pvcStatus.Err)
		} else {
			log.Printf("successfully resized PVC %s/%s", pvcStatus.PVC.Name, pvcStatus.PVC.Namespace)
		}
	}

	return nil
}

func resizePVC(kubeClient *kubernetes.Clientset, pvcToResize *corev1.PersistentVolumeClaim) error {
	currenStorage := pvcToResize.Spec.Resources.Requests[corev1.ResourceStorage]
	newStorage := float64(currenStorage.Value()) * (1 + expansion)
	newQuantity := resource.NewQuantity(int64(newStorage), resource.DecimalSI)

	pvcToResize.Spec.Resources.Requests[corev1.ResourceStorage] = *newQuantity

	ctxUpdate, cancelUpdate := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelUpdate()

	log.Printf("start updating pvc: %s", pvcToResize.Name)
	_, err := kubeClient.CoreV1().PersistentVolumeClaims(pvcToResize.Namespace).Update(ctxUpdate, pvcToResize, metav1.UpdateOptions{})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("timed out while trying to update PVC: %v", err)
		} else {
			return fmt.Errorf("failed to update PVC: %v", err)
		}
	}

	log.Printf("update successful for %s, now waiting for the pvc to accept the resize", pvcToResize.Name)

	ctxPoll, cancelPoll := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancelPoll()

	for {
		select {
		case <-ctxPoll.Done():
			return fmt.Errorf("timed out waiting for PVC %s/%s to accept resize", pvcToResize.Namespace, pvcToResize.Name)
		default:
			pvc, err := kubeClient.CoreV1().PersistentVolumeClaims(pvcToResize.Namespace).Get(ctxPoll, pvcToResize.Name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to fetch PVC %s/%s", pvcToResize.Namespace, pvcToResize.Name)
			}

			for _, condition := range pvc.Status.Conditions {
				if condition.Type == corev1.PersistentVolumeClaimFileSystemResizePending {
					log.Printf("the pvc %s is still waiting to accept the resize", pvcToResize.Name)
					continue
				} else if pvc.Status.Phase == corev1.ClaimBound && pvc.Status.Capacity[corev1.ResourceStorage] == *newQuantity {
					log.Printf("resize accepted by the pvc: %s", pvcToResize.Name)
					return nil
				}
			}
			time.Sleep(1 * time.Second)
		}
	}
}
