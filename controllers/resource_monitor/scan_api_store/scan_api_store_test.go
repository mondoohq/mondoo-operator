/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package scan_api_store

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient/mock"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
)

type ScanApiStoreSuite struct {
	suite.Suite
	ctx              context.Context
	ctxCancel        context.CancelFunc
	mockCtrl         *gomock.Controller
	mockMondooClient *mock.MockClient
	scanApiStore     *scanApiStore
}

func (s *ScanApiStoreSuite) BeforeTest(suiteName, testName string) {
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.mockCtrl = gomock.NewController(s.T())
	s.mockMondooClient = mock.NewMockClient(s.mockCtrl)
	s.scanApiStore = NewScanApiStore(s.ctx).(*scanApiStore)
}

func (s *ScanApiStoreSuite) AfterTest(suiteName, testName string) {
	s.ctxCancel()
	s.mockCtrl.Finish()
}

func (s *ScanApiStoreSuite) TestAdd() {
	go s.scanApiStore.Start()

	url := utils.RandString(10)
	token := utils.RandString(10)
	integrationMrn := utils.RandString(10)
	s.scanApiStore.mondooClientBuilder = func(opts mondooclient.ClientOptions) mondooclient.Client {
		s.Equal(url, opts.ApiEndpoint)
		s.Equal(token, opts.Token)
		return s.mockMondooClient
	}

	s.scanApiStore.Add(url, token, integrationMrn)

	clients := s.scanApiStore.GetAll()
	s.Equal(1, len(clients))
	s.Equal(integrationMrn, clients[0].IntegrationMrn)
}

func (s *ScanApiStoreSuite) TestAdd_Idempotence() {
	go s.scanApiStore.Start()

	url := utils.RandString(10)
	token := utils.RandString(10)
	integrationMrn := utils.RandString(10)
	s.scanApiStore.mondooClientBuilder = func(opts mondooclient.ClientOptions) mondooclient.Client {
		s.Equal(url, opts.ApiEndpoint)
		s.Equal(token, opts.Token)
		return s.mockMondooClient
	}

	for i := 0; i < 100; i++ {
		s.scanApiStore.Add(url, token, integrationMrn)
	}

	clients := s.scanApiStore.GetAll()
	s.Equal(1, len(clients))
	s.Equal(integrationMrn, clients[0].IntegrationMrn)
}

func (s *ScanApiStoreSuite) TestDelete() {
	go s.scanApiStore.Start()

	url := utils.RandString(10)
	token := utils.RandString(10)
	integrationMrn := utils.RandString(10)
	s.scanApiStore.mondooClientBuilder = func(opts mondooclient.ClientOptions) mondooclient.Client {
		s.Equal(url, opts.ApiEndpoint)
		s.Equal(token, opts.Token)
		return s.mockMondooClient
	}

	s.scanApiStore.Add(url, token, integrationMrn)

	url2 := url + "1"
	integrationMrn2 := integrationMrn + "1"
	s.scanApiStore.mondooClientBuilder = func(opts mondooclient.ClientOptions) mondooclient.Client {
		s.Equal(url2, opts.ApiEndpoint)
		s.Equal(token, opts.Token)
		return s.mockMondooClient
	}
	s.scanApiStore.Add(url2, token, integrationMrn2)

	clients := s.scanApiStore.GetAll()
	s.Equal(2, len(clients))

	s.scanApiStore.Delete(url)

	clients = s.scanApiStore.GetAll()
	s.Equal(1, len(clients))
	s.Equal(integrationMrn2, clients[0].IntegrationMrn)
}

func TestScanApiStoreSuite(t *testing.T) {
	suite.Run(t, new(ScanApiStoreSuite))
}
