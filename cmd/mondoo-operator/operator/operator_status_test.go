// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package operator

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
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

func (s *DeploymentHandlerSuite) TestOOMDetect() {
	mondooAuditConfig := &s.auditConfig

	oomPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mondoo-operator-123",
			Namespace: s.auditConfig.Namespace,
			Labels:    map[string]string{"app.kubernetes.io/name": "mondoo-operator"},
			CreationTimestamp: metav1.Time{
				Time: time.Now(),
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "manager",
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
					Name: "manager",
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 137,
						},
					},
				},
			},
		},
	}

	// This is needed because of https://github.com/kubernetes-sigs/controller-runtime/issues/2362
	objs := []client.Object{mondooAuditConfig, oomPod}
	k8sClient := s.fakeClientBuilder.WithScheme(clientgoscheme.Scheme).WithStatusSubresource(objs...).WithObjects(objs...).Build()

	v := &version.Info{}
	cfg := zap.NewDevelopmentConfig()
	cfg.InitialFields = map[string]interface{}{
		"controller": "terminated-test",
	}
	zapLog, err := cfg.Build()
	s.Require().NoError(err, "failed to set up logging for test cases")
	testLogger := zapr.NewLogger(zapLog)

	err = checkForTerminatedState(s.ctx, k8sClient, v, false, testLogger)
	s.NoError(err)

	mondooAuditConfigs := &mondoov1alpha2.MondooAuditConfigList{}
	err = k8sClient.List(s.ctx, mondooAuditConfigs)
	s.NoError(err)
	s.Len(mondooAuditConfigs.Items, 1)

	condition := mondooAuditConfigs.Items[0].Status.Conditions[0]
	s.Equal("Mondoo Operator controller is unavailable due to OOM", condition.Message)
	s.Len(condition.AffectedPods, 1)
	s.Contains(condition.AffectedPods, "mondoo-operator-123")
	containerMemory := oomPod.Spec.Containers[0].Resources.Limits.Memory()
	s.Equal(containerMemory.String(), condition.MemoryLimit)
	s.Equal("MondooOperatorUnavailable", condition.Reason)
	s.Equal(corev1.ConditionTrue, condition.Status)

	oomPod.Status.ContainerStatuses[0].LastTerminationState = corev1.ContainerState{}
	oomPod.Status.ContainerStatuses[0].State.Running = &corev1.ContainerStateRunning{}
	s.NoError(k8sClient.Status().Update(s.ctx, oomPod))

	err = checkForTerminatedState(s.ctx, k8sClient, v, false, testLogger)
	s.NoError(err)

	mondooAuditConfigs = &mondoov1alpha2.MondooAuditConfigList{}
	err = k8sClient.List(s.ctx, mondooAuditConfigs)
	s.NoError(err)
	s.Len(mondooAuditConfigs.Items, 1)

	condition = mondooAuditConfigs.Items[0].Status.Conditions[0]
	s.Equal("Mondoo Operator controller is available", condition.Message)
	s.Len(condition.AffectedPods, 0)
	s.Equal("", condition.MemoryLimit)
	s.Equal("MondooOperatorAvailable", condition.Reason)
	s.Equal(corev1.ConditionFalse, condition.Status)
}

func TestOperatorSuite(t *testing.T) {
	suite.Run(t, new(DeploymentHandlerSuite))
}
