// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"context"
	"fmt"
	"os"

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
}

type containerImageResolver struct {
	logger               logr.Logger
	resolveForOpenShift  bool
	imageCacher          imagecache.ImageCacher
	kubeClient           client.Client
	operatorPodName      string
	operatorPodNamespace string
}

func NewContainerImageResolver(kubeClient client.Client, isOpenShift bool) ContainerImageResolver {
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

	return c.resolveImage(image, skipImageResolution)
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

	return c.resolveImage(image, skipImageResolution)
}

func (c *containerImageResolver) resolveImage(image string, skipImageResolution bool) (string, error) {
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
