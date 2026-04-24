// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

const (
	defaultGCOlderThan = 2 * time.Hour
	gcMultiplier       = 2
)

// ManagedByLabel returns the ManagedBy value for assets owned by this operator instance.
// The cluster UID uniquely identifies which operator instance manages the assets.
func ManagedByLabel(clusterUID string) string {
	if clusterUID == "" {
		return "mondoo-operator"
	}
	return "mondoo-operator-" + clusterUID
}

// GCOlderThan returns the duration threshold for garbage collection based on
// the scan schedule. It computes 2x the interval between consecutive cron runs
// so that assets are only GC'd after missing at least one full scan cycle.
// The MONDOO_GC_OLDER_THAN env var overrides the computed value.
func GCOlderThan(schedule string) time.Duration {
	if v := os.Getenv("MONDOO_GC_OLDER_THAN"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: invalid MONDOO_GC_OLDER_THAN=%q, falling back to schedule-based computation: %v\n", v, err)
		} else {
			return d
		}
	}
	return gcOlderThanFromSchedule(schedule)
}

// gcOlderThanFromSchedule parses a cron schedule and returns 2x the interval
// between consecutive runs. Falls back to defaultGCOlderThan if parsing fails.
func gcOlderThanFromSchedule(schedule string) time.Duration {
	if schedule == "" {
		return defaultGCOlderThan
	}

	sched, err := cron.ParseStandard(schedule)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: failed to parse cron schedule %q, using default %s: %v\n", schedule, defaultGCOlderThan, err)
		return defaultGCOlderThan
	}

	now := time.Now()
	next1 := sched.Next(now)
	next2 := sched.Next(next1)
	interval := next2.Sub(next1)

	return time.Duration(gcMultiplier) * interval
}

// GarbageCollectAssets builds a Mondoo API client from the operator's credentials and
// calls GarbageCollectAssets with the provided request. The server automatically
// scopes deletion to assets created by the calling service account.
func GarbageCollectAssets(
	ctx context.Context,
	kubeClient client.Client,
	mondoo *v1alpha2.MondooAuditConfig,
	operatorConfig *v1alpha2.MondooOperatorConfig,
	clientBuilder func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error),
	req *mondooclient.GarbageCollectAssetsRequest,
	logger logr.Logger,
) error {
	if clientBuilder == nil {
		logger.Info("MondooClientBuilder not configured, skipping garbage collection")
		return nil
	}

	credsSecret := &corev1.Secret{}
	credsSecretKey := client.ObjectKey{
		Namespace: mondoo.Namespace,
		Name:      mondoo.Spec.MondooCredsSecretRef.Name,
	}
	if err := kubeClient.Get(ctx, credsSecretKey, credsSecret); err != nil {
		return fmt.Errorf("failed to get credentials secret: %w", err)
	}

	saData, ok := credsSecret.Data[constants.MondooCredsSecretServiceAccountKey]
	if !ok {
		return fmt.Errorf("credentials secret missing key %q", constants.MondooCredsSecretServiceAccountKey)
	}

	sa, err := LoadServiceAccountFromFile(saData)
	if err != nil {
		return fmt.Errorf("failed to load service account: %w", err)
	}

	token, err := GenerateTokenFromServiceAccount(*sa, logger)
	if err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}

	// Use spaceId override from MondooAuditConfig if set, otherwise use scope from SA credentials.
	if spaceMrn := k8s.SpaceMrnForAuditConfig(*mondoo); spaceMrn != "" {
		req.ScopeMrn = spaceMrn
		if saSpaceMrn := sa.SpaceMrn; saSpaceMrn != "" && saSpaceMrn != spaceMrn {
			logger.V(1).Info("spaceId override targets a different space than the service account",
				"saSpaceMrn", saSpaceMrn, "targetSpaceMrn", spaceMrn)
		}
	} else {
		req.ScopeMrn = sa.ScopeMrn
		if req.ScopeMrn == "" {
			req.ScopeMrn = sa.SpaceMrn
		}
	}

	if req.ScopeMrn == "" {
		logger.Info("Skipping garbage collection: no scope MRN determinable from service account")
		return nil
	}

	logger.Info("Preparing GarbageCollectAssets request", "scopeMrn", req.ScopeMrn, "managedBy", req.ManagedBy)

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
		return fmt.Errorf("failed to create mondoo client: %w", err)
	}

	if err := mc.GarbageCollectAssets(ctx, req); err != nil {
		return fmt.Errorf("garbage collect assets API call failed: %w", err)
	}

	logger.Info("GarbageCollectAssets completed successfully")
	return nil
}
