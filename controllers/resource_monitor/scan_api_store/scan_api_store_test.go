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
	"go.mondoo.com/mondoo-operator/pkg/client/scanapiclient"
	"go.mondoo.com/mondoo-operator/pkg/client/scanapiclient/mock"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
)

type ScanApiStoreSuite struct {
	suite.Suite
	ctx               context.Context
	ctxCancel         context.CancelFunc
	mockCtrl          *gomock.Controller
	mockScanApiClient *mock.MockScanApiClient
	scanApiStore      *scanApiStore
}

func (s *ScanApiStoreSuite) BeforeTest(suiteName, testName string) {
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.mockCtrl = gomock.NewController(s.T())
	s.mockScanApiClient = mock.NewMockScanApiClient(s.mockCtrl)
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
	s.scanApiStore.scanApiClientBuilder = func(opts scanapiclient.ScanApiClientOptions) (scanapiclient.ScanApiClient, error) {
		s.Equal(url, opts.ApiEndpoint)
		s.Equal(token, opts.Token)
		return s.mockScanApiClient, nil
	}

	s.scanApiStore.Add(&ScanApiStoreAddOpts{
		Url:            url,
		Token:          token,
		IntegrationMrn: integrationMrn,
	})

	clients := s.scanApiStore.GetAll()
	s.Equal(1, len(clients))
	s.Equal(integrationMrn, clients[0].IntegrationMrn)
}

func (s *ScanApiStoreSuite) TestAdd_Idempotence() {
	go s.scanApiStore.Start()

	url := utils.RandString(10)
	token := utils.RandString(10)
	integrationMrn := utils.RandString(10)
	s.scanApiStore.scanApiClientBuilder = func(opts scanapiclient.ScanApiClientOptions) (scanapiclient.ScanApiClient, error) {
		s.Equal(url, opts.ApiEndpoint)
		s.Equal(token, opts.Token)
		return s.mockScanApiClient, nil
	}

	for i := 0; i < 100; i++ {
		s.scanApiStore.Add(&ScanApiStoreAddOpts{
			Url:            url,
			Token:          token,
			IntegrationMrn: integrationMrn,
		})
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
	s.scanApiStore.scanApiClientBuilder = func(opts scanapiclient.ScanApiClientOptions) (scanapiclient.ScanApiClient, error) {
		s.Equal(url, opts.ApiEndpoint)
		s.Equal(token, opts.Token)
		return s.mockScanApiClient, nil
	}

	s.scanApiStore.Add(&ScanApiStoreAddOpts{
		Url:            url,
		Token:          token,
		IntegrationMrn: integrationMrn,
	})

	url2 := url + "2"
	integrationMrn2 := integrationMrn + "2"
	s.scanApiStore.scanApiClientBuilder = func(opts scanapiclient.ScanApiClientOptions) (scanapiclient.ScanApiClient, error) {
		s.Equal(url2, opts.ApiEndpoint)
		s.Equal(token, opts.Token)
		return s.mockScanApiClient, nil
	}
	s.scanApiStore.Add(&ScanApiStoreAddOpts{
		Url:            url2,
		Token:          token,
		IntegrationMrn: integrationMrn2,
	})

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
