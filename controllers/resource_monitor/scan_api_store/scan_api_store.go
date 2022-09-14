/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package scan_api_store

import (
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = log.Log.WithName("scan-api-store")

type ScanApiStore interface {
	Start()
	// Add adds a scan api url to the store. The operatorion is idempotent.
	Add(url, token, integrationMrn string)
	// Delete deletes a scan api url from the store. The operatorion is idempotent.
	Delete(url string)
	// GetAll returns all scan api urls.
	GetAll() []ClientConfiguration
}

type ClientConfiguration struct {
	Client         mondooclient.Client
	IntegrationMrn string
}

type requestType string

const (
	AddRequest    requestType = "add"
	DeleteRequest requestType = "delete"
)

type urlRequest struct {
	requestType    requestType
	url            string
	token          string
	integrationMrn string
}

type scanApiStore struct {
	// urlReqChan is used to add or delete a scan api url
	urlReqChan chan urlRequest

	// getChan is used to get all scan api urls
	getChan chan struct{}

	// outChan is used to return all scan api urls
	outChan             chan []ClientConfiguration
	scanClients         map[string]ClientConfiguration
	mondooClientBuilder func(mondooclient.ClientOptions) mondooclient.Client
}

func NewScanApiStore() ScanApiStore {
	return &scanApiStore{
		urlReqChan:          make(chan urlRequest),
		getChan:             make(chan struct{}),
		outChan:             make(chan []ClientConfiguration),
		scanClients:         make(map[string]ClientConfiguration),
		mondooClientBuilder: mondooclient.NewClient,
	}
}

func (s *scanApiStore) Start() {
	for {
		select {
		case req := <-s.urlReqChan:
			switch req.requestType {
			case AddRequest:
				s.scanClients[req.url] = ClientConfiguration{
					Client: s.mondooClientBuilder(
						mondooclient.ClientOptions{ApiEndpoint: req.url, Token: req.token}),
					IntegrationMrn: req.integrationMrn,
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

// Add adds a scan api url to the store. The operatorion is idempotent.
func (s *scanApiStore) Add(url, token, integrationMrn string) {
	s.urlReqChan <- urlRequest{
		requestType:    AddRequest,
		url:            url,
		token:          token,
		integrationMrn: integrationMrn,
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
