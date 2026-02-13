// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"fmt"

	"github.com/stretchr/testify/suite"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	fakeMondoo "go.mondoo.com/mondoo-operator/pkg/utils/mondoo/fake"
	"go.mondoo.com/mondoo-operator/pkg/utils/test"
	"go.mondoo.com/mondoo-operator/tests/credentials"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
)

type DeploymentHandlerSuite struct {
	suite.Suite
	ctx                    context.Context
	scheme                 *runtime.Scheme
	containerImageResolver mondoo.ContainerImageResolver

	auditConfig       mondoov1alpha2.MondooAuditConfig
	fakeClientBuilder *fake.ClientBuilder
}

func (s *DeploymentHandlerSuite) SetupSuite() {
	s.ctx = context.Background()
	s.scheme = clientgoscheme.Scheme
	s.Require().NoError(mondoov1alpha2.AddToScheme(s.scheme))
	s.containerImageResolver = fakeMondoo.NewNoOpContainerImageResolver()
}

func (s *DeploymentHandlerSuite) BeforeTest(suiteName, testName string) {
	s.auditConfig = utils.DefaultAuditConfig("mondoo-operator", true, false, false)
	s.fakeClientBuilder = fake.NewClientBuilder().WithObjects(test.TestKubeSystemNamespace())
}

func (s *DeploymentHandlerSuite) TestReconcile_Create() {
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify CronJob was created
	created := &batchv1.CronJob{}
	created.Name = CronJobName(s.auditConfig.Name)
	created.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(created), created))
	s.Equal(CronJobLabels(s.auditConfig), created.Labels)

	// Verify ConfigMap was created
	configMap := &corev1.ConfigMap{}
	configMap.Name = ConfigMapName(s.auditConfig.Name)
	configMap.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(configMap), configMap))
	s.Contains(configMap.Data, "inventory")
}

func (s *DeploymentHandlerSuite) TestReconcile_Create_ConsoleIntegration() {
	s.auditConfig.Spec.ConsoleIntegration.Enable = true
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	integrationMrn := utils.RandString(20)
	sa, err := json.Marshal(mondooclient.ServiceAccountCredentials{Mrn: "test-mrn"})
	s.NoError(err)
	clientSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.auditConfig.Spec.MondooCredsSecretRef.Name,
			Namespace: s.auditConfig.Namespace,
		},
		Data: map[string][]byte{
			constants.MondooCredsSecretIntegrationMRNKey: []byte(integrationMrn),
			constants.MondooCredsSecretServiceAccountKey: sa,
		},
	}

	s.NoError(d.KubeClient.Create(s.ctx, clientSecret))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify CronJob was created
	created := &batchv1.CronJob{}
	created.Name = CronJobName(s.auditConfig.Name)
	created.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(created), created))

	// Verify ConfigMap contains integration MRN
	configMap := &corev1.ConfigMap{}
	configMap.Name = ConfigMapName(s.auditConfig.Name)
	configMap.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(configMap), configMap))
	s.Contains(configMap.Data["inventory"], integrationMrn)
}

func (s *DeploymentHandlerSuite) TestReconcile_Update() {
	d := s.createDeploymentHandler()

	image, err := s.containerImageResolver.CnspecImage("", "", "", false)
	s.NoError(err)

	// Make sure a cron job exists with different container command
	cronJob := CronJob(image, &s.auditConfig, mondoov1alpha2.MondooOperatorConfig{})
	cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command = []string{"test-command"}
	s.NoError(d.KubeClient.Create(s.ctx, cronJob))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify CronJob was updated
	created := &batchv1.CronJob{}
	created.Name = CronJobName(s.auditConfig.Name)
	created.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(created), created))
	s.NotEqual([]string{"test-command"}, created.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command)
}

func (s *DeploymentHandlerSuite) TestReconcile_K8sResourceScanningStatus() {
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	// Reconcile to create all resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify container image scanning and kubernetes resources conditions
	s.Equal(1, len(d.Mondoo.Status.Conditions))
	condition := d.Mondoo.Status.Conditions[0]
	s.Equal("Kubernetes Resources Scanning is available", condition.Message)
	s.Equal("KubernetesResourcesScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)

	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs))

	now := time.Now()
	metaNow := metav1.NewTime(now)
	metaHourAgo := metav1.NewTime(now.Add(-1 * time.Hour))
	cronJobs.Items[0].Status.LastScheduleTime = &metaNow
	cronJobs.Items[0].Status.LastSuccessfulTime = &metaHourAgo

	s.NoError(d.KubeClient.Status().Update(s.ctx, &cronJobs.Items[0]))

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the kubernetes resources status is set to unavailable
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Kubernetes Resources Scanning is unavailable", condition.Message)
	s.Equal("KubernetesResourcesScanningUnavailable", condition.Reason)
	s.Equal(corev1.ConditionTrue, condition.Status)

	// Make the jobs successful again
	cronJobs.Items[0].Status.LastScheduleTime = nil
	cronJobs.Items[0].Status.LastSuccessfulTime = nil
	s.NoError(d.KubeClient.Status().Update(s.ctx, &cronJobs.Items[0]))

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the kubernetes resources scanning status is set to available
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Kubernetes Resources Scanning is available", condition.Message)
	s.Equal("KubernetesResourcesScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)

	d.Mondoo.Spec.KubernetesResources.Enable = false

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the kubernetes resources scanning status is set to disabled
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Kubernetes Resources Scanning is disabled", condition.Message)
	s.Equal("KubernetesResourcesScanningDisabled", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)
}

func (s *DeploymentHandlerSuite) TestReconcile_Disable() {
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	// Reconcile to create all resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Reconcile again to delete the resources
	d.Mondoo.Spec.KubernetesResources.Enable = false
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs))
	s.Equal(0, len(cronJobs.Items))
}

func (s *DeploymentHandlerSuite) TestReconcile_CreateWithCustomSchedule() {
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	customSchedule := "0 0 * * *"
	s.auditConfig.Spec.KubernetesResources.Schedule = customSchedule

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	created := &batchv1.CronJob{}
	created.Name = CronJobName(s.auditConfig.Name)
	created.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(created), created))

	s.Equal(customSchedule, created.Spec.Schedule)
}

func (s *DeploymentHandlerSuite) TestReconcile_ExternalCluster_Kubeconfig() {
	// Create kubeconfig secret
	kubeconfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prod-kubeconfig",
			Namespace: s.auditConfig.Namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte("apiVersion: v1\nkind: Config\nclusters: []"),
		},
	}
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(kubeconfigSecret)

	// Configure external cluster
	s.auditConfig.Spec.KubernetesResources.ExternalClusters = []mondoov1alpha2.ExternalCluster{
		{
			Name: "production",
			KubeconfigSecretRef: &corev1.LocalObjectReference{
				Name: "prod-kubeconfig",
			},
		},
	}

	d := s.createDeploymentHandler()
	s.NoError(d.KubeClient.Create(s.ctx, &s.auditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify external cluster CronJob was created
	externalCronJob := &batchv1.CronJob{}
	externalCronJob.Name = ExternalClusterCronJobName(s.auditConfig.Name, "production")
	externalCronJob.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(externalCronJob), externalCronJob))
	s.Equal("production", externalCronJob.Labels["cluster_name"])

	// Verify KUBECONFIG env var is set
	container := externalCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
	var kubeconfigEnv *corev1.EnvVar
	for i := range container.Env {
		if container.Env[i].Name == "KUBECONFIG" {
			kubeconfigEnv = &container.Env[i]
			break
		}
	}
	s.NotNil(kubeconfigEnv)
	s.Equal("/etc/opt/mondoo/kubeconfig/kubeconfig", kubeconfigEnv.Value)

	// Verify ConfigMap was created for external cluster
	configMap := &corev1.ConfigMap{}
	configMap.Name = ExternalClusterConfigMapName(s.auditConfig.Name, "production")
	configMap.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(configMap), configMap))
	s.Contains(configMap.Data, "inventory")
}

func (s *DeploymentHandlerSuite) TestReconcile_ExternalCluster_ServiceAccountAuth() {
	// Create SA credentials secret
	saSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prod-sa-credentials",
			Namespace: s.auditConfig.Namespace,
		},
		Data: map[string][]byte{
			"token":  []byte("test-token"),
			"ca.crt": []byte("test-ca-cert"),
		},
	}
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(saSecret)

	// Configure external cluster with ServiceAccountAuth
	s.auditConfig.Spec.KubernetesResources.ExternalClusters = []mondoov1alpha2.ExternalCluster{
		{
			Name: "staging",
			ServiceAccountAuth: &mondoov1alpha2.ServiceAccountAuth{
				Server: "https://staging.example.com:6443",
				CredentialsSecretRef: corev1.LocalObjectReference{
					Name: "prod-sa-credentials",
				},
			},
		},
	}

	d := s.createDeploymentHandler()
	s.NoError(d.KubeClient.Create(s.ctx, &s.auditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify SA kubeconfig ConfigMap was created
	saKubeconfigCM := &corev1.ConfigMap{}
	saKubeconfigCM.Name = ExternalClusterSAKubeconfigName(s.auditConfig.Name, "staging")
	saKubeconfigCM.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(saKubeconfigCM), saKubeconfigCM))
	s.Contains(saKubeconfigCM.Data["kubeconfig"], "https://staging.example.com:6443")
	s.Contains(saKubeconfigCM.Data["kubeconfig"], "tokenFile: /etc/opt/mondoo/sa-credentials/token")

	// Verify external cluster CronJob was created
	externalCronJob := &batchv1.CronJob{}
	externalCronJob.Name = ExternalClusterCronJobName(s.auditConfig.Name, "staging")
	externalCronJob.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(externalCronJob), externalCronJob))
}

func (s *DeploymentHandlerSuite) TestReconcile_ExternalCluster_WorkloadIdentity_GKE() {
	// Configure external cluster with GKE WorkloadIdentity
	s.auditConfig.Spec.KubernetesResources.ExternalClusters = []mondoov1alpha2.ExternalCluster{
		{
			Name: "gke-prod",
			WorkloadIdentity: &mondoov1alpha2.WorkloadIdentityConfig{
				Provider: mondoov1alpha2.CloudProviderGKE,
				GKE: &mondoov1alpha2.GKEWorkloadIdentity{
					ProjectID:            "my-project",
					ClusterName:          "prod-cluster",
					ClusterLocation:      "us-central1",
					GoogleServiceAccount: "scanner@my-project.iam.gserviceaccount.com",
				},
			},
		},
	}

	d := s.createDeploymentHandler()
	s.NoError(d.KubeClient.Create(s.ctx, &s.auditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify WIF ServiceAccount was created with correct annotation
	wifSA := &corev1.ServiceAccount{}
	wifSA.Name = WIFServiceAccountName(s.auditConfig.Name, "gke-prod")
	wifSA.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(wifSA), wifSA))
	s.Equal("scanner@my-project.iam.gserviceaccount.com", wifSA.Annotations["iam.gke.io/gcp-service-account"])

	// Verify external cluster CronJob was created with init container
	externalCronJob := &batchv1.CronJob{}
	externalCronJob.Name = ExternalClusterCronJobName(s.auditConfig.Name, "gke-prod")
	externalCronJob.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(externalCronJob), externalCronJob))

	// Verify init container exists
	initContainers := externalCronJob.Spec.JobTemplate.Spec.Template.Spec.InitContainers
	s.Len(initContainers, 1)
	s.Equal("generate-kubeconfig", initContainers[0].Name)
	s.Contains(initContainers[0].Image, "google-cloud-cli")
}

func (s *DeploymentHandlerSuite) TestReconcile_ExternalCluster_SPIFFE() {
	// Create trust bundle secret for remote cluster CA
	trustBundleSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "remote-cluster-ca",
			Namespace: s.auditConfig.Namespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("-----BEGIN CERTIFICATE-----\ntest-ca-cert\n-----END CERTIFICATE-----"),
		},
	}
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(trustBundleSecret)

	// Configure external cluster with SPIFFE auth
	s.auditConfig.Spec.KubernetesResources.ExternalClusters = []mondoov1alpha2.ExternalCluster{
		{
			Name: "spiffe-cluster",
			SPIFFEAuth: &mondoov1alpha2.SPIFFEAuthConfig{
				Server: "https://remote-cluster.example.com:6443",
				TrustBundleSecretRef: corev1.LocalObjectReference{
					Name: "remote-cluster-ca",
				},
				SocketPath: "/run/spire/sockets/agent.sock",
			},
		},
	}

	d := s.createDeploymentHandler()
	s.NoError(d.KubeClient.Create(s.ctx, &s.auditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify external cluster CronJob was created
	externalCronJob := &batchv1.CronJob{}
	externalCronJob.Name = ExternalClusterCronJobName(s.auditConfig.Name, "spiffe-cluster")
	externalCronJob.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(externalCronJob), externalCronJob))

	// Verify init container exists for SPIFFE cert fetching
	initContainers := externalCronJob.Spec.JobTemplate.Spec.Template.Spec.InitContainers
	s.Len(initContainers, 1)
	s.Equal("fetch-spiffe-certs", initContainers[0].Name)
	s.Contains(initContainers[0].Image, "spiffe-helper")

	// Verify SPIFFE-related volumes exist
	volumes := externalCronJob.Spec.JobTemplate.Spec.Template.Spec.Volumes
	volumeNames := make([]string, len(volumes))
	for i, v := range volumes {
		volumeNames[i] = v.Name
	}
	s.Contains(volumeNames, "spire-agent-socket")
	s.Contains(volumeNames, "trust-bundle")
	s.Contains(volumeNames, "spiffe-certs")

	// Verify SPIRE agent socket is mounted from host
	var spireSocketVolume *corev1.Volume
	for i := range volumes {
		if volumes[i].Name == "spire-agent-socket" {
			spireSocketVolume = &volumes[i]
			break
		}
	}
	s.NotNil(spireSocketVolume)
	s.NotNil(spireSocketVolume.HostPath)
	s.Equal("/run/spire/sockets", spireSocketVolume.HostPath.Path)

	// Verify ConfigMap was created for external cluster
	configMap := &corev1.ConfigMap{}
	configMap.Name = ExternalClusterConfigMapName(s.auditConfig.Name, "spiffe-cluster")
	configMap.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(configMap), configMap))
	s.Contains(configMap.Data, "inventory")
}

func (s *DeploymentHandlerSuite) TestReconcile_ExternalCluster_CustomSchedule() {
	// Create kubeconfig secret
	kubeconfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prod-kubeconfig",
			Namespace: s.auditConfig.Namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte("apiVersion: v1\nkind: Config\nclusters: []"),
		},
	}
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(kubeconfigSecret)

	customSchedule := "0 */4 * * *"
	s.auditConfig.Spec.KubernetesResources.ExternalClusters = []mondoov1alpha2.ExternalCluster{
		{
			Name: "production",
			KubeconfigSecretRef: &corev1.LocalObjectReference{
				Name: "prod-kubeconfig",
			},
			Schedule: customSchedule,
		},
	}

	d := s.createDeploymentHandler()
	s.NoError(d.KubeClient.Create(s.ctx, &s.auditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify external cluster CronJob uses custom schedule
	externalCronJob := &batchv1.CronJob{}
	externalCronJob.Name = ExternalClusterCronJobName(s.auditConfig.Name, "production")
	externalCronJob.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(externalCronJob), externalCronJob))
	s.Equal(customSchedule, externalCronJob.Spec.Schedule)
}

func (s *DeploymentHandlerSuite) TestReconcile_ExternalCluster_Cleanup() {
	// Create kubeconfig secret
	kubeconfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prod-kubeconfig",
			Namespace: s.auditConfig.Namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte("apiVersion: v1\nkind: Config\nclusters: []"),
		},
	}
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(kubeconfigSecret)

	// Configure external cluster
	s.auditConfig.Spec.KubernetesResources.ExternalClusters = []mondoov1alpha2.ExternalCluster{
		{
			Name: "production",
			KubeconfigSecretRef: &corev1.LocalObjectReference{
				Name: "prod-kubeconfig",
			},
		},
	}

	d := s.createDeploymentHandler()
	s.NoError(d.KubeClient.Create(s.ctx, &s.auditConfig))

	// First reconcile - creates resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify resources exist
	externalCronJob := &batchv1.CronJob{}
	externalCronJob.Name = ExternalClusterCronJobName(s.auditConfig.Name, "production")
	externalCronJob.Namespace = s.auditConfig.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(externalCronJob), externalCronJob))

	// Remove external cluster from config
	d.Mondoo.Spec.KubernetesResources.ExternalClusters = nil

	// Second reconcile - should clean up
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify CronJob was deleted
	err = d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(externalCronJob), externalCronJob)
	s.Error(err)

	// Verify ConfigMap was deleted
	configMap := &corev1.ConfigMap{}
	configMap.Name = ExternalClusterConfigMapName(s.auditConfig.Name, "production")
	configMap.Namespace = s.auditConfig.Namespace
	err = d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(configMap), configMap)
	s.Error(err)
}

func (s *DeploymentHandlerSuite) createDeploymentHandler() DeploymentHandler {
	return DeploymentHandler{
		KubeClient:             s.fakeClientBuilder.Build(),
		Mondoo:                 &s.auditConfig,
		ContainerImageResolver: s.containerImageResolver,
		MondooOperatorConfig:   &mondoov1alpha2.MondooOperatorConfig{},
		MondooClientBuilder:    mondooclient.NewClient,
	}
}

// TestValidateExternalClusterAuth tests the authentication validation function
func TestValidateExternalClusterAuth(t *testing.T) {
	tests := []struct {
		name        string
		cluster     mondoov1alpha2.ExternalCluster
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid kubeconfig auth",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				KubeconfigSecretRef: &corev1.LocalObjectReference{
					Name: "my-kubeconfig",
				},
			},
			expectError: false,
		},
		{
			name: "valid service account auth",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				ServiceAccountAuth: &mondoov1alpha2.ServiceAccountAuth{
					Server: "https://example.com:6443",
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "my-creds",
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid GKE workload identity",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				WorkloadIdentity: &mondoov1alpha2.WorkloadIdentityConfig{
					Provider: mondoov1alpha2.CloudProviderGKE,
					GKE: &mondoov1alpha2.GKEWorkloadIdentity{
						ProjectID:            "my-project",
						ClusterName:          "my-cluster",
						ClusterLocation:      "us-central1",
						GoogleServiceAccount: "sa@project.iam.gserviceaccount.com",
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid SPIFFE auth",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				SPIFFEAuth: &mondoov1alpha2.SPIFFEAuthConfig{
					Server: "https://example.com:6443",
					TrustBundleSecretRef: corev1.LocalObjectReference{
						Name: "trust-bundle",
					},
				},
			},
			expectError: false,
		},
		{
			name: "no auth method specified",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
			},
			expectError: true,
			errorMsg:    "must specify one of",
		},
		{
			name: "multiple auth methods specified",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				KubeconfigSecretRef: &corev1.LocalObjectReference{
					Name: "my-kubeconfig",
				},
				ServiceAccountAuth: &mondoov1alpha2.ServiceAccountAuth{
					Server: "https://example.com:6443",
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "my-creds",
					},
				},
			},
			expectError: true,
			errorMsg:    "mutually exclusive",
		},
		{
			name: "GKE workload identity without GKE config",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				WorkloadIdentity: &mondoov1alpha2.WorkloadIdentityConfig{
					Provider: mondoov1alpha2.CloudProviderGKE,
				},
			},
			expectError: true,
			errorMsg:    "gke config required",
		},
		{
			name: "EKS workload identity without EKS config",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				WorkloadIdentity: &mondoov1alpha2.WorkloadIdentityConfig{
					Provider: mondoov1alpha2.CloudProviderEKS,
				},
			},
			expectError: true,
			errorMsg:    "eks config required",
		},
		{
			name: "AKS workload identity without AKS config",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				WorkloadIdentity: &mondoov1alpha2.WorkloadIdentityConfig{
					Provider: mondoov1alpha2.CloudProviderAKS,
				},
			},
			expectError: true,
			errorMsg:    "aks config required",
		},
		{
			name: "service account auth without server",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				ServiceAccountAuth: &mondoov1alpha2.ServiceAccountAuth{
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "my-creds",
					},
				},
			},
			expectError: true,
			errorMsg:    "server is required",
		},
		{
			name: "service account auth without credentials",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				ServiceAccountAuth: &mondoov1alpha2.ServiceAccountAuth{
					Server: "https://example.com:6443",
				},
			},
			expectError: true,
			errorMsg:    "credentialsSecretRef.name is required",
		},
		{
			name: "kubeconfig auth without secret name",
			cluster: mondoov1alpha2.ExternalCluster{
				Name:                "test",
				KubeconfigSecretRef: &corev1.LocalObjectReference{},
			},
			expectError: true,
			errorMsg:    "kubeconfigSecretRef.name is required",
		},
		{
			name: "SPIFFE auth without server",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				SPIFFEAuth: &mondoov1alpha2.SPIFFEAuthConfig{
					TrustBundleSecretRef: corev1.LocalObjectReference{
						Name: "trust-bundle",
					},
				},
			},
			expectError: true,
			errorMsg:    "server is required",
		},
		{
			name: "SPIFFE auth without trust bundle",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				SPIFFEAuth: &mondoov1alpha2.SPIFFEAuthConfig{
					Server: "https://example.com:6443",
				},
			},
			expectError: true,
			errorMsg:    "trustBundleSecretRef.name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExternalClusterAuth(tt.cluster)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestExternalClusterSAKubeconfig tests kubeconfig generation for ServiceAccount auth
func TestExternalClusterSAKubeconfig(t *testing.T) {
	tests := []struct {
		name          string
		cluster       mondoov1alpha2.ExternalCluster
		expectServer  string
		expectTLSSkip bool
		expectCAPath  bool
	}{
		{
			name: "with CA certificate",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				ServiceAccountAuth: &mondoov1alpha2.ServiceAccountAuth{
					Server: "https://api.example.com:6443",
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "my-creds",
					},
					SkipTLSVerify: false,
				},
			},
			expectServer:  "https://api.example.com:6443",
			expectTLSSkip: false,
			expectCAPath:  true,
		},
		{
			name: "with TLS skip",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "test",
				ServiceAccountAuth: &mondoov1alpha2.ServiceAccountAuth{
					Server: "https://insecure.example.com:6443",
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "my-creds",
					},
					SkipTLSVerify: true,
				},
			},
			expectServer:  "https://insecure.example.com:6443",
			expectTLSSkip: true,
			expectCAPath:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeconfig := ExternalClusterSAKubeconfig(tt.cluster)

			if !strings.Contains(kubeconfig, tt.expectServer) {
				t.Errorf("expected kubeconfig to contain server %q", tt.expectServer)
			}

			if tt.expectTLSSkip {
				if !strings.Contains(kubeconfig, "insecure-skip-tls-verify: true") {
					t.Error("expected kubeconfig to contain insecure-skip-tls-verify: true")
				}
			}

			if tt.expectCAPath {
				if !strings.Contains(kubeconfig, "certificate-authority: /etc/opt/mondoo/sa-credentials/ca.crt") {
					t.Error("expected kubeconfig to contain certificate-authority path")
				}
			}

			// Always expect token file reference
			if !strings.Contains(kubeconfig, "tokenFile: /etc/opt/mondoo/sa-credentials/token") {
				t.Error("expected kubeconfig to contain tokenFile path")
			}
		})
	}
}

// TestWIFServiceAccount tests ServiceAccount creation with cloud-specific annotations
func TestWIFServiceAccount(t *testing.T) {
	mondooConfig := &mondoov1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "mondoo-operator",
		},
	}

	tests := []struct {
		name              string
		cluster           mondoov1alpha2.ExternalCluster
		expectAnnotations map[string]string
		expectLabels      map[string]string
	}{
		{
			name: "GKE workload identity",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "gke-cluster",
				WorkloadIdentity: &mondoov1alpha2.WorkloadIdentityConfig{
					Provider: mondoov1alpha2.CloudProviderGKE,
					GKE: &mondoov1alpha2.GKEWorkloadIdentity{
						ProjectID:            "my-project",
						ClusterName:          "my-cluster",
						ClusterLocation:      "us-central1",
						GoogleServiceAccount: "scanner@my-project.iam.gserviceaccount.com",
					},
				},
			},
			expectAnnotations: map[string]string{
				"iam.gke.io/gcp-service-account": "scanner@my-project.iam.gserviceaccount.com",
			},
		},
		{
			name: "EKS IRSA",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "eks-cluster",
				WorkloadIdentity: &mondoov1alpha2.WorkloadIdentityConfig{
					Provider: mondoov1alpha2.CloudProviderEKS,
					EKS: &mondoov1alpha2.EKSWorkloadIdentity{
						Region:      "us-west-2",
						ClusterName: "my-cluster",
						RoleARN:     "arn:aws:iam::123456789012:role/MondooScanner",
					},
				},
			},
			expectAnnotations: map[string]string{
				"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/MondooScanner",
			},
		},
		{
			name: "AKS workload identity",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "aks-cluster",
				WorkloadIdentity: &mondoov1alpha2.WorkloadIdentityConfig{
					Provider: mondoov1alpha2.CloudProviderAKS,
					AKS: &mondoov1alpha2.AKSWorkloadIdentity{
						SubscriptionID: "sub-123",
						ResourceGroup:  "my-rg",
						ClusterName:    "my-cluster",
						ClientID:       "client-123",
						TenantID:       "tenant-456",
					},
				},
			},
			expectAnnotations: map[string]string{
				"azure.workload.identity/client-id": "client-123",
			},
			expectLabels: map[string]string{
				"azure.workload.identity/use": "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sa := WIFServiceAccount(tt.cluster, mondooConfig)

			// Verify name
			expectedName := WIFServiceAccountName(mondooConfig.Name, tt.cluster.Name)
			if sa.Name != expectedName {
				t.Errorf("expected name %q, got %q", expectedName, sa.Name)
			}

			// Verify namespace
			if sa.Namespace != mondooConfig.Namespace {
				t.Errorf("expected namespace %q, got %q", mondooConfig.Namespace, sa.Namespace)
			}

			// Verify annotations
			for key, expectedValue := range tt.expectAnnotations {
				if sa.Annotations[key] != expectedValue {
					t.Errorf("expected annotation %q=%q, got %q", key, expectedValue, sa.Annotations[key])
				}
			}

			// Verify labels (for AKS)
			for key, expectedValue := range tt.expectLabels {
				if sa.Labels[key] != expectedValue {
					t.Errorf("expected label %q=%q, got %q", key, expectedValue, sa.Labels[key])
				}
			}
		})
	}
}

// TestWIFInitContainer tests init container generation for different cloud providers
func TestWIFInitContainer(t *testing.T) {
	tests := []struct {
		name           string
		cluster        mondoov1alpha2.ExternalCluster
		expectImage    string
		expectEnvVars  []string
		expectCommands []string
	}{
		{
			name: "GKE init container",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "gke-cluster",
				WorkloadIdentity: &mondoov1alpha2.WorkloadIdentityConfig{
					Provider: mondoov1alpha2.CloudProviderGKE,
					GKE: &mondoov1alpha2.GKEWorkloadIdentity{
						ProjectID:            "my-project",
						ClusterName:          "prod-cluster",
						ClusterLocation:      "us-central1-a",
						GoogleServiceAccount: "scanner@my-project.iam.gserviceaccount.com",
					},
				},
			},
			expectImage:   "gcr.io/google.com/cloudsdktool/google-cloud-cli",
			expectEnvVars: []string{"CLUSTER_NAME", "PROJECT_ID", "CLUSTER_LOCATION"},
		},
		{
			name: "EKS init container",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "eks-cluster",
				WorkloadIdentity: &mondoov1alpha2.WorkloadIdentityConfig{
					Provider: mondoov1alpha2.CloudProviderEKS,
					EKS: &mondoov1alpha2.EKSWorkloadIdentity{
						Region:      "us-west-2",
						ClusterName: "prod-cluster",
						RoleARN:     "arn:aws:iam::123456789012:role/Scanner",
					},
				},
			},
			expectImage:   "amazon/aws-cli",
			expectEnvVars: []string{"CLUSTER_NAME", "AWS_REGION"},
		},
		{
			name: "AKS init container",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "aks-cluster",
				WorkloadIdentity: &mondoov1alpha2.WorkloadIdentityConfig{
					Provider: mondoov1alpha2.CloudProviderAKS,
					AKS: &mondoov1alpha2.AKSWorkloadIdentity{
						SubscriptionID: "sub-123",
						ResourceGroup:  "my-rg",
						ClusterName:    "prod-cluster",
						ClientID:       "client-123",
						TenantID:       "tenant-456",
					},
				},
			},
			expectImage:   "mcr.microsoft.com/azure-cli",
			expectEnvVars: []string{"CLUSTER_NAME", "RESOURCE_GROUP", "SUBSCRIPTION_ID"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := wifInitContainer(tt.cluster)

			// Verify container name
			if container.Name != "generate-kubeconfig" {
				t.Errorf("expected container name 'generate-kubeconfig', got %q", container.Name)
			}

			// Verify image
			if !strings.Contains(container.Image, tt.expectImage) {
				t.Errorf("expected image to contain %q, got %q", tt.expectImage, container.Image)
			}

			// Verify expected env vars exist
			envNames := make(map[string]bool)
			for _, env := range container.Env {
				envNames[env.Name] = true
			}
			for _, expectedEnv := range tt.expectEnvVars {
				if !envNames[expectedEnv] {
					t.Errorf("expected env var %q not found", expectedEnv)
				}
			}

			// Verify volume mounts
			mountNames := make(map[string]bool)
			for _, mount := range container.VolumeMounts {
				mountNames[mount.Name] = true
			}
			if !mountNames["kubeconfig"] {
				t.Error("expected 'kubeconfig' volume mount")
			}
			if !mountNames["temp"] {
				t.Error("expected 'temp' volume mount")
			}

			// Verify security context
			if container.SecurityContext == nil {
				t.Fatal("expected security context to be set")
			}
			if *container.SecurityContext.AllowPrivilegeEscalation != false {
				t.Error("expected AllowPrivilegeEscalation to be false")
			}
			if *container.SecurityContext.ReadOnlyRootFilesystem != true {
				t.Error("expected ReadOnlyRootFilesystem to be true")
			}
		})
	}
}

// TestSpiffeInitContainer tests SPIFFE init container generation
func TestSpiffeInitContainer(t *testing.T) {
	tests := []struct {
		name             string
		cluster          mondoov1alpha2.ExternalCluster
		expectSocketPath string
	}{
		{
			name: "default socket path",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "spiffe-cluster",
				SPIFFEAuth: &mondoov1alpha2.SPIFFEAuthConfig{
					Server: "https://remote.example.com:6443",
					TrustBundleSecretRef: corev1.LocalObjectReference{
						Name: "trust-bundle",
					},
				},
			},
			expectSocketPath: "agent.sock",
		},
		{
			name: "custom socket path",
			cluster: mondoov1alpha2.ExternalCluster{
				Name: "spiffe-cluster",
				SPIFFEAuth: &mondoov1alpha2.SPIFFEAuthConfig{
					Server:     "https://remote.example.com:6443",
					SocketPath: "/custom/path/spire-agent.sock",
					TrustBundleSecretRef: corev1.LocalObjectReference{
						Name: "trust-bundle",
					},
				},
			},
			expectSocketPath: "spire-agent.sock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := spiffeInitContainer(tt.cluster)

			// Verify container name
			if container.Name != "fetch-spiffe-certs" {
				t.Errorf("expected container name 'fetch-spiffe-certs', got %q", container.Name)
			}

			// Verify image contains spiffe-helper
			if !strings.Contains(container.Image, "spiffe-helper") {
				t.Errorf("expected image to contain 'spiffe-helper', got %q", container.Image)
			}

			// Verify SOCKET_FILE env var
			var socketFileEnv string
			for _, env := range container.Env {
				if env.Name == "SOCKET_FILE" {
					socketFileEnv = env.Value
					break
				}
			}
			if socketFileEnv != tt.expectSocketPath {
				t.Errorf("expected SOCKET_FILE=%q, got %q", tt.expectSocketPath, socketFileEnv)
			}

			// Verify K8S_SERVER env var
			var serverEnv string
			for _, env := range container.Env {
				if env.Name == "K8S_SERVER" {
					serverEnv = env.Value
					break
				}
			}
			if serverEnv != tt.cluster.SPIFFEAuth.Server {
				t.Errorf("expected K8S_SERVER=%q, got %q", tt.cluster.SPIFFEAuth.Server, serverEnv)
			}

			// Verify volume mounts
			mountNames := make(map[string]bool)
			for _, mount := range container.VolumeMounts {
				mountNames[mount.Name] = true
			}
			expectedMounts := []string{"spire-agent-socket", "spiffe-certs", "trust-bundle", "kubeconfig", "temp"}
			for _, expected := range expectedMounts {
				if !mountNames[expected] {
					t.Errorf("expected volume mount %q not found", expected)
				}
			}

			// Verify security context
			if container.SecurityContext == nil {
				t.Fatal("expected security context to be set")
			}
			if *container.SecurityContext.RunAsUser != 101 {
				t.Errorf("expected RunAsUser=101, got %d", *container.SecurityContext.RunAsUser)
			}
		})
	}
}

// TestExternalClusterCronJobLabels tests label generation for external clusters
func TestExternalClusterCronJobLabels(t *testing.T) {
	config := mondoov1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "mondoo-operator",
		},
	}

	labels := ExternalClusterCronJobLabels(config, "my-cluster")

	expectedLabels := map[string]string{
		"app":          "mondoo-k8s-scan",
		"scan":         "k8s",
		"mondoo_cr":    "test-config",
		"cluster_name": "my-cluster",
	}

	for key, expected := range expectedLabels {
		if labels[key] != expected {
			t.Errorf("expected label %q=%q, got %q", key, expected, labels[key])
		}
	}
}

// TestExternalClusterNaming tests naming functions for external cluster resources
func TestExternalClusterNaming(t *testing.T) {
	tests := []struct {
		prefix      string
		clusterName string
	}{
		{"mondoo-client", "production"},
		{"my-config", "staging-cluster"},
	}

	for _, tt := range tests {
		t.Run(tt.prefix+"-"+tt.clusterName, func(t *testing.T) {
			// Test CronJob name
			cronJobName := ExternalClusterCronJobName(tt.prefix, tt.clusterName)
			if !strings.HasPrefix(cronJobName, tt.prefix) {
				t.Errorf("CronJob name should start with prefix %q, got %q", tt.prefix, cronJobName)
			}
			if !strings.HasSuffix(cronJobName, tt.clusterName) {
				t.Errorf("CronJob name should end with cluster name %q, got %q", tt.clusterName, cronJobName)
			}

			// Test ConfigMap name
			configMapName := ExternalClusterConfigMapName(tt.prefix, tt.clusterName)
			if !strings.HasPrefix(configMapName, tt.prefix) {
				t.Errorf("ConfigMap name should start with prefix %q, got %q", tt.prefix, configMapName)
			}
			if !strings.HasSuffix(configMapName, tt.clusterName) {
				t.Errorf("ConfigMap name should end with cluster name %q, got %q", tt.clusterName, configMapName)
			}

			// Test SA kubeconfig ConfigMap name
			saKubeconfigName := ExternalClusterSAKubeconfigName(tt.prefix, tt.clusterName)
			if !strings.HasPrefix(saKubeconfigName, tt.prefix) {
				t.Errorf("SA kubeconfig name should start with prefix %q, got %q", tt.prefix, saKubeconfigName)
			}
			if !strings.HasSuffix(saKubeconfigName, tt.clusterName) {
				t.Errorf("SA kubeconfig name should end with cluster name %q, got %q", tt.clusterName, saKubeconfigName)
			}

			// Test WIF ServiceAccount name
			wifSAName := WIFServiceAccountName(tt.prefix, tt.clusterName)
			if !strings.HasPrefix(wifSAName, tt.prefix) {
				t.Errorf("WIF SA name should start with prefix %q, got %q", tt.prefix, wifSAName)
			}
			if !strings.HasSuffix(wifSAName, tt.clusterName) {
				t.Errorf("WIF SA name should end with cluster name %q, got %q", tt.clusterName, wifSAName)
			}
		})
	}
}

func (s *DeploymentHandlerSuite) TestGarbageCollection_RunsAfterSuccessfulScan() {
	gcCalled := false
	d := s.createDeploymentHandlerWithGCMock(func(ctx context.Context, opts *mondooclient.GarbageCollectOptions) error {
		gcCalled = true
		s.Equal("k8s-cluster", opts.PlatformRuntime)
		s.Contains(opts.ManagedBy, "mondoo-operator-")
		s.NotEmpty(opts.OlderThan)
		return nil
	})
	s.NoError(d.KubeClient.Create(s.ctx, &s.auditConfig))

	// First reconcile - creates resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Set lastSuccessfulTime on the CronJob
	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs))
	s.Require().NotEmpty(cronJobs.Items)

	now := metav1.Now()
	cronJobs.Items[0].Status.LastSuccessfulTime = &now
	s.NoError(d.KubeClient.Status().Update(s.ctx, &cronJobs.Items[0]))

	// Second reconcile - should trigger GC
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	s.True(gcCalled, "GarbageCollectAssets should have been called")
	s.NotNil(d.Mondoo.Status.LastK8sResourceGarbageCollectionTime, "GC timestamp should be set in status")
}

func (s *DeploymentHandlerSuite) TestGarbageCollection_SkipsWhenAlreadyRun() {
	gcCalled := false
	d := s.createDeploymentHandlerWithGCMock(func(ctx context.Context, opts *mondooclient.GarbageCollectOptions) error {
		gcCalled = true
		return nil
	})
	s.NoError(d.KubeClient.Create(s.ctx, &s.auditConfig))

	// First reconcile
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Set lastSuccessfulTime on the CronJob
	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs))
	s.Require().NotEmpty(cronJobs.Items)

	successTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	cronJobs.Items[0].Status.LastSuccessfulTime = &successTime
	s.NoError(d.KubeClient.Status().Update(s.ctx, &cronJobs.Items[0]))

	// Set status to indicate GC was already done at a newer time
	gcTime := metav1.Now()
	d.Mondoo.Status.LastK8sResourceGarbageCollectionTime = &gcTime

	// Reconcile - should NOT trigger GC
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	s.False(gcCalled, "GarbageCollectAssets should NOT have been called")
}

func (s *DeploymentHandlerSuite) TestGarbageCollection_FailureStillUpdatesTimestamp() {
	d := s.createDeploymentHandlerWithGCMock(func(ctx context.Context, opts *mondooclient.GarbageCollectOptions) error {
		return fmt.Errorf("API error")
	})
	s.NoError(d.KubeClient.Create(s.ctx, &s.auditConfig))

	// First reconcile
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Set lastSuccessfulTime on the CronJob
	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs))
	s.Require().NotEmpty(cronJobs.Items)

	now := metav1.Now()
	cronJobs.Items[0].Status.LastSuccessfulTime = &now
	s.NoError(d.KubeClient.Status().Update(s.ctx, &cronJobs.Items[0]))

	// Reconcile - GC fails but reconcile should still succeed
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Timestamp should still be updated so we don't retry until the next new successful scan
	s.NotNil(d.Mondoo.Status.LastK8sResourceGarbageCollectionTime, "GC timestamp should be set even when GC fails")
}

// createDeploymentHandlerWithGCMock creates a DeploymentHandler with a mock MondooClientBuilder
// that captures calls to GarbageCollectAssets.
func (s *DeploymentHandlerSuite) createDeploymentHandlerWithGCMock(gcFunc func(context.Context, *mondooclient.GarbageCollectOptions) error) DeploymentHandler {
	// Create a mock credentials secret so GC can read it
	key := credentials.MondooServiceAccount(s.T())
	mockSA := mondooclient.ServiceAccountCredentials{
		Mrn:         "//agents.api.mondoo.app/spaces/test/serviceaccounts/test",
		PrivateKey:  key,
		ApiEndpoint: "https://us.api.mondoo.com",
	}
	saData, err := json.Marshal(mockSA)
	s.Require().NoError(err)
	credsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.auditConfig.Spec.MondooCredsSecretRef.Name,
			Namespace: s.auditConfig.Namespace,
		},
		Data: map[string][]byte{
			constants.MondooCredsSecretServiceAccountKey: saData,
		},
	}
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(credsSecret)

	return DeploymentHandler{
		KubeClient:             s.fakeClientBuilder.Build(),
		Mondoo:                 &s.auditConfig,
		ContainerImageResolver: s.containerImageResolver,
		MondooOperatorConfig:   &mondoov1alpha2.MondooOperatorConfig{},
		MondooClientBuilder: func(opts mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
			return &fakeMondooClient{gcFunc: gcFunc}, nil
		},
	}
}

// fakeMondooClient implements just enough of MondooClient to test GC
type fakeMondooClient struct {
	mondooclient.MondooClient
	gcFunc func(context.Context, *mondooclient.GarbageCollectOptions) error
}

func (f *fakeMondooClient) GarbageCollectAssets(ctx context.Context, opts *mondooclient.GarbageCollectOptions) error {
	if f.gcFunc != nil {
		return f.gcFunc(ctx, opts)
	}
	return nil
}

func TestDeploymentHandlerSuite(t *testing.T) {
	suite.Run(t, new(DeploymentHandlerSuite))
}
