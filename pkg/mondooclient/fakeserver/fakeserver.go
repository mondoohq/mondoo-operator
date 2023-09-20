// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package fakeserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
)

func FakeServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(mondooclient.HealthCheckEndpoint, func(w http.ResponseWriter, r *http.Request) {
		result := &mondooclient.HealthCheckResponse{
			Status: "SERVING",
		}
		data, err := json.Marshal(result)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err = w.Write(data); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	})

	mux.HandleFunc(mondooclient.RunAdmissionReviewEndpoint, func(w http.ResponseWriter, r *http.Request) {
		result := &mondooclient.ScanResult{
			Ok: true,
			WorstScore: &mondooclient.Score{
				Type:  mondooclient.ValidScanResult,
				Value: 100,
			},
		}
		data, err := json.Marshal(result)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err = w.Write(data); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	})

	mux.HandleFunc(mondooclient.ScanKubernetesResourcesEndpoint, func(w http.ResponseWriter, r *http.Request) {
		result := &mondooclient.ScanResult{
			Ok: true,
			WorstScore: &mondooclient.Score{
				Type:  mondooclient.ValidScanResult,
				Value: 100,
			},
		}
		data, err := json.Marshal(result)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err = w.Write(data); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	})
	return httptest.NewServer(mux)
}
