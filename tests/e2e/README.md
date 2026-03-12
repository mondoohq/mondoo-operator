# E2E Test Suite for Mondoo Operator

End-to-end tests that deploy the Mondoo operator to a real GKE cluster and verify scanning works.

## Test Cases

- **Fresh Deploy** (`run-fresh-deploy.sh`): Builds the operator from the current branch, deploys to a GKE cluster, configures scanning, and verifies everything works.
- **Upgrade** (`run-upgrade.sh`): Installs a released baseline version first, verifies it, then upgrades to the current branch and verifies again.
- **Registry Mirroring & Proxy** (`run-registry-mirroring.sh`): Deploys with an Artifact Registry mirror repo and optional Squid proxy. Verifies image references are rewritten, `imagePullSecrets` are propagated, and proxy env vars are set.

All tests pause for manual verification at each verify step (check Mondoo console for assets/scan results). Press Enter to continue or Ctrl+C to abort.

## Prerequisites

- `gcloud` CLI, authenticated to your GCP project
- `terraform >= 1.3`
- `helm >= 3`
- `docker`
- `kubectl`
- Go toolchain (for building the operator)
- `jq` (for registry mirroring test verification)
- `crane` CLI (for registry mirroring test): `go install github.com/google/go-containerregistry/cmd/crane@latest`

### Mondoo credentials

Create a **service account** with **Owner** permissions in your Mondoo organization, download the JSON credential file, and export it:

```bash
export MONDOO_CONFIG_PATH=/path/to/mondoo-service-account.json
```

This is required for the Mondoo Terraform provider to create spaces and service accounts.

## Infrastructure Setup

Terraform provisions a GKE cluster, Artifact Registry repo, and Mondoo space with a service account.

```bash
cd tests/e2e/terraform
terraform init
terraform apply -var="project_id=MY_PROJECT" -var="mondoo_org_id=MY_ORG"
```

### Cluster Mode

By default, an **Autopilot** cluster is created. Autopilot does not support node scanning (hostPath `/` is restricted). To test node scanning, use a **Standard** cluster:

```bash
terraform apply \
  -var="project_id=MY_PROJECT" \
  -var="mondoo_org_id=MY_ORG" \
  -var="autopilot=false"
```

### Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `project_id` | yes | - | GCP project ID |
| `mondoo_org_id` | yes | - | Mondoo organization ID |
| `region` | no | `europe-west3` | GCP region |
| `autopilot` | no | `true` | `true` for Autopilot, `false` for Standard cluster |
| `enable_mirror_test` | no | `false` | Create a mirror AR repo for registry mirroring/imagePullSecrets tests |
| `enable_proxy_test` | no | `false` | Provision a Squid proxy VM for proxy tests (requires `enable_mirror_test`) |

You can also set these in a `terraform.tfvars` file.

## Running Tests

### Fresh Deploy

```bash
cd tests/e2e
./run-fresh-deploy.sh
```

What it does:
1. Loads Terraform outputs (cluster credentials, registry, Mondoo creds)
2. Builds the operator image from the current branch and pushes to Artifact Registry
3. Deploys an nginx test workload (for container image scanning to discover)
4. Installs the operator via local Helm chart with the custom image
5. Creates Mondoo credentials secret and applies MondooAuditConfig
6. Waits 60s for reconciliation, then runs automated checks
7. Pauses for manual verification in the Mondoo console

### Upgrade

```bash
cd tests/e2e
./run-upgrade.sh 12.0.1    # baseline version to upgrade from
```

What it does:
1. Loads Terraform outputs
2. Builds the current branch image (so it's ready for the upgrade step)
3. Deploys an nginx test workload
4. Installs the baseline released version from the Helm repo
5. Applies MondooAuditConfig, waits, verifies, pauses for manual check
6. Upgrades to the current branch image via local Helm chart
7. Waits, verifies again, pauses for manual check

### Registry Mirroring & Proxy

```bash
# Provision infrastructure with mirror AR repo (and optional proxy VM)
cd tests/e2e/terraform
terraform apply -var="project_id=MY_PROJECT" -var="mondoo_org_id=MY_ORG" \
  -var="enable_mirror_test=true" -var="enable_proxy_test=true"

# Run the test
cd tests/e2e
./run-registry-mirroring.sh
```

What it does:
1. Loads Terraform outputs (including mirror AR repo and proxy IP if enabled)
2. Builds the operator image and pushes to Artifact Registry
3. Creates `imagePullSecret` from the mirror repo's GCP service account key
4. Uses `crane` to copy cnspec image from ghcr.io into the mirror AR repo
5. Deploys an nginx test workload
6. Helm installs the operator with `registryMirrors`, `imagePullSecrets`, and proxy `--values`
7. Applies MondooAuditConfig, waits 90s for reconciliation
8. Runs standard verification checks
9. Runs mirroring-specific checks: image refs rewritten, pull secrets propagated, proxy env vars set
10. Checks Squid proxy access logs (if proxy enabled)
11. Pauses for manual verification in the Mondoo console

## Cleanup

Remove all test resources from the cluster (everything except Terraform infra):

```bash
cd tests/e2e
./scripts/cleanup.sh
```

Destroy all infrastructure:

```bash
cd tests/e2e/terraform
terraform destroy -var="project_id=MY_PROJECT" -var="mondoo_org_id=MY_ORG"
```

## Directory Structure

```
tests/e2e/
├── README.md
├── run-fresh-deploy.sh              # Fresh deploy test orchestrator
├── run-upgrade.sh                   # Upgrade test orchestrator
├── run-registry-mirroring.sh        # Registry mirroring & proxy test orchestrator
├── terraform/
│   ├── versions.tf                  # Provider requirements
│   ├── variables.tf                 # Input variables
│   ├── main.tf                      # GKE cluster + Artifact Registry
│   ├── mondoo.tf                    # Mondoo space + service account
│   ├── proxy.tf                     # Squid proxy VM (optional)
│   └── outputs.tf                   # Outputs consumed by scripts
├── scripts/
│   ├── common.sh                    # Logging, TF output loading, wait helpers
│   ├── build-and-push.sh            # Build operator image, push to AR
│   ├── deploy-operator.sh           # Helm install from local chart
│   ├── deploy-operator-mirroring.sh # Helm install with mirror/proxy values
│   ├── deploy-baseline.sh           # Helm install released version
│   ├── deploy-test-workload.sh      # Deploy nginx for scanning
│   ├── apply-mondoo-config.sh       # Create secret + apply MondooAuditConfig
│   ├── setup-mirror-registry.sh     # Create imagePullSecret for mirror AR repo
│   ├── populate-mirror-registry.sh  # Copy cnspec image into mirror AR repo via crane
│   ├── verify.sh                    # Automated checks + manual verification pause
│   ├── verify-mirroring.sh          # Mirroring/proxy-specific verification
│   └── cleanup.sh                   # Remove all test resources from cluster
└── manifests/
    ├── mondoo-audit-config.yaml.tpl            # Standard cluster config (nodes enabled)
    ├── mondoo-audit-config-autopilot.yaml.tpl  # Autopilot config (nodes disabled)
    └── nginx-workload.yaml                     # Test workload
```

## Notes

- **GKE Autopilot** restricts `hostPath` volumes to `/var/log/`, so node scanning is disabled in Autopilot mode. Use `autopilot=false` to test node scanning.
- **GKE internal images** (in `kube-system`, `gke-managed-*` namespaces) are hosted in private registries and can't be scanned. These namespaces are excluded in the MondooAuditConfig.
- The operator code has been updated so that node scanning failures no longer block other scan types (containers, k8s resources). The `NodeScanningDegraded` condition is set and reconciliation continues.
- Scan intervals are set to every 5 minutes in the test configs for faster feedback.
