package mondoo

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
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
	s.resolver = containerImageResolver{
		logger: ctrl.Log.WithName("container-image-resolver"),
		imageCacher: NewFakeCacher(func(image string) (string, error) {
			s.remoteCallsCount++

			imageParts := strings.Split(image, ":")
			return imageParts[0] + "@sha256:" + s.testHex, nil
		}),
	}
}

func (s *ContainerImageResolverSuite) TestNewContainerImageResolver() {
	resolver := NewContainerImageResolver(false)

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

func (s *ContainerImageResolverSuite) TestMondooClientImage_OpenShift() {
	s.resolver.resolveForOpenShift = true

	res, err := s.resolver.MondooClientImage("", "", true)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s:%s", MondooClientImage, OpenShiftMondooClientTag), res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
}

func (s *ContainerImageResolverSuite) TestMondooOperatorImage() {
	image := "ghcr.io/mondoo/testimage"
	res, err := s.resolver.MondooOperatorImage(image, "testtag", false)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s@sha256:%s", image, s.testHex), res)
	s.Equalf(1, s.remoteCallsCount, "remote call has not been performed")
}

func (s *ContainerImageResolverSuite) TestMondooOperatorImage_Defaults() {
	res, err := s.resolver.MondooOperatorImage("", "", true)
	s.NoError(err)

	s.Equal(fmt.Sprintf("%s:%s", MondooOperatorImage, MondooOperatorTag), res)
	s.Equalf(0, s.remoteCallsCount, "remote call has been performed")
}

func (s *ContainerImageResolverSuite) TestMondooOperatorImage_SkipImageResolution() {
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
