// Copyright Mondoo, Inc. 2026
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
	"strings"
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
	return DefaultHttpClientWithProxy(httpProxy, nil, nil, httpTimeout)
}

func DefaultHttpClientWithProxy(httpProxy *string, httpsProxy *string, noProxy *string, httpTimeout *time.Duration) (http.Client, error) {
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

	if httpProxy != nil || httpsProxy != nil {
		var httpProxyURL, httpsProxyURL *url.URL
		var err error
		if httpProxy != nil {
			httpProxyURL, err = url.Parse(*httpProxy)
			if err != nil {
				return http.Client{}, fmt.Errorf("failed to parse httpProxy: %w", err)
			}
		}
		if httpsProxy != nil {
			httpsProxyURL, err = url.Parse(*httpsProxy)
			if err != nil {
				return http.Client{}, fmt.Errorf("failed to parse httpsProxy: %w", err)
			}
		}
		// Create a proxy function that respects noProxy and uses the correct proxy per scheme
		tr.Proxy = func(req *http.Request) (*url.URL, error) {
			if noProxy != nil && shouldBypassProxy(req.URL.Host, *noProxy) {
				return nil, nil // No proxy for this host
			}
			if req.URL.Scheme == "https" && httpsProxyURL != nil {
				return httpsProxyURL, nil
			}
			if httpProxyURL != nil {
				return httpProxyURL, nil
			}
			return nil, nil
		}
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

// shouldBypassProxy checks if the given host should bypass the proxy based on noProxy settings.
// Matching is case-insensitive since DNS names are case-insensitive.
func shouldBypassProxy(host string, noProxy string) bool {
	if noProxy == "" {
		return false
	}

	// Remove port from host if present (handles IPv6 correctly)
	hostWithoutPort := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostWithoutPort = h
	}

	// Lowercase for case-insensitive comparison (DNS is case-insensitive)
	hostLower := strings.ToLower(hostWithoutPort)

	// Check each entry in the noProxy list
	for _, entry := range strings.Split(noProxy, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Handle wildcard "*" - bypass all
		if entry == "*" {
			return true
		}

		// Lowercase entry for case-insensitive comparison
		entryLower := strings.ToLower(entry)

		// Handle domain suffix matching (e.g., ".example.com" matches "api.mondoo.example.com")
		if strings.HasPrefix(entryLower, ".") {
			if strings.HasSuffix(hostLower, entryLower) || hostLower == entryLower[1:] {
				return true
			}
			continue
		}

		// Handle CIDR notation for IP ranges
		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err == nil {
				ip := net.ParseIP(hostWithoutPort)
				if ip != nil && cidr.Contains(ip) {
					return true
				}
			}
			continue
		}

		// Exact match or suffix match
		if hostLower == entryLower || strings.HasSuffix(hostLower, "."+entryLower) {
			return true
		}
	}

	return false
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
	resp, err := client.Do(req) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %v", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("failed to close response body: %s", err)
		}
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
