// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

// RegistryWIFInitContainer creates an init container that generates docker config credentials
// using cloud-native Workload Identity Federation for container registry authentication.
func RegistryWIFInitContainer(wif *v1alpha2.WorkloadIdentityConfig) corev1.Container {
	var image, shell, script string
	var env []corev1.EnvVar

	retryWrapper := `set -euo pipefail
# Retry wrapper for transient failures
retry() {
  local max_attempts=3
  local delay=5
  local attempt=1
  while [ $attempt -le $max_attempts ]; do
    if "$@"; then
      return 0
    fi
    echo "Attempt $attempt failed, retrying in ${delay}s..."
    sleep $delay
    attempt=$((attempt + 1))
  done
  echo "All $max_attempts attempts failed"
  return 1
}
`

	switch wif.Provider {
	case v1alpha2.CloudProviderGKE:
		image = constants.GCloudSDKImage
		shell = "/bin/bash"
		script = retryWrapper + `
# Use WIF identity to get an access token for Artifact Registry / GCR
TOKEN=$(retry gcloud auth print-access-token)
AUTH=$(echo -n "oauth2accesstoken:${TOKEN}" | base64 -w0)

# All GCP regions and multi-region locations that host Artifact Registry.
# Docker config requires exact hostname matches, so we enumerate them all.
AR_LOCATIONS="
africa-south1 asia-east1 asia-east2 asia-northeast1 asia-northeast2 asia-northeast3
asia-south1 asia-south2 asia-southeast1 asia-southeast2
australia-southeast1 australia-southeast2
europe-central2 europe-north1 europe-southwest1 europe-west1 europe-west2
europe-west3 europe-west4 europe-west6 europe-west8 europe-west9 europe-west10 europe-west12
me-central1 me-central2 me-west1
northamerica-northeast1 northamerica-northeast2
southamerica-east1 southamerica-west1
us-central1 us-east1 us-east4 us-east5 us-south1 us-west1 us-west2 us-west3 us-west4
asia europe us
"

AUTHS=""
add_auth() {
  [ -n "$AUTHS" ] && AUTHS="${AUTHS},"
  AUTHS="${AUTHS}\"$1\":{\"auth\":\"${AUTH}\"}"
}

for loc in $AR_LOCATIONS; do
  add_auth "${loc}-docker.pkg.dev"
done

# Legacy GCR endpoints
for host in gcr.io us.gcr.io eu.gcr.io asia.gcr.io; do
  add_auth "$host"
done

cat > /etc/opt/mondoo/docker/config.json <<DOCKEREOF
{"auths":{${AUTHS}}}
DOCKEREOF
echo "Docker config generated for $(echo "$AUTHS" | tr ',' '\n' | wc -l) registry endpoints"
`
		env = []corev1.EnvVar{
			{Name: "HOME", Value: "/tmp"},
		}

	case v1alpha2.CloudProviderEKS:
		image = constants.AWSCLIImage
		shell = "/bin/bash"
		script = retryWrapper + `
# Use IRSA identity to get ECR login password
PASSWORD=$(retry aws ecr get-login-password --region "$AWS_REGION")

# Derive registry URL from role ARN account ID and region
ACCOUNT_ID=$(echo "$ROLE_ARN" | cut -d: -f5)
REGISTRY="${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"

# Write docker config
AUTH=$(echo -n "AWS:${PASSWORD}" | base64 -w0)
cat > /etc/opt/mondoo/docker/config.json <<DOCKEREOF
{
  "auths": {
    "${REGISTRY}": { "auth": "${AUTH}" }
  }
}
DOCKEREOF
echo "Docker config generated for ECR registry: ${REGISTRY}"
`
		env = []corev1.EnvVar{
			{Name: "HOME", Value: "/tmp"},
			{Name: "AWS_REGION", Value: wif.EKS.Region},
			{Name: "ROLE_ARN", Value: wif.EKS.RoleARN},
		}

	case v1alpha2.CloudProviderAKS:
		image = constants.AzureCLIImage
		shell = "/bin/bash"
		script = retryWrapper + `
# Azure WIF webhook injects AZURE_CLIENT_ID, AZURE_TENANT_ID, AZURE_FEDERATED_TOKEN_FILE
retry az login --federated-token "$(cat "$AZURE_FEDERATED_TOKEN_FILE")" \
  --service-principal \
  -u "$AZURE_CLIENT_ID" \
  -t "$AZURE_TENANT_ID"

# Get ACR access token
TOKEN=$(retry az acr login --name "$ACR_LOGIN_SERVER" --expose-token --output tsv --query accessToken)

# Write docker config
AUTH=$(echo -n "00000000-0000-0000-0000-000000000000:${TOKEN}" | base64 -w0)
cat > /etc/opt/mondoo/docker/config.json <<DOCKEREOF
{
  "auths": {
    "${ACR_LOGIN_SERVER}": { "auth": "${AUTH}" }
  }
}
DOCKEREOF
echo "Docker config generated for ACR: ${ACR_LOGIN_SERVER}"
`
		env = []corev1.EnvVar{
			{Name: "HOME", Value: "/tmp"},
			{Name: "ACR_LOGIN_SERVER", Value: wif.AKS.LoginServer},
		}

	default:
		image = constants.BusyBoxImage
		shell = "/bin/sh"
		script = `echo "ERROR: Unknown workload identity provider"; exit 1`
		env = []corev1.EnvVar{}
	}

	return corev1.Container{
		Name:            "generate-registry-creds",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{shell, "-c", script},
		Env:             env,
		VolumeMounts: []corev1.VolumeMount{
			{Name: "docker-config", MountPath: "/etc/opt/mondoo/docker"},
			{Name: "temp", MountPath: "/tmp"},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			ReadOnlyRootFilesystem:   ptr.To(true),
			RunAsNonRoot:             ptr.To(true),
			RunAsUser:                ptr.To(int64(101)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}
}
