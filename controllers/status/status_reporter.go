/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package status

import (
	"context"
	"reflect"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var logger = ctrl.Log.WithName("status-reporter")

type StatusReporter struct {
	kubeClient          client.Client
	mondooClientBuilder func(mondooclient.ClientOptions) mondooclient.Client
	lastReportedStatus  mondooclient.ReportStatusRequest
}

func NewStatusReporter(kubeClient client.Client, mondooClientBuilder func(mondooclient.ClientOptions) mondooclient.Client) *StatusReporter {
	return &StatusReporter{
		kubeClient:          kubeClient,
		mondooClientBuilder: mondooClientBuilder,
	}
}

func (r *StatusReporter) Report(ctx context.Context, m v1alpha2.MondooAuditConfig) error {
	if !m.Spec.ConsoleIntegration.Enable {
		return nil // If ConsoleIntegration is not disabled, we cannot report status
	}

	nodes := v1.NodeList{}
	if err := r.kubeClient.List(ctx, &nodes); err != nil {
		return err
	}

	integrationMrn, serviceAccount, err := k8s.GetIntegrationSecretForAuditConfig(ctx, r.kubeClient, m)
	if err != nil {
		return err
	}

	operatorStatus := ReportStatusRequestFromAuditConfig(integrationMrn, m, nodes.Items)
	if reflect.DeepEqual(operatorStatus, r.lastReportedStatus) {
		return nil // If the status hasn't change, don't report
	}

	token, err := mondoo.GenerateTokenFromServiceAccount(*serviceAccount, logger)
	if err != nil {
		return err
	}

	mondooClient := r.mondooClientBuilder(mondooclient.ClientOptions{
		ApiEndpoint: serviceAccount.ApiEndpoint,
		Token:       token,
	})

	if err := mondooClient.IntegrationReportStatus(ctx, &operatorStatus); err != nil {
		return err
	}

	// Update the last reported status only if we reported successfully
	r.lastReportedStatus = operatorStatus
	return nil
}
