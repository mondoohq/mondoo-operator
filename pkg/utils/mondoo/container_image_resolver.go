// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/pkg/imagecache"
	"go.mondoo.com/mondoo-operator/pkg/version"
)

const (
	CnspecImage              = "ghcr.io/mondoohq/mondoo-operator/cnspec"
	CnspecTag                = "12-rootless"
	OpenShiftMondooClientTag = "12-ubi-rootless"
	MondooOperatorImage      = "ghcr.io/mondoohq/mondoo-operator"
	PodNameEnvVar            = "POD_NAME"
	PodNamespaceEnvVar       = "POD_NAMESPACE"
)

// On a normal mondoo-operator build, the Version variable will be set at build time to match
// the $VERSION being built (or default to the git SHA). In the event that someone did a manual
// build of mondoo-operator and failed to set the Version variable it will get a default value of
// "latest".
var MondooOperatorTag = version.Version

type ContainerImageResolver interface {
	// CnspecImage return the cnspec image. If skipResolveImage is false, then the image tag is replaced
	// by a digest. If userImage or userTag are empty strings, default values are used.
	CnspecImage(userImage, userTag string, skipImageResolution bool) (string, error)

	// MondooOperatorImage return the Mondoo operator image. If skipResolveImage is false, then the image tag is replaced
	// by a digest. If userImage or userTag are empty strings, default values are used.
	MondooOperatorImage(ctx context.Context, userImage, userTag string, skipImageResolution bool) (string, error)

	// WithImageRegistry returns a new ContainerImageResolver that uses the specified image registry.
	// This allows rewriting image references to use a custom registry (e.g., corporate Artifactory mirror).
	WithImageRegistry(imageRegistry string) ContainerImageResolver
}

type containerImageResolver struct {
	logger               logr.Logger
	resolveForOpenShift  bool
	imageCacher          imagecache.ImageCacher
	kubeClient           client.Client
	operatorPodName      string
	operatorPodNamespace string
	imageRegistry        string
}

func NewContainerImageResolver(kubeClient client.Client, isOpenShift bool) ContainerImageResolver {
	return NewContainerImageResolverWithRegistry(kubeClient, isOpenShift, "")
}

func NewContainerImageResolverWithRegistry(kubeClient client.Client, isOpenShift bool, imageRegistry string) ContainerImageResolver {
	podName := os.Getenv(PodNameEnvVar)
	if podName == "" {
		podName = "mondoo-operator-controller-manager"
	}
	podNamespace := os.Getenv(PodNamespaceEnvVar)
	if podNamespace == "" {
		podNamespace = "mondoo-operator"
	}

	return &containerImageResolver{
		logger:               ctrl.Log.WithName("container-image-resolver"),
		imageCacher:          imagecache.NewImageCacher(),
		resolveForOpenShift:  isOpenShift,
		kubeClient:           kubeClient,
		operatorPodName:      podName,
		operatorPodNamespace: podNamespace,
		imageRegistry:        imageRegistry,
	}
}

func (c *containerImageResolver) CnspecImage(userImage, userTag string, skipImageResolution bool) (string, error) {
	defaultTag := CnspecTag

	if c.resolveForOpenShift {
		defaultTag = OpenShiftMondooClientTag
	}

	defaultImage := CnspecImage
	image := userImageOrDefault(defaultImage, defaultTag, userImage, userTag)
	return c.resolveImage(image, skipImageResolution)
}

func (c *containerImageResolver) MondooOperatorImage(ctx context.Context, userImage, userTag string, skipImageResolution bool) (string, error) {
	image := ""

	// If we have no user image or tag, we read the image from the operator pod
	if userImage == "" || userTag == "" {
		operatorPod := &corev1.Pod{}
		if err := c.kubeClient.Get(ctx, client.ObjectKey{Namespace: c.operatorPodNamespace, Name: c.operatorPodName}, operatorPod); err == nil {
			for _, container := range operatorPod.Spec.Containers {
				if container.Name == "manager" {
					image = container.Image
					break
				}
			}

			// If at this point we don't have an image, then something went wrong
			if image == "" {
				return "", fmt.Errorf("failed to get mondoo-operator image from operator pod")
			}
		}
	}

	// If still no image, then load the image user-specified image or use the defaults as last resort
	if image == "" {
		image = userImageOrDefault(MondooOperatorImage, MondooOperatorTag, userImage, userTag)
	}

	return c.resolveImage(image, skipImageResolution)
}

func (c *containerImageResolver) resolveImage(image string, skipImageResolution bool) (string, error) {
	// Apply custom image registry prefix if configured
	image = c.applyImageRegistry(image)

	if skipImageResolution {
		return image, nil
	}

	imageWithDigest, err := c.imageCacher.GetImage(image)
	if err != nil {
		c.logger.Error(err, "failed to resolve image plus digest")
		return "", err
	}

	return imageWithDigest, nil
}

// applyImageRegistry rewrites the image to use a custom registry if configured.
// For example, if imageRegistry is "artifactory.example.com/ghcr.io.docker" and
// the image is "ghcr.io/mondoohq/mondoo-operator:v1.0.0", it will be rewritten to
// "artifactory.example.com/ghcr.io.docker/mondoohq/mondoo-operator:v1.0.0"
func (c *containerImageResolver) applyImageRegistry(image string) string {
	if c.imageRegistry == "" {
		return image
	}

	// Parse the image to extract registry, repository, and tag
	// Image format: [registry/]repository[:tag][@digest]
	// Examples:
	//   ghcr.io/mondoohq/mondoo-operator:v1.0.0
	//   mondoohq/mondoo-operator:v1.0.0
	//   mondoo-operator:v1.0.0

	// Check if image starts with a known registry (contains a dot before the first slash)
	parts := splitImageParts(image)
	if parts.registry != "" {
		// Replace the registry with the custom one
		// e.g., ghcr.io/mondoohq/mondoo-operator -> artifactory.example.com/ghcr.io.docker/mondoohq/mondoo-operator
		return fmt.Sprintf("%s/%s", c.imageRegistry, parts.repositoryWithTag)
	}

	// No registry in the image, just prepend the custom registry
	return fmt.Sprintf("%s/%s", c.imageRegistry, image)
}

type imageParts struct {
	registry          string
	repositoryWithTag string
}

func splitImageParts(image string) imageParts {
	// Find the first slash
	slashIdx := -1
	for i, c := range image {
		if c == '/' {
			slashIdx = i
			break
		}
	}

	if slashIdx == -1 {
		// No slash, no registry (e.g., "ubuntu:latest")
		return imageParts{registry: "", repositoryWithTag: image}
	}

	potentialRegistry := image[:slashIdx]
	// Check if it looks like a registry (contains a dot or colon, or is "localhost")
	if strings.Contains(potentialRegistry, ".") || strings.Contains(potentialRegistry, ":") || potentialRegistry == "localhost" {
		return imageParts{
			registry:          potentialRegistry,
			repositoryWithTag: image[slashIdx+1:],
		}
	}

	// No registry, the first part is part of the repository (e.g., "library/ubuntu:latest")
	return imageParts{registry: "", repositoryWithTag: image}
}

func (c *containerImageResolver) WithImageRegistry(imageRegistry string) ContainerImageResolver {
	return &containerImageResolver{
		logger:               c.logger,
		resolveForOpenShift:  c.resolveForOpenShift,
		imageCacher:          c.imageCacher,
		kubeClient:           c.kubeClient,
		operatorPodName:      c.operatorPodName,
		operatorPodNamespace: c.operatorPodNamespace,
		imageRegistry:        imageRegistry,
	}
}

func userImageOrDefault(defaultImage, defaultTag, userImage, userTag string) string {
	image := defaultImage
	tag := defaultTag
	if userImage != "" {
		image = userImage
	}
	if userTag != "" {
		tag = userTag
	}
	return fmt.Sprintf("%s:%s", image, tag)
}
