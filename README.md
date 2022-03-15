# Mondoo Operator for Kubernetes

![badge](https://github.com/mondoohq/mondoo-operator/actions/workflows/e2e.yaml/badge.svg)

> **Project Status**: This project is currently in Early-Access. The API and CRD may change

## Overview

The **Mondoo Operator** provides a new [Kubernetes](https://kubernetes.io/) native way to do a security assessment of your whole Kubernetes Cluster. The purpose of this project is to simplify and automate the configuration for a Mondoo-based security assessment for Kubernetes clusters.

The Mondoo Operator provides the following features:

- Continuous validation of deployed workloads
- Continuous validation of Kubernetes nodes **without** priviledged access
- Admission Controller (coming soon)

It is backed by Mondoo's powerful [Policy-as-Code](https://mondoo.com/docs/getstarted/policy-as-code) engine and [MQL](https://mondoo.com/docs/getstarted/policy-as-code#introducing-the-mondoo-query-language-mql). Mondoo ships out-of-the-box security polices for:

- CIS Kubernetes Benchmark
- Kubernetes Application Benchmark

```
           ┌─────────────────────────────────────────────────────────────────┐
           │                       Kubernetes Cluster                        │
           │┌───┐    ┌────────────────────────┐ ┌ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┐│
           ││   │   ┌┴───────────────────────┐│           DaemonSet          │
           ││   ├──▶│      Application       ├┘ │┌─────────────────────────┐││
           ││   │   └────────────────────────┘   │┌─────────────────┐      │ │
           ││   │                ▲              │││Mondoo Client Pod│ Nodes│││
           ││   │   ┌────────────┴───────────┐   │└─────────────────┘      │ │
┌────────┐ ││   │   │┌──────────────────────┐│  │└─────────────────────────┘││
│        │ ││   │ ┌─▶│  Validating Webhook  ││                               │
│Pipeline│─▶│API│─┘ │└──────────────────────┘│  │                           ││
│        │ ││   │   │                        │   ┌─────────────────────────┐ │
└────────┘ ││   │   │K8s Admission Controller│  ││┌─────────────────┐      │││
           ││   │   └────────────────────────┘   ││Mondoo Client Pod│ Nodes│ │
           ││   │                │              ││└─────────────────┘      │││
           ││   │                ▼               └─────────┬───────────────┘ │
           ││   │       ┌─────────────────┐     │          │                ││
           ││   ◀───────│Mondoo Client Pod│                │                 │
           │└───┘       └─────────────────┘     └ ─ ─ ─ ─ ─│─ ─ ─ ─ ─ ─ ─ ─ ┘│
           └─────────────────────┬─────────────────────────┼─────────────────┘
           ┌─────────────────────▼─────────────────────────▼─────────────────┐
           │               Mondoo Platform (Policies, Reports)               │
           └─────────────────────────────────────────────────────────────────┘
```

## Getting started

The **Mondoo Operator** is available via different installation methods. The are all installing the operator into your cluster:

- [kubectl](docs/user-manual-kubectl.md)
- [helm chart](docs/user-manual-helm.md)
- [operatorhub.io / olm](docs/user-manual-olm.md)

## Tested Kuberntes environments

The operator has been tested in the following environments

- AWS EKS 1.21
- Azure AKS 1.21
- GCP GKE 1.21 and 1.22
- Minikube
- K3S

## Documentation

Please see the [docs](/docs) directory for more in-depth information.

## Contributing

Many files (documentation, manifests, ...) are auto-generated. Before proposing a pull request:

1. Commit your changes.
2. Run `make generate` and `make test`.
3. Commit the generated changes.

## Security

If you find a security vulnerability related to the Mondoo Operator, please do not report it by opening a GitHub issue. Instead send an e-mail to [security](mailto:security@mondoo.com)
