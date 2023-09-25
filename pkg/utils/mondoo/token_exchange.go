// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	jwt "github.com/golang-jwt/jwt/v4"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

type MondooClientBuilder func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error)

// CreateServiceAccountFromToken will take the provided Mondoo token and exchange it with the Mondoo API
// for a long lived Mondoo ServiceAccount
func CreateServiceAccountFromToken(ctx context.Context, kubeClient client.Client, mondooClientBuilder MondooClientBuilder, withConsoleIntegration bool, serviceAccountSecret types.NamespacedName, tokenSecretData string, httpProxy *string, log logr.Logger) error {
	jwtString := strings.TrimSpace(tokenSecretData)

	parser := &jwt.Parser{}
	token, _, err := parser.ParseUnverified(jwtString, jwt.MapClaims{})
	if err != nil {
		log.Error(err, "failed to parse token")
		return err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		err := fmt.Errorf("failed to type asesrt claims from token")
		log.Error(err, "failed to extract claim")
		return err
	}
	apiEndpoint := claims["api_endpoint"]

	opts := mondooclient.MondooClientOptions{
		ApiEndpoint: fmt.Sprintf("%v", apiEndpoint),
		Token:       jwtString,
		HttpProxy:   httpProxy,
	}

	mClient, err := mondooClientBuilder(opts)
	if err != nil {
		return err
	}

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountSecret.Name,
			Namespace: serviceAccountSecret.Namespace,
		},
	}
	if withConsoleIntegration {
		// owner is the MRN of the integration
		tokenOwner, ok := claims["owner"]
		if !ok {
			err := fmt.Errorf("'owner' claim missing from token which is expected for Mondoo integration registration")
			log.Error(err, "missing data in token Secret")
			return err
		}
		// Do an integration-style registration to associate the generated
		// service account with the Mondoo console Integration
		resp, err := mClient.IntegrationRegister(ctx, &mondooclient.IntegrationRegisterInput{
			Mrn:   fmt.Sprintf("%v", tokenOwner),
			Token: jwtString,
		})
		if err != nil {
			log.Error(err, "failed to exchange token for a service account")
			return err
		}

		integrationMrn := resp.Mrn
		credsBytes, err := json.Marshal(*resp.Creds)
		if err != nil {
			log.Error(err, "failed to marshal service account creds from IntegrationRegister()")
			return err
		}
		tokenSecret.Data = map[string][]byte{
			constants.MondooCredsSecretServiceAccountKey: credsBytes,
			constants.MondooCredsSecretIntegrationMRNKey: []byte(integrationMrn),
		}
		_, err = k8s.CreateIfNotExist(ctx, kubeClient, tokenSecret, tokenSecret)
		if err != nil {
			log.Error(err, "error while trying to save Mondoo service account into secret")
			return err
		}

		// No easy way to retry this one-off CheckIn(). An error on initial CheckIn()
		// means we'll just retry on the regularly scheduled interval via the integration controller
		_ = performInitialCheckIn(ctx, mondooClientBuilder, integrationMrn, *resp.Creds, httpProxy, log)
	} else {
		// Do a vanilla token-for-service-account exchange
		resp, err := mClient.ExchangeRegistrationToken(ctx, &mondooclient.ExchangeRegistrationTokenInput{
			Token: jwtString,
		})
		if err != nil {
			log.Error(err, "failed to exchange token for a service account")
			return err
		}

		// Save the service account
		tokenSecret.StringData = map[string]string{
			constants.MondooCredsSecretServiceAccountKey: resp.ServiceAccount,
		}
		_, err = k8s.CreateIfNotExist(ctx, kubeClient, tokenSecret, tokenSecret)
		if err != nil {
			log.Error(err, "error while trying to save Mondoo service account into secret")
			return err
		}
	}

	log.Info("saved Mondoo service account", "secret", fmt.Sprintf("%s/%s", serviceAccountSecret.Namespace, serviceAccountSecret.Name))

	return nil
}

func performInitialCheckIn(ctx context.Context, mondooClientBuilder MondooClientBuilder, integrationMrn string, sa mondooclient.ServiceAccountCredentials, httpProxy *string, logger logr.Logger) error {
	if err := IntegrationCheckIn(ctx, integrationMrn, sa, mondooClientBuilder, httpProxy, logger); err != nil {
		logger.Error(err, "initial CheckIn() failed, will CheckIn() periodically", "integrationMRN", integrationMrn)
		return err
	}
	return nil
}
