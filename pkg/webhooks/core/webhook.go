package corewebhook

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
)

// Have kubebuilder generate a ValidatingWebhookConfiguration under the path /validate-k8s-mondoo-com-core that watches Pod creation/updates
//+kubebuilder:webhook:path=/validate-k8s-mondoo-com-core,mutating=false,failurePolicy=ignore,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=core-policy.k8s.mondoo.com,admissionReviewVersions=v1

var corelog = logf.Log.WithName("core-validator")

type CoreValidator struct {
	Client  client.Client
	decoder *admission.Decoder
	Mode    string
}

var _ admission.Handler = &CoreValidator{}

func (a *CoreValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	corelog.Info("Webhook triggered", "Details", req)

	// TODO: call into Mondoo Scan Service to scan the resource

	// Depending on the mode, we either just allow the resource through no matter the scan result
	// or allow/deny based on the scan result
	switch a.Mode {
	case string(mondoov1alpha1.Permissive):
		return admission.Allowed("PASSED")
	case string(mondoov1alpha1.Enforcing):
		// FIXME: when we start calling the Scan Service, use the result of the scan
		// to decide whether to ALLOW/DENY the resource
		// For now, just allow
		return admission.Allowed("PASSED")
	default:
		err := fmt.Errorf("neither permissive nor enforcing modes defined")
		corelog.Error(err, "unexpected runtime environment, allowing the resource through")
		return admission.Allowed("PASSED")
	}
}

func (a *CoreValidator) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}
