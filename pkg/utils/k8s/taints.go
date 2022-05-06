package k8s

import corev1 "k8s.io/api/core/v1"

func TaintsToTolerations(taints []corev1.Taint) []corev1.Toleration {
	var tolerations []corev1.Toleration
	for _, t := range taints {
		tolerations = append(tolerations, TaintToToleration(t))
	}
	return tolerations
}

func TaintToToleration(t corev1.Taint) corev1.Toleration {
	return corev1.Toleration{
		Key:    t.Key,
		Effect: t.Effect,
		Value:  t.Value,
	}
}
