package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
)

const (
	OperatorManifest    = "mondoo-operator-manifests.yaml"
	AuditConfigManifest = "config/samples/k8s_v1alpha1_mondooauditconfig.yaml"
	MondooClientSecret  = "mondoo-client"
	MondooCredsFile     = "creds.json"
)

type MondooInstaller struct {
	T           func() *testing.T
	Settings    Settings
	k8sHelper   *utils.K8sHelper
	isInstalled bool
}

func NewMondooInstaller(settings Settings, t func() *testing.T) *MondooInstaller {
	k8sHelper, err := utils.CreateK8sHelper()
	if err != nil {
		panic("failed to get kubectl client :" + err.Error())
	}

	return &MondooInstaller{
		T:         t,
		Settings:  settings,
		k8sHelper: k8sHelper,
	}
}

func (i *MondooInstaller) InstallOperator() error {
	rootFolder, err := utils.FindRootFolder()
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(rootFolder, OperatorManifest)); errors.Is(err, os.ErrNotExist) {
		return errors.Errorf("File %q does not exist. Run %q!", OperatorManifest, "make generate-manifests")
	}

	_, err = i.k8sHelper.KubectlWithStdin(i.readManifestWithNamespace(OperatorManifest), utils.CreateFromStdinArgs...)
	i.isInstalled = true // If the command above has run there is a change things have been partially created.
	if err != nil {
		return errors.Errorf("Failed to create mondoo-operator pod: %v ", err)
	}

	ctx := context.TODO()
	secret := corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"config": []byte(utils.ReadFile(MondooCredsFile)),
		},
	}
	secret.Name = MondooClientSecret
	secret.Namespace = i.Settings.Namespace
	if err := i.k8sHelper.Clientset.Create(ctx, &secret); err != nil {
		return errors.Errorf("Failed to create mondoo secret. %v", err)
	}
	zap.S().Infof("Created mondoo client secret %q.", MondooClientSecret)

	if !i.k8sHelper.IsPodReady("control-plane=controller-manager", i.Settings.Namespace) {
		return errors.Errorf("Mondoo operator is not in a ready state.")
	}
	zap.S().Info("Mondoo operator is ready.")

	_, err = i.k8sHelper.KubectlWithStdin(i.readManifestWithNamespace(AuditConfigManifest), utils.CreateFromStdinArgs...)
	if err != nil {
		return errors.Errorf("Failed to create audit config crd: %v ", err)
	}

	if !i.k8sHelper.IsPodReady("mondoo_cr=mondoo-client", i.Settings.Namespace) {
		return errors.Errorf("Mondoo client is not in a ready state.")
	}
	zap.S().Info("Mondoo clients are ready.")

	return nil
}

func (i *MondooInstaller) UninstallOperator() error {
	// If the operator has not been installed do nothing
	if !i.isInstalled {
		zap.S().Warn("Operator not installed. Skip gathering logs...")
		return nil
	}
	i.k8sHelper.GetLogsFromNamespace(i.Settings.Namespace, i.T().Name())

	ctx := context.TODO()
	secret := &corev1.Secret{}
	secret.Name = MondooClientSecret
	secret.Namespace = i.Settings.Namespace
	if err := i.k8sHelper.Clientset.Delete(ctx, secret); err != nil {
		return errors.Errorf("Failed to delete mondoo-client secret. %v", err)
	}

	_, err := i.k8sHelper.KubectlWithStdin(i.readManifestWithNamespace(AuditConfigManifest), utils.DeleteFromStdinArgs...)
	if err != nil {
		return errors.Errorf("Failed to delete mondoo-operator pod: %v ", err)
	}

	_, err = i.k8sHelper.KubectlWithStdin(i.readManifestWithNamespace(OperatorManifest), utils.DeleteFromStdinArgs...)
	if err != nil {
		return errors.Errorf("Failed to delete mondoo-operator pod: %v ", err)
	}
	return nil
}

func (i *MondooInstaller) GatherAllMondooLogs(testName string, namespaces ...string) {
	zap.S().Infof("gathering all logs from the test")
	for _, namespace := range namespaces {
		i.k8sHelper.GetLogsFromNamespace(namespace, testName)
		i.k8sHelper.GetPodDescribeFromNamespace(namespace, testName)
		i.k8sHelper.GetEventsFromNamespace(namespace, testName)
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
