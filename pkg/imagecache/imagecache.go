// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package imagecache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	refreshPeriod = time.Hour * 24
)

type ImageCacher interface {
	GetImage(string) (string, error)
	// WithAuth returns a new ImageCacher that uses the provided authentication keychain
	WithAuth(keychain authn.Keychain) ImageCacher
}

type imageCache struct {
	images      map[string]imageData
	imagesMutex sync.RWMutex
	fetchImage  func(string) (string, error)
	keychain    authn.Keychain
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

func queryImageWithSHA(image string, keychain authn.Keychain) (string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", err
	}

	opts := []remote.Option{}
	if keychain != nil {
		opts = append(opts, remote.WithAuthFromKeychain(keychain))
	}

	desc, err := remote.Get(ref, opts...)
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
		images: map[string]imageData{},
		fetchImage: func(image string) (string, error) {
			return queryImageWithSHA(image, nil)
		},
	}
}

func (i *imageCache) WithAuth(keychain authn.Keychain) ImageCacher {
	return &imageCache{
		images: map[string]imageData{},
		fetchImage: func(image string) (string, error) {
			return queryImageWithSHA(image, keychain)
		},
		keychain: keychain,
	}
}

// DockerConfigJSON represents the structure of a Docker config.json file
type DockerConfigJSON struct {
	Auths map[string]DockerConfigEntry `json:"auths"`
}

// DockerConfigEntry represents a single registry entry in Docker config
type DockerConfigEntry struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Auth     string `json:"auth,omitempty"`
}

// KeychainFromSecrets creates an authn.Keychain from Kubernetes imagePullSecrets
func KeychainFromSecrets(ctx context.Context, kubeClient client.Client, namespace string, secretRefs []corev1.LocalObjectReference) (authn.Keychain, error) {
	if len(secretRefs) == 0 {
		return authn.DefaultKeychain, nil
	}

	var configs []DockerConfigJSON
	for _, secretRef := range secretRefs {
		secret := &corev1.Secret{}
		if err := kubeClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretRef.Name}, secret); err != nil {
			continue // Skip secrets that don't exist
		}

		// Handle both .dockerconfigjson and .dockercfg formats
		var configData []byte
		if data, ok := secret.Data[".dockerconfigjson"]; ok {
			configData = data
		} else if data, ok := secret.Data[".dockercfg"]; ok {
			configData = data
		} else {
			continue
		}

		var config DockerConfigJSON
		if err := json.Unmarshal(configData, &config); err != nil {
			continue
		}
		configs = append(configs, config)
	}

	return &multiKeychain{configs: configs}, nil
}

// multiKeychain implements authn.Keychain for multiple Docker configs
type multiKeychain struct {
	configs []DockerConfigJSON
}

func (k *multiKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	registry := resource.RegistryStr()

	for _, config := range k.configs {
		if entry, ok := config.Auths[registry]; ok {
			return resolveAuth(entry)
		}
		// Also try with https:// prefix
		if entry, ok := config.Auths["https://"+registry]; ok {
			return resolveAuth(entry)
		}
	}

	return authn.Anonymous, nil
}

func resolveAuth(entry DockerConfigEntry) (authn.Authenticator, error) {
	if entry.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
		if err != nil {
			return authn.Anonymous, nil
		}
		// Auth is base64 encoded "username:password"
		parts := string(decoded)
		for i, c := range parts {
			if c == ':' {
				return authn.FromConfig(authn.AuthConfig{
					Username: parts[:i],
					Password: parts[i+1:],
				}), nil
			}
		}
	}

	if entry.Username != "" && entry.Password != "" {
		return authn.FromConfig(authn.AuthConfig{
			Username: entry.Username,
			Password: entry.Password,
		}), nil
	}

	return authn.Anonymous, nil
}
