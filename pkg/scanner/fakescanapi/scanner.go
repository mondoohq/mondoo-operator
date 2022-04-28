package fakescanapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"go.mondoo.com/mondoo-operator/pkg/scanner"
)

func FakeServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(scanner.HealthCheckEndpoint, func(w http.ResponseWriter, r *http.Request) {
		result := &scanner.HealthCheckResponse{
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

	mux.HandleFunc(scanner.ScanKubernetesEndpoint, func(w http.ResponseWriter, r *http.Request) {
		result := &scanner.ScanResult{
			Ok: true,
			WorstScore: &scanner.Score{
				Type:  scanner.ValidScanResult,
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
