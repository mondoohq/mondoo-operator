// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	fakeMondoo "go.mondoo.com/mondoo-operator/pkg/utils/mondoo/fake"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testNamespace = "mondoo-operator"

type DeploymentHandlerSuite struct {
	suite.Suite
	ctx                    context.Context
	scheme                 *runtime.Scheme
	containerImageResolver mondoo.ContainerImageResolver

	auditConfig       v1alpha2.MondooAuditConfig
	fakeClientBuilder *fake.ClientBuilder
}

func (s *DeploymentHandlerSuite) SetupSuite() {
	s.ctx = context.Background()
	s.scheme = clientgoscheme.Scheme
	s.Require().NoError(v1alpha2.AddToScheme(s.scheme))
	s.containerImageResolver = fakeMondoo.NewNoOpContainerImageResolver()
}

func (s *DeploymentHandlerSuite) BeforeTest(suiteName, testName string) {
	s.auditConfig = utils.DefaultAuditConfig(testNamespace, false, false, true, false)
	s.fakeClientBuilder = fake.NewClientBuilder()
	s.seedKubeSystemNamespace()
}

func (s *DeploymentHandlerSuite) TestReconcile_CreateConfigMap() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	for _, node := range nodes.Items {
		cfgMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: ConfigMapName(s.auditConfig.Name, node.Name), Namespace: s.auditConfig.Namespace,
		}}
		s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(cfgMap), cfgMap))

		cfgMapExpected := cfgMap.DeepCopy()
		s.Require().NoError(UpdateConfigMap(cfgMapExpected, node, "", testClusterUID, s.auditConfig))
		s.True(equality.Semantic.DeepEqual(cfgMapExpected, cfgMap))
	}
}

func (s *DeploymentHandlerSuite) TestReconcile_CreateConfigMapWithIntegrationMRN() {
	const testIntegrationMRN = "//test-integration-MRN"

	s.seedNodes()

	sa, err := json.Marshal(mondooclient.ServiceAccountCredentials{Mrn: "test-mrn"})
	s.NoError(err)

	s.auditConfig.Spec.ConsoleIntegration.Enable = true
	credsWithIntegration := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.MondooClientSecret,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			constants.MondooCredsSecretIntegrationMRNKey: []byte(testIntegrationMRN),
			constants.MondooCredsSecretServiceAccountKey: sa,
		},
	}
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(credsWithIntegration)

	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	for _, node := range nodes.Items {
		cfgMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: ConfigMapName(s.auditConfig.Name, node.Name), Namespace: s.auditConfig.Namespace,
		}}
		s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(cfgMap), cfgMap))

		cfgMapExpected := cfgMap.DeepCopy()
		s.Require().NoError(UpdateConfigMap(cfgMapExpected, node, testIntegrationMRN, testClusterUID, s.auditConfig))
		s.True(equality.Semantic.DeepEqual(cfgMapExpected, cfgMap))
	}
}

func (s *DeploymentHandlerSuite) TestReconcile_UpdateConfigMap() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	for _, node := range nodes.Items {
		cfgMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: ConfigMapName(s.auditConfig.Name, node.Name), Namespace: s.auditConfig.Namespace,
		}}
		s.Require().NoError(UpdateConfigMap(cfgMap, node, "", testClusterUID, s.auditConfig))
		cfgMap.Data["inventory"] = ""
		s.NoError(d.KubeClient.Create(s.ctx, cfgMap))
	}

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	for _, node := range nodes.Items {
		cfgMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: ConfigMapName(s.auditConfig.Name, node.Name), Namespace: s.auditConfig.Namespace,
		}}
		s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(cfgMap), cfgMap))

		cfgMapExpected := cfgMap.DeepCopy()
		s.Require().NoError(UpdateConfigMap(cfgMapExpected, node, "", testClusterUID, s.auditConfig))
		s.True(equality.Semantic.DeepEqual(cfgMapExpected, cfgMap))
	}
}

func (s *DeploymentHandlerSuite) TestReconcile_CronJob_CleanConfigMapsForDeletedNodes() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	// Reconcile to create the initial cron jobs
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	// Delete one node
	s.NoError(d.KubeClient.Delete(s.ctx, &nodes.Items[1]))

	// Reconcile again to delete the cron job for the deleted node
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	configMaps := &corev1.ConfigMapList{}
	s.NoError(d.KubeClient.List(s.ctx, configMaps))

	s.Equal(1, len(configMaps.Items))

	cfgMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: ConfigMapName(s.auditConfig.Name, nodes.Items[0].Name), Namespace: s.auditConfig.Namespace,
	}}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(cfgMap), cfgMap))

	cfgMapExpected := cfgMap.DeepCopy()
	s.Require().NoError(UpdateConfigMap(cfgMapExpected, nodes.Items[0], "", testClusterUID, s.auditConfig))
	s.True(equality.Semantic.DeepEqual(cfgMapExpected, cfgMap))
}

func (s *DeploymentHandlerSuite) TestReconcile_Deployment_CleanConfigMapsForDeletedNodes() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	s.auditConfig.Spec.Nodes.Style = v1alpha2.NodeScanStyle_Deployment
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	// Reconcile to create the initial cron jobs
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	// Delete one node
	s.NoError(d.KubeClient.Delete(s.ctx, &nodes.Items[1]))

	// Reconcile again to delete the cron job for the deleted node
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	configMaps := &corev1.ConfigMapList{}
	s.NoError(d.KubeClient.List(s.ctx, configMaps))

	s.Equal(1, len(configMaps.Items))

	cfgMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: ConfigMapName(s.auditConfig.Name, nodes.Items[0].Name), Namespace: s.auditConfig.Namespace,
	}}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(cfgMap), cfgMap))

	cfgMapExpected := cfgMap.DeepCopy()
	s.Require().NoError(UpdateConfigMap(cfgMapExpected, nodes.Items[0], "", testClusterUID, s.auditConfig))
	s.True(equality.Semantic.DeepEqual(cfgMapExpected, cfgMap))
}

func (s *DeploymentHandlerSuite) TestReconcile_CreateCronJobs() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	for _, n := range nodes.Items {
		cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: CronJobName(s.auditConfig.Name, n.Name), Namespace: s.auditConfig.Namespace}}
		s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(cj), cj))

		cjExpected := cj.DeepCopy()
		UpdateCronJob(cjExpected, image, n, &s.auditConfig, false, v1alpha2.MondooOperatorConfig{})
		s.True(equality.Semantic.DeepEqual(cjExpected, cj))
	}

	operatorImage, err := s.containerImageResolver.MondooOperatorImage(s.ctx, "", "", false)
	s.NoError(err)

	// Verify node garbage collection cronjob
	gcCj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: GarbageCollectCronJobName(s.auditConfig.Name), Namespace: s.auditConfig.Namespace}}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(gcCj), gcCj))

	gcCjExpected := gcCj.DeepCopy()
	UpdateGarbageCollectCronJob(gcCjExpected, operatorImage, "abcdefg", s.auditConfig)
	s.True(equality.Semantic.DeepEqual(gcCjExpected, gcCj))
}

func (s *DeploymentHandlerSuite) TestReconcile_CreateCronJobs_Switch() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	for _, n := range nodes.Items {
		cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: CronJobName(s.auditConfig.Name, n.Name), Namespace: s.auditConfig.Namespace}}
		s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(cj), cj))

		cjExpected := cj.DeepCopy()
		UpdateCronJob(cjExpected, image, n, &s.auditConfig, false, v1alpha2.MondooOperatorConfig{})
		s.True(equality.Semantic.DeepEqual(cjExpected, cj))
	}

	mondooAuditConfig.Spec.Nodes.Style = v1alpha2.NodeScanStyle_Deployment
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	listOpts := &client.ListOptions{
		Namespace:     s.auditConfig.Namespace,
		LabelSelector: labels.SelectorFromSet(CronJobLabels(s.auditConfig)),
	}
	cronjobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronjobs, listOpts))

	s.Empty(cronjobs.Items)

	operatorImage, err := s.containerImageResolver.MondooOperatorImage(s.ctx, "", "", false)
	s.NoError(err)

	// Verify node garbage collection cronjob
	gcCj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: GarbageCollectCronJobName(s.auditConfig.Name), Namespace: s.auditConfig.Namespace}}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(gcCj), gcCj))

	gcCjExpected := gcCj.DeepCopy()
	UpdateGarbageCollectCronJob(gcCjExpected, operatorImage, "abcdefg", s.auditConfig)
	s.True(equality.Semantic.DeepEqual(gcCjExpected, gcCj))
}

func (s *DeploymentHandlerSuite) TestReconcile_UpdateCronJobs() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	// Make sure a cron job exists for one of the nodes
	cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: CronJobName(s.auditConfig.Name, nodes.Items[1].Name), Namespace: s.auditConfig.Namespace}}
	UpdateCronJob(cj, image, nodes.Items[1], &s.auditConfig, false, v1alpha2.MondooOperatorConfig{})
	cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command = []string{"test-command"}
	s.NoError(d.KubeClient.Create(s.ctx, cj))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	for _, n := range nodes.Items {
		cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: CronJobName(s.auditConfig.Name, n.Name), Namespace: s.auditConfig.Namespace}}
		s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(cj), cj))

		cjExpected := cj.DeepCopy()
		UpdateCronJob(cjExpected, image, n, &s.auditConfig, false, v1alpha2.MondooOperatorConfig{})
		s.True(equality.Semantic.DeepEqual(cjExpected, cj))
	}
}

func (s *DeploymentHandlerSuite) TestReconcile_CleanCronJobsForDeletedNodes() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	// Reconcile to create the initial cron jobs
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	// Delete one node
	s.NoError(d.KubeClient.Delete(s.ctx, &nodes.Items[1]))

	// Reconcile again to delete the cron job for the deleted node
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	listOpts := &client.ListOptions{
		Namespace:     s.auditConfig.Namespace,
		LabelSelector: labels.SelectorFromSet(CronJobLabels(s.auditConfig)),
	}
	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs, listOpts))

	s.Equal(1, len(cronJobs.Items))

	cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: CronJobName(s.auditConfig.Name, nodes.Items[0].Name), Namespace: s.auditConfig.Namespace}}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(cj), cj))

	cjExpected := cj.DeepCopy()
	UpdateCronJob(cjExpected, image, nodes.Items[0], &s.auditConfig, false, v1alpha2.MondooOperatorConfig{})
	s.True(equality.Semantic.DeepEqual(cjExpected, cj))
}

func (s *DeploymentHandlerSuite) TestReconcile_CreateDeployments() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	s.auditConfig.Spec.Nodes.Style = v1alpha2.NodeScanStyle_Deployment
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	for _, n := range nodes.Items {
		dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName(s.auditConfig.Name, n.Name), Namespace: s.auditConfig.Namespace}}
		s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(dep), dep))

		depExpected := dep.DeepCopy()
		UpdateDeployment(depExpected, n, s.auditConfig, false, image, v1alpha2.MondooOperatorConfig{})
		s.True(equality.Semantic.DeepEqual(depExpected, dep))
	}

	operatorImage, err := s.containerImageResolver.MondooOperatorImage(s.ctx, "", "", false)
	s.NoError(err)

	// Verify node garbage collection cronjob
	gcCj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: GarbageCollectCronJobName(s.auditConfig.Name), Namespace: s.auditConfig.Namespace}}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(gcCj), gcCj))

	gcCjExpected := gcCj.DeepCopy()
	UpdateGarbageCollectCronJob(gcCjExpected, operatorImage, "abcdefg", s.auditConfig)
	s.True(equality.Semantic.DeepEqual(gcCjExpected, gcCj))
}

func (s *DeploymentHandlerSuite) TestReconcile_CreateDeployments_Switch() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	s.auditConfig.Spec.Nodes.Style = v1alpha2.NodeScanStyle_Deployment
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	for _, n := range nodes.Items {
		dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName(s.auditConfig.Name, n.Name), Namespace: s.auditConfig.Namespace}}
		s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(dep), dep))

		depExpected := dep.DeepCopy()
		UpdateDeployment(depExpected, n, s.auditConfig, false, image, v1alpha2.MondooOperatorConfig{})
		s.True(equality.Semantic.DeepEqual(depExpected, dep))
	}

	mondooAuditConfig.Spec.Nodes.Style = v1alpha2.NodeScanStyle_CronJob
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	listOpts := &client.ListOptions{
		Namespace:     s.auditConfig.Namespace,
		LabelSelector: labels.SelectorFromSet(CronJobLabels(s.auditConfig)),
	}
	deployments := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, deployments, listOpts))

	s.Empty(deployments.Items)

	operatorImage, err := s.containerImageResolver.MondooOperatorImage(s.ctx, "", "", false)
	s.NoError(err)

	// Verify node garbage collection cronjob
	gcCj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: GarbageCollectCronJobName(s.auditConfig.Name), Namespace: s.auditConfig.Namespace}}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(gcCj), gcCj))

	gcCjExpected := gcCj.DeepCopy()
	UpdateGarbageCollectCronJob(gcCjExpected, operatorImage, "abcdefg", s.auditConfig)
	s.True(equality.Semantic.DeepEqual(gcCjExpected, gcCj))
}

func (s *DeploymentHandlerSuite) TestReconcile_UpdateDeployments() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	s.auditConfig.Spec.Nodes.Style = v1alpha2.NodeScanStyle_Deployment
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	// Make sure a deployment exists for one of the nodes
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName(s.auditConfig.Name, nodes.Items[1].Name), Namespace: s.auditConfig.Namespace}}
	UpdateDeployment(dep, nodes.Items[1], s.auditConfig, false, image, v1alpha2.MondooOperatorConfig{})
	dep.Spec.Template.Spec.Containers[0].Command = []string{"test-command"}
	s.NoError(d.KubeClient.Create(s.ctx, dep))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	for _, n := range nodes.Items {
		dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName(s.auditConfig.Name, n.Name), Namespace: s.auditConfig.Namespace}}
		s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(dep), dep))

		depExpected := dep.DeepCopy()
		UpdateDeployment(depExpected, n, s.auditConfig, false, image, v1alpha2.MondooOperatorConfig{})
		s.True(equality.Semantic.DeepEqual(depExpected, dep))
	}
}

func (s *DeploymentHandlerSuite) TestReconcile_CleanDeploymentsForDeletedNodes() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	s.auditConfig.Spec.Nodes.Style = v1alpha2.NodeScanStyle_Deployment
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	// Reconcile to create the initial cron jobs
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	// Delete one node
	s.NoError(d.KubeClient.Delete(s.ctx, &nodes.Items[1]))

	// Reconcile again to delete the cron job for the deleted node
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	image, err := s.containerImageResolver.CnspecImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	listOpts := &client.ListOptions{
		Namespace:     s.auditConfig.Namespace,
		LabelSelector: labels.SelectorFromSet(CronJobLabels(s.auditConfig)),
	}
	deployments := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, deployments, listOpts))

	s.Equal(1, len(deployments.Items))

	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName(s.auditConfig.Name, nodes.Items[0].Name), Namespace: s.auditConfig.Namespace}}
	UpdateDeployment(dep, nodes.Items[0], s.auditConfig, false, image, v1alpha2.MondooOperatorConfig{})
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(dep), dep))

	depExpected := dep.DeepCopy()
	UpdateDeployment(depExpected, nodes.Items[0], s.auditConfig, false, image, v1alpha2.MondooOperatorConfig{})
	s.True(equality.Semantic.DeepEqual(depExpected, dep))
}

func (s *DeploymentHandlerSuite) TestReconcile_CronJob_NodeScanningStatus() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	// Reconcile to create all resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the node scanning status is set to available
	s.Equal(1, len(d.Mondoo.Status.Conditions))
	condition := d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is available", condition.Message)
	s.Equal("NodeScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)

	listOpts := &client.ListOptions{
		Namespace:     s.auditConfig.Namespace,
		LabelSelector: labels.SelectorFromSet(CronJobLabels(s.auditConfig)),
	}
	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs, listOpts))

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

	// Verify the node scanning status is set to unavailable
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is unavailable", condition.Message)
	s.Equal("NodeScanningUnavailable", condition.Reason)
	s.Equal(corev1.ConditionTrue, condition.Status)

	// Make the jobs successful again
	cronJobs.Items[0].Status.LastScheduleTime = nil
	cronJobs.Items[0].Status.LastSuccessfulTime = nil
	s.NoError(d.KubeClient.Status().Update(s.ctx, &cronJobs.Items[0]))

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the node scanning status is set to available
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is available", condition.Message)
	s.Equal("NodeScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)

	d.Mondoo.Spec.Nodes.Enable = false

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the node scanning status is set to disabled
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is disabled", condition.Message)
	s.Equal("NodeScanningDisabled", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)
}

func (s *DeploymentHandlerSuite) TestReconcile_Deployment_NodeScanningStatus() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	s.auditConfig.Spec.Nodes.Style = v1alpha2.NodeScanStyle_Deployment
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	// Reconcile to create all resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the node scanning status is set to available
	s.Equal(1, len(d.Mondoo.Status.Conditions))
	condition := d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is unavailable", condition.Message)
	s.Equal("NodeScanningUnavailable", condition.Reason)
	s.Equal(corev1.ConditionTrue, condition.Status)

	listOpts := &client.ListOptions{
		Namespace:     s.auditConfig.Namespace,
		LabelSelector: labels.SelectorFromSet(CronJobLabels(s.auditConfig)),
	}
	deployments := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, deployments, listOpts))

	// Make sure all deployments are ready
	deployments.Items[0].Status.ReadyReplicas = 1
	s.NoError(d.KubeClient.Status().Update(s.ctx, &deployments.Items[0]))
	deployments.Items[1].Status.ReadyReplicas = 1
	s.NoError(d.KubeClient.Status().Update(s.ctx, &deployments.Items[1]))

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the node scanning status is set to unavailable
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is available", condition.Message)
	s.Equal("NodeScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)

	// // Make a deployment fail again
	s.NoError(d.KubeClient.List(s.ctx, deployments, listOpts))
	deployments.Items[0].Status.ReadyReplicas = 0
	s.NoError(d.KubeClient.Status().Update(s.ctx, &deployments.Items[0]))

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the node scanning status is set to available
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is unavailable", condition.Message)
	s.Equal("NodeScanningUnavailable", condition.Reason)
	s.Equal(corev1.ConditionTrue, condition.Status)

	d.Mondoo.Spec.Nodes.Enable = false

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the node scanning status is set to disabled
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is disabled", condition.Message)
	s.Equal("NodeScanningDisabled", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)
}

func (s *DeploymentHandlerSuite) TestReconcile_NodeScanningOOMStatus() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	// Reconcile to create all resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the node scanning status is set to available
	s.Equal(1, len(d.Mondoo.Status.Conditions))
	condition := d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is available", condition.Message)
	s.Equal("NodeScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)
	s.Len(condition.AffectedPods, 0)

	listOpts := &client.ListOptions{
		Namespace:     s.auditConfig.Namespace,
		LabelSelector: labels.SelectorFromSet(CronJobLabels(s.auditConfig)),
	}
	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs, listOpts))

	oomPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-scan-123",
			Namespace: s.auditConfig.Namespace,
			Labels:    CronJobLabels(s.auditConfig),
			CreationTimestamp: metav1.Time{
				Time: time.Now(),
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "cnspec",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: *resource.NewQuantity(1, resource.BinarySI),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "cnspec",
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 137,
						},
					},
				},
			},
		},
	}

	err = d.KubeClient.Create(s.ctx, oomPod)
	s.NoError(err)

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	pods := &corev1.PodList{}
	s.NoError(d.KubeClient.List(s.ctx, pods, listOpts))
	s.Equal(1, len(pods.Items))

	// Verify the node scanning status is set to unavailable
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is unavailable due to OOM", condition.Message)
	s.Len(condition.AffectedPods, 1)
	s.Contains(condition.AffectedPods, "node-scan-123")
	containerMemory := pods.Items[0].Spec.Containers[0].Resources.Limits.Memory()
	s.Equal(containerMemory.String(), condition.MemoryLimit)
	s.Equal("NodeScanningUnavailable", condition.Reason)
	s.Equal(corev1.ConditionTrue, condition.Status)

	err = d.KubeClient.Delete(s.ctx, &pods.Items[0])
	s.NoError(err)
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the node scanning status is set to available again
	s.Equal(1, len(d.Mondoo.Status.Conditions))
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is available", condition.Message)
	s.Equal("NodeScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)
	s.Len(condition.AffectedPods, 0)
}

func (s *DeploymentHandlerSuite) TestReconcile_DisableNodeScanning() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	// Reconcile to create all resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Reconcile again to delete the resources
	d.Mondoo.Spec.Nodes.Enable = false
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	configMaps := &corev1.ConfigMapList{}
	s.NoError(d.KubeClient.List(s.ctx, configMaps))
	s.Equal(0, len(configMaps.Items))

	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs))
	s.Equal(0, len(cronJobs.Items))

	deployments := &appsv1.DeploymentList{}
	s.NoError(d.KubeClient.List(s.ctx, deployments))
	s.Equal(0, len(deployments.Items))
}

func (s *DeploymentHandlerSuite) TestReconcile_CronJob_CustomSchedule() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	customSchedule := "0 0 * * *"
	s.auditConfig.Spec.Nodes.Schedule = customSchedule

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: CronJobName(s.auditConfig.Name, nodes.Items[0].Name), Namespace: s.auditConfig.Namespace}}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(cj), cj))

	s.Equal(cj.Spec.Schedule, customSchedule)
}

func (s *DeploymentHandlerSuite) TestReconcile_Deployment_CustomInterval() {
	s.seedNodes()
	d := s.createDeploymentHandler()
	s.auditConfig.Spec.Nodes.Style = v1alpha2.NodeScanStyle_Deployment
	mondooAuditConfig := &s.auditConfig
	s.NoError(d.KubeClient.Create(s.ctx, mondooAuditConfig))

	s.auditConfig.Spec.Nodes.IntervalTimer = 1034

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName(s.auditConfig.Name, nodes.Items[0].Name), Namespace: s.auditConfig.Namespace}}
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(dep), dep))

	s.Contains(dep.Spec.Template.Spec.Containers[0].Command, "--timer")
	s.Contains(dep.Spec.Template.Spec.Containers[0].Command, fmt.Sprintf("%d", s.auditConfig.Spec.Nodes.IntervalTimer))
}

func (s *DeploymentHandlerSuite) createDeploymentHandler() DeploymentHandler {
	return DeploymentHandler{
		KubeClient:             s.fakeClientBuilder.Build(),
		Mondoo:                 &s.auditConfig,
		ContainerImageResolver: s.containerImageResolver,
		MondooOperatorConfig:   &v1alpha2.MondooOperatorConfig{},
	}
}

func (s *DeploymentHandlerSuite) seedNodes() {
	master := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node01"},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "node-role.kubernetes.io/master", Value: "true", Effect: corev1.TaintEffectNoExecute},
			},
		},
	}
	worker := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node02"}}
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(master, worker)
}

func (s *DeploymentHandlerSuite) seedKubeSystemNamespace() {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  testClusterUID,
		},
	}
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(namespace)
}

func TestDeploymentHandlerSuite(t *testing.T) {
	suite.Run(t, new(DeploymentHandlerSuite))
}
