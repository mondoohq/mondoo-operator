# E2E Test Suite for Mondoo Operator

End-to-end tests that deploy the Mondoo operator to real Kubernetes clusters (GKE, EKS, and AKS) and verify scanning works.

## Architecture

The e2e test suite is structured for multi-cloud support:

- **`scripts/`** — Cloud-agnostic shared scripts (build, deploy, verify, etc.)
- **`manifests/`** — Shared manifests (e.g., nginx test workload)
- **`run-*.sh`** — Shared test runners that accept a cloud argument (e.g., `./run-fresh-deploy.sh gke`)
- **`gke/`** — GKE-specific terraform and manifests
- **`eks/`** — EKS-specific terraform and manifests
- **`aks/`** — AKS-specific terraform and manifests

Each cloud directory contains:
- `terraform/` — Infrastructure provisioning (cluster, registry, Mondoo space, optional WIF/target cluster)
- `manifests/` — Cloud-specific MondooAuditConfig templates

## Test Cases

All runners accept a cloud argument: `./run-<test>.sh <cloud>` where `<cloud>` is `gke`, `eks`, or `aks`.

| Test | Runner | Description |
|------|--------|-------------|
| Fresh Deploy | `run-fresh-deploy.sh` | Build operator, deploy to cluster, verify scanning |
| Upgrade | `run-upgrade.sh` | Install baseline version, upgrade to current branch, verify |
| External Cluster | `run-external-cluster.sh` | External cluster scanning via static kubeconfig Secret |
| Vault External | `run-vault-external-cluster.sh` | External cluster scanning via HashiCorp Vault |
| WIF External | `run-wif-external-cluster.sh` | External cluster scanning via Workload Identity (GKE WIF / EKS IRSA) |
| Registry Mirroring | `run-registry-mirroring.sh` | Registry mirror + proxy configuration |
| Operator Only | `run-operator-only.sh` | Deploy operator only (no build/push) |

Not all tests have manifests/terraform for every cloud yet. Currently supported:

| Test | GKE | EKS | AKS |
|------|-----|-----|-----|
| Fresh Deploy | yes | yes | yes |
| Upgrade | yes | yes | yes |
| External Cluster | yes | yes | yes |
| Vault External | yes | - | - |
| WIF External | yes | yes | yes |
| Registry Mirroring | yes | - | - |
| Operator Only | yes | yes | yes |

All tests pause for manual verification at each verify step (check Mondoo console for assets/scan results). Press Enter to continue or Ctrl+C to abort.

## Prerequisites

### Common

- `terraform >= 1.3`
- `helm >= 3`
- `docker`
- `kubectl`
- Go toolchain (for building the operator)

### GKE

- `gcloud` CLI, authenticated to your GCP project
- `jq` (for registry mirroring test verification)
- `crane` CLI (for registry mirroring test): `go install github.com/google/go-containerregistry/cmd/crane@latest`

### EKS

- `aws` CLI, authenticated to your AWS account
- `envsubst` (usually part of `gettext`)

### AKS

- `az` CLI, authenticated to your Azure subscription
- `kubelogin` (Azure Kubernetes Service AAD plugin): `az aks install-cli`
- `envsubst` (usually part of `gettext`)

### Mondoo credentials

Create a **service account** with **Owner** permissions in your Mondoo organization, download the JSON credential file, and export it:

```bash
export MONDOO_CONFIG_PATH=/path/to/mondoo-service-account.json
```

This is required for the Mondoo Terraform provider to create spaces and service accounts.

## Infrastructure Setup

### GKE

```bash
cd tests/e2e/gke/terraform
terraform init
terraform apply -var="project_id=MY_PROJECT" -var="mondoo_org_id=MY_ORG"
```

#### GKE Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `project_id` | yes | - | GCP project ID |
| `mondoo_org_id` | yes | - | Mondoo organization ID |
| `region` | no | `europe-west3` | GCP region |
| `autopilot` | no | `true` | `true` for Autopilot, `false` for Standard cluster |
| `enable_target_cluster` | no | `false` | Create a second GKE cluster for external cluster tests |
| `enable_wif_test` | no | `false` | Enable Workload Identity Federation resources |
| `enable_mirror_test` | no | `false` | Create a mirror AR repo for registry mirroring tests |
| `enable_proxy_test` | no | `false` | Provision a Squid proxy VM for proxy tests |

#### GKE Cluster Mode

By default, an **Autopilot** cluster is created. Autopilot does not support node scanning (hostPath `/` is restricted). To test node scanning, use a **Standard** cluster:

```bash
terraform apply \
  -var="project_id=MY_PROJECT" \
  -var="mondoo_org_id=MY_ORG" \
  -var="autopilot=false"
```

### EKS

```bash
cd tests/e2e/eks/terraform
terraform init
terraform apply -var="mondoo_org_id=MY_ORG"
```

#### EKS Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `mondoo_org_id` | yes | - | Mondoo organization ID |
| `region` | no | `eu-central-1` | AWS region |
| `k8s_version` | no | `1.30` | Kubernetes version |
| `enable_target_cluster` | no | `false` | Create a second EKS cluster for external cluster tests |
| `enable_wif_test` | no | `false` | Enable IRSA resources (IAM role, OIDC trust, Access Entries) |

### AKS

```bash
cd tests/e2e/aks/terraform
terraform init
terraform apply -var="subscription_id=MY_SUB" -var="mondoo_org_id=MY_ORG"
```

#### AKS Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `subscription_id` | yes | - | Azure subscription ID |
| `mondoo_org_id` | yes | - | Mondoo organization ID |
| `location` | no | `westeurope` | Azure region |
| `k8s_version` | no | `null` (latest) | Kubernetes version |
| `enable_target_cluster` | no | `false` | Create a second AKS cluster for external cluster tests |
| `enable_wif_test` | no | `false` | Enable Azure Workload Identity resources (managed identity, federated credential, role assignments) |

## Running Tests

### GKE Fresh Deploy

```bash
./tests/e2e/run-fresh-deploy.sh gke
```

### GKE WIF External Cluster

```bash
cd tests/e2e/gke/terraform
terraform apply -var="project_id=MY_PROJECT" -var="mondoo_org_id=MY_ORG" \
  -var="enable_target_cluster=true" -var="enable_wif_test=true"

./tests/e2e/run-wif-external-cluster.sh gke
```

### GKE External Cluster (Static Kubeconfig)

```bash
cd tests/e2e/gke/terraform
terraform apply -var="project_id=MY_PROJECT" -var="mondoo_org_id=MY_ORG" \
  -var="enable_target_cluster=true"

./tests/e2e/run-external-cluster.sh gke
```

### GKE Vault External Cluster

```bash
cd tests/e2e/gke/terraform
terraform apply -var="project_id=MY_PROJECT" -var="mondoo_org_id=MY_ORG" \
  -var="enable_target_cluster=true"

./tests/e2e/run-vault-external-cluster.sh gke
```

### GKE Registry Mirroring & Proxy

```bash
cd tests/e2e/gke/terraform
terraform apply -var="project_id=MY_PROJECT" -var="mondoo_org_id=MY_ORG" \
  -var="enable_mirror_test=true" -var="enable_proxy_test=true"

./tests/e2e/run-registry-mirroring.sh gke
```

### EKS Fresh Deploy

```bash
cd tests/e2e/eks/terraform
terraform apply -var="mondoo_org_id=MY_ORG"

./tests/e2e/run-fresh-deploy.sh eks
```

### EKS WIF External Cluster (IRSA)

```bash
cd tests/e2e/eks/terraform
terraform apply -var="mondoo_org_id=MY_ORG" \
  -var="enable_target_cluster=true" -var="enable_wif_test=true"

./tests/e2e/run-wif-external-cluster.sh eks
```

### AKS Fresh Deploy

```bash
cd tests/e2e/aks/terraform
terraform init
terraform apply -var="subscription_id=MY_SUB" -var="mondoo_org_id=MY_ORG"

./tests/e2e/run-fresh-deploy.sh aks
```

### AKS WIF External Cluster (Azure Workload Identity)

```bash
cd tests/e2e/aks/terraform
terraform apply -var="subscription_id=MY_SUB" -var="mondoo_org_id=MY_ORG" \
  -var="enable_target_cluster=true" -var="enable_wif_test=true"

./tests/e2e/run-wif-external-cluster.sh aks
```

### AKS External Cluster (Static Kubeconfig)

```bash
cd tests/e2e/aks/terraform
terraform apply -var="subscription_id=MY_SUB" -var="mondoo_org_id=MY_ORG" \
  -var="enable_target_cluster=true"

./tests/e2e/run-external-cluster.sh aks
```

## Cleanup

Remove all test resources from the cluster:

```bash
./tests/e2e/scripts/cleanup.sh
```

Destroy infrastructure:

```bash
# GKE
cd tests/e2e/gke/terraform
terraform destroy -var="project_id=MY_PROJECT" -var="mondoo_org_id=MY_ORG"

# EKS
cd tests/e2e/eks/terraform
terraform destroy -var="mondoo_org_id=MY_ORG"

# AKS
cd tests/e2e/aks/terraform
terraform destroy -var="subscription_id=MY_SUB" -var="mondoo_org_id=MY_ORG"
```

## Directory Structure

```
tests/e2e/
├── README.md
├── run-fresh-deploy.sh                               # ./run-fresh-deploy.sh <cloud>
├── run-upgrade.sh                                    # ./run-upgrade.sh <cloud> <version>
├── run-external-cluster.sh                           # ./run-external-cluster.sh <cloud>
├── run-vault-external-cluster.sh                     # ./run-vault-external-cluster.sh <cloud>
├── run-wif-external-cluster.sh                       # ./run-wif-external-cluster.sh <cloud>
├── run-registry-mirroring.sh                         # ./run-registry-mirroring.sh <cloud>
├── run-operator-only.sh                              # ./run-operator-only.sh <cloud>
├── manifests/
│   └── nginx-workload.yaml                           # Shared test workload
├── scripts/                                          # Cloud-agnostic shared scripts
│   ├── common.sh                                     # Logging, TF output loading, wait helpers
│   ├── common-gke.sh                                 # GKE-specific: AR auth, gcloud credentials, WIF vars
│   ├── common-eks.sh                                 # EKS-specific: ECR auth, aws credentials, IRSA vars
│   ├── common-aks.sh                                 # AKS-specific: ACR auth, az credentials, WIF vars
│   ├── build-and-push.sh                             # Build operator image, push to registry
│   ├── deploy-operator.sh                            # Helm install from local chart
│   ├── deploy-operator-mirroring.sh                  # Helm install with mirror/proxy values
│   ├── deploy-baseline.sh                            # Helm install released version
│   ├── deploy-test-workload.sh                       # Deploy nginx for scanning
│   ├── deploy-target-workload.sh                     # Deploy nginx + kubeconfig Secret for external cluster
│   ├── deploy-target-workload-only.sh                # Deploy nginx to target (no kubeconfig Secret)
│   ├── deploy-vault.sh                               # Deploy + configure Vault for external cluster auth
│   ├── apply-mondoo-config.sh                        # Create secret + apply MondooAuditConfig
│   ├── setup-wif.sh                                  # Setup WIF RBAC (delegates to cloud-specific)
│   ├── setup-mirror-registry.sh                      # Create imagePullSecret for mirror repo
│   ├── populate-mirror-registry.sh                   # Copy cnspec image into mirror repo via crane
│   ├── verify.sh                                     # Automated checks + manual verification pause
│   ├── verify-external.sh                            # External cluster verification
│   ├── verify-wif-external.sh                        # WIF external cluster verification (cloud-agnostic)
│   ├── verify-vault-external.sh                      # Vault external cluster verification
│   ├── verify-mirroring.sh                           # Mirroring/proxy-specific verification
│   └── cleanup.sh                                    # Remove all test resources from cluster
├── gke/                                              # GKE-specific
│   ├── terraform/
│   │   ├── versions.tf
│   │   ├── variables.tf
│   │   ├── main.tf                                   # GKE cluster + Artifact Registry + WIF GSA
│   │   ├── mondoo.tf                                 # Mondoo space + service account
│   │   ├── proxy.tf                                  # Squid proxy VM (optional)
│   │   └── outputs.tf
│   └── manifests/
│       ├── mondoo-audit-config.yaml.tpl
│       ├── mondoo-audit-config-autopilot.yaml.tpl
│       ├── mondoo-audit-config-external.yaml.tpl
│       ├── mondoo-audit-config-external-autopilot.yaml.tpl
│       ├── mondoo-audit-config-vault-external.yaml.tpl
│       ├── mondoo-audit-config-vault-external-autopilot.yaml.tpl
│       ├── mondoo-audit-config-wif-external.yaml.tpl
│       └── mondoo-audit-config-wif-external-autopilot.yaml.tpl
├── eks/                                              # EKS-specific
│   ├── terraform/
│   │   ├── versions.tf
│   │   ├── variables.tf
│   │   ├── main.tf                                   # VPC + EKS cluster + ECR + IRSA
│   │   ├── mondoo.tf                                 # Mondoo space + service account
│   │   ├── kubeconfig.tpl                            # Kubeconfig template (aws eks get-token)
│   │   └── outputs.tf
│   └── manifests/
│       ├── mondoo-audit-config.yaml.tpl
│       ├── mondoo-audit-config-external.yaml.tpl
│       └── mondoo-audit-config-wif-external.yaml.tpl
└── aks/                                              # AKS-specific
    ├── terraform/
    │   ├── versions.tf
    │   ├── variables.tf
    │   ├── main.tf                                   # VNet + AKS cluster + ACR + Azure WIF
    │   ├── mondoo.tf                                 # Mondoo space + service account
    │   ├── kubeconfig.tpl                            # Kubeconfig template (kubelogin)
    │   └── outputs.tf
    └── manifests/
        ├── mondoo-audit-config.yaml.tpl
        ├── mondoo-audit-config-external.yaml.tpl
        └── mondoo-audit-config-wif-external.yaml.tpl
```

## Notes

- **GKE Autopilot** restricts `hostPath` volumes to `/var/log/`, so node scanning is disabled in Autopilot mode. Use `autopilot=false` to test node scanning.
- **GKE internal images** (in `kube-system`, `gke-managed-*` namespaces) are hosted in private registries and can't be scanned. These namespaces are excluded in the GKE MondooAuditConfig.
- **EKS IRSA** (IAM Roles for Service Accounts) is the AWS equivalent of GKE Workload Identity. The IAM role trust policy and EKS Access Entries are configured in Terraform; no manual RBAC setup is needed.
- **AKS Workload Identity** uses Azure AD federated credentials. An AAD application with a federated identity credential is created in Terraform, trusting the scanner cluster's OIDC issuer for the WIF KSA. Azure RBAC role assignments (`AKS Cluster User Role` + `AKS RBAC Reader`) grant access to the target cluster. Both clusters share the same VNet with an NSG rule allowing scanner-to-target API traffic on port 443.
- The operator code has been updated so that node scanning failures no longer block other scan types (containers, k8s resources). The `NodeScanningDegraded` condition is set and reconciliation continues.
- Scan intervals are set to every 5 minutes in the test configs for faster feedback.
- **WIF external cluster scanning** uses an init container (gcloud CLI for GKE, aws-cli for EKS, azure-cli for AKS) to generate a kubeconfig at runtime using the federated identity. The scan container then uses this kubeconfig to access the target cluster.
