package webhookhandler

import (
	"bytes"
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/yaml"

	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	"go.mondoo.com/mondoo-operator/pkg/scanner"
	"go.mondoo.com/mondoo-operator/pkg/webhooks/utils"
)

// Have kubebuilder generate a ValidatingWebhookConfiguration under the path /validate-k8s-mondoo-com that watches Pod/Deployment creation/updates
//+kubebuilder:webhook:path=/validate-k8s-mondoo-com,mutating=false,failurePolicy=ignore,sideEffects=None,groups="";apps,resources=pods;deployments,verbs=create;update,versions=v1,name=policy.k8s.mondoo.com,admissionReviewVersions=v1

var handlerlog = logf.Log.WithName("webhook-validator")

type webhookValidator struct {
	client  client.Client
	decoder *admission.Decoder
	mode    mondoov1alpha1.WebhookMode
	scanner *scanner.Scanner
}

// NewWebhookValidator will initialize a CoreValidator with the provided k8s Client and
// set it to the provided mode. Returns error if mode is invalid.
func NewWebhookValidator(client client.Client, mode, scanURL, token string) (admission.Handler, error) {
	webhookMode, err := utils.ModeStringToWebhookMode(mode)
	if err != nil {
		return nil, err
	}

	return &webhookValidator{
		client: client,
		mode:   webhookMode,
		scanner: &scanner.Scanner{
			Endpoint: scanURL,
			Token:    token,
		},
	}, nil
}

var _ admission.Handler = &webhookValidator{}

func (a *webhookValidator) Handle(ctx context.Context, req admission.Request) (response admission.Response) {
	resource := fmt.Sprintf("%s/%s", req.Namespace, req.Name)
	handlerlog.Info("Webhook triggered", "kind", req.Kind.Kind, "resource", resource)

	// the default/safe response
	response = admission.Allowed("DEFAULT MONDOO PASSED")

	// Call into Mondoo Scan Service to scan the resource
	k8sObjectData, err := yaml.Marshal(req.Object)
	if err != nil {
		handlerlog.Error(err, "failed to marshal incoming request")
		return
	}

	k8sLabels, err := generateLabels(k8sObjectData)
	if err != nil {
		handlerlog.Error(err, "failed to extract labels from incoming request")
		return
	}
	k8sLabels["k8s.mondoo.com/author"] = req.UserInfo.Username
	k8sLabels["k8s.mondoo.com/operator"] = string(req.Operation)

	result, err := a.scanner.RunKubernetesManifest(ctx, &scanner.KubernetesManifestJob{
		Files: []*scanner.File{
			{
				Data: k8sObjectData,
			},
		},
		Labels: k8sLabels,
	})
	if err != nil {
		handlerlog.Error(err, "error returned from scan request")
		return
	}

	passed := false
	if result.WorstScore != nil && result.WorstScore.Type == scanner.ValidScanResult && result.WorstScore.Value == 100 {
		passed = true
	}

	handlerlog.Info("Scan result", "shouldAdmit", passed, "kind", req.Kind.Kind, "resource", resource)

	// Depending on the mode, we either just allow the resource through no matter the scan result
	// or allow/deny based on the scan result
	switch a.mode {
	case mondoov1alpha1.Permissive:
		response = admission.Allowed("PASSED MONDOO SCAN")
	case mondoov1alpha1.Enforcing:
		if passed {
			response = admission.Allowed("PASSED MONDOO SCAN")
		} else {
			response = admission.Denied("FAILED MONDOO SCAN")
		}
	default:
		err := fmt.Errorf("neither permissive nor enforcing modes defined")
		handlerlog.Error(err, "unexpected runtime environment, allowing the resource through")
	}
	return
}

var _ admission.DecoderInjector = &webhookValidator{}

func (a *webhookValidator) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}

type customObjectMeta struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func generateLabels(k8sObject []byte) (map[string]string, error) {

	r := bytes.NewReader(k8sObject)
	objMeta := &customObjectMeta{}

	yamlDecoder := yamlutil.NewYAMLOrJSONDecoder(r, 4096)

	if err := yamlDecoder.Decode(objMeta); err != nil {
		return nil, err
	}

	labels := map[string]string{
		"k8s.mondoo.com/namespace": objMeta.Namespace,
		"k8s.mondoo.com/uid":       string(objMeta.UID),
		"k8s.mondoo.com/name":      objMeta.Name,
		"k8s.mondoo.com/kind":      objMeta.Kind,
	}

	for _, or := range objMeta.GetOwnerReferences() {
		if or.Controller != nil && *or.Controller == true {
			labels["k8s.mondoo.com/owner-name"] = or.Name
			labels["k8s.mondoo.com/owner-kind"] = or.Kind
			labels["k8s.mondoo.com/owner-uid"] = string(or.UID)
		}
	}

	return labels, nil
}
