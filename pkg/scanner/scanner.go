package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

const (
	healthCheckEndpoint        = "/Health/Check"
	scanKubernetesEndpoint     = "/Scan/RunKubernetesManifest"
	defaultHttpTimeout         = 30 * time.Second
	defaultIdleConnTimeout     = 30 * time.Second
	defaultKeepAlive           = 30 * time.Second
	defaultTLSHandshakeTimeout = 10 * time.Second
	maxIdleConnections         = 100
)

func DefaultHttpClient() *http.Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   defaultHttpTimeout,
			KeepAlive: defaultKeepAlive,
		}).DialContext,
		MaxIdleConns:          maxIdleConnections,
		IdleConnTimeout:       defaultIdleConnTimeout,
		TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Transport: tr,
		Timeout:   defaultHttpTimeout,
	}
	return httpClient
}

type Scanner struct {
	Endpoint   string
	Token      string
	httpclient http.Client
}

func (s *Scanner) request(ctx context.Context, url string, reqBodyBytes []byte) ([]byte, error) {
	client := s.httpclient

	header := make(http.Header)
	header.Set("Accept", "application/json")
	header.Set("Content-Type", "application/json")
	if s.Token != "" {
		header.Set("Authorization", "Bearer "+s.Token)
	}

	reader := bytes.NewReader(reqBodyBytes)
	req, err := http.NewRequest(http.MethodPost, url, reader)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.Header = header

	// do http call
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %v", err)
	}

	defer func() {
		resp.Body.Close()
	}()

	return ioutil.ReadAll(resp.Body)
}

func (s *Scanner) HealthCheck(ctx context.Context, in *HealthCheckRequest) (*HealthCheckResponse, error) {
	url := s.Endpoint + healthCheckEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &HealthCheckResponse{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proto response: %v", err)
	}

	return out, nil
}

func (s *Scanner) RunKubernetesManifest(ctx context.Context, in *KubernetesManifestJob) (*ScanResult, error) {
	url := s.Endpoint + scanKubernetesEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &ScanResult{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proto response: %v", err)
	}

	return out, nil
}

type KubernetesManifestJob struct {
	Files []*File `json:"files,omitempty"`
}

type File struct {
	Data []byte `json:"data,omitempty"`
}

type ScanResult struct {
	WorstScore *Score `json:"worstScore,omitempty"`
	Ok         bool   `json:"ok,omitempty"`
}

type Score struct {
	QrId            string `json:"qr_id,omitempty"`
	Type            uint32 `json:"type,omitempty"`
	Value           uint32 `json:"value,omitempty"`
	Weight          uint32 `json:"weight,omitempty"`
	ScoreCompletion uint32 `json:"score_completion,omitempty"`
	DataTotal       uint32 `json:"data_total,omitempty"`
	DataCompletion  uint32 `json:"data_completion,omitempty"`
	Message         string `json:"message,omitempty"`
}

type HealthCheckRequest struct{}

type HealthCheckResponse struct {
	Status string `json:"status,omitempty"`
	// returns rfc 3339 timestamp
	Time string `json:"time,omitempty"`
	// returns the major api version
	ApiVersion string `json:"apiVersion,omitempty"`
	// returns the git commit checksum
	Build string `json:"build,omitempty"`
}
