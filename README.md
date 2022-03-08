# Mondoo Operator for Kubernetes

![badge](https://github.com/mondoohq/mondoo-operator/actions/workflows/e2e.yaml/badge.svg)

> **This project is currently in Early-Access**

## Overview

The **Mondoo Operator** provides a new way to do a security assessment of your Kubernetes Cluster. Once deployed, the Mondoo Operator provides:

- Continious validation of deployed workloads
- Continious validation of Kubernetes nodes (no priviledged access required)

## Getting started

The **Mondoo Operator** is available via

- kubectl manifest
- helm chart
- operatorhub.io

Read the seperate installation instructions:

- [Install Mondoo Operator with kubectl](docs/user-manual-kubectl.md)
- [Install Mondoo Operator with helm](docs/user-manual-helm.md)
- [Install Mondoo Operator with olm](docs/user-manual-olm.md)

## Tested environments

The operator has been tested in the following environments

- EKS: 1.21
- AKS: 1.21
- GKE: 1.21 and 1.22
