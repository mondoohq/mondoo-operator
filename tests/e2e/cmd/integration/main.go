// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	mondoogql "go.mondoo.com/mondoo-go"
	"go.mondoo.com/mondoo-go/option"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <create|delete>\n", os.Args[0])
		os.Exit(1)
	}

	switch os.Args[1] {
	case "create":
		if err := createIntegration(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "delete":
		if err := deleteIntegration(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func loadSAData() ([]byte, error) {
	if path := os.Getenv("MONDOO_CONFIG_PATH"); path != "" {
		return os.ReadFile(path) //nolint:gosec // trusted env-var path from CI
	}
	if b64 := os.Getenv("MONDOO_CONFIG_BASE64"); b64 != "" {
		return base64.StdEncoding.DecodeString(b64)
	}
	if b64 := os.Getenv("MONDOO_CREDS_B64"); b64 != "" {
		return base64.StdEncoding.DecodeString(b64)
	}
	return nil, fmt.Errorf("set MONDOO_CONFIG_PATH, MONDOO_CONFIG_BASE64, or MONDOO_CREDS_B64")
}

func newClient() (*mondoogql.Client, error) {
	data, err := loadSAData()
	if err != nil {
		return nil, err
	}
	return mondoogql.NewClient(option.WithServiceAccount(data))
}

func createIntegration() error {
	spaceMrn := os.Getenv("MONDOO_SPACE_MRN")
	if spaceMrn == "" {
		return fmt.Errorf("MONDOO_SPACE_MRN must be set")
	}
	name := os.Getenv("INTEGRATION_NAME")
	if name == "" {
		name = "e2e-k8s-integration"
	}

	client, err := newClient()
	if err != nil {
		return err
	}

	var m struct {
		CreateIntegration struct {
			Integration struct {
				Mrn   string
				Token string
			}
		} `graphql:"createClientIntegration(input: $input)"`
	}
	err = client.Mutate(context.Background(), &m, mondoogql.CreateClientIntegrationInput{
		SpaceMrn: mondoogql.NewStringPtr(mondoogql.String(spaceMrn)),
		Name:     mondoogql.String(name),
		Type:     mondoogql.ClientIntegrationTypeK8s,
		ConfigurationOptions: mondoogql.ClientIntegrationConfigurationInput{
			K8sConfigurationOptions: &mondoogql.K8sConfigurationOptionsInput{
				ScanNodes:        mondoogql.Boolean(true),
				ScanWorkloads:    mondoogql.Boolean(true),
				ScanPublicImages: mondoogql.NewBooleanPtr(mondoogql.Boolean(true)),
			},
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("createClientIntegration: %w", err)
	}

	fmt.Printf("MRN=%s\n", m.CreateIntegration.Integration.Mrn)
	fmt.Printf("TOKEN=%s\n", m.CreateIntegration.Integration.Token)
	return nil
}

func deleteIntegration() error {
	mrn := os.Getenv("INTEGRATION_MRN")
	if mrn == "" {
		return fmt.Errorf("INTEGRATION_MRN must be set")
	}

	client, err := newClient()
	if err != nil {
		return err
	}

	var m struct {
		DeleteIntegration struct {
			Mrn string
		} `graphql:"deleteClientIntegration(input: $input)"`
	}
	return client.Mutate(context.Background(), &m, mondoogql.DeleteClientIntegrationInput{
		Mrn: mondoogql.String(mrn),
	}, nil)
}
