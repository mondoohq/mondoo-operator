# Mondoo Operator for Kubernetes

[![Tests](https://github.com/mondoohq/mondoo-operator/actions/workflows/tests.yaml/badge.svg)](https://github.com/mondoohq/mondoo-operator/actions/workflows/tests.yaml)
<!-- [![Edge integration tests](https://github.com/mondoohq/mondoo-operator/actions/workflows/edge-integration-tests.yaml/badge.svg)](https://github.com/mondoohq/mondoo-operator/actions/workflows/edge-integration-tests.yaml) -->
<!-- [![Cloud tests](https://github.com/mondoohq/mondoo-operator/actions/workflows/cloud-tests.yaml/badge.svg)](https://github.com/mondoohq/mondoo-operator/actions/workflows/cloud-tests.yaml) -->

> **Project Status**: This project is stable. Any API and CRD changes will be handled in way where previous versions are kept working or migrated.

![mondoo operator illustration](.github/social/preview.jpg)

## Overview

The **Mondoo Operator** provides a [Kubernetes](https://kubernetes.io/) native way to do continuous security assessment of your Kubernetes clusters. The purpose of this project is to simplify and automate the configuration for Mondoo-based security scanning.

It is backed by Mondoo's powerful policy-as-code engine [cnspec](https://mondoo.com/docs/cnspec/cnspec-about/) and [MQL](https://mondoo.com/docs/mql/resources/). Mondoo ships out-of-the-box security policies for:

- CIS Kubernetes Benchmarks
- CIS AKS/EKS/GKE/OpenShift Benchmarks
- NSA/CISA Kubernetes Hardening Guide
- Kubernetes Cluster and Workload Security
- Kubernetes Best Practices

## How It Works

The Mondoo Operator uses a simple, CronJob-based architecture to scan Kubernetes clusters:

```
┌─────────────────────────────────────┐
│         Your Kubernetes Cluster     │
│                                     │
│  ┌─────────────────────────────┐   │
│  │      Mondoo Operator        │   │
│  │                             │   │
│  │  • K8s Resources Scanning   │   │
│  │  • Node Scanning            │   │
│  │  • Container Image Scanning │   │
│  └─────────────────────────────┘   │
│               │                     │
│               ▼                     │
│     CronJobs run cnspec scans       │
│               │                     │
│               ▼                     │
│        Mondoo Platform              │
└─────────────────────────────────────┘
```

Each scan type runs as an independent CronJob that executes `cnspec` directly against the target.

## Configuration

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: mondoo-operator
spec:
  mondooCredsSecretRef:
    name: mondoo-client

  # Scan Kubernetes resources (RBAC, workloads, etc.)
  kubernetesResources:
    enable: true

  # Scan nodes for security compliance
  nodes:
    enable: true

  # Scan container images for vulnerabilities
  containers:
    enable: true
```

## Features

| Feature | Status |
|---------|:------:|
| Kubernetes Resources Scanning | ✅ |
| Node Scanning | ✅ |
| Container Image Scanning | ✅ |
| Namespace Filtering | ✅ |
| Private Registry Support | ✅ |

![Architecture](docs/img/architecture.svg)

## Getting Started

The **Mondoo Operator** can be installed via different methods depending on your Kubernetes workflow:

- [User manual](docs/user-manual.md)

## Tested Kubernetes Environments

The following Kubernetes environments are tested:

<!-- - AWS EKS 1.23, 1.24, 1.25, and 1.26
- Azure AKS 1.24, 1.25, and 1.26
- GCP GKE 1.23, 1.24, 1.25, and 1.26 -->
- Minikube with Kubernetes versions 1.31, 1.32, 1.33, and 1.34
- K3S 1.31, 1.32, 1.33, and 1.34

## Documentation

Please see the [docs](./docs) directory for more in-depth information.

## Contributing

Many files (documentation, manifests, ...) are auto-generated. Before proposing a pull request:

1. Commit your changes.
2. Run `make generate` and `make test`.
3. Commit the generated changes.

### Running the integration tests locally

To run the integration tests locally copy the `.env.example` file:

```bash
cp .env.example .env
```

Go to Mondoo Platform and create an API token for an organization of choice. Add the API token to the `.env` file. Double-check that the API is set to the correct environment, then run:

```bash
make test/integration
```

## Security

If you find a security vulnerability related to the Mondoo Operator, please do not report it by opening a GitHub issue. Instead, send an email to [security@mondoo.com](mailto:security@mondoo.com)

## Join the community!

Join the [Mondoo Community GitHub Discussions](https://github.com/orgs/mondoohq/discussions) to collaborate on policy as code and security automation.
