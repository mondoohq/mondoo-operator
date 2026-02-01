// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package fakeserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"go.mondoo.com/mondoo-operator/pkg/client/common"
	"go.mondoo.com/mondoo-operator/pkg/client/scanapiclient"
)

func FakeServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/Scan/HealthCheck", func(w http.ResponseWriter, r *http.Request) {
		result := &common.HealthCheckResponse{
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

	mux.HandleFunc(scanapiclient.RunAdmissionReviewEndpoint, func(w http.ResponseWriter, r *http.Request) {
		result := &scanapiclient.ScanResult{
			Ok: true,
			WorstScore: &scanapiclient.Score{
				Type:  scanapiclient.ValidScanResult,
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

	mux.HandleFunc(scanapiclient.ScanKubernetesResourcesEndpoint, func(w http.ResponseWriter, r *http.Request) {
		result := &scanapiclient.ScanResult{
			Ok: true,
			WorstScore: &scanapiclient.Score{
				Type:  scanapiclient.ValidScanResult,
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
