package webhooks

import (
	"context"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	fakeMondoo "go.mondoo.com/mondoo-operator/pkg/utils/mondoo/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	testNamespace             = "mondoo-operator"
	testMondooAuditConfigName = "mondoo-client"
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
		mondooAuditConfigSpec mondoov1alpha2.MondooAuditConfigData
		existingObjects       func(mondoov1alpha2.MondooAuditConfig) []client.Object
		validate              func(*testing.T, client.Client)
	}{
		{
			name: "admission disabled",
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				Admission: mondoov1alpha2.Admission{Enable: false},
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				objects := defaultResourcesWhenEnabled()

				for _, obj := range objects {
					err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
					assert.True(t, errors.IsNotFound(err), "unexpectedly found admission resource when admission disabled: %s", client.ObjectKeyFromObject(obj))
				}
			},
		},
		{
			name: "admission enabled",
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				Admission: mondoov1alpha2.Admission{Enable: true},
			},
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
			name: "admission enabled with cert-manager",
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				CertificateProvisioning: mondoov1alpha2.CertificateProvisioning{
					Mode: mondoov1alpha2.CertManagerProvisioning,
				},
				Admission: mondoov1alpha2.Admission{Enable: true},
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
			name: "cleanup when admission change to disabled",
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				Admission: mondoov1alpha2.Admission{Enable: false},
			},
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
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				Admission: mondoov1alpha2.Admission{
					Enable: true,
					Mode:   mondoov1alpha2.Permissive,
				},
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: webhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Admission Deployment to exist")

				assert.Contains(t, deployment.Spec.Template.Spec.Containers[0].Args, string(mondoov1alpha2.Permissive), "expected Webhook mode to be set to 'permissive'")
			},
		},
		{
			name: "pass ClusterID mode down to Deployment",
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				Admission: mondoov1alpha2.Admission{
					Enable: true,
					Mode:   mondoov1alpha2.Permissive,
				},
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: webhookDeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected Webhook Deployment to exist")

				assert.Contains(t, deployment.Spec.Template.Spec.Containers[0].Args, testClusterID, "expected Webhook mode to be set to 'permissive'")
			},
		},
		{
			name: "update admission Deployment when mode changes",
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				Admission: mondoov1alpha2.Admission{
					Enable: true,
					Mode:   mondoov1alpha2.Permissive,
				},
			},
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
			name: "update webhook Deployment when changed externally",
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				Admission: mondoov1alpha2.Admission{Enable: true},
			},
			existingObjects: func(m mondoov1alpha2.MondooAuditConfig) []client.Object {
				deployment := WebhookDeployment(testNamespace, "wrong", "test", m, testClusterID)
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

				img, err := containerImageResolver.MondooOperatorImage("", "", false)
				require.NoErrorf(t, err, "failed to get mondoo operator image.")
				expectedDeployment := WebhookDeployment(testNamespace, img, mondoov1alpha2.Permissive, *auditConfig, testClusterID)
				require.NoError(t, ctrl.SetControllerReference(auditConfig, expectedDeployment, kubeClient.Scheme()))
				assert.Truef(t, k8s.AreDeploymentsEqual(*deployment, *expectedDeployment), "deployment has not been updated")
			},
		},
		{
			name: "update webhook Service when changed externally",
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				Admission: mondoov1alpha2.Admission{Enable: true},
			},
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
		{
			name: "update scan API Deployment when changed externally",
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				Admission: mondoov1alpha2.Admission{Enable: true},
			},
			existingObjects: func(m mondoov1alpha2.MondooAuditConfig) []client.Object {
				deployment := scanapi.ScanApiDeployment(testNamespace, "wrong", m)
				return []client.Object{deployment}
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: scanapi.DeploymentName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), deploymentKey, deployment)
				require.NoError(t, err, "expected scan API Deployment to exist")

				auditConfig := &mondoov1alpha2.MondooAuditConfig{}
				auditConfig.Name = testMondooAuditConfigName
				auditConfig.Namespace = testNamespace
				require.NoError(
					t, kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(auditConfig), auditConfig), "failed to retrieve mondoo audit config")

				img, err := containerImageResolver.MondooClientImage("", "", false)
				require.NoErrorf(t, err, "failed to get mondoo operator image.")
				expectedDeployment := scanapi.ScanApiDeployment(testNamespace, img, *auditConfig)
				require.NoError(t, ctrl.SetControllerReference(auditConfig, expectedDeployment, kubeClient.Scheme()))
				assert.Truef(t, k8s.AreDeploymentsEqual(*deployment, *expectedDeployment), "deployment has not been updated")
			},
		},
		{
			name: "update scan API Service when changed externally",
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				Admission: mondoov1alpha2.Admission{Enable: true},
			},
			existingObjects: func(m mondoov1alpha2.MondooAuditConfig) []client.Object {
				service := scanapi.ScanApiService(testNamespace, m)
				service.Spec.Type = corev1.ServiceTypeExternalName
				return []client.Object{service}
			},
			validate: func(t *testing.T, kubeClient client.Client) {
				service := &corev1.Service{}
				serviceKey := types.NamespacedName{Name: scanapi.ServiceName(testMondooAuditConfigName), Namespace: testNamespace}
				err := kubeClient.Get(context.TODO(), serviceKey, service)
				require.NoError(t, err, "expected scan API Service to exist")

				auditConfig := &mondoov1alpha2.MondooAuditConfig{}
				auditConfig.Name = testMondooAuditConfigName
				auditConfig.Namespace = testNamespace
				require.NoError(
					t, kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(auditConfig), auditConfig), "failed to retrieve mondoo audit config")

				expectedService := scanapi.ScanApiService(testNamespace, *auditConfig)
				require.NoError(t, ctrl.SetControllerReference(auditConfig, expectedService, kubeClient.Scheme()))
				assert.Truef(t, k8s.AreServicesEqual(*service, *expectedService), "service has not been updated")
			},
		},
		{
			name: "cleanup old-style admission",
			mondooAuditConfigSpec: mondoov1alpha2.MondooAuditConfigData{
				Admission: mondoov1alpha2.Admission{Enable: true},
			},
			// existing objects from webhooks being previously enabled
			existingObjects: func(m mondoov1alpha2.MondooAuditConfig) []client.Object {
				objects := defaultResourcesWhenEnabled()

				vwc := &webhooksv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: testMondooAuditConfigName + "-mondoo-webhook",
					},
				}
				objects = append(objects, vwc)

				return objects
			},
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
			fakeClient := fake.NewClientBuilder().WithObjects(existingObj...).Build()

			webhooks := &Webhooks{
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

	scanApiService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scanapi.ServiceName(testMondooAuditConfigName),
			Namespace: testNamespace,
		},
	}
	objects = append(objects, scanApiService)

	scanApiDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scanapi.DeploymentName(testMondooAuditConfigName),
			Namespace: testNamespace,
		},
	}
	objects = append(objects, scanApiDep)

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
