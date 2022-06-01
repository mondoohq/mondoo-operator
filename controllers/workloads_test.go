/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	k8sv1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var _ = Describe("workloads", func() {
	const (
		name      = "workloads"
		namespace = "workloads-namespace"
		timeout   = time.Second * 10
		interval  = time.Millisecond * 250
	)
	BeforeEach(func() {
		os.Setenv("MONDOO_NAMESPACE_OVERRIDE", "mondoo-operator")
	})
	Context("When deploying the operator with workloads enabled", func() {
		It("Should create a new Deployment", func() {
			ctx := context.Background()

			By("Creating a namespace")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			By("Creating a secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Data: map[string][]byte{"config": []byte("foo")},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			By("Creating a serviceaccount")
			serviceaccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, serviceaccount)).Should(Succeed())

			By("Creating the mondoo crd")
			createdMondoo := &k8sv1alpha2.MondooAuditConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: k8sv1alpha2.MondooAuditConfigSpec{
					MondooCredsSecretRef: corev1.LocalObjectReference{Name: name},
					Scanner:              k8sv1alpha2.Scanner{ServiceAccountName: name},
					KubernetesResources: k8sv1alpha2.KubernetesResources{
						Enable: true,
					},
					Admission: k8sv1alpha2.Admission{
						CertificateProvisioning: k8sv1alpha2.CertificateProvisioning{
							Mode: k8sv1alpha2.ManualProvisioning,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, createdMondoo)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.Background(), createdMondoo)).Should(Succeed())
				time.Sleep(time.Second * 5)
			}()

			By("Checking that the mondoo crd is found")
			foundMondoo := &k8sv1alpha2.MondooAuditConfig{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, foundMondoo)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			workloadDeploymentName := fmt.Sprintf(WorkloadDeploymentNameTemplate, name)
			By("Checking that the deployment is found")
			foundDeployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: workloadDeploymentName, Namespace: namespace}, foundDeployment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Updating the deployment to be false")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, foundMondoo)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			foundMondoo.Spec.KubernetesResources.Enable = false
			Expect(k8sClient.Update(ctx, foundMondoo)).Should(Succeed())

			By("Checking that the deployment is NOT found")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: workloadDeploymentName, Namespace: namespace}, foundDeployment)
				return err == nil
			}, timeout, interval).Should(BeFalse())

		})
	})

})
