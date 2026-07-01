# Minimum System Requirements for Running KServe Standalone (Local Setup)

This document provides recommended minimum hardware requirements to run KServe standalone on a single-node Kubernetes cluster such as Minikube for development and testing purposes.

## Recommended Minimum Hardware

For local experimentation and basic model serving workloads:

- **CPU**: 4 vCPUs
- **Memory (RAM)**: 8 GB minimum (16 GB recommended for smoother experience)
- **Disk**: 20 GB available space
- **Kubernetes Version**: v1.24+ recommended

> Note: Lower configurations may work for lightweight examples, but performance may degrade when running inference services with multiple replicas or larger models.

## Starting Minikube

Example command to start Minikube with recommended resources:

```bash
minikube start --cpus=4 --memory=8192 --disk-size=20g
