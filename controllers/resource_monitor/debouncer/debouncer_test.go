/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package debouncer

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/controllers/resource_monitor/scan_api_store"
	scanapistoremock "go.mondoo.com/mondoo-operator/controllers/resource_monitor/scan_api_store/mock"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient/mock"
)

type DebouncerSuite struct {
	suite.Suite
	ctx              context.Context
	ctxCancel        context.CancelFunc
	mockCtrl         *gomock.Controller
	mockMondooClient *mock.MockClient
	scanApiStore     *scanapistoremock.MockScanApiStore
	debouncer        *debouncer
}

func (s *DebouncerSuite) BeforeTest(suiteName, testName string) {
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.mockCtrl = gomock.NewController(s.T())
	s.mockMondooClient = mock.NewMockClient(s.mockCtrl)
	s.scanApiStore = scanapistoremock.NewMockScanApiStore(s.mockCtrl)
	s.debouncer = NewDebouncer(s.scanApiStore).(*debouncer)
	s.debouncer.flushTimeout = 1 * time.Second
}

func (s *DebouncerSuite) AfterTest(suiteName, testName string) {
	s.ctxCancel()
	s.mockCtrl.Finish()
}

func (s *DebouncerSuite) TestStart_IgnoreInitialResources() {
	go s.debouncer.Start(s.ctx, "")

	keys := []string{"pod:default:test", "deployment:test-ns:dep"}
	for _, k := range keys {
		for i := 0; i < 100; i++ {
			s.debouncer.Add(k)
		}
	}

	time.Sleep(s.debouncer.flushTimeout + 100*time.Millisecond)

	s.Empty(s.debouncer.resources)
}

func (s *DebouncerSuite) TestStart_Debounce() {
	s.debouncer.isFirstFlush = false
	go s.debouncer.Start(s.ctx, "")

	keys := []string{"pod:default:test", "deployment:test-ns:dep"}
	for _, k := range keys {
		for i := 0; i < 100; i++ {
			s.debouncer.Add(k)
		}
	}

	integrationMrn := "integration-mrn"
	s.scanApiStore.EXPECT().GetAll().Times(1).Return([]scan_api_store.ClientConfiguration{
		{Client: s.mockMondooClient, IntegrationMrn: integrationMrn},
	})

	// Verify we schedule a scan once per resource.
	for _, k := range keys {
		s.mockMondooClient.EXPECT().
			ScheduleKubernetesResourceScan(gomock.Any(), integrationMrn, k, "").
			Times(1).
			Return(nil, nil)
	}

	time.Sleep(s.debouncer.flushTimeout + 100*time.Millisecond)

	s.Empty(s.debouncer.resources)
}

func (s *DebouncerSuite) TestStart_DebounceManagedBy() {
	s.debouncer.isFirstFlush = false
	go s.debouncer.Start(s.ctx, "test")

	keys := []string{"pod:default:test", "deployment:test-ns:dep"}
	for _, k := range keys {
		for i := 0; i < 100; i++ {
			s.debouncer.Add(k)
		}
	}

	integrationMrn := "integration-mrn"
	s.scanApiStore.EXPECT().GetAll().Times(1).Return([]scan_api_store.ClientConfiguration{
		{Client: s.mockMondooClient, IntegrationMrn: integrationMrn},
	})

	// Verify we schedule a scan once per resource.
	for _, k := range keys {
		s.mockMondooClient.EXPECT().
			ScheduleKubernetesResourceScan(gomock.Any(), integrationMrn, k, "test").
			Times(1).
			Return(nil, nil)
	}

	time.Sleep(s.debouncer.flushTimeout + 100*time.Millisecond)

	s.Empty(s.debouncer.resources)
}

func (s *DebouncerSuite) TestStart_NoScanApiClients() {
	s.debouncer.isFirstFlush = false
	go s.debouncer.Start(s.ctx, "")

	keys := []string{"pod:default:test", "deployment:test-ns:dep"}
	for _, k := range keys {
		for i := 0; i < 100; i++ {
			s.debouncer.Add(k)
		}
	}

	s.scanApiStore.EXPECT().GetAll().Times(1).Return([]scan_api_store.ClientConfiguration{})

	time.Sleep(s.debouncer.flushTimeout + 100*time.Millisecond)

	// Verify the resources are flushed even when there are no scan APIs.
	s.Empty(s.debouncer.resources)
}

func (s *DebouncerSuite) TestStart_MultipleScanApiClients() {
	s.debouncer.isFirstFlush = false
	go s.debouncer.Start(s.ctx, "")

	keys := []string{"pod:default:test", "deployment:test-ns:dep"}
	for _, k := range keys {
		for i := 0; i < 100; i++ {
			s.debouncer.Add(k)
		}
	}

	mockMondooClient2 := mock.NewMockClient(s.mockCtrl)

	integrationMrn := "integration-mrn"
	s.scanApiStore.EXPECT().GetAll().Times(1).Return([]scan_api_store.ClientConfiguration{
		{Client: s.mockMondooClient, IntegrationMrn: integrationMrn},
		{Client: mockMondooClient2, IntegrationMrn: integrationMrn + "2"},
	})

	// Verify we schedule a scan once per resource per client.
	for _, k := range keys {
		s.mockMondooClient.EXPECT().
			ScheduleKubernetesResourceScan(gomock.Any(), integrationMrn, k, "").
			Times(1).
			Return(nil, nil)
		mockMondooClient2.EXPECT().
			ScheduleKubernetesResourceScan(gomock.Any(), integrationMrn+"2", k, "").
			Times(1).
			Return(nil, nil)
	}

	time.Sleep(s.debouncer.flushTimeout + 100*time.Millisecond)

	// Verify the resources are flushed even when there are no scan APIs.
	s.Empty(s.debouncer.resources)
}

func TestDebouncerSuite(t *testing.T) {
	suite.Run(t, new(DebouncerSuite))
}
