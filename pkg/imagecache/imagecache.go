// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package imagecache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"go.mondoo.com/mql/v13/providers/os/connection/container/auth"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	refreshPeriod = time.Hour * 24
)

type ImageCacher interface {
	GetImage(string) (string, error)
	// GetImageVersion returns the OCI version label for a previously resolved image.
	GetImageVersion(image string) string
	// WithAuth returns a new ImageCacher that uses the provided authentication keychain
	WithAuth(keychain authn.Keychain) ImageCacher
}

type imageCache struct {
	images      map[string]imageData
	imagesMutex *sync.RWMutex
	fetchImage  func(string) (fetchResult, error)
	keychain    authn.Keychain
}

type fetchResult struct {
	url     string
	version string
}

type imageData struct {
	url         string
	version     string
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

func (i *imageCache) GetImageVersion(image string) string {
	i.imagesMutex.RLock()
	defer i.imagesMutex.RUnlock()

	if img, ok := i.images[image]; ok {
		return img.version
	}
	return ""
}

// updateImage will make a query out to the registry and store the sha for the image
func (i *imageCache) updateImage(image string) error {
	result, err := i.fetchImage(image)
	if err != nil {
		return err
	}

	i.images[image] = imageData{
		url:         result.url,
		version:     result.version,
		lastUpdated: time.Now(),
	}

	return nil
}

func queryImageWithSHA(image string, keychain authn.Keychain) (fetchResult, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return fetchResult{}, err
	}

	// Use provided keychain if given, otherwise use cnquery's auth which supports ECR, GCR, etc.
	var kc authn.Keychain
	if keychain != nil {
		kc = keychain
	} else {
		kc = auth.ConstructKeychain(ref.Name())
	}

	desc, err := remote.Get(ref, remote.WithAuthFromKeychain(kc))
	if err != nil {
		return fetchResult{}, err
	}
	imgDigest := desc.Digest.String()
	repoName := ref.Context().Name()
	imageUrl := repoName + "@" + imgDigest

	version := extractVersionLabel(ref, desc, kc)

	return fetchResult{url: imageUrl, version: version}, nil
}

func extractVersionLabel(ref name.Reference, desc *remote.Descriptor, kc authn.Keychain) string {
	cf, err := configFileFromDescriptor(ref, desc, kc)
	if err != nil || cf == nil {
		return ""
	}
	version := cf.Config.Labels["org.opencontainers.image.version"]
	for _, suffix := range []string{"-ubi-rootless", "-rootless"} {
		if v, ok := strings.CutSuffix(version, suffix); ok {
			return v
		}
	}
	return version
}

// configFileFromDescriptor extracts the image config, handling both single-arch
// images and multi-arch manifest lists (picking the first child manifest).
func configFileFromDescriptor(ref name.Reference, desc *remote.Descriptor, kc authn.Keychain) (*v1.ConfigFile, error) {
	if desc.MediaType.IsImage() {
		img, err := desc.Image()
		if err != nil {
			return nil, err
		}
		return img.ConfigFile()
	}

	if !desc.MediaType.IsIndex() {
		return nil, fmt.Errorf("unexpected media type: %s", desc.MediaType)
	}

	idx, err := desc.ImageIndex()
	if err != nil {
		return nil, err
	}
	manifest, err := idx.IndexManifest()
	if err != nil {
		return nil, err
	}
	if len(manifest.Manifests) == 0 {
		return nil, fmt.Errorf("empty image index")
	}
	childRef := ref.Context().Digest(manifest.Manifests[0].Digest.String())
	childDesc, err := remote.Get(childRef, remote.WithAuthFromKeychain(kc))
	if err != nil {
		return nil, err
	}
	img, err := childDesc.Image()
	if err != nil {
		return nil, err
	}
	return img.ConfigFile()
}

func NewImageCacher() ImageCacher {
	return &imageCache{
		images:      map[string]imageData{},
		imagesMutex: &sync.RWMutex{},
		fetchImage: func(image string) (fetchResult, error) {
			return queryImageWithSHA(image, nil)
		},
	}
}

func (i *imageCache) WithAuth(keychain authn.Keychain) ImageCacher {
	return &imageCache{
		images:      i.images,
		imagesMutex: i.imagesMutex,
		fetchImage: func(image string) (fetchResult, error) {
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
	Password string `json:"password,omitempty"` //nolint:gosec
	Auth     string `json:"auth,omitempty"`
}

// KeychainFromSecrets creates an authn.Keychain from Kubernetes imagePullSecrets
func KeychainFromSecrets(ctx context.Context, kubeClient client.Client, namespace string, secretRefs []corev1.LocalObjectReference) (authn.Keychain, error) {
	log := ctrl.Log.WithName("imagecache")

	if len(secretRefs) == 0 {
		return authn.DefaultKeychain, nil
	}

	var configs []DockerConfigJSON
	for _, secretRef := range secretRefs {
		secret := &corev1.Secret{}
		if err := kubeClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretRef.Name}, secret); err != nil {
			if errors.IsNotFound(err) {
				log.Info("imagePullSecret not found, skipping", "secret", secretRef.Name, "namespace", namespace)
				continue
			}
			return nil, fmt.Errorf("failed to get imagePullSecret %s/%s: %w", namespace, secretRef.Name, err)
		}

		// Handle both .dockerconfigjson and .dockercfg formats
		var configData []byte
		if data, ok := secret.Data[".dockerconfigjson"]; ok {
			configData = data
		} else if data, ok := secret.Data[".dockercfg"]; ok {
			configData = data
		} else {
			log.Info("imagePullSecret has no .dockerconfigjson or .dockercfg data, skipping", "secret", secretRef.Name, "namespace", namespace)
			continue
		}

		var config DockerConfigJSON
		if err := json.Unmarshal(configData, &config); err != nil {
			log.Error(err, "failed to parse imagePullSecret data", "secret", secretRef.Name, "namespace", namespace)
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

	// Build candidate keys to look up in the docker config auths map.
	// Docker configs can use various formats: "registry.io", "https://registry.io",
	// "https://registry.io/v1/", "https://registry.io/v2/", etc.
	candidates := []string{
		registry,
		"https://" + registry,
		"https://" + registry + "/v1/",
		"https://" + registry + "/v2/",
	}

	for _, config := range k.configs {
		for _, candidate := range candidates {
			if entry, ok := config.Auths[candidate]; ok {
				return resolveAuth(entry)
			}
		}
	}

	return authn.Anonymous, nil
}

func resolveAuth(entry DockerConfigEntry) (authn.Authenticator, error) {
	if entry.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
		if err != nil {
			return nil, fmt.Errorf("failed to decode auth field: %w", err)
		}
		// Auth is base64 encoded "username:password"
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) == 2 {
			return authn.FromConfig(authn.AuthConfig{
				Username: parts[0],
				Password: parts[1],
			}), nil
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
