// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package common

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultHttpTimeout         = 30 * time.Second
	defaultIdleConnTimeout     = 30 * time.Second
	defaultKeepAlive           = 30 * time.Second
	defaultTLSHandshakeTimeout = 10 * time.Second
	maxIdleConnections         = 100
)

func DefaultHttpClient(httpProxy *string, httpTimeout *time.Duration) (http.Client, error) {
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

	if httpProxy != nil {
		urlParsed, err := url.Parse(*httpProxy)
		if err != nil {
			return http.Client{}, err
		}
		tr.Proxy = http.ProxyURL(urlParsed)
	}
	timeout := defaultHttpTimeout
	if httpTimeout != nil {
		timeout = *httpTimeout
	}
	return http.Client{
		Transport: tr,
		Timeout:   timeout,
	}, nil
}

func Request(ctx context.Context, client http.Client, url, token string, reqBodyBytes []byte) ([]byte, error) {
	header := make(http.Header)
	header.Set("Accept", "application/json")
	header.Set("Content-Type", "application/json")
	if token != "" {
		header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read http response body: %s", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, respBody)
	}

	return respBody, nil
}
