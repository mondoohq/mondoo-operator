// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type ContainerImageResolverSuite struct {
	suite.Suite
	remoteCallsCount  int
	testHex           string
	fakeClientBuilder *fake.ClientBuilder
}

type fakeCacher struct {
	fakeGetImage func(string) (string, error)
}

func (f *fakeCacher) GetImage(img string) (string, error) {
	return f.fakeGetImage(img)
}

func NewFakeCacher(f func(string) (string, error)) *fakeCacher {
	return &fakeCacher{
		fakeGetImage: f,
	}
}

func (s *ContainerImageResolverSuite) BeforeTest(suiteName, testName string) {
	s.remoteCallsCount = 0
	s.testHex = "test"
	s.fakeClientBuilder = fake.NewClientBuilder().WithObjects(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mondoo-operator-controller-manager",
			Namespace: "mondoo-operator",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "random-container",
					Image: "ghcr.io/mondoohq/test:random",
				},
				{
					Name:  "manager",
					Image: "ghcr.io/mondoohq/mondoo-operator:testtag",
				},
			},
		},
	})
}

func (s *ContainerImageResolverSuite) containerImageResolver() containerImageResolver {
	return containerImageResolver{
		logger: ctrl.Log.WithName("container-image-resolver"),
		imageCacher: NewFakeCacher(func(image string) (string, error) {
			s.remoteCallsCount++

			imageParts := strings.Split(image, ":")
			return imageParts[0] + "@sha256:" + s.testHex, nil
		}),
		kubeClient:           s.fakeClientBuilder.Build(),
		operatorPodName:      "mondoo-operator-controller-manager",
		operatorPodNamespace: "mondoo-operator",
	}
}

func (s *ContainerImageResolverSuite) TestNewContainerImageResolver() {
	resolver := NewContainerImageResolver(s.fakeClientBuilder.Build(), false)

	ref, err := name.ParseReference(fmt.Sprintf("%s:%s", CnspecImage, CnspecTag))
	s.NoError(err)
	desc, err := remote.Get(ref)

	// If the remote call gets a network error, then skip the test so it does not fail because of
	// network issues.
	if err != nil && strings.Contains(err.Error(), "dial tcp: lookup") {
		s.T().SkipNow()
	}

	s.NoError(err)
	expected := fmt.Sprintf("%s@%s", ref.Context().Name(), desc.Digest.String())

	imageWithDigest, err := resolver.CnspecImage(CnspecImage, CnspecTag, "", false)
	s.NoError(err)
	s.Equal(expected, imageWithDigest)
}

func (s *ContainerImageResolverSuite) TestCnspecImage() {
	resolver := s.containerImageResolver()
	image := "ghcr.io/mondoo/testimage"
	res, err := resolver.CnspecImage(image, "testtag", "", false)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s@sha256:%s", image, s.testHex), res)
	s.Equalf(1, s.remoteCallsCount, "remote call has not been performed")
}

func (s *ContainerImageResolverSuite) TestCnspecImage_Defaults() {
	resolver := s.containerImageResolver()
	res, err := resolver.CnspecImage("", "", "", true)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s:%s", CnspecImage, CnspecTag), res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
}

func (s *ContainerImageResolverSuite) TestCnspecImage_SkipImageResolution() {
	resolver := s.containerImageResolver()
	image := "ghcr.io/mondoo/testimage"
	tag := "testtag"

	res, err := resolver.CnspecImage(image, tag, "", true)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s:%s", image, tag), res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
}

func (s *ContainerImageResolverSuite) TestCnspecImage_OpenShift() {
	resolver := s.containerImageResolver()
	resolver.resolveForOpenShift = true

	res, err := resolver.CnspecImage("", "", "", true)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s:%s", CnspecImage, OpenShiftMondooClientTag), res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
}

func (s *ContainerImageResolverSuite) TestMondooOperatorImage() {
	resolver := s.containerImageResolver()
	res, err := resolver.MondooOperatorImage(context.Background(), "", "", "", false)
	s.NoError(err)

	s.Equal("ghcr.io/mondoohq/mondoo-operator@sha256:test", res)
}

func (s *ContainerImageResolverSuite) TestMondooOperatorImage_CustomImage() {
	image := "ghcr.io/mondoo/testimage"
	tag := "testtag"

	resolver := s.containerImageResolver()
	res, err := resolver.MondooOperatorImage(context.Background(), image, tag, "", false)
	s.NoError(err)

	s.Equal("ghcr.io/mondoo/testimage@sha256:test", res)
}

func (s *ContainerImageResolverSuite) TestMondooOperatorImage_SkipImageResolution() {
	image := "ghcr.io/mondoo/testimage"
	tag := "testtag"

	resolver := s.containerImageResolver()
	res, err := resolver.MondooOperatorImage(context.Background(), image, tag, "", true)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s:%s", image, tag), res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
	s.Equal("ghcr.io/mondoo/testimage:testtag", res)
}

func (s *ContainerImageResolverSuite) TestCnspecImage_DigestOnly() {
	resolver := s.containerImageResolver()
	image := "ghcr.io/mondoo/testimage"
	digest := "sha256:abc123def456"

	res, err := resolver.CnspecImage(image, "", digest, false)
	s.NoError(err)

	// When digest is specified, it should be used and no image resolution should occur
	s.Equal(fmt.Sprintf("%s@%s", image, digest), res)
	s.Equalf(0, s.remoteCallsCount, "remote call should not be performed when digest is specified")
}

func (s *ContainerImageResolverSuite) TestCnspecImage_DigestWithTag() {
	resolver := s.containerImageResolver()
	image := "ghcr.io/mondoo/testimage"
	tag := "v2"
	digest := "sha256:abc123def456"

	res, err := resolver.CnspecImage(image, tag, digest, false)
	s.NoError(err)

	// Digest takes precedence over tag
	s.Equal(fmt.Sprintf("%s@%s", image, digest), res)
	s.Equalf(0, s.remoteCallsCount, "remote call should not be performed when digest is specified")
}

func (s *ContainerImageResolverSuite) TestCnspecImage_DigestWithDefaultImage() {
	resolver := s.containerImageResolver()
	digest := "sha256:abc123def456"

	res, err := resolver.CnspecImage("", "", digest, false)
	s.NoError(err)

	// Uses default image with user-specified digest
	s.Equal(fmt.Sprintf("%s@%s", CnspecImage, digest), res)
	s.Equalf(0, s.remoteCallsCount, "remote call should not be performed when digest is specified")
}

func (s *ContainerImageResolverSuite) TestMondooOperatorImage_DigestOnly() {
	resolver := s.containerImageResolver()
	image := "ghcr.io/mondoo/testimage"
	digest := "sha256:abc123def456"

	res, err := resolver.MondooOperatorImage(context.Background(), image, "", digest, false)
	s.NoError(err)

	// When digest is specified, it should be used and no image resolution should occur
	s.Equal(fmt.Sprintf("%s@%s", image, digest), res)
	s.Equalf(0, s.remoteCallsCount, "remote call should not be performed when digest is specified")
}

func (s *ContainerImageResolverSuite) TestMondooOperatorImage_DigestWithTag() {
	resolver := s.containerImageResolver()
	image := "ghcr.io/mondoo/testimage"
	tag := "v2"
	digest := "sha256:abc123def456"

	res, err := resolver.MondooOperatorImage(context.Background(), image, tag, digest, false)
	s.NoError(err)

	// Digest takes precedence over tag
	s.Equal(fmt.Sprintf("%s@%s", image, digest), res)
	s.Equalf(0, s.remoteCallsCount, "remote call should not be performed when digest is specified")
}

func TestContainerImageResolverSuite(t *testing.T) {
	suite.Run(t, new(ContainerImageResolverSuite))
}
