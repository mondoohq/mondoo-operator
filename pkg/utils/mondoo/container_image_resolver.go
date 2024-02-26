// Copyright (c) Mondoo, Inc.
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
	CnspecTag                = "10-rootless"
	OpenShiftMondooClientTag = "10-ubi-rootless"
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

	// First try with the provided user image and tag (if any)
	if userImage != "" || userTag != "" {
		image = userImageOrDefault(MondooOperatorImage, MondooOperatorTag, userImage, userTag)
	}

	// If still no image, then load the image from the operator pod
	if image == "" {
		operatorPod := &corev1.Pod{}
		if err := c.kubeClient.Get(ctx, client.ObjectKey{Namespace: c.operatorPodNamespace, Name: c.operatorPodName}, operatorPod); err != nil {
			return "", err
		}
		image = operatorPod.Spec.Containers[0].Image
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
