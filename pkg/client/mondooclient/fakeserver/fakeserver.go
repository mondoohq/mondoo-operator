package fakeserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"go.mondoo.com/mondoo-operator/pkg/client/common"
)

func FakeServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(common.HealthCheckEndpoint, func(w http.ResponseWriter, r *http.Request) {
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
	return httptest.NewServer(mux)
}
