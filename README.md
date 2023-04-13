# Mondoo Operator for Kubernetes

[![Tests](https://github.com/mondoohq/mondoo-operator/actions/workflows/tests.yaml/badge.svg)](https://github.com/mondoohq/mondoo-operator/actions/workflows/tests.yaml)
[![Edge integration tests](https://github.com/mondoohq/mondoo-operator/actions/workflows/edge-integration-tests.yaml/badge.svg)](https://github.com/mondoohq/mondoo-operator/actions/workflows/edge-integration-tests.yaml)
[![Cloud tests](https://github.com/mondoohq/mondoo-operator/actions/workflows/cloud-tests.yaml/badge.svg)](https://github.com/mondoohq/mondoo-operator/actions/workflows/cloud-tests.yaml)
![License](https://img.shields.io/github/license/mondoohq/mondoo-operator)

> **Project Status**: This project is stable. Any API and CRD changes will be handled in way where previous versions are kept working or migrated.

## Overview

The **Mondoo Operator** provides a new [Kubernetes](https://kubernetes.io/) native way to do a security assessment of your whole Kubernetes Cluster. The purpose of this project is to simplify and automate the configuration for a Mondoo-based security assessment for Kubernetes clusters.

The Mondoo Operator provides the following features:

- Continuous validation of deployed workloads
- Continuous validation of Kubernetes nodes **without** privileged access
- Admission Controller

It is backed by Mondoo's powerful Policy-as-Code the Mondoo Query Language (MQL). Mondoo ships out-of-the-box security policies for Kubernetes:

- CIS Kubernetes Benchmark
- Kubernetes Application Benchmark

![Architecture](docs/img/architecture.svg)

## Getting Started

The **Mondoo Operator** can be installed via different methods depending on your Kubernetes workflow:

- [User manual](docs/user-manual.md)

## Tested Kubernetes Environments

The following Kubernetes environments are tested:

- AWS EKS 1.22, 1.23, and 1.24
- Azure AKS 1.23, 1.24, and 1.25
- GCP GKE 1.22, 1.23, and 1.24
- Minikube with Kubernetes versions 1.22, 1.23 and 1.24
- Rancher RKE1 1.22 and 1.23
- K3S 1.22, 1.23 and 1.24

## Documentation

Please see the [docs](/docs) directory for more in-depth information.

## Contributing

Many files (documentation, manifests, ...) are auto-generated. Before proposing a pull request:

1. Commit your changes.
2. Run `make generate` and `make test`.
3. Commit the generated changes.

## Security

If you find a security vulnerability related to the Mondoo Operator, please do not report it by opening a GitHub issue. Instead, send an e-mail to [security@mondoo.com](mailto:security@mondoo.com)

## Join the community!

Join the [Mondoo Community GitHub Discussions](https://github.com/orgs/mondoohq/discussions) to collaborate on policy as code and security automation.

## License

[Mozilla Public License v2.0](https://github.com/mondoohq/mondoo-operator/blob/main/LICENSE)