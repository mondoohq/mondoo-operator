/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package scan_api_store

import "sigs.k8s.io/controller-runtime/pkg/log"

var logger = log.Log.WithName("scan-api-store")

type requestType string

const (
	AddRequest    requestType = "add"
	DeleteRequest requestType = "delete"
)

type urlRequest struct {
	requestType requestType
	url         string
}

type ScanApiStore interface {
	Start()
	Add(url string)
	Delete(url string)
	GetAll() []string
}

type scanApiStore struct {
	// urlReqChan is used to add or delete a scan api url
	urlReqChan chan urlRequest

	// getChan is used to get all scan api urls
	getChan chan struct{}

	// outChan is used to return all scan api urls
	outChan     chan []string
	scanApiUrls map[string]struct{}
}

func NewScanApiStore() ScanApiStore {
	return &scanApiStore{
		urlReqChan:  make(chan urlRequest),
		getChan:     make(chan struct{}),
		outChan:     make(chan []string),
		scanApiUrls: make(map[string]struct{}),
	}
}

func (s *scanApiStore) Start() {
	for {
		select {
		case req := <-s.urlReqChan:
			switch req.requestType {
			case AddRequest:
				s.scanApiUrls[req.url] = struct{}{}
			case DeleteRequest:
				delete(s.scanApiUrls, req.url)
			default:
				logger.Error(nil, "Unknown request type", "requestType", req.requestType)
			}
		case <-s.getChan:
			urls := make([]string, 0, len(s.scanApiUrls))
			for u := range s.scanApiUrls {
				urls = append(urls, u)
			}
			s.outChan <- urls
		}
	}
}

// Add adds a scan api url to the store. The operatorion is idempotent.
func (s *scanApiStore) Add(url string) {
	s.urlReqChan <- urlRequest{
		requestType: AddRequest,
		url:         url,
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
func (s *scanApiStore) GetAll() []string {
	s.getChan <- struct{}{}
	return <-s.outChan
}
