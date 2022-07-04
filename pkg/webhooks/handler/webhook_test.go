package webhookhandler

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/yaml"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient/fakeserver"
)

const (
	testNamespace = "test-namespace"
)

func TestWebhookValidate(t *testing.T) {
	decoder := setupDecoder(t)
	tests := []struct {
		name          string
		mode          mondoov1alpha2.AdmissionMode
		expectAllowed bool
		expectReason  string
		object        runtime.RawExtension
	}{
		{
			name:          "example test",
			expectAllowed: true,
			expectReason:  passedScan,
			object:        testExamplePod(),
		},
		{
			name:          "pod from replicaset test",
			expectAllowed: true,
			expectReason:  defaultScanPass,
			object: testExamplePod(func(p *corev1.Pod) {
				p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{
					Kind:       "ReplicaSet",
					Name:       "testReplicaSet",
					UID:        types.UID("abcd-1234"),
					Controller: pointer.Bool(true),
				})
			}),
		},
		{
			name:          "pod from statefulset test",
			expectAllowed: true,
			expectReason:  defaultScanPass,
			object: testExamplePod(func(p *corev1.Pod) {
				p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{
					Kind:       "StatefulSet",
					Name:       "testStatefulSet",
					UID:        types.UID("abcd-1234"),
					Controller: pointer.Bool(true),
				})
			}),
		},
		{
			name:          "pod from daemonset test",
			expectAllowed: true,
			expectReason:  defaultScanPass,
			object: testExamplePod(func(p *corev1.Pod) {
				p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{
					Kind:       "DaemonSet",
					Name:       "testDaemonSet",
					UID:        types.UID("abcd-1234"),
					Controller: pointer.Bool(true),
				})
			}),
		},
		{
			name:          "pod from job test",
			expectAllowed: true,
			expectReason:  defaultScanPass,
			object: testExamplePod(func(p *corev1.Pod) {
				p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{
					Kind:       "Job",
					Name:       "testJob",
					UID:        types.UID("abcd-1234"),
					Controller: pointer.Bool(true),
				})
			}),
		},
		{
			name:          "job test",
			expectAllowed: true,
			expectReason:  passedScan,
			object:        testExampleJob(),
		},
		{
			name:          "job from cronjob test",
			expectAllowed: true,
			expectReason:  defaultScanPass,
			object: testExampleJob(func(j *batchv1.Job) {
				j.OwnerReferences = append(j.OwnerReferences, metav1.OwnerReference{
					Kind:       "CronJob",
					Name:       "testCronJob",
					UID:        types.UID("abcd-1234"),
					Controller: pointer.Bool(true),
				})
			}),
		},
		{
			name:          "example Deployment",
			expectAllowed: true,
			expectReason:  passedScan,
			object:        testExampleDeployment(),
		},
		{
			name:          "malformed object",
			expectAllowed: true,
			expectReason:  defaultScanPass,
			object: func() runtime.RawExtension {
				var pod runtime.RawExtension
				pod.Raw = []byte("not valid json")
				return pod
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			if test.mode == "" {
				test.mode = mondoov1alpha2.Permissive
			}

			testserver := fakeserver.FakeServer()
			validator := &webhookValidator{
				decoder: decoder,
				mode:    test.mode,
				scanner: mondooclient.NewClient(mondooclient.ClientOptions{
					ApiEndpoint: testserver.URL,
				}),
				uniDecoder: serializer.NewCodecFactory(clientgoscheme.Scheme).UniversalDeserializer(),
			}

			request := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Object: test.object,
				},
			}

			// Act
			response := validator.Handle(context.TODO(), request)

			t.Logf("Handle() response: %+v", response)

			// Assert
			assert.Equal(t, test.expectAllowed, response.AdmissionResponse.Allowed)

			if test.expectReason != "" {
				assert.Equal(t, test.expectReason, string(response.AdmissionResponse.Result.Reason))
			}
		})
	}
}

var webhookPayload = mustRead("../../../tests/data/webhook-payload.json")

func TestLabels(t *testing.T) {
	req := admission.Request{}
	require.NoError(t, yaml.Unmarshal(webhookPayload, &req), "failed to unmarshal webhook payload")

	webhook := &webhookValidator{
		integrationMRN: "testIntegrationMRN",
		clusterID:      "testClusterID",
		uniDecoder:     serializer.NewCodecFactory(clientgoscheme.Scheme).UniversalDeserializer(),
	}

	obj, err := webhook.objFromRaw(req.Object)
	require.NoError(t, err, "unexpected error while converting request object")

	labels, err := webhook.generateLabels(req, obj)

	require.NoError(t, err, "unexpected error while testing label creation")
	assert.Contains(t, labels, mondooClusterIDLabel, "cluster ID label missing")
	assert.Equal(t, "testClusterID", labels[mondooClusterIDLabel], "cluster ID label not as expected")
	assert.Contains(t, labels, constants.MondooAssetsIntegrationLabel, "integration label missing")
	assert.Equal(t, "testIntegrationMRN", labels[constants.MondooAssetsIntegrationLabel], "integration label not as expected")

	// string literals being compared to are taken from example webhook payload json
	require.Contains(t, labels, mondooNameLabel, "Name label missing")
	require.Equal(t, "memcached-sample-5c8cffd96c-42z72", labels[mondooNameLabel])
	require.Contains(t, labels, mondooNamespaceLabel, "Namespace label missing")
	require.Equal(t, "default", labels[mondooNamespaceLabel])
	require.Contains(t, labels, mondooUIDLabel, "UID label missing")
	require.Equal(t, "a94b5098-731d-4dda-9a0b-d516c1702b53", labels[mondooUIDLabel])
	require.Contains(t, labels, mondooKindLabel, "Kind label missing")
	require.Equal(t, "Pod", labels[mondooKindLabel])
	require.Contains(t, labels, mondooOwnerNameLabel, "OwnerName label missing")
	require.Equal(t, "memcached-sample-5c8cffd96c", labels[mondooOwnerNameLabel])
	require.Contains(t, labels, mondooOwnerKindLabel, "OwnerKind label missing")
	require.Equal(t, "ReplicaSet", labels[mondooOwnerKindLabel])
	require.Contains(t, labels, mondooOwnerUIDLabel, "OwnerUID label missing")
	require.Equal(t, "833fd5a2-2377-4766-b324-545e5e449a2f", labels[mondooOwnerUIDLabel])
	require.Contains(t, labels, mondooOperationLabel, "Operation label missing")
	require.Equal(t, "CREATE", labels[mondooOperationLabel])
	require.Contains(t, labels, mondooResourceVersionLabel, "ResourceVersion label missing")
	require.Equal(t, "", labels[mondooResourceVersionLabel], "Expect empty value for a CREATE webhook")
}

func testExamplePod(modifiers ...func(*corev1.Pod)) runtime.RawExtension {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind: "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testPod-abcd",
			Namespace: testNamespace,
			UID:       types.UID("1234-abcd"),
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "Pod",
					Name: "notControllerOwner",
					UID:  "another-uid",
				},
			},
		},
	}

	for _, m := range modifiers {
		m(pod)
	}

	data, err := json.Marshal(pod)
	if err != nil {
		panic(err)
	}

	return runtime.RawExtension{
		Raw:    data,
		Object: pod,
	}
}

func testExampleJob(modifiers ...func(*batchv1.Job)) runtime.RawExtension {
	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind: "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testPod-abcd",
			Namespace: testNamespace,
			UID:       types.UID("1234-abcd"),
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "Pod",
					Name: "notControllerOwner",
					UID:  "another-uid",
				},
			},
		},
	}

	for _, m := range modifiers {
		m(job)
	}

	data, err := json.Marshal(job)
	if err != nil {
		panic(err)
	}

	return runtime.RawExtension{
		Raw:    data,
		Object: job,
	}
}

func testExampleDeployment() runtime.RawExtension {
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testDeployment",
			Namespace: testNamespace,
		},
	}

	data, err := json.Marshal(dep)
	if err != nil {
		panic(err)
	}

	return runtime.RawExtension{
		Raw:    data,
		Object: dep,
	}
}

func setupDecoder(t *testing.T) *admission.Decoder {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	decoder, err := admission.NewDecoder(scheme)
	require.NoError(t, err, "Failed to setup decoder for testing")

	return decoder
}

func mustRead(filePath string) []byte {
	bytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		panic("failed to read in file")
	}
	return bytes
}
