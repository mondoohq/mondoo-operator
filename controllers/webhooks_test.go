package controllers

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	webhooksv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	scheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha1"
	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

const (
	testNamespace             = "mondoo-operator"
	testMondooAuditConfigName = "mondoo-client"
)

// A fake implementation of the getImage function that does not query remote container registries.
var fakeGetRemoteImageFunc = func(ref name.Reference, options ...remote.Option) (*remote.Descriptor, error) {
	h := sha1.New()
	h.Write([]byte(ref.Identifier()))
	hash, _ := v1.NewHash(hex.EncodeToString(h.Sum(nil))) // should never fail

	return &remote.Descriptor{
		Descriptor: v1.Descriptor{
			Digest: hash,
		},
	}, nil
}

func init() {
	utilruntime.Must(mondoov1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(certmanagerv1.AddToScheme(scheme.Scheme))

}

func TestWebhooksReconcile(t *testing.T) {
	tests := []struct {
		name                  string
		mondooAuditConfigSpec mondoov1alpha1.MondooAuditConfigData
		existingObjects       []client.Object
		validate              func(*testing.T, client.Client)
	}{
		{
			name: "webhooks disabled",
			mondooAuditConfigSpec: mondoov1alpha1.MondooAuditConfigData{
				Webhooks: mondoov1alpha1.Webhooks{
					Enable: false,
				},
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				objects := defaultResourcesWhenEnabled()

				for _, obj := range objects {
					err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
					assert.True(t, errors.IsNotFound(err), "unexpectedly found webhook resource when webhooks disabled: %s", client.ObjectKeyFromObject(obj))
				}
			},
		},
		{
			name: "webhooks enabled",
			mondooAuditConfigSpec: mondoov1alpha1.MondooAuditConfigData{
				Webhooks: mondoov1alpha1.Webhooks{
					Enable: true,
				},
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				objects := defaultResourcesWhenEnabled()
				for _, obj := range objects {
					err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
					assert.NoError(t, err, "error retrieving k8s resource that should exist: %s", client.ObjectKeyFromObject(obj))
				}
			},
		},
		{
			name: "webhooks enabled with cert-manager",
			mondooAuditConfigSpec: mondoov1alpha1.MondooAuditConfigData{
				Webhooks: mondoov1alpha1.Webhooks{
					Enable: true,
					CertificateConfig: mondoov1alpha1.WebhookCertificateConfig{
						InjectionStyle: string(mondoov1alpha1.CertManager),
					},
				},
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				objects := defaultResourcesWhenEnabled()
				for _, obj := range objects {
					err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
					assert.NoError(t, err, "error retrieving k8s resource that should exist: %s", client.ObjectKeyFromObject(obj))
				}

				// Check for cert-manager-specific Issuer and Certificate
				issuerList := &certmanagerv1.IssuerList{}
				err := kubeClient.List(context.TODO(), issuerList, &client.ListOptions{
					Namespace: testNamespace,
					Raw: &metav1.ListOptions{
						FieldSelector: fmt.Sprintf("metadata.name=%s", certManagerIssuerName),
					},
				})
				assert.NoError(t, err, "error listing cert-manager Issuer resources")
				assert.Equal(t, 1, len(issuerList.Items), "expected only one Issuer to be returned")

				cert := &certmanagerv1.Certificate{}
				certKey := types.NamespacedName{Name: certManagerCertificateName, Namespace: testNamespace}
				err = kubeClient.Get(context.TODO(), certKey, cert)
				assert.NoError(t, err, "error retrieving cert-manager Certificate that should exist")

			},
		},
		{
			name: "cleanup when webhooks change to disabled",
			mondooAuditConfigSpec: mondoov1alpha1.MondooAuditConfigData{
				Webhooks: mondoov1alpha1.Webhooks{
					Enable: false,
				},
			},
			// existing objects from webhooks being previously enabled
			existingObjects: func() []client.Object {
				objects := defaultResourcesWhenEnabled()

				issuer := &certmanagerv1.Issuer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      certManagerIssuerName,
						Namespace: testNamespace,
					},
				}
				objects = append(objects, issuer)

				cert := &certmanagerv1.Certificate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      certManagerCertificateName,
						Namespace: testNamespace,
					},
				}
				objects = append(objects, cert)

				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      GetTLSCertificatesSecretName(testMondooAuditConfigName),
						Namespace: testNamespace,
					},
				}
				objects = append(objects, secret)

				return objects
			}(),
			validate: func(t *testing.T, kubeClient client.Client) {
				objects := defaultResourcesWhenEnabled()
				for _, obj := range objects {
					err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
					assert.True(t, errors.IsNotFound(err), "expected IsNotFound for resource that should not exist: %s", client.ObjectKeyFromObject(obj))
				}

				// Check for cert-manager-specific Issuer and Certificate
				issuerList := &certmanagerv1.IssuerList{}
				err := kubeClient.List(context.TODO(), issuerList, &client.ListOptions{
					Namespace: testNamespace,
					Raw: &metav1.ListOptions{
						FieldSelector: fmt.Sprintf("metadata.name=%s", certManagerIssuerName),
					},
				})
				assert.NoError(t, err, "error listing cert-manager Issuer resources")
				assert.Equal(t, 0, len(issuerList.Items), "expected zero Issuer resources to be returned when webhooks disabled")

				cert := &certmanagerv1.Certificate{}
				certKey := types.NamespacedName{Name: certManagerCertificateName, Namespace: testNamespace}
				err = kubeClient.Get(context.TODO(), certKey, cert)
				assert.True(t, errors.IsNotFound(err), "expected cert-manager Certificate resource to not exist when webhooks disabled")

				// The Secret generated by cert-manager is left behind as it wasn't created by us
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      GetTLSCertificatesSecretName(testMondooAuditConfigName),
						Namespace: testNamespace,
					},
				}
				err = kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(secret), secret)
				assert.NoError(t, err, "expected cert-manager-generated Secret to still exist")
			},
		},
		{
			name: "pass webhook mode down to Deployment",
			mondooAuditConfigSpec: mondoov1alpha1.MondooAuditConfigData{
				Webhooks: mondoov1alpha1.Webhooks{
					Enable: true,
					Mode:   string(mondoov1alpha1.Permissive),
				},
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: getWebhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Webhook Deployment to exist")

				// Find and check the value of the webhook mode env var
				found := false
				for _, env := range deployment.Spec.Template.Spec.Containers[0].Env {
					if env.Name == mondoov1alpha1.WebhookModeEnvVar {
						found = true
						assert.Equal(t, string(mondoov1alpha1.Permissive), env.Value, "expected Webhook mode to be set to 'permissive'")
					}
				}
				assert.True(t, found, "did not find Webhook Mode environment variable to be defined/set")
			},
		},
		{
			name: "update webhook Deployment when mode changes",
			mondooAuditConfigSpec: mondoov1alpha1.MondooAuditConfigData{
				Webhooks: mondoov1alpha1.Webhooks{
					Enable: true,
					Mode:   string(mondoov1alpha1.Permissive),
				},
			},
			existingObjects: func() []client.Object {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      getWebhookDeploymentName(testMondooAuditConfigName),
						Namespace: testNamespace,
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Env: []corev1.EnvVar{
											{
												Name:  mondoov1alpha1.WebhookModeEnvVar,
												Value: string(mondoov1alpha1.Enforcing),
											},
										},
									},
								},
							},
						},
					},
				}

				return []client.Object{
					deployment,
				}
			}(),
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: getWebhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Webhook Deployment to exist")

				// Find and check the value of the webhook mode env var
				found := false
				for _, env := range deployment.Spec.Template.Spec.Containers[0].Env {
					if env.Name == mondoov1alpha1.WebhookModeEnvVar {
						found = true
						assert.Equal(t, string(mondoov1alpha1.Permissive), env.Value, "expected Webhook mode to be updated to 'permissive'")
					}
				}
				assert.True(t, found, "did not find Webhook Mode environment variable to be defined/set")
			},
		},
		{
			name: "cleanup old-style webhook",
			mondooAuditConfigSpec: mondoov1alpha1.MondooAuditConfigData{
				Webhooks: mondoov1alpha1.Webhooks{
					Enable: true,
				},
			},
			// existing objects from webhooks being previously enabled
			existingObjects: func() []client.Object {
				objects := defaultResourcesWhenEnabled()

				vwc := &webhooksv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: testMondooAuditConfigName + "-mondoo-webhook",
					},
				}
				objects = append(objects, vwc)

				return objects
			}(),
			validate: func(t *testing.T, kubeClient client.Client) {
				vwc := &webhooksv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: testMondooAuditConfigName + "-mondoo-webhook",
					},
				}
				err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(vwc), vwc)
				assert.True(t, errors.IsNotFound(err), "expected old-style named webhook %s to not be orphaned", client.ObjectKeyFromObject(vwc))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// Mock the retrieval of the actual image from the remote registry
			mondoo.GetRemoteImage = fakeGetRemoteImageFunc

			fakeClient := fake.NewClientBuilder().WithObjects(test.existingObjects...).Build()

			auditConfig := &mondoov1alpha1.MondooAuditConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testMondooAuditConfigName,
					Namespace: testNamespace,
				},
				Spec: test.mondooAuditConfigSpec,
			}
			webhooks := &Webhooks{
				Mondoo:               auditConfig,
				KubeClient:           fakeClient,
				TargetNamespace:      testNamespace,
				Scheme:               scheme.Scheme,
				MondooOperatorConfig: &v1alpha1.MondooOperatorConfig{},
			}

			// Act
			result, err := webhooks.Reconcile(context.TODO())

			// Assert
			require.NoError(t, err)
			assert.NotNil(t, result)

			test.validate(t, fakeClient)
		})
	}
}

func defaultResourcesWhenEnabled() []client.Object {
	objects := []client.Object{}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getWebhookServiceName(testMondooAuditConfigName),
			Namespace: testNamespace,
		},
	}
	objects = append(objects, service)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getWebhookDeploymentName(testMondooAuditConfigName),
			Namespace: testNamespace,
		},
	}
	objects = append(objects, dep)

	vwcName, err := getValidatingWebhookName(&mondoov1alpha1.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testMondooAuditConfigName,
			Namespace: testNamespace,
		},
	})
	// Should never happen...
	if err != nil {
		panic(fmt.Errorf("unexpected failure while generating Webhook name: %s", err))
	}

	vwc := &webhooksv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: vwcName,
		},
	}
	objects = append(objects, vwc)

	return objects
}
