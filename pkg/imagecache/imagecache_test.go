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
		name                string
		imagesMap           map[string]imageData
		fetchImageFunc      func(string) (string, error)
		expectedImage       string
		extraValidation     func(*testing.T, *imageCache)
		overrideLastCleanup time.Time
		expectError         bool
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
			name:          "recent image in cache",
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
			name:          "clean up old images",
			expectedImage: testCurrentImageDigest,
			imagesMap: map[string]imageData{
				"stale:image": {
					url:         "stale@sha256:abcdefg",
					lastUpdated: time.Now().Add(-25 * time.Hour),
				},
				"imageA:latest": {
					url:         testCurrentImageDigest,
					lastUpdated: time.Now().Add(-2 * time.Hour),
				},
			},
			extraValidation: func(t *testing.T, imageCache *imageCache) {
				assert.Equal(t, 1, len(imageCache.images), "expected old image data to be cleaned up")
			},
			overrideLastCleanup: time.Now().Add(-25 * time.Hour),
			fetchImageFunc: func(string) (string, error) {
				return "", fmt.Errorf("should not call fetchImage")
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
				images:      test.imagesMap,
				fetchImage:  test.fetchImageFunc,
				lastCleanup: time.Now(),
			}

			if !test.overrideLastCleanup.IsZero() {
				testCache.lastCleanup = test.overrideLastCleanup
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
