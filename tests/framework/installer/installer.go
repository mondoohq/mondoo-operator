package installer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	OperatorManifest           = "mondoo-operator-manifests.yaml"
	AuditConfigManifest        = "config/samples/k8s_v1alpha2_mondooauditconfig_minimal.yaml"
	MondooCredsFile            = "creds.json"
	MondooClientsLabel         = "mondoo_cr=mondoo-client"
	MondooClientsNodesLabel    = "audit=node"
	MondooClientsK8sLabel      = "audit=k8s"
	ExternalInstallationEnvVar = "EXTERNAL_INSTALLATION"
	PreviousVersionEnvVar      = "PREVIOUS_RELEASE"
)

type MondooInstaller struct {
	T                     func() *testing.T
	Settings              Settings
	K8sHelper             *utils.K8sHelper
	isInstalled           bool
	ctx                   context.Context
	isInstalledExternally bool
	PreviousVersion       string
}

func NewMondooInstaller(settings Settings, t func() *testing.T) *MondooInstaller {
	k8sHelper, err := utils.CreateK8sHelper()
	if err != nil {
		panic("failed to get kubectl client :" + err.Error())
	}

	_, externalInstall := os.LookupEnv(ExternalInstallationEnvVar)

	previousVersion, found := os.LookupEnv(PreviousVersionEnvVar)
	if !found {
		panic("failed to get previous version tag for this repo")
	}

	return &MondooInstaller{
		T:                     t,
		Settings:              settings,
		K8sHelper:             k8sHelper,
		ctx:                   context.Background(),
		isInstalled:           externalInstall,
		isInstalledExternally: externalInstall,
		PreviousVersion:       previousVersion,
	}
}

func (i *MondooInstaller) InstallOperator() error {
	if i.isInstalledExternally {
		if err := i.CreateClientSecret(i.Settings.Namespace); err != nil {
			return err
		}

		zap.S().Info("The Mondoo operator is installed externally. Skipping installation...")
		return nil
	}

	rootFolder, err := utils.FindRootFolder()
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(rootFolder, OperatorManifest)); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("file %q does not exist. Run %q", OperatorManifest, "make generate-manifests")
	}

	if i.Settings.installRelease {
		zap.S().Info("Installing Mondoo operator release. ", "version=", i.PreviousVersion)
		releaseManifestUrl := fmt.Sprintf("https://github.com/mondoohq/mondoo-operator/releases/download/%s/mondoo-operator-manifests.yaml", i.PreviousVersion)
		_, err = i.K8sHelper.Kubectl(append(utils.ApplyArgs, releaseManifestUrl)...)
	} else {
		zap.S().Info("Installing Mondoo operator with local manifest")
		_, err = i.K8sHelper.KubectlWithStdin(i.readManifestWithNamespace(OperatorManifest), utils.ApplyFromStdinArgs...)
	}
	i.isInstalled = true // If the command above has run there is a chance things have been partially created.
	if err != nil {
		return fmt.Errorf("failed to create mondoo-operator manifest(s): %v ", err)
	}

	if err := i.CreateClientSecret(i.Settings.Namespace); err != nil {
		return err
	}

	if !i.K8sHelper.IsPodReady("control-plane=controller-manager", i.Settings.Namespace) {
		return fmt.Errorf("mondoo operator is not in a ready state")
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
	zap.S().Infof("Deleted Mondoo client secret %s/%s.", secret.Namespace, secret.Name)

	if i.isInstalledExternally {
		zap.S().Info("The Mondoo operator has been installed externally. Skipping uninstall...")
		return nil
	}

	_, err := i.K8sHelper.KubectlWithStdin(i.readManifestWithNamespace(OperatorManifest), utils.DeleteIngoreNotFoundFromStdinArgs...)
	if err != nil {
		return fmt.Errorf("failed to delete mondoo-operator manifest(s): %v ", err)
	}
	return nil
}

func (i *MondooInstaller) CleanupAuditConfigs() error {
	// Make sure all Mondoo audit configs are deleted so the namespace can be deleted. Leaving
	// audit configs will result in a stuck namespace.
	cfgs := &mondoov2.MondooAuditConfigList{}
	if err := i.K8sHelper.Clientset.List(i.ctx, cfgs); err != nil {
		return fmt.Errorf("failed to get Mondoo audit configs. %v", err)
	}

	for _, c := range cfgs.Items {
		if err := i.K8sHelper.Clientset.Delete(i.ctx, &c); err != nil {
			return fmt.Errorf("failed to delete Mondoo audit config %s/%s. %v", c.Namespace, c.Name, err)
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
	created, err := k8s.CreateIfNotExist(i.ctx, i.K8sHelper.Clientset, &secret, &secret)
	if err != nil {
		return fmt.Errorf("failed to create Мondoo secret in namespace %s. %v", ns, err)
	}
	if created {
		zap.S().Infof("Created Мondoo client secret %q.", utils.MondooClientSecret)
	} else {
		zap.S().Infof("Мondoo client secret %s/%s already exists.", secret.Namespace, secret.Name)
	}
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

// GenerateServiceCerts will generate a CA along with signed certificates for the provided dnsNames, and save
// it into secretName. It will return the CA certificate and any error encountered.
func (i *MondooInstaller) GenerateServiceCerts(auditConfig *mondoov2.MondooAuditConfig, secretName string, serviceDNSNames []string) (*bytes.Buffer, error) {
	if auditConfig == nil {
		return nil, fmt.Errorf("cannot generate certificates for a nil MondooAuditConfig")
	}

	caCert, serverCert, serverPrivKey, err := utils.GenerateTLSCerts(serviceDNSNames)
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificates: %s", err)
	}

	// Save cert/key to the Secret name the Webhook Deployment will expect
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: auditConfig.Namespace,
		},
		StringData: map[string]string{
			"ca.crt":  caCert.String(),
			"tls.crt": serverCert.String(),
			"tls.key": serverPrivKey.String(),
		},
	}

	if err := i.K8sHelper.Clientset.Create(i.ctx, secret); err != nil {
		return nil, fmt.Errorf("failed to create Secret with certificate data: %s", err)
	}

	return caCert, nil
}
