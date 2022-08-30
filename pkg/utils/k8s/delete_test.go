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
