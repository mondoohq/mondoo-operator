// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package container_image

import (
	"fmt"
	"strings"

	// That's the mod k8s relies on https://github.com/kubernetes/kubernetes/blob/master/go.mod#L63

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	k8s_scan "go.mondoo.com/mondoo-operator/controllers/k8s_scan"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	mondoo "go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mql/v13/providers-sdk/v1/inventory"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

const (
	// TODO: remove in next version
	OldCronJobNameSuffix = "-k8s-images-scan"

	CronJobNameSuffix      = "-containers-scan"
	InventoryConfigMapBase = "-containers-inventory"
)

func CronJob(image, integrationMrn, clusterUid, privateRegistrySecretName string, m *v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) *batchv1.CronJob {
	ls := CronJobLabels(*m)

	cmd := []string{
		"cnspec", "scan", "k8s",
		"--config", "/etc/opt/mondoo/config/mondoo.yml",
		"--inventory-file", "/etc/opt/mondoo/config/inventory.yml",
	}

	// Only add proxy settings if SkipProxyForCnspec is false
	// cnspec-based components may not properly handle NO_PROXY for internal domains
	if !cfg.Spec.SkipProxyForCnspec {
		if apiProxy := k8s.APIProxyURL(cfg); apiProxy != nil {
			cmd = append(cmd, "--api-proxy", *apiProxy)
		}
	}

	envVars := feature_flags.AllFeatureFlagsAsEnv()
	envVars = append(envVars, corev1.EnvVar{Name: "MONDOO_AUTO_UPDATE", Value: "false"})
	envVars = append(envVars, corev1.EnvVar{Name: "MONDOO_TMP_DIR", Value: "/tmp"})

	// Add proxy environment variables from MondooOperatorConfig only if SkipProxyForCnspec is false
	if !cfg.Spec.SkipProxyForCnspec {
		envVars = append(envVars, k8s.ProxyEnvVars(cfg)...)
	}

	envVars = k8s.MergeEnv(envVars, m.Spec.Containers.Env)

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CronJobName(m.Name),
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          m.Spec.Containers.Schedule,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: ls},
				Spec: batchv1.JobSpec{
					// Don't retry failed scans - re-running won't fix the issue
					BackoffLimit: ptr.To(int32(0)),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: ls},
						Spec: corev1.PodSpec{
							// The scan can fail when an asset has an error. However, re-scanning won't result in the error
							// being fixed. Therefore, we don't want to restart the job.
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Image:           image,
									ImagePullPolicy: corev1.PullIfNotPresent,
									Name:            "mondoo-containers-scan",
									Command:         cmd,
									Resources:       k8s.ResourcesRequirementsWithDefaults(m.Spec.Containers.Resources, k8s.DefaultContainerScanningResources),
									SecurityContext: &corev1.SecurityContext{
										AllowPrivilegeEscalation: ptr.To(false),
										ReadOnlyRootFilesystem:   ptr.To(true),
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{
												"ALL",
											},
										},
										RunAsNonRoot: ptr.To(true),
										// This is needed to prevent:
										// Error: container has runAsNonRoot and image has non-numeric user (mondoo), cannot verify user is non-root ...
										RunAsUser:  ptr.To(int64(101)),
										Privileged: ptr.To(false),
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "config",
											ReadOnly:  true,
											MountPath: "/etc/opt/mondoo/config",
										},
										{
											Name:      "temp",
											MountPath: "/tmp",
										},
									},
									Env:                      envVars,
									TerminationMessagePath:   "/dev/termination-log",
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								},
							},
							ServiceAccountName: m.Spec.Scanner.ServiceAccountName,
							Volumes: []corev1.Volume{
								{
									Name: "temp",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "config",
									VolumeSource: corev1.VolumeSource{
										Projected: &corev1.ProjectedVolumeSource{
											DefaultMode: ptr.To(int32(corev1.ProjectedVolumeSourceDefaultMode)),
											Sources: []corev1.VolumeProjection{
												{
													ConfigMap: &corev1.ConfigMapProjection{
														LocalObjectReference: corev1.LocalObjectReference{Name: ConfigMapName(m.Name)},
														Items: []corev1.KeyToPath{{
															Key:  "inventory",
															Path: "inventory.yml",
														}},
													},
												},
												{
													Secret: &corev1.SecretProjection{
														LocalObjectReference: k8s.ConfigSecretRef(*m),
														Items: []corev1.KeyToPath{{
															Key:  "config",
															Path: "mondoo.yml",
														}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			SuccessfulJobsHistoryLimit: ptr.To(int32(1)),
			FailedJobsHistoryLimit:     ptr.To(int32(1)),
		},
	}

	// Add WIF support for cloud registry authentication
	if wif := m.Spec.Containers.WorkloadIdentity; wif != nil {
		podSpec := &cronjob.Spec.JobTemplate.Spec.Template.Spec

		// Use WIF ServiceAccount
		podSpec.ServiceAccountName = WIFServiceAccountName(m.Name)
		podSpec.AutomountServiceAccountToken = ptr.To(true)

		// Add docker config volume (generated by init container)
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: "docker-config",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

		// Mount docker config in main container
		podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "docker-config",
				ReadOnly:  true,
				MountPath: "/etc/opt/mondoo/docker",
			},
		)
		podSpec.Containers[0].Env = append(podSpec.Containers[0].Env,
			corev1.EnvVar{Name: "DOCKER_CONFIG", Value: "/etc/opt/mondoo/docker"},
		)

		// Add init container for registry credential generation
		podSpec.InitContainers = append(podSpec.InitContainers, registryWIFInitContainer(wif))

		// AKS Workload Identity webhook requires this label on the pod template only.
		// Copy labels so we don't mutate the CronJob/Job metadata.
		if wif.Provider == v1alpha2.CloudProviderAKS {
			podLabels := make(map[string]string, len(ls)+1)
			for k, v := range ls {
				podLabels[k] = v
			}
			podLabels["azure.workload.identity/use"] = "true"
			cronjob.Spec.JobTemplate.Spec.Template.Labels = podLabels
		}
	} else {
		// Add private registry secret if specified (static credentials path)
		k8s.AddPrivateRegistryPullSecretToSpec(&cronjob.Spec.JobTemplate.Spec.Template.Spec, privateRegistrySecretName)
	}

	// Append imagePullSecrets from MondooOperatorConfig (don't overwrite existing secrets)
	if len(cfg.Spec.ImagePullSecrets) > 0 {
		cronjob.Spec.JobTemplate.Spec.Template.Spec.ImagePullSecrets = append(
			cronjob.Spec.JobTemplate.Spec.Template.Spec.ImagePullSecrets,
			cfg.Spec.ImagePullSecrets...)
	}

	return cronjob
}

func CronJobLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo-container-scan",
		"scan":      "k8s",
		"mondoo_cr": m.Name,
	}
}

// TODO: remove in next version
func OldCronJobName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, OldCronJobNameSuffix)
}

func CronJobName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, CronJobNameSuffix)
}

func ConfigMap(integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) (*corev1.ConfigMap, error) {
	inv, err := Inventory(integrationMRN, clusterUID, m, cfg)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: m.Namespace,
			Name:      ConfigMapName(m.Name),
		},
		Data: map[string]string{"inventory": inv},
	}, nil
}

func ConfigMapName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
}

func Inventory(integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) (string, error) {
	inv := &inventory.Inventory{
		Metadata: &inventory.ObjectMeta{
			Name: "mondoo-k8s-containers-inventory",
		},
		Spec: &inventory.InventorySpec{
			Assets: []*inventory.Asset{
				{
					Connections: []*inventory.Config{
						{
							Type: "k8s",
							Options: map[string]string{
								"namespaces":         strings.Join(m.Spec.Filtering.Namespaces.Include, ","),
								"namespaces-exclude": strings.Join(m.Spec.Filtering.Namespaces.Exclude, ","),
								"disable-cache":      "false",
							},
							Discover: &inventory.Discovery{
								Targets: []string{"container-images"},
							},
						},
					},
					Labels: map[string]string{
						"k8s.mondoo.com/kind": "node",
					},
					ManagedBy: mondoo.ManagedByLabel(clusterUID),
				},
			},
		},
	}

	if integrationMRN != "" {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].Labels[constants.MondooAssetsIntegrationLabel] = integrationMRN
		}
	}

	if cfg.Spec.ContainerProxy != nil {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].Connections[0].Options["container-proxy"] = *cfg.Spec.ContainerProxy
		}
	}

	// Add user-defined annotations to all assets
	if len(m.Spec.Annotations) > 0 {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].AddAnnotations(m.Spec.Annotations)
		}
	}

	invBytes, err := yaml.Marshal(inv)
	if err != nil {
		return "", err
	}

	return string(invBytes), nil
}

// WIFServiceAccountName returns the name for the container registry WIF ServiceAccount
func WIFServiceAccountName(prefix string) string {
	return fmt.Sprintf("%s-cr-wif", prefix)
}

// WIFServiceAccount creates a ServiceAccount with cloud-specific annotations for container registry WIF
func WIFServiceAccount(m *v1alpha2.MondooAuditConfig) *corev1.ServiceAccount {
	wif := m.Spec.Containers.WorkloadIdentity
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        WIFServiceAccountName(m.Name),
			Namespace:   m.Namespace,
			Labels:      CronJobLabels(*m),
			Annotations: make(map[string]string),
		},
	}

	switch wif.Provider {
	case v1alpha2.CloudProviderGKE:
		sa.Annotations["iam.gke.io/gcp-service-account"] = wif.GKE.GoogleServiceAccount
	case v1alpha2.CloudProviderEKS:
		sa.Annotations["eks.amazonaws.com/role-arn"] = wif.EKS.RoleARN
	case v1alpha2.CloudProviderAKS:
		sa.Annotations["azure.workload.identity/client-id"] = wif.AKS.ClientID
		if sa.Labels == nil {
			sa.Labels = make(map[string]string)
		}
		sa.Labels["azure.workload.identity/use"] = "true"
	}

	return sa
}

// validateContainerRegistryWIF validates the container registry WIF configuration
func validateContainerRegistryWIF(wif *v1alpha2.WorkloadIdentityConfig) error {
	if wif == nil {
		return nil
	}

	switch wif.Provider {
	case v1alpha2.CloudProviderGKE:
		if wif.GKE == nil {
			return fmt.Errorf("containers.workloadIdentity: gke config required when provider is gke")
		}
		if wif.GKE.GoogleServiceAccount == "" {
			return fmt.Errorf("containers.workloadIdentity: gke.googleServiceAccount is required for container registry WIF")
		}
	case v1alpha2.CloudProviderEKS:
		if wif.EKS == nil {
			return fmt.Errorf("containers.workloadIdentity: eks config required when provider is eks")
		}
		if wif.EKS.Region == "" {
			return fmt.Errorf("containers.workloadIdentity: eks.region is required for container registry WIF")
		}
	case v1alpha2.CloudProviderAKS:
		if wif.AKS == nil {
			return fmt.Errorf("containers.workloadIdentity: aks config required when provider is aks")
		}
		if wif.AKS.LoginServer == "" {
			return fmt.Errorf("containers.workloadIdentity: aks.loginServer is required for container registry WIF")
		}
	}

	return nil
}

// registryWIFInitContainer creates an init container that generates docker config credentials
// using cloud-native Workload Identity Federation
func registryWIFInitContainer(wif *v1alpha2.WorkloadIdentityConfig) corev1.Container {
	var image, script string
	var env []corev1.EnvVar

	// Common retry wrapper for transient failures
	retryWrapper := `
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
		image = k8s_scan.GCloudSDKImage
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
		image = k8s_scan.AWSCLIImage
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
		image = k8s_scan.AzureCLIImage
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
		image = "busybox:1.36"
		script = `echo "ERROR: Unknown workload identity provider"; exit 1`
		env = []corev1.EnvVar{}
	}

	return corev1.Container{
		Name:            "generate-registry-creds",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-c", script},
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
