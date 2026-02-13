// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package imagecache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testImage              = "imageA:latest"
	testCurrentImageDigest = "imageA@sha256:CURRENT"
	testUpdatedImageDigest = "imageA@sha256:UPDATED"
)

func TestCache(t *testing.T) {
	tests := []struct {
		name            string
		imagesMap       map[string]imageData
		fetchImageFunc  func(string) (string, error)
		expectedImage   string
		extraValidation func(*testing.T, *imageCache)
		expectError     bool
	}{
		{
			name:          "image in cache",
			expectedImage: testCurrentImageDigest,
			imagesMap: map[string]imageData{
				"imageA:latest": {
					url:         testCurrentImageDigest,
					lastUpdated: time.Now(),
				},
			},
			fetchImageFunc: func(string) (string, error) {
				return "", fmt.Errorf("should not call fetchImage")
			},
		},
		{
			name:          "stale image in cache",
			expectedImage: testUpdatedImageDigest,
			imagesMap: map[string]imageData{
				"imageA:latest": {
					url:         "imageA@sha256:STALE",
					lastUpdated: time.Now().Add(-25 * time.Hour),
				},
			},
			fetchImageFunc: func(string) (string, error) {
				return testUpdatedImageDigest, nil
			},
		},
		{
			name:          "image not in cache",
			expectedImage: testCurrentImageDigest,
			imagesMap:     map[string]imageData{},
			fetchImageFunc: func(string) (string, error) {
				return testCurrentImageDigest, nil
			},
		},
		{
			name: "error during image fetching",
			fetchImageFunc: func(string) (string, error) {
				return "", fmt.Errorf("example error while fetching image")
			},
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			testCache := &imageCache{
				images:     test.imagesMap,
				fetchImage: test.fetchImageFunc,
			}

			// Act
			img, err := testCache.GetImage(testImage)

			// Assert
			if test.expectError {
				require.Error(t, err, "expected error during test case")
			} else {
				require.NoError(t, err, "unexpected error response during test")
				assert.Equal(t, test.expectedImage, img)

				if test.extraValidation != nil {
					test.extraValidation(t, testCache)
				}
			}
		})
	}
}

func TestKeychainFromSecrets(t *testing.T) {
	tests := []struct {
		name           string
		secrets        []corev1.Secret
		secretRefs     []corev1.LocalObjectReference
		registry       string
		expectUsername string
		expectPassword string
		expectAnon     bool
	}{
		{
			name:       "empty secret refs returns default keychain",
			secrets:    nil,
			secretRefs: nil,
			expectAnon: true,
		},
		{
			name: "secret not found is skipped",
			secretRefs: []corev1.LocalObjectReference{
				{Name: "nonexistent-secret"},
			},
			expectAnon: true,
		},
		{
			name: "dockerconfigjson with auth field",
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret",
						Namespace: "test-ns",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						".dockerconfigjson": mustMarshalDockerConfig(t, DockerConfigJSON{
							Auths: map[string]DockerConfigEntry{
								"ghcr.io": {
									Auth: base64.StdEncoding.EncodeToString([]byte("myuser:mypass")),
								},
							},
						}),
					},
				},
			},
			secretRefs: []corev1.LocalObjectReference{
				{Name: "my-secret"},
			},
			registry:       "ghcr.io",
			expectUsername: "myuser",
			expectPassword: "mypass",
		},
		{
			name: "dockerconfigjson with username/password fields",
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret",
						Namespace: "test-ns",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						".dockerconfigjson": mustMarshalDockerConfig(t, DockerConfigJSON{
							Auths: map[string]DockerConfigEntry{
								// Use index.docker.io as that's what go-containerregistry normalizes docker.io to
								"index.docker.io": {
									Username: "dockeruser",
									Password: "dockerpass",
								},
							},
						}),
					},
				},
			},
			secretRefs: []corev1.LocalObjectReference{
				{Name: "my-secret"},
			},
			registry:       "docker.io",
			expectUsername: "dockeruser",
			expectPassword: "dockerpass",
		},
		{
			name: "multiple secrets - first match wins",
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret1",
						Namespace: "test-ns",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						".dockerconfigjson": mustMarshalDockerConfig(t, DockerConfigJSON{
							Auths: map[string]DockerConfigEntry{
								"ghcr.io": {
									Username: "user1",
									Password: "pass1",
								},
							},
						}),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret2",
						Namespace: "test-ns",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						".dockerconfigjson": mustMarshalDockerConfig(t, DockerConfigJSON{
							Auths: map[string]DockerConfigEntry{
								"ghcr.io": {
									Username: "user2",
									Password: "pass2",
								},
							},
						}),
					},
				},
			},
			secretRefs: []corev1.LocalObjectReference{
				{Name: "secret1"},
				{Name: "secret2"},
			},
			registry:       "ghcr.io",
			expectUsername: "user1",
			expectPassword: "pass1",
		},
		{
			name: "registry with https prefix in secret",
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret",
						Namespace: "test-ns",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						".dockerconfigjson": mustMarshalDockerConfig(t, DockerConfigJSON{
							Auths: map[string]DockerConfigEntry{
								"https://ghcr.io": {
									Username: "httpsuser",
									Password: "httpspass",
								},
							},
						}),
					},
				},
			},
			secretRefs: []corev1.LocalObjectReference{
				{Name: "my-secret"},
			},
			registry:       "ghcr.io",
			expectUsername: "httpsuser",
			expectPassword: "httpspass",
		},
		{
			name: "secret with wrong data key is skipped",
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"wrongkey": []byte("somedata"),
					},
				},
			},
			secretRefs: []corev1.LocalObjectReference{
				{Name: "my-secret"},
			},
			expectAnon: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build fake client with secrets
			builder := fake.NewClientBuilder()
			for i := range tt.secrets {
				builder = builder.WithObjects(&tt.secrets[i])
			}
			kubeClient := builder.Build()

			keychain, err := KeychainFromSecrets(context.Background(), kubeClient, "test-ns", tt.secretRefs)
			require.NoError(t, err)

			if tt.registry == "" {
				return
			}

			// Create a fake resource to resolve auth for
			ref, err := name.ParseReference(tt.registry + "/test/image:latest")
			require.NoError(t, err)

			auth, err := keychain.Resolve(ref.Context())
			require.NoError(t, err)

			if tt.expectAnon {
				assert.Equal(t, authn.Anonymous, auth)
				return
			}

			authConfig, err := auth.Authorization()
			require.NoError(t, err)
			assert.Equal(t, tt.expectUsername, authConfig.Username)
			assert.Equal(t, tt.expectPassword, authConfig.Password)
		})
	}
}

func mustMarshalDockerConfig(t *testing.T, config DockerConfigJSON) []byte {
	data, err := json.Marshal(config)
	require.NoError(t, err)
	return data
}
