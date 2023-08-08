[![Go Report Card](https://goreportcard.com/badge/github.com/lorenzophys/pvc-autoscaler)](https://goreportcard.com/report/github.com/lorenzophys/pvc-autoscaler)
![GitHub Workflow Status (with event)](https://img.shields.io/github/actions/workflow/status/lorenzophys/pvc-autoscaler/go-lint-test-build.yaml?logo=Go)
![GitHub release (with filter)](https://img.shields.io/github/v/release/lorenzophys/pvc-autoscaler?filter=v*&logo=Go)
![GitHub Workflow Status (with event)](https://img.shields.io/github/actions/workflow/status/lorenzophys/pvc-autoscaler/helm-lint-test.yaml?logo=helm&label=Helm)
![GitHub release (with filter)](https://img.shields.io/github/v/release/lorenzophys/pvc-autoscaler?filter=pvcautoscaler-*&logo=Helm&label=Helm%20release)
![GitHub](https://img.shields.io/github/license/lorenzophys/pvc-autoscaler)

# PVC autoscaler for Kubernetes

PVC Autoscaler is an open-source project aimed at providing autoscaling functionality to Persistent Volume Claims (PVCs) in Kubernetes environments. It allows you to automatically scale your PVCs based on your workloads and the metrics collected.

Please note that PVC Autoscaler is currently in a heavy development phase. As such, it's not recommended for production usage at this point.

## Motivation

The motivation behind the PVC Autoscaler project is to provide developers with an easy and efficient way of managing storage resources within their Kubernetes clusters: sometimes is difficult to estimate how much storage an application needs. With the PVC Autoscaler, there's no need to manually adjust the size of your PVCs as your storage needs change. The Autoscaler handles this for you, freeing you up to focus on other areas of your development work.

## How it works

![pvc-autoscaler-architecture](https://github.com/lorenzophys/pvc-autoscaler/assets/63981558/5dce9455-c7e1-49df-ba1c-4f88964139a3)

## Limitations

Currently it only supports Prometheus for collecting metrics

## Requirements

1. Managed Kubernetes cluster (EKS, AKS, etc...)
2. CSI driver that supports [`VolumeExpansion`](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#csi-volume-expansion)
3. A storage class with the `allowVolumeExpansion` field set to `true`
4. Only volumes with `Filesystem` mode are supported
5. A metrics collector (default: [Prometheus](https://github.com/prometheus-community/helm-charts))

## Installation

PVC Autoscaler comes with a Helm chart for easy deployment in a Kubernetes cluster.

To install the PVC Autoscaler using its Helm chart, first add the repository:

```console
helm repo add pvc-autoscaler https://lorenzophys.github.io/pvc-autoscaler
```

then you can install the chart by running:

```console
helm install <release-name> pvc-autoscaler/pvcautoscaler -n kube-system
```

Replace `<release-name>` with the name you'd like to give to this Helm release.

## Usage

Using `pvc-autoscaler` requires a `StorageClass` that allows volume expansion, i.e. with the `allowVolumeExpansion` field set to `true`. In case of `EKS` you can define:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3-expandable
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  fsType: ext4
reclaimPolicy: Delete
allowVolumeExpansion: true
```

Then set up the `PersistentVolumeClaim` based on the following example:

```yaml
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: my-pvc
  annotations:
    pvc-autoscaler.lorenzophys.io/enabled: "true"
    pvc-autoscaler.lorenzophys.io/threshold: 80%
    pvc-autoscaler.lorenzophys.io/ceiling: 20Gi
    pvc-autoscaler.lorenzophys.io/increase: 20%
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: gp3-expandable
  volumeMode: Filesystem
  resources:
    requests:
      storage: 10Gi
```

* set `spec.storageClassName` to the name of the expandable `StorageClass` defined above
* make sure `spec.volumeMode` is set to `Filesystem` (if you have a block storage this won't work)

Then setup `metadata.annotations` this way:

* to enable autoscaling set `metadata.annotations.pvc-autoscaler.lorenzophys.io/enabled` to `"true"`
* the `metadata.annotations.pvc-autoscaler.lorenzophys.io/threshold` annotation fixes the volume usage above which the resizing will be triggered (default: 80%)
* set how much to increase via `metadata.annotations.pvc-autoscaler.lorenzophys.io/increase` (default 20%)
* to avoid infinite scaling you can set a maximum size for your volume via `metadata.annotations.pvc-autoscaler.lorenzophys.io/ceiling` (default: max size set by the volume provider)

## Contributions

Contributions to PVC Autoscaler are more than welcome! Whether you want to help me improve the code, add new features, fix bugs, or improve our documentation, I would be glad to receive your pull requests and issues.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
