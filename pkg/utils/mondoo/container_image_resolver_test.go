package mondoo

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/suite"
	ctrl "sigs.k8s.io/controller-runtime"
)

type ContainerImageResolverSuite struct {
	suite.Suite
	resolver         containerImageResolver
	remoteCallsCount int
	testHex          string
}

func (s *ContainerImageResolverSuite) BeforeTest(suiteName, testName string) {
	s.remoteCallsCount = 0
	s.testHex = "test"
	s.resolver = containerImageResolver{
		imageCache: make(map[string]string),
		logger:     ctrl.Log.WithName("container-image-resolver"),
		getRemoteImage: func(ref name.Reference, options ...remote.Option) (*remote.Descriptor, error) {
			s.remoteCallsCount++

			return &remote.Descriptor{
				Descriptor: v1.Descriptor{
					Digest: v1.Hash{Algorithm: "sha256", Hex: s.testHex},
				},
			}, nil
		},
	}
}

func (s *ContainerImageResolverSuite) TestNewContainerImageResolver() {
	resolver := NewContainerImageResolver()

	ref, err := name.ParseReference(fmt.Sprintf("%s:%s", MondooClientImage, MondooClientTag))
	s.NoError(err)
	desc, err := remote.Get(ref)

	// If the remote call gets a network error, then skip the test so it does not fail because of
	// network issues.
	if err != nil && strings.Contains(err.Error(), "dial tcp: lookup") {
		s.T().SkipNow()
	}

	s.NoError(err)
	expected := fmt.Sprintf("%s@%s", ref.Context().Name(), desc.Digest.String())

	imageWithDigest, err := resolver.MondooClientImage(MondooClientImage, MondooClientTag, false)
	s.NoError(err)
	s.Equal(expected, imageWithDigest)
}

func (s *ContainerImageResolverSuite) TestMondooClientImage() {
	image := "ghcr.io/mondoo/testimage"
	res, err := s.resolver.MondooClientImage(image, "testtag", false)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s@sha256:%s", image, s.testHex), res)
	s.Equalf(1, s.remoteCallsCount, "remote call has not been performed")
}

func (s *ContainerImageResolverSuite) TestMondooClientImage_Cached() {
	image := "ghcr.io/mondoo/testimage"
	tag := "testtag"
	cachedDigest := "testDigest"

	s.resolver.imageCache[fmt.Sprintf("%s:%s", image, tag)] = cachedDigest
	res, err := s.resolver.MondooClientImage(image, tag, false)
	s.NoError(err)

	s.Equal(cachedDigest, res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
}

func (s *ContainerImageResolverSuite) TestMondooClientImage_Defaults() {
	res, err := s.resolver.MondooClientImage("", "", true)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s:%s", MondooClientImage, MondooClientTag), res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
}

func (s *ContainerImageResolverSuite) TestMondooClientImage_SkipImageResolution() {
	image := "ghcr.io/mondoo/testimage"
	tag := "testtag"

	res, err := s.resolver.MondooClientImage(image, tag, true)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s:%s", image, tag), res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
}

func (s *ContainerImageResolverSuite) MondooOperatorImage() {
	image := "ghcr.io/mondoo/testimage"
	res, err := s.resolver.MondooOperatorImage(image, "testtag", false)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s@sha256:%s", image, s.testHex), res)
	s.Equalf(1, s.remoteCallsCount, "remote call has not been performed")
}

func (s *ContainerImageResolverSuite) MondooOperatorImage_Cached() {
	image := "ghcr.io/mondoo/testimage"
	tag := "testtag"
	cachedDigest := "testDigest"

	s.resolver.imageCache[fmt.Sprintf("%s:%s", image, tag)] = cachedDigest
	res, err := s.resolver.MondooOperatorImage(image, tag, false)
	s.NoError(err)

	s.Equal(cachedDigest, res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
}

func (s *ContainerImageResolverSuite) MondooOperatorImage_Defaults() {
	res, err := s.resolver.MondooOperatorImage("", "", true)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s:%s", MondooOperatorImage, MondooOperatorTag), res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
}

func (s *ContainerImageResolverSuite) MondooOperatorImage_SkipImageResolution() {
	image := "ghcr.io/mondoo/testimage"
	tag := "testtag"

	res, err := s.resolver.MondooOperatorImage(image, tag, true)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s:%s", image, tag), res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
}

func TestContainerImageResolverSuite(t *testing.T) {
	suite.Run(t, new(ContainerImageResolverSuite))
}
