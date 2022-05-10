/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodes

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	fakeMondoo "go.mondoo.com/mondoo-operator/pkg/utils/mondoo/fake"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
	s.Require().NoError(mondoov1alpha2.AddToScheme(s.scheme))
	s.containerImageResolver = fakeMondoo.NewNoOpContainerImageResolver()
}

func (s *DeploymentHandlerSuite) BeforeTest(suiteName, testName string) {
	s.auditConfig = utils.DefaultAuditConfig("mondoo-operator", false, true, false)
	s.fakeClientBuilder = fake.NewClientBuilder()
}

func (s *DeploymentHandlerSuite) TestReconcile_CreateConfigMap() {
	d := s.createDeploymentHandler()

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	expected := ConfigMap(s.auditConfig)
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, expected, d.KubeClient.Scheme()))

	// Set some fields that the kube client sets
	gvk, err := apiutil.GVKForObject(expected, d.KubeClient.Scheme())
	s.NoError(err)
	expected.SetGroupVersionKind(gvk)
	expected.ResourceVersion = "1"

	created := &corev1.ConfigMap{}
	created.Name = expected.Name
	created.Namespace = expected.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(created), created))

	s.Equal(expected, created)
}

func (s *DeploymentHandlerSuite) TestReconcile_UpdateConfigMap() {
	existing := ConfigMap(s.auditConfig)
	existing.Data["inventory"] = ""

	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(existing)
	d := s.createDeploymentHandler()

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	expected := ConfigMap(s.auditConfig)
	s.NoError(ctrl.SetControllerReference(&s.auditConfig, expected, d.KubeClient.Scheme()))

	// Set some fields that the kube client sets
	gvk, err := apiutil.GVKForObject(expected, d.KubeClient.Scheme())
	s.NoError(err)
	expected.SetGroupVersionKind(gvk)
	expected.ResourceVersion = "1000"

	created := &corev1.ConfigMap{}
	created.Name = expected.Name
	created.Namespace = expected.Namespace
	s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(created), created))

	s.Equal(expected, created)
}

func (s *DeploymentHandlerSuite) TestReconcile_CreateCronJobs() {
	s.seedNodes()
	d := s.createDeploymentHandler()

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.MondooClientImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	for _, n := range nodes.Items {
		expected := CronJob(image, n, s.auditConfig)
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
}

func (s *DeploymentHandlerSuite) TestReconcile_UpdateCronJobs() {
	s.seedNodes()
	d := s.createDeploymentHandler()

	nodes := &corev1.NodeList{}
	s.NoError(d.KubeClient.List(s.ctx, nodes))

	image, err := s.containerImageResolver.MondooClientImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	// Make sure a cron job exists for one of the nodes
	cronJob := CronJob(image, nodes.Items[1], s.auditConfig)
	cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command = []string{"test-command"}
	s.NoError(d.KubeClient.Create(s.ctx, cronJob))

	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	for i, n := range nodes.Items {
		expected := CronJob(image, n, s.auditConfig)
		s.NoError(ctrl.SetControllerReference(&s.auditConfig, expected, d.KubeClient.Scheme()))

		// Set some fields that the kube client sets
		gvk, err := apiutil.GVKForObject(expected, d.KubeClient.Scheme())
		s.NoError(err)
		expected.SetGroupVersionKind(gvk)

		// The second node has an updated cron job so resource version is +1
		expected.ResourceVersion = fmt.Sprintf("%d", 1+i)

		created := &batchv1.CronJob{}
		created.Name = expected.Name
		created.Namespace = expected.Namespace
		s.NoError(d.KubeClient.Get(s.ctx, client.ObjectKeyFromObject(created), created))

		s.Equal(expected, created)
	}
}

func (s *DeploymentHandlerSuite) TestReconcile_CleanCronJobsForDeletedNodes() {
	s.seedNodes()
	d := s.createDeploymentHandler()

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

	image, err := s.containerImageResolver.MondooClientImage(
		s.auditConfig.Spec.Scanner.Image.Name, s.auditConfig.Spec.Scanner.Image.Tag, false)
	s.NoError(err)

	cronJobs := &batchv1.CronJobList{}
	s.NoError(d.KubeClient.List(s.ctx, cronJobs))

	s.Equal(1, len(cronJobs.Items))

	expected := CronJob(image, nodes.Items[0], s.auditConfig)
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

func (s *DeploymentHandlerSuite) TestReconcile_NodeScanningStatus() {
	s.seedNodes()
	d := s.createDeploymentHandler()

	// Reconcile to create all resources
	result, err := d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify no conditions were added
	s.Equal(0, len(d.Mondoo.Status.Conditions))

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

	// Verify the node scanning status is set to unavailable
	condition := d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is unavailable", condition.Message)
	s.Equal("NodeScanningUnavailable", condition.Reason)
	s.Equal(corev1.ConditionTrue, condition.Status)

	// Make the jobs successful again
	cronJobs.Items[0].Status.LastScheduleTime = nil
	cronJobs.Items[0].Status.LastSuccessfulTime = nil
	s.NoError(d.KubeClient.Update(s.ctx, &cronJobs.Items[0]))

	// Reconcile to update the audit config status
	result, err = d.Reconcile(s.ctx)
	s.NoError(err)
	s.True(result.IsZero())

	// Verify the node scanning status is set to available
	condition = d.Mondoo.Status.Conditions[0]
	s.Equal("Node Scanning is available", condition.Message)
	s.Equal("NodeScanningAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)
}

func (s *DeploymentHandlerSuite) TestReconcile_DisableNodeScanning() {
	s.seedNodes()
	d := s.createDeploymentHandler()

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
}

func (s *DeploymentHandlerSuite) createDeploymentHandler() DeploymentHandler {
	return DeploymentHandler{
		KubeClient:             s.fakeClientBuilder.Build(),
		Mondoo:                 &s.auditConfig,
		ContainerImageResolver: s.containerImageResolver,
		MondooOperatorConfig:   &mondoov1alpha2.MondooOperatorConfig{},
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

func TestDeploymentHandlerSuite(t *testing.T) {
	suite.Run(t, new(DeploymentHandlerSuite))
}
