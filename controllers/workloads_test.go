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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	k8sv1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var _ = Describe("workloads", func() {
	const (
		name      = "workloads"
		namespace = "default"
		timeout   = time.Second * 10
		duration  = time.Second * 10
		interval  = time.Millisecond * 250
	)
	Context("When deploying the operator with workloads enabled", func() {
		It("Should create a new Deployment", func() {
			ctx := context.Background()
			createdMondoo := &k8sv1alpha1.MondooAuditConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: k8sv1alpha1.MondooAuditConfigData{
					Workloads: k8sv1alpha1.Workloads{
						Enable: true,
					}},
			}

			Expect(k8sClient.Create(ctx, createdMondoo)).Should(Succeed())

			foundMondoo := &k8sv1alpha1.MondooAuditConfig{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, foundMondoo)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(foundMondoo.Spec.Workloads.Enable).Should(Equal(true))

		})
	})

})
