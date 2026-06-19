// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package constants

const (
	// Google Cloud SDK image - slim variant for smaller size
	// https://cloud.google.com/sdk/docs/downloads-docker
	GCloudSDKImage = "gcr.io/google.com/cloudsdktool/google-cloud-cli:499.0.0-slim"

	// AWS CLI image
	// https://hub.docker.com/r/amazon/aws-cli
	AWSCLIImage = "amazon/aws-cli:2.22.0"

	// Azure CLI image
	// https://mcr.microsoft.com/en-us/artifact/mar/azure-cli/tags
	AzureCLIImage = "mcr.microsoft.com/azure-cli:2.67.0"

	// SPIFFE Helper image
	// https://github.com/spiffe/spiffe-helper/releases
	SPIFFEHelperImage = "ghcr.io/spiffe/spiffe-helper:0.8.0"

	// Curl image used by lightweight auth init containers
	// https://github.com/curl/curl-container
	CurlImage = "curlimages/curl:8.17.0"
)
