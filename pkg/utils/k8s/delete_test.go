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

type DeleteSuite struct {
	suite.Suite
	ctx context.Context
}

func (s *DeleteSuite) SetupSuite() {
	s.ctx = context.Background()
}

func (s *DeleteSuite) TestDeleteIfExists() {
	deployment := &appsv1.Deployment{}
	deployment.Name = "test-name"
	deployment.Namespace = "test-ns"

	client := fake.NewClientBuilder().WithObjects(deployment).Build()
	s.NoError(DeleteIfExists(s.ctx, client, deployment))

	dList := &appsv1.DeploymentList{}
	s.NoError(client.List(s.ctx, dList))
	s.Equal(0, len(dList.Items))
}

func (s *DeleteSuite) TestDeleteIfExists_DoesNotExist() {
	deployment := &appsv1.Deployment{}
	deployment.Name = "test-name"
	deployment.Namespace = "test-ns"

	client := fake.NewClientBuilder().Build()

	s.NoError(DeleteIfExists(s.ctx, client, deployment))

	dList := &appsv1.DeploymentList{}
	s.NoError(client.List(s.ctx, dList))
	s.Equal(0, len(dList.Items))
}

func (s *DeleteSuite) TestDeleteIfExists_Error() {
	deployment := &appsv1.Deployment{}
	deployment.Namespace = "test-ns"

	client := fake.NewClientBuilder().Build()

	s.Error(DeleteIfExists(s.ctx, client, deployment))

	dList := &appsv1.DeploymentList{}
	s.NoError(client.List(s.ctx, dList))
	s.Equal(0, len(dList.Items))
}

func TestDeleteSuite(t *testing.T) {
	suite.Run(t, new(CreateSuite))
}
