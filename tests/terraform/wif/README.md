# WIF Testing Terraform

Terraform configurations for provisioning cloud infrastructure to test Workload Identity Federation (WIF) with the Mondoo Operator.

Each subdirectory creates a **management cluster** (where the operator runs) and a **target cluster** (to be scanned), along with the IAM/RBAC plumbing needed for WIF authentication.

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.3
- Cloud provider CLI authenticated:
  - **GKE**: `gcloud auth application-default login`
  - **EKS**: `aws configure` or environment variables
  - **AKS**: `az login`

## Usage

Each provider is independent. Navigate to the desired directory and run:

```bash
cd gke/   # or eks/ or aks/
terraform init
terraform plan
terraform apply
```

After `apply` completes, check the outputs for the `mondoo_audit_config_snippet` which shows the exact YAML to use in your `MondooAuditConfig`.

```bash
terraform output mondoo_audit_config_snippet
```

## Providers

### GKE (`gke/`)

Creates two GKE Standard clusters with Workload Identity enabled. Sets up a Google Service Account with the necessary IAM bindings for the management cluster's KSA to authenticate to the target cluster.

**Required variables:**
- `project_id` - GCP project ID

**Post-apply manual step:** Create a `ClusterRoleBinding` on the target cluster granting the GSA read access (shown in outputs).

### GKE Autopilot (`gke-autopilot/`)

Same as GKE but uses Autopilot clusters. Autopilot clusters have Workload Identity enabled by default and manage node pools automatically.

**Required variables:**
- `project_id` - GCP project ID

**Post-apply manual step:** Create a `ClusterRoleBinding` on the target cluster granting the GSA read access (shown in outputs).

### EKS (`eks/`)

Creates two EKS clusters in a shared VPC with IRSA (IAM Roles for Service Accounts) configured. Sets up an IAM role with a trust policy for the management cluster's OIDC provider and maps it into the target cluster's `aws-auth` ConfigMap.

**Required variables:** None (uses defaults)

### AKS (`aks/`)

Creates two AKS clusters with Azure Workload Identity configured. Sets up an Azure AD application with a federated identity credential and grants it RBAC on the target cluster.

**Required variables:** None (uses defaults)

## Cleanup

```bash
terraform destroy
```

## Design Notes

- Spot/preemptible instances are used for cost savings
- Random 4-character suffixes ensure unique resource names
- Terraform state is stored locally (not in a remote backend)
- These configs are for manual developer testing, not CI
