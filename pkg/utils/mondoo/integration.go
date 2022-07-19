/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mondoo

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
)

func IntegrationCheckIn(
	ctx context.Context,
	integrationMrn string,
	sa mondooclient.ServiceAccountCredentials,
	mondooClientBuilder func(mondooclient.ClientOptions) mondooclient.Client,
	logger logr.Logger,
) error {
	token, err := GenerateTokenFromServiceAccount(sa, logger)
	if err != nil {
		msg := "unable to generate token from service account"
		return fmt.Errorf("%s: %s", msg, err)
	}
	mondooClient := mondooClientBuilder(mondooclient.ClientOptions{
		ApiEndpoint: sa.ApiEndpoint,
		Token:       token,
	})

	// Do the actual check-in
	if _, err := mondooClient.IntegrationCheckIn(ctx, &mondooclient.IntegrationCheckInInput{
		Mrn: integrationMrn,
	}); err != nil {
		msg := "failed to CheckIn() to Mondoo API"
		return fmt.Errorf("%s: %s", msg, err)
	}

	return nil
}
