package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
)

const (
	OperatorManifest = "mondoo-operator-manifests.yaml"
	MondooNamespace  = "mondoo-operator"
)

type MondooInstaller struct {
	k8sHelper *utils.K8sHelper
	//Manifests *Manifests
	T           func() *testing.T
	isInstalled bool
}

func NewMondooInstaller(t func() *testing.T) *MondooInstaller {
	k8sHelper, err := utils.CreateK8sHelper(t)
	if err != nil {
		panic("failed to get kubectl client :" + err.Error())
	}

	return &MondooInstaller{
		k8sHelper: k8sHelper,
		//Manifests: &Manifests{},
		T: t,
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

	_, err = i.k8sHelper.KubectlWithStdin(utils.ReadManifest(OperatorManifest), utils.CreateFromStdinArgs...)
	if err != nil {
		return errors.Errorf("Failed to create mondoo-operator pod: %v ", err)
	}

	if !i.k8sHelper.IsPodInExpectedState("control-plane=controller-manager", MondooNamespace, "Running") {
		return errors.Errorf("Mondoo operator is not in a running state.")
	}

	i.isInstalled = true
	return nil
}

func (i *MondooInstaller) UninstallOperator() error {
	// If the operator has not been installed do nothing
	if !i.isInstalled {
		return nil
	}
	i.k8sHelper.GetLogsFromNamespace(MondooNamespace, i.T().Name())

	_, err := i.k8sHelper.KubectlWithStdin(utils.ReadManifest(OperatorManifest), utils.DeleteFromStdinArgs...)
	if err != nil {
		return errors.Errorf("Failed to delete mondoo-operator pod: %v ", err)
	}
	return nil
}

func (i *MondooInstaller) GatherAllMondooLogs(testName string, namespaces ...string) {
	if !i.T().Failed() {
		return
	}
	zap.S().Infof("gathering all logs from the test")
	for _, namespace := range namespaces {
		i.k8sHelper.GetLogsFromNamespace(namespace, testName)
		i.k8sHelper.GetPodDescribeFromNamespace(namespace, testName)
		i.k8sHelper.GetEventsFromNamespace(namespace, testName)
	}
}
