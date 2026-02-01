// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

	image, err := s.containerImageResolver.MondooOperatorImage(s.ctx, "", "", false)
	s.NoError(err)

	// Make sure a cron job exists with different container command
	cronJob := CronJob(image, "", test.KubeSystemNamespaceUid, &s.auditConfig, mondoov1alpha2.MondooOperatorConfig{})
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

func (s *DeploymentHandlerSuite) createDeploymentHandler() DeploymentHandler {
	return DeploymentHandler{
		KubeClient:             s.fakeClientBuilder.Build(),
		Mondoo:                 &s.auditConfig,
		ContainerImageResolver: s.containerImageResolver,
		MondooOperatorConfig:   &mondoov1alpha2.MondooOperatorConfig{},
	}
}

func TestDeploymentHandlerSuite(t *testing.T) {
	suite.Run(t, new(DeploymentHandlerSuite))
}
