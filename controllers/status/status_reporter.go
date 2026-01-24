// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package status

import (
	"context"
	"reflect"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/version"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var logger = ctrl.Log.WithName("status-reporter")

type StatusReporter struct {
	kubeClient          client.Client
	k8sVersion          *version.Info
	mondooClientBuilder func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error)
	lastReportedStatus  mondooclient.ReportStatusRequest
}

func NewStatusReporter(kubeClient client.Client, mondooClientBuilder func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error), k8sVersion *version.Info) *StatusReporter {
	return &StatusReporter{
		kubeClient:          kubeClient,
		k8sVersion:          k8sVersion,
		mondooClientBuilder: mondooClientBuilder,
	}
}

func (r *StatusReporter) Report(ctx context.Context, m v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) error {
	if !m.Spec.ConsoleIntegration.Enable {
		return nil // If ConsoleIntegration is not enabled, we cannot report status
	}

	nodes := v1.NodeList{}
	if err := r.kubeClient.List(ctx, &nodes); err != nil {
		return err
	}

	secret, err := k8s.GetIntegrationSecretForAuditConfig(ctx, r.kubeClient, m)
	if err != nil {
		return err
	}

	integrationMrn, err := k8s.GetIntegrationMrnFromSecret(*secret)
	if err != nil {
		return err
	}

	operatorStatus := ReportStatusRequestFromAuditConfig(integrationMrn, m, nodes.Items, r.k8sVersion, logger)
	if reflect.DeepEqual(operatorStatus, r.lastReportedStatus) {
		return nil // If the status hasn't change, don't report
	}

	serviceAccount, err := k8s.GetServiceAccountFromSecret(*secret)
	if err != nil {
		return err
	}

	token, err := mondoo.GenerateTokenFromServiceAccount(*serviceAccount, logger)
	if err != nil {
		return err
	}

	mondooClient, err := r.mondooClientBuilder(mondooclient.MondooClientOptions{
		ApiEndpoint: serviceAccount.ApiEndpoint,
		Token:       token,
		HttpProxy:   cfg.Spec.HttpProxy,
		NoProxy:     cfg.Spec.NoProxy,
	})
	if err != nil {
		return err
	}

	if err := mondooClient.IntegrationReportStatus(ctx, &operatorStatus); err != nil {
		return err
	}

	// Update the last reported status only if we reported successfully
	r.lastReportedStatus = operatorStatus
	return nil
}
