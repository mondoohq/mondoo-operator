// Copyright Mondoo, Inc. 2026
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
	// by a digest. If userImage, userTag, or userDigest are empty strings, default values are used.
	// When userDigest is specified, it takes precedence over userTag.
	CnspecImage(userImage, userTag, userDigest string, skipImageResolution bool) (string, error)

	// MondooOperatorImage return the Mondoo operator image. If skipResolveImage is false, then the image tag is replaced
	// by a digest. If userImage, userTag, or userDigest are empty strings, default values are used.
	// When userDigest is specified, it takes precedence over userTag.
	MondooOperatorImage(ctx context.Context, userImage, userTag, userDigest string, skipImageResolution bool) (string, error)

	// WithImageRegistry returns a new ContainerImageResolver that uses the specified image registry prefix.
	// Use this for simple registry mirrors where all images go to the same mirror.
	WithImageRegistry(imageRegistry string) ContainerImageResolver

	// WithRegistryMirrors returns a new ContainerImageResolver that uses the specified registry mirrors.
	// Use this when you need to map different source registries to different mirrors.
	// The mirrors map public registries to private mirrors (e.g., "ghcr.io" -> "artifactory.example.com/ghcr.io.docker").
	WithRegistryMirrors(registryMirrors map[string]string) ContainerImageResolver

	// WithImagePullSecrets returns a new ContainerImageResolver that uses the specified imagePullSecrets for authentication.
	WithImagePullSecrets(imagePullSecrets []corev1.LocalObjectReference) ContainerImageResolver
}

type containerImageResolver struct {
	logger               logr.Logger
	resolveForOpenShift  bool
	imageCacher          imagecache.ImageCacher
	kubeClient           client.Client
	operatorPodName      string
	operatorPodNamespace string
	imageRegistry        string
	registryMirrors      map[string]string
	imagePullSecrets     []corev1.LocalObjectReference
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

func (c *containerImageResolver) CnspecImage(userImage, userTag, userDigest string, skipImageResolution bool) (string, error) {
	defaultTag := CnspecTag

	if c.resolveForOpenShift {
		defaultTag = OpenShiftMondooClientTag
	}

	defaultImage := CnspecImage
	image := userImageOrDefault(defaultImage, defaultTag, userImage, userTag, userDigest)

	// If user specified a digest, skip image resolution since we already have a digest
	if userDigest != "" {
		skipImageResolution = true
	}

	return c.resolveImage(context.Background(), image, skipImageResolution)
}

func (c *containerImageResolver) MondooOperatorImage(ctx context.Context, userImage, userTag, userDigest string, skipImageResolution bool) (string, error) {
	image := ""

	// If we have no user image, tag, or digest, we read the image from the operator pod
	if userImage == "" || (userTag == "" && userDigest == "") {
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
		image = userImageOrDefault(MondooOperatorImage, MondooOperatorTag, userImage, userTag, userDigest)
	}

	// If user specified a digest, skip image resolution since we already have a digest
	if userDigest != "" {
		skipImageResolution = true
	}

	return c.resolveImage(ctx, image, skipImageResolution)
}

func (c *containerImageResolver) resolveImage(ctx context.Context, image string, skipImageResolution bool) (string, error) {
	// Apply custom image registry prefix if configured
	image = c.applyImageRegistry(image)

	if skipImageResolution {
		return image, nil
	}

	// Apply authentication if imagePullSecrets are configured
	cacher := c.imageCacher
	if len(c.imagePullSecrets) > 0 {
		keychain, err := imagecache.KeychainFromSecrets(ctx, c.kubeClient, c.operatorPodNamespace, c.imagePullSecrets)
		if err == nil {
			cacher = cacher.WithAuth(keychain)
		}
	}

	imageWithDigest, err := cacher.GetImage(image)
	if err != nil {
		c.logger.Error(err, "failed to resolve image plus digest")
		return "", err
	}

	return imageWithDigest, nil
}

// applyImageRegistry rewrites the image to use a custom registry if configured.
// It first checks registryMirrors for a specific mapping, then falls back to imageRegistry.
// For example, if registryMirrors has "ghcr.io" -> "artifactory.example.com/ghcr.io.docker" and
// the image is "ghcr.io/mondoohq/mondoo-operator:v1.0.0", it will be rewritten to
// "artifactory.example.com/ghcr.io.docker/mondoohq/mondoo-operator:v1.0.0"
func (c *containerImageResolver) applyImageRegistry(image string) string {
	// Parse the image to extract registry, repository, and tag
	parts := splitImageParts(image)

	// First, check if we have a specific mirror for this registry
	if len(c.registryMirrors) > 0 && parts.registry != "" {
		if mirror, ok := c.registryMirrors[parts.registry]; ok {
			return fmt.Sprintf("%s/%s", mirror, parts.repositoryWithTag)
		}
	}

	// Fall back to the legacy imageRegistry if set
	if c.imageRegistry == "" {
		return image
	}

	if parts.registry != "" {
		// Replace the registry with the custom one
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

// clone creates a shallow copy of the resolver
func (c *containerImageResolver) clone() *containerImageResolver {
	return &containerImageResolver{
		logger:               c.logger,
		resolveForOpenShift:  c.resolveForOpenShift,
		imageCacher:          c.imageCacher,
		kubeClient:           c.kubeClient,
		operatorPodName:      c.operatorPodName,
		operatorPodNamespace: c.operatorPodNamespace,
		imageRegistry:        c.imageRegistry,
		registryMirrors:      c.registryMirrors,
		imagePullSecrets:     c.imagePullSecrets,
	}
}

func (c *containerImageResolver) WithImageRegistry(imageRegistry string) ContainerImageResolver {
	clone := c.clone()
	clone.imageRegistry = imageRegistry
	return clone
}

func (c *containerImageResolver) WithRegistryMirrors(registryMirrors map[string]string) ContainerImageResolver {
	clone := c.clone()
	clone.registryMirrors = registryMirrors
	return clone
}

func (c *containerImageResolver) WithImagePullSecrets(imagePullSecrets []corev1.LocalObjectReference) ContainerImageResolver {
	clone := c.clone()
	clone.imagePullSecrets = imagePullSecrets
	return clone
}

func userImageOrDefault(defaultImage, defaultTag, userImage, userTag, userDigest string) string {
	image := defaultImage
	if userImage != "" {
		image = userImage
	}

	// Digest takes precedence over tag
	if userDigest != "" {
		return fmt.Sprintf("%s@%s", image, userDigest)
	}

	tag := defaultTag
	if userTag != "" {
		tag = userTag
	}
	return fmt.Sprintf("%s:%s", image, tag)
}
