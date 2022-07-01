package status

import (
	"context"
	"reflect"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StatusReporter struct {
	client             client.Client
	lastReportedStatus *OperatorStatus
}

func (r *StatusReporter) Report(ctx context.Context, m v1alpha2.MondooAuditConfig) error {
	nodes := v1.NodeList{}
	if err := r.client.List(ctx, &nodes); err != nil {
		return err
	}

	operatorStatus := OperatorStatusFromAuditConfig(m, nodes.Items)
	if reflect.DeepEqual(operatorStatus, r.lastReportedStatus) {
		return nil // If the status hasn't change, don't report
	}

	// TODO: report status and if successful update lastReportedStatus
	r.lastReportedStatus = &operatorStatus
	return nil
}
