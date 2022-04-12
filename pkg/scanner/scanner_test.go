package scanner

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/yaml"
)

func testServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/Health/Check", func(w http.ResponseWriter, r *http.Request) {
		result := &HealthCheckResponse{
			Status: "SERVING",
		}
		data, err := json.Marshal(result)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Write(data)
	})

	mux.HandleFunc("/Scan/RunKubernetesManifest", func(w http.ResponseWriter, r *http.Request) {
		result := &ScanResult{
			Ok: true,
			WorstScore: &Score{
				Type:  uint32(2),
				Value: 100,
			},
		}
		data, err := json.Marshal(result)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Write(data)
	})
	return httptest.NewServer(mux)
}

func TestScanner(t *testing.T) {
	testserver := testServer()
	url := testserver.URL
	token := ""

	// To test with a real client, just set
	// url := "http://127.0.0.1:8990"
	// token := "<token here>"

	// do client request
	s := &Scanner{
		Endpoint: url,
		Token:    token,
	}

	// Run Health Check
	healthResp, err := s.HealthCheck(context.Background(), &HealthCheckRequest{})
	require.NoError(t, err)
	assert.True(t, healthResp.Status == "SERVING")

	// Run Manifest Scan
	data, err := ioutil.ReadFile("./testdata/webhook-payload.json")
	require.NoError(t, err)

	request := admission.Request{}
	err = yaml.Unmarshal(data, &request)
	require.NoError(t, err)

	k8sObjectData, err := yaml.Marshal(request.Object)
	require.NoError(t, err)

	result, err := s.RunKubernetesManifest(context.Background(), &KubernetesManifestJob{
		Files: []*File{
			{
				Data: k8sObjectData,
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	// check if the scan passed
	passed := result.WorstScore.Type == 2 && result.WorstScore.Value == 100
	assert.True(t, passed)
}
