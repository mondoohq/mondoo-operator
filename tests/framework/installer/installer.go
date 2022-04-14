package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mondoov1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
)

const (
	OperatorManifest        = "mondoo-operator-manifests.yaml"
	AuditConfigManifest     = "config/samples/k8s_v1alpha1_mondooauditconfig.yaml"
	MondooCredsFile         = "creds.json"
	MondooClientsLabel      = "mondoo_cr=mondoo-client"
	MondooClientsNodesLabel = "audit=node"
	MondooClientsK8sLabel   = "audit=k8s"
)

type MondooInstaller struct {
	T           func() *testing.T
	Settings    Settings
	K8sHelper   *utils.K8sHelper
	isInstalled bool
	ctx         context.Context
}

func NewMondooInstaller(settings Settings, t func() *testing.T) *MondooInstaller {
	k8sHelper, err := utils.CreateK8sHelper()
	if err != nil {
		panic("failed to get kubectl client :" + err.Error())
	}

	return &MondooInstaller{
		T:         t,
		Settings:  settings,
		K8sHelper: k8sHelper,
		ctx:       context.Background(),
	}
}

func (i *MondooInstaller) InstallOperator() error {
	rootFolder, err := utils.FindRootFolder()
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(rootFolder, OperatorManifest)); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("File %q does not exist. Run %q!", OperatorManifest, "make generate-manifests")
	}

	_, err = i.K8sHelper.KubectlWithStdin(i.readManifestWithNamespace(OperatorManifest), utils.CreateFromStdinArgs...)
	i.isInstalled = true // If the command above has run there is a change things have been partially created.
	if err != nil {
		return fmt.Errorf("Failed to create mondoo-operator pod: %v ", err)
	}

	if err := i.CreateClientSecret(i.Settings.Namespace); err != nil {
		return err
	}

	if !i.K8sHelper.IsPodReady("control-plane=controller-manager", i.Settings.Namespace) {
		return fmt.Errorf("Mondoo operator is not in a ready state.")
	}
	zap.S().Info("Mondoo operator is ready.")

	return nil
}

func (i *MondooInstaller) UninstallOperator() error {
	// If the operator has not been installed do nothing
	if !i.isInstalled {
		zap.S().Warn("Operator not installed. Skip gathering logs...")
		return nil
	}
	i.K8sHelper.GetLogsFromNamespace(i.Settings.Namespace, i.T().Name())

	if err := i.CleanupAuditConfigs(); err != nil {
		return err
	}

	secret := &corev1.Secret{}
	secret.Name = utils.MondooClientSecret
	secret.Namespace = i.Settings.Namespace
	if err := i.K8sHelper.DeleteResourceIfExists(secret); err != nil {
		return err
	}

	_, err := i.K8sHelper.KubectlWithStdin(i.readManifestWithNamespace(OperatorManifest), utils.DeleteIngoreNotFoundFromStdinArgs...)
	if err != nil {
		return fmt.Errorf("Failed to delete mondoo-operator pod: %v ", err)
	}
	return nil
}

func (i *MondooInstaller) CleanupAuditConfigs() error {
	// Make sure all Mondoo audit configs are deleted so the namespace can be deleted. Leaving
	// audit configs will result in a stuck namespace.
	cfgs := &mondoov1.MondooAuditConfigList{}
	if err := i.K8sHelper.Clientset.List(i.ctx, cfgs); err != nil {
		return fmt.Errorf("Failed to get Mondoo audit configs. %v", err)
	}

	for _, c := range cfgs.Items {
		if err := i.K8sHelper.Clientset.Delete(i.ctx, &c); err != nil {
			return fmt.Errorf("Failed to delete Mondoo audit config %s/%s. %v", c.Namespace, c.Name, err)
		}

		// Wait until the CRs are fully deleted.
		if err := i.K8sHelper.WaitForResourceDeletion(&c); err != nil {
			return err
		}
	}
	return nil
}

func (i *MondooInstaller) CreateClientSecret(ns string) error {
	secret := corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"config": []byte(utils.ReadFile(MondooCredsFile)),
		},
	}
	secret.Name = utils.MondooClientSecret
	secret.Namespace = ns
	if err := i.K8sHelper.Clientset.Create(i.ctx, &secret); err != nil {
		return fmt.Errorf("Failed to create Мondoo secret. %v", err)
	}
	zap.S().Infof("Created Мondoo client secret %q.", utils.MondooClientSecret)
	return nil
}

func (i *MondooInstaller) GatherAllMondooLogs(testName string, namespaces ...string) {
	zap.S().Infof("gathering all logs from the test")
	for _, namespace := range namespaces {
		i.K8sHelper.GetLogsFromNamespace(namespace, testName)
		i.K8sHelper.GetPodDescribeFromNamespace(namespace, testName)
		i.K8sHelper.GetEventsFromNamespace(namespace, testName)
	}
}

// readManifestWithNamespace Reads the specified manifest and replaces all namespace specification with the
// provided namespace.
func (i *MondooInstaller) readManifestWithNamespace(manifest string) string {
	original := utils.ReadFile(manifest)
	if i.Settings.Namespace == MondooNamespace {
		return original
	}

	// TODO: The first occurence of "name: mondoo-operator" is actually the namespace that is going to be
	// create. This hack allows us to change the name of that namespace. It is needed because all resources
	// are bundled in a single file and we cannot create the namespace separately at the moment.
	updatedNamespace := strings.Replace(
		original, "name: mondoo-operator", fmt.Sprintf("name: %s", i.Settings.Namespace), 1)

	return strings.ReplaceAll(
		updatedNamespace,
		"namespace: mondoo-operator",
		fmt.Sprintf("namespace: %s", i.Settings.Namespace))
}
