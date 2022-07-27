# Mondoo Operator for Kubernetes

![CI](https://github.com/mondoohq/mondoo-operator/actions/workflows/tests.yaml/badge.svg)
![License](https://img.shields.io/github/license/mondoohq/mondoo-operator)

> **Project Status**: This project is stable. Any API and CRD changes will be handled in way where previous versions are kept working or migrated.

## Overview

The **Mondoo Operator** provides a new [Kubernetes](https://kubernetes.io/) native way to do a security assessment of your whole Kubernetes Cluster. The purpose of this project is to simplify and automate the configuration for a Mondoo-based security assessment for Kubernetes clusters.

The Mondoo Operator provides the following features:

- Continuous validation of deployed workloads
- Continuous validation of Kubernetes nodes **without** privileged access
- Admission Controller (alpha version)

It is backed by Mondoo's powerful Policy-as-Code the Mondoo Query Language (MQL). Mondoo ships out-of-the-box security policies for Kubernetes:

- CIS Kubernetes Benchmark
- Kubernetes Application Benchmark

![Architecture](docs/img/architecture.svg)

## Getting Started

The **Mondoo Operator** can be installed via different methods depending on your Kubernetes workflow:

- [User manual](docs/user-manual.md)

## Tested Kubernetes Environments

The following Kubernetes environments are tested:

- AWS EKS 1.21
- Azure AKS 1.21
- GCP GKE 1.22 and 1.23
- Minikube with Kubernetes versions 1.22, 1.23 and 1.24
- Rancher RKE1 1.22 and 1.23
- K3S

## Documentation

Please see the [docs](/docs) directory for more in-depth information.

## Contributing

Many files (documentation, manifests, ...) are auto-generated. Before proposing a pull request:

1. Commit your changes.
2. Run `make generate` and `make test`.
3. Commit the generated changes.

## Security

If you find a security vulnerability related to the Mondoo Operator, please do not report it by opening a GitHub issue. Instead, send an e-mail to [security@mondoo.com](mailto:security@mondoo.com)

## License

```text
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```
