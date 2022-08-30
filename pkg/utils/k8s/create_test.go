/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1 "k8s.io/api/apps/v1"
)

type CreateSuite struct {
	suite.Suite
	ctx context.Context
}

func (s *CreateSuite) SetupSuite() {
	s.ctx = context.Background()
}

func (s *CreateSuite) TestCreateIfNotExist() {
	client := fake.NewClientBuilder().Build()

	deployment := &appsv1.Deployment{}
	deployment.Name = "test-name"
	deployment.Namespace = "test-ns"
	created, err := CreateIfNotExist(s.ctx, client, &appsv1.Deployment{}, deployment)
	s.NoError(err)
	s.True(created)

	dList := &appsv1.DeploymentList{}
	s.NoError(client.List(s.ctx, dList))

	s.Equal(1, len(dList.Items))

	s.Equal("test-name", dList.Items[0].Name)
	s.Equal("test-ns", dList.Items[0].Namespace)
}

func (s *CreateSuite) TestCreateIfNotExist_AlreadyExists() {
	deployment := &appsv1.Deployment{}
	deployment.Name = "test-name"
	deployment.Namespace = "test-ns"

	client := fake.NewClientBuilder().WithObjects(deployment).Build()

	retrievedDeployment := &appsv1.Deployment{}
	created, err := CreateIfNotExist(s.ctx, client, retrievedDeployment, deployment)
	s.NoError(err)
	s.False(created)

	s.Equal("test-name", retrievedDeployment.Name)
	s.Equal("test-ns", retrievedDeployment.Namespace)

	dList := &appsv1.DeploymentList{}
	s.NoError(client.List(s.ctx, dList))
	s.Equal(1, len(dList.Items))

	s.Equal("test-name", dList.Items[0].Name)
	s.Equal("test-ns", dList.Items[0].Namespace)
}

func (s *CreateSuite) TestCreateIfNotExist_Error() {
	deployment := &appsv1.Deployment{}
	deployment.Namespace = "test-ns"

	client := fake.NewClientBuilder().Build()

	created, err := CreateIfNotExist(s.ctx, client, &appsv1.Deployment{}, deployment)
	s.Error(err)
	s.False(created)

	dList := &appsv1.DeploymentList{}
	s.NoError(client.List(s.ctx, dList))
	s.Equal(0, len(dList.Items))
}

func TestCreateSuite(t *testing.T) {
	suite.Run(t, new(CreateSuite))
}
