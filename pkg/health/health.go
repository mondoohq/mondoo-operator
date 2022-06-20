package health

import (
	"context"
	"errors"
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

func (h *HealthChecks) CheckMondooAuditConfigsForOperatorVersion(req *http.Request) error {
	ctx := context.TODO()
	mondooAuditConfigs := &v1alpha2.MondooAuditConfigList{}
	if err := h.Client.List(ctx, mondooAuditConfigs); err != nil {
		h.Log.Error(err, "error listing MondooAuditConfigs")
		return err
	}

	for _, mac := range mondooAuditConfigs.Items {
		if mac.Status.ReconciledByOperatorVersion != version.Version {
			errorMsg := fmt.Sprintf("MondooAuditConfig not yet completly reconciled: namespace=%s MondooAuditConfig=%s", mac.Namespace, mac.Name)
			h.Log.Info(errorMsg)
			return errors.New(errorMsg)
		} else {
			infoMsg := fmt.Sprintf("MondooAuditconfig reconciled: namespace=%s MondooAuditConfig=%s", mac.Namespace, mac.Name)
			h.Log.Info(infoMsg)
		}
	}

	return nil
}
