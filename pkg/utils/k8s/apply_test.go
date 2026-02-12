// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

type ApplySuite struct {
	suite.Suite
	ctx        context.Context
	fakeClient client.Client
	logger     logr.Logger
}

func (s *ApplySuite) SetupTest() {
	s.ctx = context.Background()
	s.fakeClient = fake.NewClientBuilder().Build()
	s.logger = logr.Discard()
}

func (s *ApplySuite) newDeployment(envValue string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "test",
						Image: "test:latest",
						Env: []corev1.EnvVar{{
							Name:  "SHARED_VAR",
							Value: envValue,
						}},
					}},
				},
			},
		},
	}
}

func (s *ApplySuite) newOwner() *v1alpha2.MondooAuditConfig {
	return &v1alpha2.MondooAuditConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "k8s.mondoo.com/v1alpha2",
			Kind:       "MondooAuditConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-owner",
			Namespace: "default",
			UID:       types.UID("test-uid-12345"),
		},
	}
}

func (s *ApplySuite) TestApply_CreateReturnsCreated() {
	owner := s.newOwner()
	dep := s.newDeployment("initial")

	op, err := Apply(s.ctx, s.fakeClient, dep, owner, s.logger, DefaultApplyOptions())
	s.NoError(err)
	s.Equal(ApplyCreated, op)

	// Verify creation
	var created appsv1.Deployment
	err = s.fakeClient.Get(s.ctx, client.ObjectKey{Namespace: "default", Name: "test-deployment"}, &created)
	s.NoError(err)
	s.Equal("initial", created.Spec.Template.Spec.Containers[0].Env[0].Value)
	s.Len(created.OwnerReferences, 1)
	s.Equal("test-owner", created.OwnerReferences[0].Name)
}

func (s *ApplySuite) TestApply_UpdateReturnsUpdated() {
	owner := s.newOwner()

	// Create
	dep := s.newDeployment("initial")
	op, err := Apply(s.ctx, s.fakeClient, dep, owner, s.logger, DefaultApplyOptions())
	s.NoError(err)
	s.Equal(ApplyCreated, op)

	// Update with changed value
	dep2 := s.newDeployment("updated")
	op, err = Apply(s.ctx, s.fakeClient, dep2, owner, s.logger, DefaultApplyOptions())
	s.NoError(err)
	s.Equal(ApplyUpdated, op)

	// Verify update
	var updated appsv1.Deployment
	err = s.fakeClient.Get(s.ctx, client.ObjectKey{Namespace: "default", Name: "test-deployment"}, &updated)
	s.NoError(err)
	s.Equal("updated", updated.Spec.Template.Spec.Containers[0].Env[0].Value)
}

func (s *ApplySuite) TestApply_UnchangedReturnsUnchanged() {
	owner := s.newOwner()

	// Create
	dep := s.newDeployment("same")
	op, err := Apply(s.ctx, s.fakeClient, dep, owner, s.logger, DefaultApplyOptions())
	s.NoError(err)
	s.Equal(ApplyCreated, op)

	// Apply same thing again
	dep2 := s.newDeployment("same")
	op, err = Apply(s.ctx, s.fakeClient, dep2, owner, s.logger, DefaultApplyOptions())
	s.NoError(err)
	s.Equal(ApplyUnchanged, op)
}

func (s *ApplySuite) TestApply_MigrationFromClientSideApply() {
	// Simulate pre-existing resource created with client-side apply
	existing := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "migrated-deployment",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "test",
						Image: "old:v1",
					}},
				},
			},
		},
	}
	err := s.fakeClient.Create(s.ctx, existing)
	s.Require().NoError(err)

	// Now apply with SSA (simulating migration)
	owner := s.newOwner()
	newDep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "migrated-deployment",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "test",
						Image: "new:v2",
					}},
				},
			},
		},
	}

	op, err := Apply(s.ctx, s.fakeClient, newDep, owner, s.logger, DefaultApplyOptions())
	s.NoError(err)
	s.Equal(ApplyUpdated, op)

	// Verify SSA took over
	var migrated appsv1.Deployment
	err = s.fakeClient.Get(s.ctx, client.ObjectKey{Namespace: "default", Name: "migrated-deployment"}, &migrated)
	s.NoError(err)
	s.Equal("new:v2", migrated.Spec.Template.Spec.Containers[0].Image)
	s.Len(migrated.OwnerReferences, 1)
}

func (s *ApplySuite) TestApply_MissingTypeMeta() {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	op, err := Apply(s.ctx, s.fakeClient, dep, nil, s.logger, DefaultApplyOptions())
	s.Error(err)
	s.Contains(err.Error(), "TypeMeta")
	s.Equal(ApplyUnchanged, op)
}

func (s *ApplySuite) TestApplyWithoutOwner() {
	dep := s.newDeployment("no-owner")

	op, err := ApplyWithoutOwner(s.ctx, s.fakeClient, dep, s.logger, DefaultApplyOptions())
	s.NoError(err)
	s.Equal(ApplyCreated, op)

	// Verify no owner references
	var created appsv1.Deployment
	err = s.fakeClient.Get(s.ctx, client.ObjectKey{Namespace: "default", Name: "test-deployment"}, &created)
	s.NoError(err)
	s.Empty(created.OwnerReferences)
}

func (s *ApplySuite) TestDefaultApplyOptions() {
	opts := DefaultApplyOptions()
	s.True(opts.ForceOwnership, "ForceOwnership should be true by default for migration")
}

func TestApplySuite(t *testing.T) {
	suite.Run(t, new(ApplySuite))
}
