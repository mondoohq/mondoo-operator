// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
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

// DeleteStaleAssets builds a Mondoo API client from the operator's credentials and
// calls DeleteAssets with the provided request. This is shared between node scan
// and k8s resource scan garbage collection.
func DeleteStaleAssets(
	ctx context.Context,
	kubeClient client.Client,
	mondoo *v1alpha2.MondooAuditConfig,
	operatorConfig *v1alpha2.MondooOperatorConfig,
	clientBuilder func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error),
	req *mondooclient.DeleteAssetsRequest,
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

	// Set the SpaceMrn from the service account credentials.
	// Some SA credentials (e.g. terraform-created) omit space_mrn, so derive it from the SA MRN.
	// SA MRN format: //agents.api.mondoo.app/spaces/<id>/serviceaccounts/<id>
	req.SpaceMrn = sa.SpaceMrn
	if req.SpaceMrn == "" {
		req.SpaceMrn = SpaceMrnFromServiceAccountMrn(sa.Mrn)
	}
	logger.Info("Preparing DeleteAssets request", "spaceMrn", req.SpaceMrn, "managedBy", req.ManagedBy)

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

	resp, err := mc.DeleteAssets(ctx, req)
	if err != nil {
		return fmt.Errorf("delete assets API call failed: %w", err)
	}

	logger.Info("DeleteAssets response", "deletedCount", len(resp.AssetMrns), "errorCount", len(resp.Errors))
	if len(resp.Errors) > 0 {
		logger.Info("DeleteAssets completed with errors", "errors", resp.Errors)
	}
	if len(resp.AssetMrns) > 0 {
		logger.Info("Deleted stale assets", "mrns", resp.AssetMrns)
	}

	return nil
}

// SpaceMrnFromServiceAccountMrn extracts the space MRN from a service account MRN.
// SA MRN format: //agents.api.mondoo.app/spaces/<space-id>/serviceaccounts/<sa-id>
// Space MRN format: //captain.api.mondoo.app/spaces/<space-id>
func SpaceMrnFromServiceAccountMrn(saMrn string) string {
	const spacesSegment = "/spaces/"
	idx := strings.Index(saMrn, spacesSegment)
	if idx < 0 {
		return ""
	}
	after := saMrn[idx+len(spacesSegment):]
	spaceID, _, _ := strings.Cut(after, "/")
	if spaceID == "" {
		return ""
	}
	return "//captain.api.mondoo.app/spaces/" + spaceID
}
