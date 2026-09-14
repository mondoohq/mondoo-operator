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
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	// AWSPlatformIDEnvVar carries the node's EC2 platform identifier into the
	// scan pod. The node inventory is one ConfigMap shared by every node, so a
	// per-node value has to travel as an environment variable.
	AWSPlatformIDEnvVar = "MONDOO_AWS_PLATFORM_ID"

	regionLabel       = "topology.kubernetes.io/region"
	legacyRegionLabel = "failure-domain.beta.kubernetes.io/region"
)

// awsProviderIDScheme prefixes the providerID the AWS cloud controller writes,
// e.g. "aws:///eu-central-1a/i-0a8caeccfd813a595".
const awsProviderIDScheme = "aws://"

// awsZoneRegex matches an availability zone segment. It is deliberately loose:
// awsRegion decides separately whether a zone has a shape a region can be
// derived from, and zone naming has grown local and wavelength forms over time.
var awsZoneRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

// ec2InstanceIDRegex matches an EC2 instance ID -- "i-" followed by the 8-hex
// legacy form or the 17-hex current one. Fargate nodes carry a providerID under
// the same aws:// scheme whose last segment is a task UUID, not an instance, so
// the shape has to be checked rather than assumed.
var ec2InstanceIDRegex = regexp.MustCompile(`^i-[0-9a-f]{8,}$`)

// standardAZRegex matches the ordinary "<region><letter>" availability zone
// form (eu-central-1a), where trimming the trailing letter yields the region.
// Local and wavelength zones (us-west-2-lax-1a) do not follow that rule, so
// they are deliberately not matched -- guessing there would produce a region
// that does not exist.
var standardAZRegex = regexp.MustCompile(`^([a-z]{2}(?:-[a-z]+)+-\d)[a-z]$`)

// awsAccountIDRegex matches an AWS account ID.
var awsAccountIDRegex = regexp.MustCompile(`^\d{12}$`)

// AWSPlatformID builds the Mondoo platform identifier for the EC2 instance
// backing a node, or "" when the node is not an EC2 instance or any part of the
// identity is unknown.
//
// The identifier has to match the one the AWS integration reports for the same
// instance, byte for byte, or the two end up as separate assets for one
// machine. That means all three parts must be right, and returning nothing is
// always better than returning a guess: a wrong account or region does not fail
// loudly, it silently merges this node with a different machine's asset.
//
// The instance ID and availability zone come from node.spec.providerID, the
// region from the node's own topology label, and the account from the caller
// (see awsAccountResolver) -- none of which requires the instance metadata
// service, which a scan pod on a hop-limit-1 node cannot reach anyway.
func AWSPlatformID(node corev1.Node, accountID string) string {
	if !awsAccountIDRegex.MatchString(accountID) {
		return ""
	}

	instanceID, zone, ok := parseAWSProviderID(node.Spec.ProviderID)
	if !ok {
		return ""
	}

	region := awsRegion(node, zone)
	if region == "" {
		return ""
	}

	return fmt.Sprintf(
		"//platformid.api.mondoo.app/runtime/aws/ec2/v1/accounts/%s/regions/%s/instances/%s",
		accountID, region, instanceID)
}

// parseAWSProviderID splits an EC2 node's providerID into its instance ID and
// availability zone. It reports false for every other provider and for AWS
// nodes that are not EC2 instances.
//
// Empty path segments are skipped so that the canonical three-slash form and a
// two-slash spelling of the same thing parse identically; anything that does
// not come down to exactly one zone and one instance ID is rejected rather than
// guessed at.
func parseAWSProviderID(providerID string) (instanceID string, zone string, ok bool) {
	rest, found := strings.CutPrefix(providerID, awsProviderIDScheme)
	if !found {
		return "", "", false
	}

	segments := make([]string, 0, 2)
	for _, segment := range strings.Split(rest, "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	if len(segments) != 2 {
		return "", "", false
	}

	zone, instanceID = segments[0], segments[1]
	if !awsZoneRegex.MatchString(zone) || !ec2InstanceIDRegex.MatchString(instanceID) {
		return "", "", false
	}
	return instanceID, zone, true
}

// awsRegion returns the node's region, preferring the topology labels the cloud
// controller sets over deriving it from the availability zone.
func awsRegion(node corev1.Node, zone string) string {
	if region := node.Labels[regionLabel]; region != "" {
		return region
	}
	if region := node.Labels[legacyRegionLabel]; region != "" {
		return region
	}

	if match := standardAZRegex.FindStringSubmatch(zone); match != nil {
		return match[1]
	}
	return ""
}
