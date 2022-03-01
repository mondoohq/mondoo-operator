package corewebhook

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Have kubebuilder generate a ValidatingWebhookConfiguration under the path /validate-k8s-mondoo-com-core that watches Pod creation/updates
//+kubebuilder:webhook:path=/validate-k8s-mondoo-com-core,mutating=false,failurePolicy=ignore,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=core-policy.k8s.mondoo.com,admissionReviewVersions=v1

var corelog = logf.Log.WithName("core-validator")

type CoreValidator struct {
	Client  client.Client
	decoder *admission.Decoder
}

var _ admission.Handler = &CoreValidator{}

func (a *CoreValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	corelog.Info("Webhook triggered", "Details", req)

	return admission.Allowed("PASSED")
}

func (a *CoreValidator) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}
