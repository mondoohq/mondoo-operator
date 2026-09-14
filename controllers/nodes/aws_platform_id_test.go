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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func awsNode(providerID string, labels map[string]string) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-10-4-200-202.eu-central-1.compute.internal", Labels: labels},
		Spec:       corev1.NodeSpec{ProviderID: providerID},
	}
}

func TestAWSPlatformID(t *testing.T) {
	const account = "959975882244"

	tests := []struct {
		name    string
		node    corev1.Node
		account string
		want    string
	}{
		{
			name:    "EKS node with a region label",
			node:    awsNode("aws:///eu-central-1a/i-0a8caeccfd813a595", map[string]string{regionLabel: "eu-central-1"}),
			account: account,
			want:    "//platformid.api.mondoo.app/runtime/aws/ec2/v1/accounts/959975882244/regions/eu-central-1/instances/i-0a8caeccfd813a595",
		},
		{
			// No topology label, so the region is derived from the zone.
			name:    "region derived from the availability zone",
			node:    awsNode("aws:///us-west-2b/i-0123456789abcdef0", nil),
			account: account,
			want:    "//platformid.api.mondoo.app/runtime/aws/ec2/v1/accounts/959975882244/regions/us-west-2/instances/i-0123456789abcdef0",
		},
		{
			name:    "legacy region label",
			node:    awsNode("aws:///eu-west-1c/i-0a8caeccfd813a595", map[string]string{legacyRegionLabel: "eu-west-1"}),
			account: account,
			want:    "//platformid.api.mondoo.app/runtime/aws/ec2/v1/accounts/959975882244/regions/eu-west-1/instances/i-0a8caeccfd813a595",
		},
		{
			// A local zone does not end in "<region><letter>", so trimming the
			// last character would invent a region that does not exist. Without
			// a label there is nothing trustworthy to use.
			name:    "local zone without a region label yields nothing",
			node:    awsNode("aws:///us-west-2-lax-1a/i-0a8caeccfd813a595", nil),
			account: account,
			want:    "",
		},
		{
			name:    "local zone with a region label",
			node:    awsNode("aws:///us-west-2-lax-1a/i-0a8caeccfd813a595", map[string]string{regionLabel: "us-west-2"}),
			account: account,
			want:    "//platformid.api.mondoo.app/runtime/aws/ec2/v1/accounts/959975882244/regions/us-west-2/instances/i-0a8caeccfd813a595",
		},
		{
			// Fargate reuses the aws:// scheme with a task id. Turning that
			// into an EC2 platform ID would invent an instance.
			name:    "fargate node is not an EC2 instance",
			node:    awsNode("aws:///eu-central-1a/a1b2c3d4-1234-5678-9abc-def012345678", map[string]string{regionLabel: "eu-central-1"}),
			account: account,
			want:    "",
		},
		{
			name:    "non-AWS provider",
			node:    awsNode("gce://my-project/europe-west1-b/my-node", map[string]string{regionLabel: "europe-west1"}),
			account: account,
			want:    "",
		},
		{
			name:    "node without a providerID",
			node:    awsNode("", map[string]string{regionLabel: "eu-central-1"}),
			account: account,
			want:    "",
		},
		{
			// The common non-AWS case: no account was resolved. Returning
			// nothing keeps the node exactly as it is scanned today.
			name:    "unknown account",
			node:    awsNode("aws:///eu-central-1a/i-0a8caeccfd813a595", map[string]string{regionLabel: "eu-central-1"}),
			account: "",
			want:    "",
		},
		{
			name:    "malformed account is rejected rather than used",
			node:    awsNode("aws:///eu-central-1a/i-0a8caeccfd813a595", map[string]string{regionLabel: "eu-central-1"}),
			account: "not-an-account",
			want:    "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, AWSPlatformID(test.node, test.account))
		})
	}
}

func TestParseAWSProviderID(t *testing.T) {
	t.Run("three-slash form", func(t *testing.T) {
		id, zone, ok := parseAWSProviderID("aws:///eu-central-1a/i-0a8caeccfd813a595")
		assert.True(t, ok)
		assert.Equal(t, "i-0a8caeccfd813a595", id)
		assert.Equal(t, "eu-central-1a", zone)
	})

	t.Run("two-slash form", func(t *testing.T) {
		// Tolerated so the same providerID spelled with one fewer slash
		// parses identically rather than being silently dropped.
		id, zone, ok := parseAWSProviderID("aws://eu-central-1a/i-0a8caeccfd813a595")
		assert.True(t, ok)
		assert.Equal(t, "i-0a8caeccfd813a595", id)
		assert.Equal(t, "eu-central-1a", zone)
	})

	t.Run("trailing segment is rejected", func(t *testing.T) {
		_, _, ok := parseAWSProviderID("aws:///eu-central-1a/i-0a8caeccfd813a595/extra")
		assert.False(t, ok)
	})
}

func TestAWSPlatformIDEnv(t *testing.T) {
	t.Run("set when the identity is known", func(t *testing.T) {
		env := awsPlatformIDEnv(
			awsNode("aws:///eu-central-1a/i-0a8caeccfd813a595", map[string]string{regionLabel: "eu-central-1"}),
			"959975882244")

		assert.Equal(t, []corev1.EnvVar{{
			Name:  AWSPlatformIDEnvVar,
			Value: "//platformid.api.mondoo.app/runtime/aws/ec2/v1/accounts/959975882244/regions/eu-central-1/instances/i-0a8caeccfd813a595",
		}}, env)
	})

	t.Run("omitted when it is not", func(t *testing.T) {
		// Nothing at all, rather than an empty value: the inventory template
		// resolves an unset variable to "", which the provider ignores.
		assert.Nil(t, awsPlatformIDEnv(awsNode("", nil), ""))
	})
}
