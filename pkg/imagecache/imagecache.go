// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package imagecache

import (
	"strings"
	"sync"
	"time"

	ecr "github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	refreshPeriod = time.Hour * 24
)

type ImageCacher interface {
	GetImage(string) (string, error)
}

type imageCache struct {
	images      map[string]imageData
	imagesMutex sync.RWMutex
	fetchImage  func(string) (string, error)
}

type imageData struct {
	url         string
	lastUpdated time.Time
}

// GetImage will return a "recent" (ie less than refreshPeriod old) image
// with SHA if a recent cache entry is available. Otherwise, it will
// create/update any entry and return an up-to-date image+sha
func (i *imageCache) GetImage(image string) (string, error) {
	sha, err := i.getImageWithSHA(image)
	if err != nil {
		return "", err
	}

	return sha, nil
}

func (i *imageCache) getImageWithSHA(image string) (string, error) {
	i.imagesMutex.Lock()
	defer i.imagesMutex.Unlock()

	img, ok := i.images[image]
	if !ok {
		if err := i.updateImage(image); err != nil {
			return "", err
		}
		img = i.images[image]
	}

	// refresh, if image data is stale
	if img.lastUpdated.Add(refreshPeriod).Before(time.Now()) {
		if err := i.updateImage(image); err != nil {
			return "", err
		}
		img = i.images[image]
	}

	return img.url, nil
}

// updateImage will make a query out to the registry and store the sha for the image
func (i *imageCache) updateImage(image string) error {
	imageUrl, err := i.fetchImage(image)
	if err != nil {
		return err
	}

	i.images[image] = imageData{
		url:         imageUrl,
		lastUpdated: time.Now(),
	}

	return nil
}

func queryImageWithSHA(image string) (string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", err
	}

	var kc authn.Keychain = authn.DefaultKeychain
	if strings.Contains(ref.Name(), ".ecr.") {
		kc = authn.NewMultiKeychain(
			authn.DefaultKeychain,
			authn.NewKeychainFromHelper(ecr.NewECRHelper()),
		)
	}

	desc, err := remote.Get(ref, remote.WithAuthFromKeychain(kc))
	if err != nil {
		return "", err
	}
	imgDigest := desc.Digest.String()
	repoName := ref.Context().Name()
	imageUrl := repoName + "@" + imgDigest

	return imageUrl, nil
}

func NewImageCacher() ImageCacher {
	return &imageCache{
		images:     map[string]imageData{},
		fetchImage: queryImageWithSHA,
	}
}
