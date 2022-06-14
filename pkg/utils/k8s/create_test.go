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
