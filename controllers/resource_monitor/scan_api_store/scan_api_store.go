// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package scan_api_store

import (
	"context"

	"go.mondoo.com/mondoo-operator/pkg/client/scanapiclient"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = log.Log.WithName("scan-api-store")

//go:generate ./../../../bin/mockgen -source=./scan_api_store.go -destination=./mock/scan_api_store_generated.go -package=mock

type ScanApiStore interface {
	Start()
	// Add adds a scan api url to the store. The operatorion is idempotent.
	Add(opts *ScanApiStoreAddOpts)
	// Delete deletes a scan api url from the store. The operatorion is idempotent.
	Delete(url string)
	// GetAll returns all scan api urls.
	GetAll() []ClientConfiguration
}

type ClientConfiguration struct {
	Client            scanapiclient.ScanApiClient
	IntegrationMrn    string
	IncludeNamespaces []string
	ExcludeNamespaces []string
}

type requestType string

const (
	AddRequest    requestType = "add"
	DeleteRequest requestType = "delete"
)

type urlRequest struct {
	requestType       requestType
	url               string
	token             string
	integrationMrn    string
	includeNamespaces []string
	excludeNamespaces []string
}

type scanApiStore struct {
	ctx context.Context

	// urlReqChan is used to add or delete a scan api url
	urlReqChan chan urlRequest

	// getChan is used to get all scan api urls
	getChan chan struct{}

	// outChan is used to return all scan api urls
	outChan              chan []ClientConfiguration
	scanClients          map[string]ClientConfiguration
	scanApiClientBuilder func(scanapiclient.ScanApiClientOptions) (scanapiclient.ScanApiClient, error)
}

func NewScanApiStore(ctx context.Context) ScanApiStore {
	return &scanApiStore{
		ctx:                  ctx,
		urlReqChan:           make(chan urlRequest),
		getChan:              make(chan struct{}),
		outChan:              make(chan []ClientConfiguration),
		scanClients:          make(map[string]ClientConfiguration),
		scanApiClientBuilder: scanapiclient.NewClient,
	}
}

func (s *scanApiStore) Start() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case req := <-s.urlReqChan:
			switch req.requestType {
			case AddRequest:
				client, err := s.scanApiClientBuilder(
					scanapiclient.ScanApiClientOptions{ApiEndpoint: req.url, Token: req.token})
				if err != nil {
					logger.Error(err, "Failed to create scan api client", "url", req.url)
					continue
				}
				s.scanClients[req.url] = ClientConfiguration{
					Client:            client,
					IntegrationMrn:    req.integrationMrn,
					IncludeNamespaces: req.includeNamespaces,
					ExcludeNamespaces: req.excludeNamespaces,
				}
			case DeleteRequest:
				delete(s.scanClients, req.url)
			default:
				logger.Error(nil, "Unknown request type", "requestType", req.requestType)
			}
		case <-s.getChan:
			clients := make([]ClientConfiguration, 0, len(s.scanClients))
			for _, c := range s.scanClients {
				clients = append(clients, c)
			}
			s.outChan <- clients
		}
	}
}

type ScanApiStoreAddOpts struct {
	Url               string
	Token             string
	IntegrationMrn    string
	IncludeNamespaces []string
	ExcludeNamespaces []string
}

// Add adds a scan api url to the store. The operatorion is idempotent.
func (s *scanApiStore) Add(opts *ScanApiStoreAddOpts) {
	s.urlReqChan <- urlRequest{
		requestType:       AddRequest,
		url:               opts.Url,
		token:             opts.Token,
		integrationMrn:    opts.IntegrationMrn,
		includeNamespaces: opts.IncludeNamespaces,
		excludeNamespaces: opts.ExcludeNamespaces,
	}
}

// Delete deletes a scan api url from the store. The operatorion is idempotent.
func (s *scanApiStore) Delete(url string) {
	s.urlReqChan <- urlRequest{
		requestType: DeleteRequest,
		url:         url,
	}
}

// GetAll returns all scan api urls.
func (s *scanApiStore) GetAll() []ClientConfiguration {
	s.getChan <- struct{}{}
	return <-s.outChan
}
