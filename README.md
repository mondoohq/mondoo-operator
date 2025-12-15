# Mondoo Operator for Kubernetes

[![Tests](https://github.com/mondoohq/mondoo-operator/actions/workflows/tests.yaml/badge.svg)](https://github.com/mondoohq/mondoo-operator/actions/workflows/tests.yaml)
<!-- [![Edge integration tests](https://github.com/mondoohq/mondoo-operator/actions/workflows/edge-integration-tests.yaml/badge.svg)](https://github.com/mondoohq/mondoo-operator/actions/workflows/edge-integration-tests.yaml) -->
<!-- [![Cloud tests](https://github.com/mondoohq/mondoo-operator/actions/workflows/cloud-tests.yaml/badge.svg)](https://github.com/mondoohq/mondoo-operator/actions/workflows/cloud-tests.yaml) -->

> **Project Status**: This project is stable. Any API and CRD changes will be handled in way where previous versions are kept working or migrated.

![mondoo operator illustration](.github/social/preview.jpg)

## Overview

The **Mondoo Operator** provides a new [Kubernetes](https://kubernetes.io/) native way to do a security assessment of your whole Kubernetes Cluster. The purpose of this project is to simplify and automate the configuration for a Mondoo-based security assessment for Kubernetes clusters.

The Mondoo Operator provides the following features:

- Continuous validation of deployed workloads
- Continuous validation of Kubernetes nodes **without** privileged access
- Admission Controller

It is backed by Mondoo's powerful policy-as-code engine [cnspec](https://mondoo.com/docs/cnspec/cnspec-about/) and [MQL](https://mondoo.com/docs/mql/resources/). Mondoo ships out-of-the-box security policies for:

- CIS Kubernetes Benchmarks
- CIS AKS/EKS/GKE/OpenShift Benchmarks
- NSA/CISA Kubernetes Hardening Guide
- Kubernetes Cluster and Workload Security
- Kubernetes Best Practices

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
