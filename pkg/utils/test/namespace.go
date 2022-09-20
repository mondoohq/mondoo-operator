package test

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KubeSystemNamespaceUid = "1234-5678"
)

func TestKubeSystemNamespace() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  KubeSystemNamespaceUid,
		},
	}
}
