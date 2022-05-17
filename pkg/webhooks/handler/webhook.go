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
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	customobject "go.mondoo.com/mondoo-operator/pkg/utils/genericobjectdecoder"
	"go.mondoo.com/mondoo-operator/pkg/webhooks/utils"
)

// Have kubebuilder generate a ValidatingWebhookConfiguration under the path /validate-k8s-mondoo-com that watches Pod/Deployment creation/updates
//+kubebuilder:webhook:path=/validate-k8s-mondoo-com,mutating=false,failurePolicy=ignore,sideEffects=None,groups="";apps,resources=pods;deployments,verbs=create;update,versions=v1,name=policy.k8s.mondoo.com,admissionReviewVersions=v1

var handlerlog = logf.Log.WithName("webhook-validator")

const (
	// defaultScanPass is our default Allowed result in the event that we never even made it to the scan
	defaultScanPass = "DEFAULT MONDOO PASSED"
	// passedScan is the Allowed result when the sacn came back with a passing result
	passedScan = "PASSED MONDOO SCAN"
	// failedScanPermitted is the Allowed result when in Permissive mode but the scan result was a failing result
	failedScanPermitted = "PERMITTING FAILED SCAN"
	// failedScan is the Denied result when in Enforcing mode and the scan result was a failing result
	failedScan = "FAILED MONDOO SCAN"

	mondooLabelPrefix          = "k8s.mondoo.com/"
	mondooNamespaceLabel       = mondooLabelPrefix + "namespace"
	mondooUIDLabel             = mondooLabelPrefix + "uid"
	mondooResourceVersionLabel = mondooLabelPrefix + "resource-version"
	mondooNameLabel            = mondooLabelPrefix + "name"
	mondooKindLabel            = mondooLabelPrefix + "kind"
	mondooOwnerNameLabel       = mondooLabelPrefix + "owner-name"
	mondooOwnerKindLabel       = mondooLabelPrefix + "owner-kind"
	mondooOwnerUIDLabel        = mondooLabelPrefix + "owner-uid"
	mondooAuthorLabel          = mondooLabelPrefix + "author"
	mondooOperationLabel       = mondooLabelPrefix + "operation"
	mondooClusterIDLabel       = mondooLabelPrefix + "cluster-id"
)

type webhookValidator struct {
	client    client.Client
	decoder   *admission.Decoder
	mode      mondoov1alpha1.WebhookMode
	scanner   mondooclient.Client
	clusterID string
}

// NewWebhookValidator will initialize a CoreValidator with the provided k8s Client and
// set it to the provided mode. Returns error if mode is invalid.
func NewWebhookValidator(client client.Client, mode, scanURL, token, clusterID string) (admission.Handler, error) {
	webhookMode, err := utils.ModeStringToWebhookMode(mode)
	if err != nil {
		return nil, err
	}

	return &webhookValidator{
		client: client,
		mode:   webhookMode,
		scanner: mondooclient.NewClient(mondooclient.ClientOptions{
			ApiEndpoint: scanURL,
			Token:       token,
		}),
		clusterID: clusterID,
	}, nil
}

var _ admission.Handler = &webhookValidator{}

func (a *webhookValidator) Handle(ctx context.Context, req admission.Request) (response admission.Response) {
	resource := fmt.Sprintf("%s/%s", req.Namespace, req.Name)
	handlerlog.Info("Webhook triggered", "kind", req.Kind.Kind, "resource", resource)

	// the default/safe response
	response = admission.Allowed(defaultScanPass)

	// Call into Mondoo Scan Service to scan the resource
	k8sObjectData, err := yaml.Marshal(req.Object)
	if err != nil {
		handlerlog.Error(err, "failed to marshal incoming request")
		return
	}

	k8sLabels, err := generateLabelsFromAdmissionRequest(req)
	if err != nil {
		handlerlog.Error(err, "failed to extract labels from incoming request")
		return
	}
	k8sLabels[mondooClusterIDLabel] = a.clusterID

	result, err := a.scanner.RunKubernetesManifest(ctx, &mondooclient.KubernetesManifestJob{
		Files: []*mondooclient.File{
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
	if result.WorstScore != nil && result.WorstScore.Type == mondooclient.ValidScanResult && result.WorstScore.Value == 100 {
		passed = true
	}

	handlerlog.Info("Scan result", "shouldAdmit", passed, "kind", req.Kind.Kind, "resource", resource)

	// Depending on the mode, we either just allow the resource through no matter the scan result
	// or allow/deny based on the scan result
	switch a.mode {
	case mondoov1alpha1.Permissive:
		if passed {
			response = admission.Allowed(passedScan)
		} else {
			response = admission.Allowed(failedScanPermitted)
		}
	case mondoov1alpha1.Enforcing:
		if passed {
			response = admission.Allowed(passedScan)
		} else {
			response = admission.Denied(failedScan)
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

func generateLabelsFromAdmissionRequest(req admission.Request) (map[string]string, error) {

	k8sObjectData, err := yaml.Marshal(req.Object)
	if err != nil {
		handlerlog.Error(err, "failed to marshal incoming request")
		return nil, err
	}

	r := bytes.NewReader(k8sObjectData)
	objMeta := &customobject.CustomObjectMeta{}

	yamlDecoder := yamlutil.NewYAMLOrJSONDecoder(r, 4096)

	if err := yamlDecoder.Decode(objMeta); err != nil {
		return nil, err
	}

	labels := map[string]string{
		mondooNamespaceLabel:       objMeta.Namespace,
		mondooUIDLabel:             string(objMeta.UID),
		mondooResourceVersionLabel: objMeta.ResourceVersion,
		mondooNameLabel:            objMeta.Name,
		mondooKindLabel:            objMeta.Kind,
		mondooAuthorLabel:          req.UserInfo.Username,
		mondooOperationLabel:       string(req.Operation),
	}

	controllerRef := metav1.GetControllerOf(objMeta)
	if controllerRef != nil {
		labels[mondooOwnerNameLabel] = controllerRef.Name
		labels[mondooOwnerKindLabel] = controllerRef.Kind
		labels[mondooOwnerUIDLabel] = string(controllerRef.UID)
	}

	return labels, nil
}
