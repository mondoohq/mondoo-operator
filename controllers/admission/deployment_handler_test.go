// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	fakeMondoo "go.mondoo.com/mondoo-operator/pkg/utils/mondoo/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	testNamespace             = "mondoo-operator"
	testMondooAuditConfigName = "mondoo-client"
	testCredsSecretName       = "mondoo-client'" //nolint:gosec
	testClusterID             = "abcd-1234"
)

func init() {
	utilruntime.Must(mondoov1alpha2.AddToScheme(scheme.Scheme))
	utilruntime.Must(certmanagerv1.AddToScheme(scheme.Scheme))
}

func TestReconcile(t *testing.T) {
	containerImageResolver := fakeMondoo.NewNoOpContainerImageResolver()

	tests := []struct {
		name                  string
		mondooAuditConfigSpec mondoov1alpha2.MondooAuditConfigSpec
		existingObjects       func(mondoov1alpha2.MondooAuditConfig) []client.Object
		validate              func(*testing.T, client.Client)
	}{
		{
			name:                  "admission disabled",
			mondooAuditConfigSpec: testMondooAuditConfigSpec(false, false),
			validate: func(t *testing.T, kubeClient client.Client) {
				objects := defaultResourcesWhenEnabled()

				for _, obj := range objects {
					err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
					assert.True(t, errors.IsNotFound(err), "unexpectedly found admission resource when admission disabled: %s", client.ObjectKeyFromObject(obj))
				}
			},
		},
		{
			name:                  "admission enabled",
			mondooAuditConfigSpec: testMondooAuditConfigSpec(true, false),
			validate: func(t *testing.T, kubeClient client.Client) {
				list := &corev1.ServiceList{}
				assert.NoError(t, kubeClient.List(context.TODO(), list))
				objects := defaultResourcesWhenEnabled()
				for _, obj := range objects {
					err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
					assert.NoError(t, err, "error retrieving k8s resource that should exist: %s", client.ObjectKeyFromObject(obj))
				}
			},
		},
		{
			name: "admission enabled with mode enforcing",
			mondooAuditConfigSpec: func() mondoov1alpha2.MondooAuditConfigSpec {
				mac := testMondooAuditConfigSpec(true, false)
				mac.Admission.Mode = mondoov1alpha2.Enforcing
				return mac
			}(),
			existingObjects: func(m mondoov1alpha2.MondooAuditConfig) []client.Object {
				mac := &mondoov1alpha2.MondooAuditConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testMondooAuditConfigName,
						Namespace: testNamespace,
					},
					Spec: mondoov1alpha2.MondooAuditConfigSpec{
						Admission: mondoov1alpha2.Admission{
							Mode:     mondoov1alpha2.Enforcing,
							Replicas: ptr.To(int32(1)),
						},
					},
				}

				deployment := WebhookDeployment(testNamespace, "ghcr.io/mondoohq/mondoo-operator:latest", *mac, "", testClusterID)
				err := ctrl.SetControllerReference(mac, deployment, scheme.Scheme)
				if err != nil {
					panic("failed to set controller ref for sample object")
				}
				deployment.Status = appsv1.DeploymentStatus{
					AvailableReplicas: 1,
					ReadyReplicas:     1,
					Replicas:          1,
				}

				deploymentScanApi := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mondoo-client-scan-api",
						Namespace: testNamespace,
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Args: []string{
											"--enforcement-mode",
											string(mondoov1alpha2.Enforcing),
										},
									},
								},
							},
						},
					},
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 1,
						ReadyReplicas:     1,
						Replicas:          1,
					},
				}

				return []client.Object{deployment, deploymentScanApi}
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: webhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Admission Deployment to exist")

				assert.Equal(t, deployment.Spec.Replicas, ptr.To(int32(1)))
				assert.Contains(t, deployment.Spec.Template.Spec.Containers[0].Args, string(mondoov1alpha2.Enforcing), "expected Webhook mode to be set to 'enforcing'")

				vwcName, err := validatingWebhookName(&mondoov1alpha2.MondooAuditConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testMondooAuditConfigName,
						Namespace: testNamespace,
					},
				})
				require.NoError(t, err, "unexpected failure while generating Webhook name")

				vwc := &webhooksv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: vwcName,
					},
				}
				err = kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(vwc), vwc)
				require.NoError(t, err, "error retrieving k8s resource that should exist: %s", client.ObjectKeyFromObject(vwc))

				assert.Equalf(t, *vwc.Webhooks[0].FailurePolicy, webhooksv1.Fail, "expected Webhook failure policy to be set to 'Fail'")
			},
		},
		{
			name: "admission enabled with mode enforcing and set replicas",
			mondooAuditConfigSpec: func() mondoov1alpha2.MondooAuditConfigSpec {
				mac := testMondooAuditConfigSpec(true, false)
				mac.Admission.Mode = mondoov1alpha2.Enforcing
				mac.Admission.Replicas = ptr.To(int32(2))
				return mac
			}(),
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: webhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Admission Deployment to exist")

				assert.Equal(t, deployment.Spec.Replicas, ptr.To(int32(2)))
			},
		},
		{
			name: "admission enabled with cert-manager",
			mondooAuditConfigSpec: func() mondoov1alpha2.MondooAuditConfigSpec {
				mac := testMondooAuditConfigSpec(true, false)
				mac.Admission.CertificateProvisioning = mondoov1alpha2.CertificateProvisioning{
					Mode: mondoov1alpha2.CertManagerProvisioning,
				}
				return mac
			}(),
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
			name:                  "cleanup when admission change to disabled",
			mondooAuditConfigSpec: testMondooAuditConfigSpec(false, false),
			// existing objects from admission being previously enabled
			existingObjects: func(m mondoov1alpha2.MondooAuditConfig) []client.Object {
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
			},
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
			name: "pass admission mode down to Deployment",
			mondooAuditConfigSpec: func() mondoov1alpha2.MondooAuditConfigSpec {
				mac := testMondooAuditConfigSpec(true, false)
				mac.Admission.Mode = mondoov1alpha2.Permissive
				return mac
			}(),
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: webhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Admission Deployment to exist")

				assert.Equal(t, deployment.Spec.Replicas, ptr.To(int32(1)))
				assert.Contains(t, deployment.Spec.Template.Spec.Containers[0].Args, string(mondoov1alpha2.Permissive), "expected Webhook mode to be set to 'permissive'")
			},
		},
		{
			name: "pass admission mode down to Deployment and set replicas",
			mondooAuditConfigSpec: func() mondoov1alpha2.MondooAuditConfigSpec {
				mac := testMondooAuditConfigSpec(true, false)
				mac.Admission.Mode = mondoov1alpha2.Permissive
				mac.Admission.Replicas = ptr.To(int32(3))
				return mac
			}(),
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: webhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Admission Deployment to exist")

				assert.Equal(t, ptr.To(int32(3)), deployment.Spec.Replicas)
				assert.Contains(t, deployment.Spec.Template.Spec.Containers[0].Args, string(mondoov1alpha2.Permissive), "expected Webhook mode to be set to 'permissive'")
			},
		},
		{
			name:                  "pass ClusterID mode down to Deployment",
			mondooAuditConfigSpec: testMondooAuditConfigSpec(true, false),
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: webhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Webhook Deployment to exist")

				assert.Contains(t, deployment.Spec.Template.Spec.Containers[0].Args, testClusterID, "expected Webhook mode to be set to 'permissive'")
			},
		},
		{
			name:                  "pass Integration MRN down to Deployment",
			mondooAuditConfigSpec: testMondooAuditConfigSpec(true, true),
			existingObjects: func(m mondoov1alpha2.MondooAuditConfig) []client.Object {
				sa := mondooclient.ServiceAccountCredentials{Mrn: "test-mrn"}
				saData, err := json.Marshal(sa)
				if err != nil {
					panic(err)
				}

				return []client.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testCredsSecretName,
							Namespace: testNamespace,
						},
						Data: map[string][]byte{
							constants.MondooCredsSecretIntegrationMRNKey: []byte("exampleIntegrationMRN"),
							constants.MondooCredsSecretServiceAccountKey: saData,
						},
					},
				}
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: webhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Webhook Deployment to exist")

				assert.Contains(t, deployment.Spec.Template.Spec.Containers[0].Args, "exampleIntegrationMRN", "expected args to include integration MRN")
			},
		},
		{
			name: "update admission Deployment when mode changes",
			mondooAuditConfigSpec: func() mondoov1alpha2.MondooAuditConfigSpec {
				mac := testMondooAuditConfigSpec(true, false)
				mac.Admission.Mode = mondoov1alpha2.Permissive
				return mac
			}(),
			existingObjects: func(m mondoov1alpha2.MondooAuditConfig) []client.Object {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      webhookDeploymentName(testMondooAuditConfigName),
						Namespace: testNamespace,
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Args: []string{
											"--enforcement-mode",
											string(mondoov1alpha2.Enforcing),
										},
									},
								},
							},
						},
					},
				}

				return []client.Object{deployment}
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: webhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Webhook Deployment to exist")

				assert.Contains(t, deployment.Spec.Template.Spec.Containers[0].Args, string(mondoov1alpha2.Permissive), "expected Webhook mode to be updated to 'permissive'")
			},
		},
		{
			name:                  "update webhook Deployment when changed externally",
			mondooAuditConfigSpec: testMondooAuditConfigSpec(true, false),
			existingObjects: func(m mondoov1alpha2.MondooAuditConfig) []client.Object {
				deployment := WebhookDeployment(testNamespace, "wrong", m, "", testClusterID)
				return []client.Object{deployment}
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: webhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Webhook Deployment to exist")

				auditConfig := &mondoov1alpha2.MondooAuditConfig{}
				auditConfig.Name = testMondooAuditConfigName
				auditConfig.Namespace = testNamespace
				require.NoError(
					t, kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(auditConfig), auditConfig), "failed to retrieve mondoo audit config")

				img, err := containerImageResolver.MondooOperatorImage(context.Background(), "", "", false)
				require.NoErrorf(t, err, "failed to get mondoo operator image.")
				expectedDeployment := WebhookDeployment(testNamespace, img, *auditConfig, "", testClusterID)
				require.NoError(t, ctrl.SetControllerReference(auditConfig, expectedDeployment, kubeClient.Scheme()))
				assert.Truef(t, k8s.AreDeploymentsEqual(*deployment, *expectedDeployment), "deployment has not been updated")
			},
		},
		{
			name:                  "update webhook Service when changed externally",
			mondooAuditConfigSpec: testMondooAuditConfigSpec(true, false),
			existingObjects: func(m mondoov1alpha2.MondooAuditConfig) []client.Object {
				service := WebhookService(testNamespace, m)
				service.Spec.Type = corev1.ServiceTypeExternalName
				return []client.Object{service}
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				service := &corev1.Service{}
				serviceKey := types.NamespacedName{Name: webhookServiceName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), serviceKey, service)
				require.NoError(t, err, "expected Webhook Service to exist")

				auditConfig := &mondoov1alpha2.MondooAuditConfig{}
				auditConfig.Name = testMondooAuditConfigName
				auditConfig.Namespace = testNamespace
				require.NoError(
					t, kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(auditConfig), auditConfig), "failed to retrieve mondoo audit config")

				expectedService := WebhookService(testNamespace, *auditConfig)
				require.NoError(t, ctrl.SetControllerReference(auditConfig, expectedService, kubeClient.Scheme()))
				assert.Truef(t, k8s.AreServicesEqual(*service, *expectedService), "service has not been updated")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			auditConfig := &mondoov1alpha2.MondooAuditConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testMondooAuditConfigName,
					Namespace: testNamespace,
				},
				Spec: test.mondooAuditConfigSpec,
			}
			kubeSystemNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-system",
					UID:  types.UID(testClusterID),
				},
			}

			existingObj := []client.Object{auditConfig, kubeSystemNamespace}
			if test.existingObjects != nil {
				existingObj = append(existingObj, test.existingObjects(*auditConfig)...)
			}
			fakeClient := fake.NewClientBuilder().
				WithStatusSubresource(existingObj...).
				WithObjects(existingObj...).
				Build()

			webhooks := &DeploymentHandler{
				Mondoo:                 auditConfig,
				KubeClient:             fakeClient,
				TargetNamespace:        testNamespace,
				MondooOperatorConfig:   &mondoov1alpha2.MondooOperatorConfig{},
				ContainerImageResolver: containerImageResolver,
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
			Name:      webhookServiceName(testMondooAuditConfigName),
			Namespace: testNamespace,
		},
	}
	objects = append(objects, service)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookDeploymentName(testMondooAuditConfigName),
			Namespace: testNamespace,
		},
	}
	objects = append(objects, dep)

	vwcName, err := validatingWebhookName(&mondoov1alpha2.MondooAuditConfig{
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

func testMondooAuditConfigSpec(admissionEnabled, integrationEnabled bool) mondoov1alpha2.MondooAuditConfigSpec {
	return mondoov1alpha2.MondooAuditConfigSpec{
		Admission: mondoov1alpha2.Admission{
			Enable:   admissionEnabled,
			Replicas: ptr.To(int32(1)),
		},
		ConsoleIntegration: mondoov1alpha2.ConsoleIntegration{
			Enable: integrationEnabled,
		},
		MondooCredsSecretRef: corev1.LocalObjectReference{
			Name: testCredsSecretName,
		},
	}
}
