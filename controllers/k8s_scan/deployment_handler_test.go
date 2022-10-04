/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package k8s_scan

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
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
	scanapistoremock "go.mondoo.com/mondoo-operator/controllers/resource_monitor/scan_api_store/mock"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
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
	mockCtrl          *gomock.Controller
	scanApiStoreMock  *scanapistoremock.MockScanApiStore
}

func (s *DeploymentHandlerSuite) SetupSuite() {
	s.ctx = context.Background()
	s.scheme = clientgoscheme.Scheme
	s.Require().NoError(mondoov1alpha2.AddToScheme(s.scheme))
	s.containerImageResolver = fakeMondoo.NewNoOpContainerImageResolver()
	s.mockCtrl = gomock.NewController(s.T())
	s.scanApiStoreMock = scanapistoremock.NewMockScanApiStore(s.mockCtrl)
}

func (s *DeploymentHandlerSuite) BeforeTest(suiteName, testName string) {
	s.auditConfig = utils.DefaultAuditConfig("mondoo-operator", true, false, false)
	s.fakeClientBuilder = fake.NewClientBuilder().WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scanapi.TokenSecretName(s.auditConfig.Name),
			Namespace: s.auditConfig.Namespace,
		},
		Data: map[string][]byte{"token": []byte("token")},
	}, test.TestKubeSystemNamespace())
}

func (s *DeploymentHandlerSuite) AfterTest(suiteName, testName string) {
	s.mockCtrl.Finish()
}

func (s *DeploymentHandlerSuite) TestReconcile_Create() {
	d := s.createDeploymentHandler()

	scanApiUrl := scanapi.ScanApiServiceUrl(*d.Mondoo)
	s.scanApiStoreMock.EXPECT().Add(scanApiUrl, "token", "").Times(1)

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.MondooOperatorImage("", "", false)
	s.NoError(err)

	expected := CronJob(image, "", test.KubeSystemNamespaceUid, s.auditConfig)
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
	scanApiUrl := scanapi.ScanApiServiceUrl(*d.Mondoo)
	s.scanApiStoreMock.EXPECT().Add(scanApiUrl, "token", integrationMrn).Times(1)

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

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.MondooOperatorImage("", "", false)
	s.NoError(err)

	expected := CronJob(image, integrationMrn, test.KubeSystemNamespaceUid, s.auditConfig)
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

	scanApiUrl := scanapi.ScanApiServiceUrl(*d.Mondoo)
	s.scanApiStoreMock.EXPECT().Add(scanApiUrl, "token", "").Times(1)

	image, err := s.containerImageResolver.MondooOperatorImage("", "", false)
	s.NoError(err)

	// Make sure a cron job exists with different container command
	cronJob := CronJob(image, "", "", s.auditConfig)
	cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command = []string{"test-command"}
	s.NoError(d.KubeClient.Create(s.ctx, cronJob))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	expected := CronJob(image, "", test.KubeSystemNamespaceUid, s.auditConfig)
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

func (s *DeploymentHandlerSuite) TestReconcile_K8sResourceScanningStatus() {
	d := s.createDeploymentHandler()

	scanApiUrl := scanapi.ScanApiServiceUrl(*d.Mondoo)
	s.scanApiStoreMock.EXPECT().Add(scanApiUrl, "token", "").Times(4)

	// Reconcile to create all resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify container image scanning and kubernetes resources conditions
	s.Equal(2, len(d.Mondoo.Status.Conditions))
	condition := d.Mondoo.Status.Conditions[0]
	s.Equal("Kubernetes Container Image Scanning is disabled", condition.Message)
	s.Equal("KubernetesContainerImageScanningDisabled", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)

	condition = d.Mondoo.Status.Conditions[1]
	s.Equal("Kubernetes Resources Scanning is Available", condition.Message)
	s.Equal("KubernetesResourcesScanningAvailable", condition.Reason)
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

	// Verify the kubernetes resources status is set to unavailable
	condition = d.Mondoo.Status.Conditions[1]
	s.Equal("Kubernetes Resources Scanning is Unavailable", condition.Message)
	s.Equal("KubernetesResourcesScanningUnavailable", condition.Reason)
	s.Equal(corev1.ConditionTrue, condition.Status)

	// Make the jobs successful again
	cronJobs.Items[0].Status.LastScheduleTime = nil
	cronJobs.Items[0].Status.LastSuccessfulTime = nil
	s.NoError(d.KubeClient.Update(s.ctx, &cronJobs.Items[0]))

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the kubernetes resources scanning status is set to available
	condition = d.Mondoo.Status.Conditions[1]
	s.Equal("Kubernetes Resources Scanning is Available", condition.Message)
	s.Equal("KubernetesResourcesScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)

	// Make the jobs active
	cronJobs.Items[0].Status.LastScheduleTime = &metaNow
	cronJobs.Items[0].Status.LastSuccessfulTime = &metaHourAgo
	// Add an entry of an active job
	cronJobs.Items[0].Status.Active = append(cronJobs.Items[0].Status.Active, corev1.ObjectReference{})
	s.NoError(d.KubeClient.Update(s.ctx, &cronJobs.Items[0]))

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the kubernetes resources scanning status is set to available when there is an active scan
	condition = d.Mondoo.Status.Conditions[1]
	s.Equal("Kubernetes Resources Scanning is Available", condition.Message)
	s.Equal("KubernetesResourcesScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)

	d.Mondoo.Spec.KubernetesResources.Enable = false
	s.scanApiStoreMock.EXPECT().Delete(scanApiUrl).Times(1)

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the kubernetes resources scanning status is set to disabled
	condition = d.Mondoo.Status.Conditions[1]
	s.Equal("Kubernetes Resources Scanning is disabled", condition.Message)
	s.Equal("KubernetesResourcesScanningDisabled", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)
}

func (s *DeploymentHandlerSuite) TestReconcile_Disable() {
	d := s.createDeploymentHandler()

	scanApiUrl := scanapi.ScanApiServiceUrl(*d.Mondoo)
	s.scanApiStoreMock.EXPECT().Add(scanApiUrl, "token", "").Times(1)

	// Reconcile to create all resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Reconcile again to delete the resources
	d.Mondoo.Spec.KubernetesResources.Enable = false
	s.scanApiStoreMock.EXPECT().Delete(scanApiUrl).Times(1)
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
		ScanApiStore:           s.scanApiStoreMock,
	}
}

func TestDeploymentHandlerSuite(t *testing.T) {
	suite.Run(t, new(DeploymentHandlerSuite))
}
