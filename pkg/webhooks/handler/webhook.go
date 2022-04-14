package webhookhandler

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	"go.mondoo.com/mondoo-operator/pkg/webhooks/utils"
)

// Have kubebuilder generate a ValidatingWebhookConfiguration under the path /validate-k8s-mondoo-com that watches Pod/Deployment creation/updates
//+kubebuilder:webhook:path=/validate-k8s-mondoo-com-core,mutating=false,failurePolicy=ignore,sideEffects=None,groups="";apps,resources=pods;deployments,verbs=create;update,versions=v1,name=core-policy.k8s.mondoo.com,admissionReviewVersions=v1

var handlerlog = logf.Log.WithName("webhook-validator")

type webhookValidator struct {
	client  client.Client
	decoder *admission.Decoder
	mode    mondoov1alpha1.WebhookMode
}

// NewWebhookValidator will initialize a CoreValidator with the provided k8s Client and
// set it to the provided mode. Returns error if mode is invalid.
func NewWebhookValidator(client client.Client, mode string) (admission.Handler, error) {
	webhookMode, err := utils.ModeStringToWebhookMode(mode)
	if err != nil {
		return nil, err
	}

	return &webhookValidator{
		client: client,
		mode:   webhookMode,
	}, nil
}

var _ admission.Handler = &webhookValidator{}

func (a *webhookValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	handlerlog.Info("Webhook triggered", "Details", req)

	// TODO: call into Mondoo Scan Service to scan the resource

	// Depending on the mode, we either just allow the resource through no matter the scan result
	// or allow/deny based on the scan result
	switch a.mode {
	case mondoov1alpha1.Permissive:
		return admission.Allowed("PASSED")
	case mondoov1alpha1.Enforcing:
		// FIXME: when we start calling the Scan Service, use the result of the scan
		// to decide whether to ALLOW/DENY the resource
		// For now, just allow
		return admission.Allowed("PASSED")
	default:
		err := fmt.Errorf("neither permissive nor enforcing modes defined")
		handlerlog.Error(err, "unexpected runtime environment, allowing the resource through")
		return admission.Allowed("PASSED")
	}
}

var _ admission.DecoderInjector = &webhookValidator{}

func (a *webhookValidator) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}
