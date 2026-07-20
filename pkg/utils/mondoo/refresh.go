// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"context"
	"fmt"
	"sort"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

func RefreshAssetScores(
	ctx context.Context,
	kubeClient client.Client,
	mondoo *v1alpha2.MondooAuditConfig,
	operatorConfig *v1alpha2.MondooOperatorConfig,
	clientBuilder func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error),
	clusterUID string,
	logger logr.Logger,
) ([]string, error) {
	if clientBuilder == nil {
		logger.Info("MondooClientBuilder not configured, skipping refresh")
		return nil, nil
	}

	credsSecret := &corev1.Secret{}
	credsSecretKey := client.ObjectKey{
		Namespace: mondoo.Namespace,
		Name:      mondoo.Spec.MondooCredsSecretRef.Name,
	}
	if err := kubeClient.Get(ctx, credsSecretKey, credsSecret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret: %w", err)
	}

	saData, ok := credsSecret.Data[constants.MondooCredsSecretServiceAccountKey]
	if !ok {
		return nil, fmt.Errorf("credentials secret missing key %q", constants.MondooCredsSecretServiceAccountKey)
	}

	sa, err := LoadServiceAccountFromFile(saData)
	if err != nil {
		return nil, fmt.Errorf("failed to load service account: %w", err)
	}

	token, err := GenerateTokenFromServiceAccount(*sa, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	req := &mondooclient.RefreshAssetScoresRequest{
		ManagedBy:       ManagedByLabel(clusterUID),
		PlatformRuntime: "docker-image",
	}

	if spaceMrn := k8s.SpaceMrnForAuditConfig(*mondoo); spaceMrn != "" {
		req.ScopeMrn = spaceMrn
	} else {
		req.ScopeMrn = sa.ScopeMrn
		if req.ScopeMrn == "" {
			req.ScopeMrn = sa.SpaceMrn
		}
	}

	if req.ScopeMrn == "" {
		logger.Info("Skipping refresh: no scope MRN determinable from service account")
		return nil, nil
	}

	logger.Info("Calling RefreshAssetScores", "scopeMrn", req.ScopeMrn, "managedBy", req.ManagedBy)

	opts := mondooclient.MondooClientOptions{
		ApiEndpoint: sa.ApiEndpoint,
		Token:       token,
	}
	if operatorConfig != nil {
		opts.HttpProxy = operatorConfig.Spec.HttpProxy
		opts.HttpsProxy = operatorConfig.Spec.HttpsProxy
		opts.NoProxy = operatorConfig.Spec.NoProxy
	}

	mc, err := clientBuilder(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create mondoo client: %w", err)
	}

	resp, err := mc.RefreshAssetScores(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("RefreshAssetScores API call failed: %w", err)
	}

	platformIds := extractPlatformIds(resp)
	logger.Info("RefreshAssetScores completed",
		"refreshed", len(resp.Refreshed), "missing", len(resp.Missing), "platformIdsToExclude", len(platformIds))
	return platformIds, nil
}

func extractPlatformIds(resp *mondooclient.RefreshAssetScoresResponse) []string {
	if resp == nil {
		return nil
	}

	seen := make(map[string]struct{})
	for _, r := range resp.Refreshed {
		for _, pid := range r.PlatformIds {
			seen[pid] = struct{}{}
		}
	}

	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
