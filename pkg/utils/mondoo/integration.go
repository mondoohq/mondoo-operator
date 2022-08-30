/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
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
