package nodes

import (
	_ "embed"
	"fmt"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	InventorySuffix = "-nodes-inventory"
)

var (
	//go:embed inventory.yaml
	inventoryYaml []byte
)

func ConfigMap(m v1alpha2.MondooAuditConfig) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: m.Namespace,
			Name:      ConfigMapName(m.Name),
		},
		Data: map[string]string{"inventory": Inventory(m)},
	}
}

func ConfigMapName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, InventorySuffix)
}

func Inventory(m v1alpha2.MondooAuditConfig) string {
	return string(inventoryYaml)
}

func CronJobLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo-node-scanning",
		"mondoo_cr": m.Name,
	}
}
