/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package container_image

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
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
	s.auditConfig = utils.DefaultAuditConfig("mondoo-operator", false, true, false, false)

	s.fakeClientBuilder = fake.NewClientBuilder().WithObjects(test.TestKubeSystemNamespace())
}

func (s *DeploymentHandlerSuite) TestReconcile_Create() {
	d := s.createDeploymentHandler()

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.CnspecImage("", "", false)
	s.NoError(err)

	expected := CronJob(image, "", test.KubeSystemNamespaceUid, "", s.auditConfig, mondoov1alpha2.MondooOperatorConfig{})
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, expected, d.KubeClient.Scheme()))

	// Set some fields that the kube client sets
	gvk, err := apiutil.GVKForObject(expected, d.KubeClient.Scheme())
	s.NoError(err)
	expected.SetGroupVersionKind(gvk)
	expected.ResourceVersion = "1"

	created := &batchv1.CronJob{}
	created.Name = expected.Name
	created.Namespace = expected.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(created), created))

	s.Equal(expected, created)
}

func (s *DeploymentHandlerSuite) TestReconcile_Create_PrivateRegistriesSecret() {
	d := s.createDeploymentHandler()

	s.auditConfig.Spec.Scanner.PrivateRegistriesPullSecretRef.Name = "my-pull-secrets"

	privateRegistriesSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.auditConfig.Namespace,
			Name:      s.auditConfig.Spec.Scanner.PrivateRegistriesPullSecretRef.Name,
		},
		StringData: map[string]string{
			".dockerconfigjson": "{	\"auths\": { \"https://registry.example.com/v1/\": { \"auth\": \"c3R...zE2\" } } }",
		},
	}
	s.NoError(d.KubeClient.Create(s.ctx, privateRegistriesSecret), "Error creating the private registries secret")

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.CnspecImage("", "", false)
	s.NoError(err)

	expected := CronJob(image, "", test.KubeSystemNamespaceUid, s.auditConfig.Spec.Scanner.PrivateRegistriesPullSecretRef.Name, s.auditConfig, mondoov1alpha2.MondooOperatorConfig{})
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, expected, d.KubeClient.Scheme()))

	// Set some fields that the kube client sets
	gvk, err := apiutil.GVKForObject(expected, d.KubeClient.Scheme())
	s.NoError(err)
	expected.SetGroupVersionKind(gvk)
	expected.ResourceVersion = "1"

	created := &batchv1.CronJob{}
	created.Name = expected.Name
	created.Namespace = expected.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(created), created))

	s.Equal(expected, created)
}

func (s *DeploymentHandlerSuite) TestReconcile_Create_ConsoleIntegration() {
	s.auditConfig.Spec.ConsoleIntegration.Enable = true
	d := s.createDeploymentHandler()

	integrationMrn := utils.RandString(20)
	clientSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.auditConfig.Spec.MondooCredsSecretRef.Name,
			Namespace: s.auditConfig.Namespace,
		},
		Data: map[string][]byte{constants.MondooCredsSecretIntegrationMRNKey: []byte(integrationMrn)},
	}

	s.NoError(d.KubeClient.Create(s.ctx, clientSecret))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.CnspecImage("", "", false)
	s.NoError(err)

	expected := CronJob(image, integrationMrn, test.KubeSystemNamespaceUid, "", s.auditConfig, mondoov1alpha2.MondooOperatorConfig{})
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, expected, d.KubeClient.Scheme()))

	// Set some fields that the kube client sets
	gvk, err := apiutil.GVKForObject(expected, d.KubeClient.Scheme())
	s.NoError(err)
	expected.SetGroupVersionKind(gvk)
	expected.ResourceVersion = "1"

	created := &batchv1.CronJob{}
	created.Name = expected.Name
	created.Namespace = expected.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(created), created))

	s.Equal(expected, created)
}

func (s *DeploymentHandlerSuite) TestReconcile_Update() {
	d := s.createDeploymentHandler()

	image, err := s.containerImageResolver.CnspecImage("", "", false)
	s.NoError(err)

	// Make sure a cron job exists with different container command
	cronJob := CronJob(image, "", "", "", s.auditConfig, mondoov1alpha2.MondooOperatorConfig{})
	cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command = []string{"test-command"}
	s.NoError(d.KubeClient.Create(s.ctx, cronJob))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	expected := CronJob(image, "", test.KubeSystemNamespaceUid, "", s.auditConfig, mondoov1alpha2.MondooOperatorConfig{})
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, expected, d.KubeClient.Scheme()))

	// Set some fields that the kube client sets
	gvk, err := apiutil.GVKForObject(expected, d.KubeClient.Scheme())
	s.NoError(err)
	expected.SetGroupVersionKind(gvk)

	// The second node has an updated cron job so resource version is +1
	expected.ResourceVersion = fmt.Sprintf("%d", 2)

	created := &batchv1.CronJob{}
	created.Name = expected.Name
	created.Namespace = expected.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(created), created))

	s.Equal(expected, created)
}

func (s *DeploymentHandlerSuite) TestReconcile_K8sContainerImageScanningStatus() {
	d := s.createDeploymentHandler()

	// Reconcile to create all resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the image scanning status is set to available
	s.Equal(1, len(d.Mondoo.Status.Conditions))
	condition := d.Mondoo.Status.Conditions[0]
	s.Equal("Kubernetes Container Image Scanning is Available", condition.Message)
	s.Equal("KubernetesContainerImageScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)

	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs))

	now := time.Now()
	metaNow := metav1.NewTime(now)
	metaHourAgo := metav1.NewTime(now.Add(-1 * time.Hour))
	cronJobs.Items[0].Status.LastScheduleTime = &metaNow
	cronJobs.Items[0].Status.LastSuccessfulTime = &metaHourAgo

	s.NoError(d.KubeClient.Update(s.ctx, &cronJobs.Items[0]))

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the image scanning status is set to unavailable
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Kubernetes Container Image Scanning is Unavailable", condition.Message)
	s.Equal("KubernetesContainerImageScanningUnavailable", condition.Reason)
	s.Equal(corev1.ConditionTrue, condition.Status)

	// Make the jobs successful again
	cronJobs.Items[0].Status.LastScheduleTime = nil
	cronJobs.Items[0].Status.LastSuccessfulTime = nil
	s.NoError(d.KubeClient.Update(s.ctx, &cronJobs.Items[0]))

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the image scanning status is set to available
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Kubernetes Container Image Scanning is Available", condition.Message)
	s.Equal("KubernetesContainerImageScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)

	d.Mondoo.Spec.Containers.Enable = false

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the image scanning status is set to disabled
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Kubernetes Container Image Scanning is disabled", condition.Message)
	s.Equal("KubernetesContainerImageScanningDisabled", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)
}

func (s *DeploymentHandlerSuite) TestReconcile_DisableContainerImageScanning() {
	d := s.createDeploymentHandler()

	// Reconcile to create all resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Reconcile again to delete the resources
	d.Mondoo.Spec.Containers.Enable = false
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs))
	s.Equal(0, len(cronJobs.Items))
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
