// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package imagecache

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
