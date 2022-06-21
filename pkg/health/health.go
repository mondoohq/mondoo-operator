package health

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type HealthChecks struct {
	Client client.Client
	Log    logr.Logger
}

func (h *HealthChecks) AreAllMondooAuditConfigsReconciled(req *http.Request) error {
	ctx := context.Background()
	mondooAuditConfigs := &v1alpha2.MondooAuditConfigList{}
	if err := h.Client.List(ctx, mondooAuditConfigs); err != nil {
		h.Log.Error(err, "error listing MondooAuditConfigs")
		return err
	}

	for _, mac := range mondooAuditConfigs.Items {
		if mac.Status.ReconciledByOperatorVersion != version.Version {
			errorMsg := fmt.Sprintf("MondooAuditConfig %s/%s not yet completly reconciled by operator. Current value of ReconciledByOperatorVersion: '%s', expected: '%s'", mac.Namespace, mac.Name, mac.Status.ReconciledByOperatorVersion, version.Version)
			h.Log.Info(errorMsg)
			return fmt.Errorf(errorMsg)
		}
	}

	return nil
}
