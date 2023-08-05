
[![Go Report Card](https://goreportcard.com/badge/github.com/lorenzophys/pvc-autoscaler)](https://goreportcard.com/report/github.com/lorenzophys/pvc-autoscaler)
![GitHub release (with filter)](https://img.shields.io/github/v/release/lorenzophys/pvc-autoscaler?filter=v*&logo=Go)
![GitHub Workflow Status (with event)](https://img.shields.io/github/actions/workflow/status/lorenzophys/pvc-autoscaler/helm-lint-test.yaml?logo=helm&label=Helm)
![GitHub release (with filter)](https://img.shields.io/github/v/release/lorenzophys/pvc-autoscaler?filter=pvcautoscaler-*&logo=Helm&label=Helm%20release)
![GitHub](https://img.shields.io/github/license/lorenzophys/pvc-autoscaler)

# PVC autoscaler for Kubernetes

PVC Autoscaler is an open-source project aimed at providing autoscaling functionality to Persistent Volume Claims (PVCs) in Kubernetes environments. It allows you to automatically scale your PVCs based on your workloads and the metrics collected.

Please note that PVC Autoscaler is currently in a heavy development phase. As such, it's not recommended for production usage at this point.

## Motivation

The motivation behind the PVC Autoscaler project is to provide developers with an easy and efficient way of managing storage resources within their Kubernetes clusters. With the PVC Autoscaler, there's no need to manually adjust the size of your PVCs as your storage needs change. The Autoscaler handles this for you, freeing you up to focus on other areas of your development work.

## Limitations

At this stage of development only one update is possible per PVC. This is due to the fact that different storage types of different cloud providers have constraints on multiple resizing that need to be researched.

Currently it only supports Prometheus for collecting metrics

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

## Testing

To test PVC Autoscaler, you'll need a Kubernetes cluster that supports expandable storage classes, i.e. it contains `allowVolumeExpansion: true`. As an example you can consider:

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

Remember that if you work with EKS you need the EBS CSI Driver. Please refer to [this page](https://docs.aws.amazon.com/eks/latest/userguide/ebs-csi.html) for mor info.

## Contributions

Contributions to PVC Autoscaler are more than welcome! Whether you want to help me improve the code, add new features, fix bugs, or improve our documentation, I would be glad to receive your pull requests and issues.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
