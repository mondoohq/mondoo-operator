// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"go.mondoo.com/cnquery/v9/cli/config"
	"go.mondoo.com/cnquery/v9/providers-sdk/v1/upstream"
)

const ServiceAccountEnv = "MONDOO_SERVICE_ACCOUNT_EDGE"

func GetServiceAccount() (*upstream.ServiceAccountCredentials, error) {
	saBase64, ok := os.LookupEnv(ServiceAccountEnv)
	if !ok {
		return nil, fmt.Errorf("Service account not found in environment variable %s", ServiceAccountEnv)
	}

	saString, err := base64.StdEncoding.DecodeString(saBase64)
	if err != nil {
		return nil, err
	}

	config := &config.CommonOpts{}
	err = json.Unmarshal(saString, config)
	if err != nil {
		return nil, err
	}
	return config.GetServiceCredential(), err
}
