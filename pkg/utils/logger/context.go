// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package logger

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WithMondooAuditConfig adds stable reconcile object fields to controller logs.
func WithMondooAuditConfig(log logr.Logger, auditConfig client.Object, scanType string) logr.Logger {
	values := []any{}
	if auditConfig != nil {
		key := client.ObjectKeyFromObject(auditConfig)
		values = append(values,
			"mondooAuditConfig", key.String(),
		)
	}
	if scanType != "" {
		values = append(values, "scanType", scanType)
	}
	if len(values) == 0 {
		return log
	}
	return log.WithValues(values...)
}

// WithExternalCluster adds the external cluster name to cluster-scoped scan logs.
func WithExternalCluster(log logr.Logger, clusterName string) logr.Logger {
	if clusterName == "" {
		return log
	}
	return log.WithValues("externalCluster", clusterName)
}
