package mondooclient

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
	defaultHttpTimeout         = 30 * time.Second
	defaultIdleConnTimeout     = 30 * time.Second
	defaultKeepAlive           = 30 * time.Second
	defaultTLSHandshakeTimeout = 10 * time.Second
	maxIdleConnections         = 100
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

type Client interface {
	ExchangeRegistrationToken(context.Context, *ExchangeRegistrationTokenInput) (*ExchangeRegistrationTokenOutput, error)
}

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

type ClientOptions struct {
	ApiEndpoint string
	Token       string
}

type mondooClient struct {
	ApiEndpoint string
	Token       string
	httpclient  http.Client
}

func (s *mondooClient) request(ctx context.Context, url string, reqBodyBytes []byte) ([]byte, error) {
	client := s.httpclient

	header := make(http.Header)
	header.Set("Accept", "application/json")
	header.Set("Content-Type", "application/json")
	if s.Token != "" {
		header.Set("Authorization", fmt.Sprintf("Bearer %s", s.Token))
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

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read http response body: %s", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, respBody)
	}

	return respBody, nil
}

func (s *mondooClient) ExchangeRegistrationToken(ctx context.Context, in *ExchangeRegistrationTokenInput) (*ExchangeRegistrationTokenOutput, error) {
	url := s.ApiEndpoint + "/AgentManager/ExchangeRegistrationToken"

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &ExchangeRegistrationTokenOutput{
		ServiceAccount: string(respBodyBytes),
	}

	return out, nil
}

// ExchangeRegistrationTokenInput is used for converting a JWT to a Mondoo serivce account
type ExchangeRegistrationTokenInput struct {
	// JWT token, only available during creation
	Token string `protobuf:"bytes,1,opt,name=token,proto3" json:"token,omitempty"`
}

type ExchangeRegistrationTokenOutput struct {
	ServiceAccount string `json:"serviceAccount,omitempty"`
}

func NewClient(opts ClientOptions) Client {
	mClient := &mondooClient{
		ApiEndpoint: opts.ApiEndpoint,
		Token:       opts.Token,
	}
	return mClient
}
