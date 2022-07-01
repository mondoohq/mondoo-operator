package status

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mondoo-operator/pkg/version"
	v1 "k8s.io/api/core/v1"
)

// operator release
// kubernetes version
// node count

// message ReportStatusRequest {
// 	string mrn = 1;
// 	 // this is the status of the integration itself (is it active/checking in, errored, etc)
// 	Status status = 2;
// 	// FIXME: should be deprecated and removed, the IntegrationMessages can be used to report an error.
// 	google.protobuf.Struct error = 3; // if the integration is in an errored state, this field should hold the error info
// 	// this can be any information about the current state of the integration. it will be displayed to the user as-is where supported
// 	google.protobuf.Struct last_state = 4;
// 	// Allows the agent to report its current version
// 	string version = 5;
// 	// scan status conveys information about the status of various types of scans
// 	// FIXME: deprecated, can be replaced by IntegrationMessages. remove once lambda is updated.
// 	repeated ScanMessage scan_status = 6;
// 	// messages that convey extra information about the integration - these messages can be informational, warnings or errors. Can be used
// 	// to report non-critical errors/warnings without neccesarily changing the whole integration status.
// 	IntegrationMessages messages = 7;
//   }

type OperatorStatus struct {
	OperatorVersion           string
	KubernetesVersion         string
	Nodes                     []string
	K8sResourcesScannerStatus ScannerStatus
	NodeScannerStatus         ScannerStatus
	AdmissionControllerStatus ScannerStatus
}

type ScannerStatus struct {
	Status   ComponentStatus
	Messages []string
}

type ComponentStatus string

const (
	HealthyStatus  ComponentStatus = "healthy"
	DegradedStatus ComponentStatus = "degraded"
	DisabledStatus ComponentStatus = "disabled"
)

func OperatorStatusFromAuditConfig(m v1alpha2.MondooAuditConfig, nodes []v1.Node) OperatorStatus {
	nodeNames := make([]string, len(nodes))
	for i := range nodes {
		nodeNames[i] = nodes[i].Name
	}

	// Kubernetes resources scanning status
	k8sResourcesScannerStatus := ScannerStatus{}
	if m.Spec.KubernetesResources.Enable {
		k8sResourcesScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.K8sResourcesScanningDegraded)
		if k8sResourcesScanning.Status == v1.ConditionTrue {
			k8sResourcesScannerStatus.Status = DegradedStatus
		} else {
			k8sResourcesScannerStatus.Status = HealthyStatus
		}
		k8sResourcesScannerStatus.Messages = []string{k8sResourcesScanning.Message}
	} else {
		k8sResourcesScannerStatus.Status = DisabledStatus
	}

	// Node scanning status
	nodeScannerStatus := ScannerStatus{}
	if m.Spec.Nodes.Enable {
		nodeScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.NodeScanningDegraded)
		if nodeScanning.Status == v1.ConditionTrue {
			nodeScannerStatus.Status = DegradedStatus
		} else {
			nodeScannerStatus.Status = HealthyStatus
		}
		nodeScannerStatus.Messages = []string{nodeScanning.Message}
	} else {
		nodeScannerStatus.Status = DisabledStatus
	}

	// Admission controller status
	admissionControllerStatus := ScannerStatus{}
	if m.Spec.Admission.Enable {
		admissionControllerScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.AdmissionDegraded)
		if admissionControllerScanning.Status == v1.ConditionTrue {
			admissionControllerStatus.Status = DegradedStatus
		} else {
			admissionControllerStatus.Status = HealthyStatus
		}
		admissionControllerStatus.Messages = []string{admissionControllerScanning.Message}
	} else {
		admissionControllerStatus.Status = DisabledStatus
	}

	return OperatorStatus{
		OperatorVersion:           version.Version,
		KubernetesVersion:         "TODO", // We only need to resolve this once at the startup of the operator
		Nodes:                     nodeNames,
		K8sResourcesScannerStatus: k8sResourcesScannerStatus,
		NodeScannerStatus:         nodeScannerStatus,
		AdmissionControllerStatus: admissionControllerStatus,
	}
}
