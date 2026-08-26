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

package nodes

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAccountFromRoleARN(t *testing.T) {
	tests := []struct {
		name string
		arn  string
		want string
	}{
		{
			name: "IRSA role",
			arn:  "arn:aws:iam::959975882244:role/ecr-image-pull",
			want: "959975882244",
		},
		{
			name: "path-scoped role",
			arn:  "arn:aws:iam::959975882244:role/some/path/ecr-image-pull",
			want: "959975882244",
		},
		{
			name: "govcloud partition",
			arn:  "arn:aws-us-gov:iam::123456789012:role/example",
			want: "123456789012",
		},
		{
			name: "china partition",
			arn:  "arn:aws-cn:iam::123456789012:role/example",
			want: "123456789012",
		},
		{
			name: "iso partition",
			arn:  "arn:aws-iso-b:iam::123456789012:role/example",
			want: "123456789012",
		},
		{
			// Starts with the right letters but is not a partition.
			name: "made-up partition",
			arn:  "arn:awsxyz:iam::123456789012:role/example",
			want: "",
		},
		{
			name: "not an ARN",
			arn:  "ecr-image-pull",
			want: "",
		},
		{
			name: "empty",
			arn:  "",
			want: "",
		},
		{
			name: "account is not twelve digits",
			arn:  "arn:aws:iam::12345:role/example",
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, accountFromRoleARN(test.arn))
		})
	}
}

func serviceAccount(name, namespace string, annotations map[string]string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Annotations: annotations},
	}
}

func TestAccountFromServiceAccounts(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	t.Run("reads the account off an IRSA annotation", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			serviceAccount("controller-manager", "mondoo-operator", nil),
			serviceAccount("mondoo-operator-k8s-resources-scanning", "mondoo-operator", map[string]string{
				irsaRoleAnnotation: "arn:aws:iam::959975882244:role/ecr-image-pull",
			}),
		).Build()

		r := &awsAccountResolver{Namespace: "mondoo-operator"}
		assert.Equal(t, "959975882244", r.accountFromServiceAccounts(context.Background(), c))
	})

	t.Run("ignores service accounts in other namespaces", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			serviceAccount("some-app", "default", map[string]string{
				irsaRoleAnnotation: "arn:aws:iam::111111111111:role/other",
			}),
		).Build()

		r := &awsAccountResolver{Namespace: "mondoo-operator"}
		assert.Equal(t, "", r.accountFromServiceAccounts(context.Background(), c))
	})

	t.Run("no annotated service account", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			serviceAccount("controller-manager", "mondoo-operator", nil),
		).Build()

		r := &awsAccountResolver{Namespace: "mondoo-operator"}
		assert.Equal(t, "", r.accountFromServiceAccounts(context.Background(), c))
	})

	t.Run("no client", func(t *testing.T) {
		r := &awsAccountResolver{Namespace: "mondoo-operator"}
		assert.Equal(t, "", r.accountFromServiceAccounts(context.Background(), nil))
	})
}

func TestAWSAccountResolverUsesTheEnvironmentFirst(t *testing.T) {
	// IRSA puts AWS_ROLE_ARN in the pod, so the account is answerable without
	// a network call or any cluster lookup.
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::959975882244:role/ecr-image-pull")

	r := &awsAccountResolver{Namespace: "mondoo-operator"}
	assert.Equal(t, "959975882244", r.Resolve(context.Background(), nil))
}

func TestAWSAccountResolverCachesTheAnswer(t *testing.T) {
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::959975882244:role/ecr-image-pull")

	r := &awsAccountResolver{Namespace: "mondoo-operator"}
	require.Equal(t, "959975882244", r.Resolve(context.Background(), nil))

	// A cluster's account does not change, so a second call must not re-resolve
	// -- including after the environment that produced the first answer is gone.
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::111111111111:role/other")
	assert.Equal(t, "959975882244", r.Resolve(context.Background(), nil))
}

func TestAWSAccountResolverRetriesAfterAFailure(t *testing.T) {
	// A failed resolution must not be cached the way a successful one is: a
	// cancelled reconcile, or credentials that arrive a moment later, would
	// otherwise leave every node without its cloud identity until the operator
	// pod restarted.
	t.Setenv("AWS_ROLE_ARN", "")

	r := &awsAccountResolver{Namespace: "mondoo-operator"}
	require.Equal(t, "", r.Resolve(context.Background(), nil))
	require.False(t, r.resolved)
	require.False(t, r.retryAfter.IsZero(), "a failure should schedule a retry")

	// Within the backoff the resolver holds off, so a cluster that really is
	// not on AWS does not pay for a lookup on every reconcile.
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::959975882244:role/ecr-image-pull")
	assert.Equal(t, "", r.Resolve(context.Background(), nil))

	// Once it expires the answer is picked up.
	r.retryAfter = time.Now().Add(-time.Second)
	assert.Equal(t, "959975882244", r.Resolve(context.Background(), nil))
	assert.True(t, r.resolved)
}
