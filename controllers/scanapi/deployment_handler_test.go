/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package scanapi

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	fakeMondoo "go.mondoo.com/mondoo-operator/pkg/utils/mondoo/fake"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
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
	s.auditConfig = utils.DefaultAuditConfig("mondoo-operator", true, false, false, false)
	s.fakeClientBuilder = fake.NewClientBuilder()
}

func (s *DeploymentHandlerSuite) TestReconcile_Create_CustomEnvVars() {
	s.auditConfig.Spec.Scanner.Env = []corev1.EnvVar{{Name: "TEST_ENV", Value: "TEST_VALUE"}}
	d := s.createDeploymentHandler()
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.auditConfig.Namespace,
			Name:      TokenSecretName(s.auditConfig.Name),
		},
	}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(tokenSecret), tokenSecret), "Error checking for token secret")
	// This really should be checking tokenSecret.Data, but the fake kubeClient just takes and stores the objects given to it
	// and our code populates the Secret through Secret.StringData["token"]
	s.Contains(tokenSecret.StringData, "token")

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(1, len(ds.Items))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	deployment := ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "", false)
	deployment.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, deployment, s.scheme))
	s.True(k8s.AreDeploymentsEqual(*deployment, ds.Items[0]))

	ss := &corev1.ServiceList{}
	s.NoError(d.KubeClient.List(s.ctx, ss))
	s.Equal(1, len(ss.Items))

	service := ScanApiService(d.Mondoo.Namespace, s.auditConfig)
	service.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, service, s.scheme))
	s.Equal(*service, ss.Items[0])
}

func (s *DeploymentHandlerSuite) TestReconcile_Create_KubernetesResources() {
	d := s.createDeploymentHandler()
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.auditConfig.Namespace,
			Name:      TokenSecretName(s.auditConfig.Name),
		},
	}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(tokenSecret), tokenSecret), "Error checking for token secret")
	// This really should be checking tokenSecret.Data, but the fake kubeClient just takes and stores the objects given to it
	// and our code populates the Secret through Secret.StringData["token"]
	s.Contains(tokenSecret.StringData, "token")

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(1, len(ds.Items))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	deployment := ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "", false)
	deployment.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, deployment, s.scheme))
	s.True(k8s.AreDeploymentsEqual(*deployment, ds.Items[0]))

	ss := &corev1.ServiceList{}
	s.NoError(d.KubeClient.List(s.ctx, ss))
	s.Equal(1, len(ss.Items))

	service := ScanApiService(d.Mondoo.Namespace, s.auditConfig)
	service.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, service, s.scheme))
	s.Equal(*service, ss.Items[0])
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

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(1, len(ds.Items))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	deployment := ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "my-pull-secrets", false)
	deployment.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, deployment, s.scheme))
	s.True(k8s.AreDeploymentsEqual(*deployment, ds.Items[0]))
}

func (s *DeploymentHandlerSuite) TestReconcile_Create_PrivateRegistriesSecretNotSpecifiedButPresent() {
	d := s.createDeploymentHandler()

	privateRegistriesSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.auditConfig.Namespace,
			Name:      "mondoo-private-registries-secrets",
		},
		StringData: map[string]string{
			".dockerconfigjson": "{	\"auths\": { \"https://registry.example.com/v1/\": { \"auth\": \"c3R...zE2\" } } }",
		},
	}
	s.NoError(d.KubeClient.Create(s.ctx, privateRegistriesSecret), "Error creating the private registries secret")

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(1, len(ds.Items))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	deployment := ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "mondoo-private-registries-secrets", false)
	deployment.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, deployment, s.scheme))
	s.True(k8s.AreDeploymentsEqual(*deployment, ds.Items[0]))
}

func (s *DeploymentHandlerSuite) TestReconcile_Create_PrivateRegistriesSecretWrongName() {
	d := s.createDeploymentHandler()

	privateRegistriesSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.auditConfig.Namespace,
			Name:      "mondoo-private-pull-secrets",
		},
		StringData: map[string]string{
			".dockerconfigjson": "{	\"auths\": { \"https://registry.example.com/v1/\": { \"auth\": \"c3R...zE2\" } } }",
		},
	}
	s.NoError(d.KubeClient.Create(s.ctx, privateRegistriesSecret), "Error creating the private registries secret")

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(1, len(ds.Items))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	deployment := ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "", false)
	deployment.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, deployment, s.scheme))
	s.True(k8s.AreDeploymentsEqual(*deployment, ds.Items[0]))
}

func (s *DeploymentHandlerSuite) TestReconcile_Create_Admission() {
	s.auditConfig = utils.DefaultAuditConfig("mondoo-operator", false, false, false, true)

	d := s.createDeploymentHandler()
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.auditConfig.Namespace,
			Name:      TokenSecretName(s.auditConfig.Name),
		},
	}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(tokenSecret), tokenSecret), "Error checking for token secret")
	// This really should be checking tokenSecret.Data, but the fake kubeClient just takes and stores the objects given to it
	// and our code populates the Secret through Secret.StringData["token"]
	s.Contains(tokenSecret.StringData, "token")

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(1, len(ds.Items))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	deployment := ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "", false)
	deployment.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, deployment, s.scheme))
	s.True(k8s.AreDeploymentsEqual(*deployment, ds.Items[0]))

	ss := &corev1.ServiceList{}
	s.NoError(d.KubeClient.List(s.ctx, ss))
	s.Equal(1, len(ss.Items))

	service := ScanApiService(d.Mondoo.Namespace, s.auditConfig)
	service.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, service, s.scheme))
	s.Equal(*service, ss.Items[0])
}

func (s *DeploymentHandlerSuite) TestReconcile_Create_NodeScanning() {
	s.auditConfig = utils.DefaultAuditConfig("mondoo-operator", false, false, true, false)

	d := s.createDeploymentHandler()
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.auditConfig.Namespace,
			Name:      TokenSecretName(s.auditConfig.Name),
		},
	}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(tokenSecret), tokenSecret), "Error checking for token secret")
	// This really should be checking tokenSecret.Data, but the fake kubeClient just takes and stores the objects given to it
	// and our code populates the Secret through Secret.StringData["token"]
	s.Contains(tokenSecret.StringData, "token")

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(1, len(ds.Items))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	deployment := ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "", false)
	deployment.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, deployment, s.scheme))
	s.True(k8s.AreDeploymentsEqual(*deployment, ds.Items[0]))

	ss := &corev1.ServiceList{}
	s.NoError(d.KubeClient.List(s.ctx, ss))
	s.Equal(1, len(ss.Items))

	service := ScanApiService(d.Mondoo.Namespace, s.auditConfig)
	service.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, service, s.scheme))
	s.Equal(*service, ss.Items[0])
}

func (s *DeploymentHandlerSuite) TestDeploy_CreateMissingServiceAccount() {
	ns := "test-ns"
	s.auditConfig = utils.DefaultAuditConfig(ns, false, false, false, true)
	s.auditConfig.Spec.Scanner.ServiceAccountName = "missing-serviceaccount"

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	deployment := ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "", false)
	deployment.Status.UnavailableReplicas = 1
	deployment.Status.Conditions = []appsv1.DeploymentCondition{
		{
			Type:    appsv1.DeploymentConditionType(mondoov1alpha2.ScanAPIDegraded),
			Status:  "ScanAPI degarded",
			Message: "pods \"scan-api-123\" is forbidden: error looking up service account test-ns/missing-serviceaccount: serviceaccount \"missing-serviceaccount\" not found",
		},
	}

	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(&s.auditConfig, deployment)

	d := s.createDeploymentHandler()
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(1, len(ds.Items))

	conditions := s.auditConfig.Status.Conditions
	foundMissingServiceAccountCondition := false
	s.Assertions.NotEmpty(conditions)
	for _, condition := range conditions {
		if strings.Contains(condition.Message, "error looking up service account") {
			foundMissingServiceAccountCondition = true
			break
		}
	}
	s.Assertions.Truef(foundMissingServiceAccountCondition, "No Condition for missing service account found")
}

func (s *DeploymentHandlerSuite) TestReconcile_Update() {
	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	deployment := ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "", false)
	deployment.Spec.Replicas = pointer.Int32(3)

	service := ScanApiService(s.auditConfig.Namespace, s.auditConfig)
	service.Spec.Ports[0].Port = 1234

	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(deployment, service)

	d := s.createDeploymentHandler()
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(1, len(ds.Items))

	deployment = ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "", false)
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, deployment, s.scheme))
	deployment.ResourceVersion = "1000" // Needed because the fake client sets it.

	s.True(k8s.AreDeploymentsEqual(*deployment, ds.Items[0]))

	ss := &corev1.ServiceList{}
	s.NoError(d.KubeClient.List(s.ctx, ss))
	s.Equal(1, len(ss.Items))

	service = ScanApiService(s.auditConfig.Namespace, s.auditConfig)
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, service, s.scheme))
	service.ResourceVersion = "1000" // Needed because the fake client sets it.

	s.True(k8s.AreServicesEqual(*service, ss.Items[0]))
}

func (s *DeploymentHandlerSuite) TestReconcile_Cleanup_NoScanning() {
	// Disable all scanning
	s.auditConfig = utils.DefaultAuditConfig("mondoo-operator", false, false, false, false)

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	deployment := ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "", false)
	service := ScanApiService(s.auditConfig.Namespace, s.auditConfig)
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(deployment, service)

	d := s.createDeploymentHandler()
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(0, len(ds.Items))

	sec := &corev1.SecretList{}
	s.NoError(d.KubeClient.List(s.ctx, sec))
	s.Equal(0, len(sec.Items))

	ss := &corev1.ServiceList{}
	s.NoError(d.KubeClient.List(s.ctx, ss))
	s.Equal(0, len(ss.Items))
}

func (s *DeploymentHandlerSuite) TestReconcile_Cleanup_AuditConfigDeletion() {
	// Set the audit config for deletion
	now := metav1.Now()
	s.auditConfig.SetDeletionTimestamp(&now)

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	deployment := ScanApiDeployment(s.auditConfig.Namespace, image, s.auditConfig, "", false)
	service := ScanApiService(s.auditConfig.Namespace, s.auditConfig)
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(deployment, service)

	d := s.createDeploymentHandler()
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(0, len(ds.Items))

	sec := &corev1.SecretList{}
	s.NoError(d.KubeClient.List(s.ctx, sec))
	s.Equal(0, len(sec.Items))

	ss := &corev1.ServiceList{}
	s.NoError(d.KubeClient.List(s.ctx, ss))
	s.Equal(0, len(ss.Items))
}

func (s *DeploymentHandlerSuite) TestCleanup_AlreadyClean() {
	// Set the audit config for deletion
	now := metav1.Now()
	s.auditConfig.SetDeletionTimestamp(&now)

	d := s.createDeploymentHandler()
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Equal(0, len(ds.Items))

	ss := &corev1.ServiceList{}
	s.NoError(d.KubeClient.List(s.ctx, ss))
	s.Equal(0, len(ss.Items))
}

func (s *DeploymentHandlerSuite) TestReconcile_Create_KubernetesResources_OpenShift() {
	d := s.createDeploymentHandler()
	d.DeployOnOpenShift = true

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	ds := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, ds))
	s.Require().Equal(1, len(ds.Items))

	s.Nil(ds.Items[0].Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser, "expecting unset RunAsUser on OpenShift to allow OpenShift to select a UID from the allowed range (for the Namespace)")
}

func TestDeploymentHandlerSuite(t *testing.T) {
	suite.Run(t, new(DeploymentHandlerSuite))
}

func (s *DeploymentHandlerSuite) createDeploymentHandler() DeploymentHandler {
	return DeploymentHandler{
		KubeClient:             s.fakeClientBuilder.Build(),
		Mondoo:                 &s.auditConfig,
		ContainerImageResolver: s.containerImageResolver,
		MondooOperatorConfig:   &mondoov1alpha2.MondooOperatorConfig{},
	}
}
