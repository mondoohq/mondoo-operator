// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
)

// IntegrationCheckInResult carries the pause-state change information returned by IntegrationCheckIn.
type IntegrationCheckInResult struct {
	// ConfigFetched is true when the server signalled a configuration mismatch and Configure was
	// successfully fetched. When false, Paused should be ignored.
	ConfigFetched bool
	// Paused is the desired scanning-paused value from the server, valid only when ConfigFetched is true.
	Paused bool
	// ConfigurationHash is the SHA-256 hex digest of the raw configuration JSON returned by the
	// server's Configure endpoint. The caller should persist this and pass it back on the next
	// CheckIn so the server can detect configuration changes.
	ConfigurationHash string
}

func IntegrationCheckIn(
	ctx context.Context,
	integrationMrn string,
	configurationHash string,
	sa mondooclient.ServiceAccountCredentials,
	mondooClientBuilder MondooClientBuilder,
	httpProxy *string,
	httpsProxy *string,
	noProxy *string,
	logger logr.Logger,
) (*IntegrationCheckInResult, error) {
	token, err := GenerateTokenFromServiceAccount(sa, logger)
	if err != nil {
		msg := "unable to generate token from service account"
		return nil, fmt.Errorf("%s: %s", msg, err)
	}
	mondooClient, err := mondooClientBuilder(mondooclient.MondooClientOptions{
		ApiEndpoint: sa.ApiEndpoint,
		Token:       token,
		HttpProxy:   httpProxy,
		HttpsProxy:  httpsProxy,
		NoProxy:     noProxy,
	})
	if err != nil {
		return nil, err
	}

	checkInResp, err := mondooClient.IntegrationCheckIn(ctx, &mondooclient.IntegrationCheckInInput{
		Mrn:               integrationMrn,
		ConfigurationHash: configurationHash,
	})
	if err != nil {
		msg := "failed to CheckIn() to Mondoo API"
		return nil, fmt.Errorf("%s: %s", msg, err)
	}

	logger.Info("CheckIn response", "configurationMatch", checkInResp.ConfigurationMatch, "sentHash", configurationHash)

	result := &IntegrationCheckInResult{
		ConfigurationHash: configurationHash,
	}

	// Fetch config when:
	// 1. The server signals a change (hash mismatch), OR
	// 2. We have no stored hash (first run after restart) — the server may treat
	//    an empty hash as matching, so we must fetch to establish initial state.
	if !checkInResp.ConfigurationMatch || configurationHash == "" {
		configResp, err := mondooClient.IntegrationConfigure(ctx, &mondooclient.IntegrationConfigureInput{
			Mrn: integrationMrn,
		})
		if err != nil {
			logger.Error(err, "failed to fetch configuration, will retry next cycle")
			return result, nil
		}

		if configResp.Details != nil && configResp.Details.Config != "" {
			logger.Info("Configure response", "rawConfig", configResp.Details.Config)
			var k8sCfg mondooclient.K8sIntegrationConfig
			if err := json.Unmarshal([]byte(configResp.Details.Config), &k8sCfg); err != nil {
				logger.Error(err, "failed to parse integration config JSON, will retry next cycle")
				return result, nil
			}
			result.ConfigFetched = true
			result.Paused = k8sCfg.PauseScanning
			h := sha256.Sum256([]byte(configResp.Details.Config))
			result.ConfigurationHash = fmt.Sprintf("%x", h[:])
			logger.Info("parsed integration config", "pauseScanning", k8sCfg.PauseScanning, "newHash", result.ConfigurationHash)
		} else {
			logger.Info("Configure response had no config details", "hasDetails", configResp.Details != nil)
		}
	}

	return result, nil
}
