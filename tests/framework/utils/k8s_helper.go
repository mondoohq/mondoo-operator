/*
Copyright 2016 The Rook Authors. All rights reserved.
Copyright 2022 Mondoo, Inc.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	api "go.mondoo.com/mondoo-operator/api/v1alpha1"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	cmd           = "kubectl"
	retryInterval = 5
	retryLoop     = 20
)

var (
	CreateArgs                        = []string{"create", "-f"}
	CreateFromStdinArgs               = append(CreateArgs, "-")
	DeleteArgs                        = []string{"delete", "-f"}
	DeleteArgsIgnoreNotFound          = []string{"delete", "--ignore-not-found=true", "-f"}
	DeleteFromStdinArgs               = append(DeleteArgs, "-")
	DeleteIngoreNotFoundFromStdinArgs = append(DeleteArgsIgnoreNotFound, "-")
)

type K8sHelper struct {
	Clientset        client.Client
	RunningInCluster bool
	executor         *CommandExecutor
	kubeClient       *kubernetes.Clientset // needed only to retrieve logs
}

// CreateK8sHelper creates a instance of k8sHelper
func CreateK8sHelper() (*K8sHelper, error) {
	executor := &CommandExecutor{}
	config, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kube client. %+v", err)
	}
	clientset, err := client.New(config, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("Failed to get clientset. %+v", err)
	}
	if err := api.AddToScheme(clientset.Scheme()); err != nil {
		return nil, fmt.Errorf("Failed to register schemes for Kube client. %+v", err)
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes client. %+v", err)
	}

	h := &K8sHelper{executor: executor, Clientset: clientset, kubeClient: kubeClient}
	if strings.Contains(config.Host, "//10.") {
		h.RunningInCluster = true
	}
	return h, err
}

// Kubectl is wrapper for executing kubectl commands
func (k8sh *K8sHelper) Kubectl(args ...string) (string, error) {
	result, err := k8sh.executor.ExecuteCommandWithTimeout(15*time.Second, "kubectl", args...)
	if err != nil {
		zap.S().Errorf("Failed to execute: %s %+v : %+v. %s", cmd, args, err, result)
		if args[0] == "delete" {
			// allow the tests to continue if we were deleting a resource that timed out
			return result, nil
		}
		return result, fmt.Errorf("Failed to run: %s %v : %v", cmd, args, err)
	}
	return result, nil
}

// KubectlWithStdin is wrapper for executing kubectl commands in stdin
func (k8sh *K8sHelper) KubectlWithStdin(stdin string, args ...string) (string, error) {

	cmdStruct := CommandArgs{Command: cmd, PipeToStdIn: stdin, CmdArgs: args}
	cmdOut := ExecuteCommand(cmdStruct)

	if cmdOut.ExitCode != 0 {
		zap.S().Errorf("Failed to execute stdin: %s %v : %v", cmd, args, cmdOut.Err.Error())
		if strings.Contains(cmdOut.Err.Error(), "(NotFound)") || strings.Contains(cmdOut.StdErr, "(NotFound)") {
			return cmdOut.StdErr, kerrors.NewNotFound(schema.GroupResource{}, "")
		}
		return cmdOut.StdErr, fmt.Errorf("Failed to run stdin: %s %v : %v", cmd, args, cmdOut.StdErr)
	}
	if cmdOut.StdOut == "" {
		return cmdOut.StdErr, nil
	}

	return cmdOut.StdOut, nil
}

// DeleteResourceIfExists Deletes the requested resource if it exists. If the resource does not
// exist, the function does nothing (return no error).
func (k8sh *K8sHelper) DeleteResourceIfExists(r client.Object) error {
	ctx := context.Background()
	if err := k8sh.Clientset.Delete(ctx, r); err != nil && !kerrors.IsNotFound(err) {
		return fmt.Errorf(
			"Failed to delete %s %s/%s. %v",
			r.GetObjectKind().GroupVersionKind().String(),
			r.GetNamespace(),
			r.GetName(),
			err)
	}
	return nil
}

// IsPodInExpectedState waits for a pod to be in a Ready state
// If the pod is in expected state within the time retry limit true is returned, if not false
func (k8sh *K8sHelper) IsPodReady(labelSelector, namespace string) bool {
	listOpts, err := LabelSelectorListOptions(labelSelector)
	if err != nil {
		return false
	}
	listOpts.Namespace = namespace
	ctx := context.Background()
	podList := &v1.PodList{}
	for i := 0; i < retryLoop; i++ {
		err := k8sh.Clientset.List(ctx, podList, listOpts)
		if err == nil {
			if len(podList.Items) >= 1 {
				for _, pod := range podList.Items {
					for _, c := range pod.Status.Conditions {
						if c.Type == v1.PodReady && c.Status == v1.ConditionTrue {
							return true
						}
					}

				}
			}
		}
		time.Sleep(retryInterval * time.Second)
	}
	zap.S().Debugf("%+v", podList)
	return false
}

func LabelSelectorListOptions(labelSelector string) (*client.ListOptions, error) {
	selector, err := labels.Parse(labelSelector)
	if err != nil {
		zap.S().Errorf("Failed to parse label selector. %+v", err)
		return nil, err
	}
	return &client.ListOptions{LabelSelector: selector}, nil
}

// GetLogsFromNamespace collects logs for all containers in all pods in the namespace
func (k8sh *K8sHelper) GetLogsFromNamespace(namespace, testName string) {
	ctx := context.TODO()
	zap.S().Infof("Gathering logs for all pods in namespace %s", namespace)
	pods, err := k8sh.kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		zap.S().Errorf("failed to list pods in namespace %s. %+v", namespace, err)
		return
	}
	k8sh.getPodsLogs(pods, namespace, testName)
}

func (k8sh *K8sHelper) GetPodDescribeFromNamespace(namespace, testName string) {
	ctx := context.TODO()
	zap.S().Infof("Gathering pod describe for all pods in namespace %s", namespace)
	pods, err := k8sh.kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		zap.S().Errorf("failed to list pods in namespace %s. %+v", namespace, err)
		return
	}

	file, err := k8sh.createTestLogFile("", "podDescribe", namespace, testName)
	if err != nil {
		return
	}
	defer file.Close()

	for _, p := range pods.Items {
		k8sh.appendPodDescribe(file, namespace, p.Name)
	}
}

func (k8sh *K8sHelper) GetEventsFromNamespace(namespace, testName string) {
	zap.S().Infof("Gathering events in namespace %q", namespace)

	file, err := k8sh.createTestLogFile("", "events", namespace, testName)
	if err != nil {
		zap.S().Errorf("failed to create event file. %v", err)
		return
	}
	defer file.Close()

	args := []string{"get", "events", "-n", namespace}
	events, err := k8sh.Kubectl(args...)
	if err != nil {
		zap.S().Errorf("failed to get events. %v. %v", args, err)
	}
	if events == "" {
		return
	}
	file.WriteString(events) //nolint // ok to ignore this test logging
}

// WaitForResourceDeletion waits for a resource deletion
func (k8sh *K8sHelper) WaitForResourceDeletion(r client.Object) error {
	ctx := context.Background()
	key := client.ObjectKeyFromObject(r)
	kind := r.GetObjectKind().GroupVersionKind().String()
	for i := 0; i < retryLoop; i++ {

		err := k8sh.Clientset.Get(ctx, key, r)
		if err == nil {
			zap.S().Infof("Resource %s %s/%s still exists.", kind, key.Namespace, key.Name)
			time.Sleep(retryInterval * time.Second)
			continue
		}
		if kerrors.IsNotFound(err) {
			zap.S().Infof("Resource %s %s/%s deleted.", kind, key.Namespace, key.Name)
			return nil
		}
		return err
	}
	return fmt.Errorf("Gave up deleting %s %s/%s ", kind, key.Namespace, key.Name)
}

func (k8sh *K8sHelper) appendPodDescribe(file *os.File, namespace, name string) {
	description := k8sh.getPodDescribe(namespace, name)
	if description == "" {
		return
	}
	writeHeader(file, fmt.Sprintf("Pod: %s\n", name)) //nolint // ok to ignore this test logging
	file.WriteString(description)                     //nolint // ok to ignore this test logging
	file.WriteString("\n")                            //nolint // ok to ignore this test logging
}

func (k8sh *K8sHelper) PrintPodDescribe(namespace string, args ...string) {
	description := k8sh.getPodDescribe(namespace, args...)
	if description == "" {
		return
	}
	zap.S().Infof("POD Description:\n%s", description)
}

func (k8sh *K8sHelper) getPodDescribe(namespace string, args ...string) string {
	args = append([]string{"describe", "pod", "-n", namespace}, args...)
	description, err := k8sh.Kubectl(args...)
	if err != nil {
		zap.S().Errorf("failed to describe pod. %v %+v", args, err)
		return ""
	}
	return description
}

func (k8sh *K8sHelper) getPodsLogs(pods *v1.PodList, namespace, testName string) {
	for _, p := range pods.Items {
		k8sh.getPodLogs(p, namespace, testName, false)
		if strings.Contains(p.Name, "operator") {
			// get the previous logs for the operator
			k8sh.getPodLogs(p, namespace, testName, true)
		}
	}
}

func (k8sh *K8sHelper) createTestLogFile(name, namespace, testName, suffix string) (*os.File, error) {
	dir, _ := os.Getwd()
	logDir := path.Join(dir, "_output/tests/")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		err := os.MkdirAll(logDir, 0777)
		if err != nil {
			zap.S().Errorf("Cannot get logs files dir for app : %v in namespace %v, err: %v", name, namespace, err)
			return nil, err
		}
	}

	fileName := fmt.Sprintf("%s_%s_%s%s_%d.log", testName, namespace, name, suffix, time.Now().Unix())
	fileName = strings.ReplaceAll(fileName, "/", "")
	filePath := path.Join(logDir, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		zap.S().Errorf("Cannot create file %s. %v", filePath, err)
		return nil, err
	}

	zap.S().Debugf("created log file: %s", filePath)
	return file, nil
}

func (k8sh *K8sHelper) getPodLogs(pod v1.Pod, namespace, testName string, previousLog bool) {
	suffix := ""
	if previousLog {
		suffix = "_previous"
	}
	file, err := k8sh.createTestLogFile(pod.Name, namespace, testName, suffix)
	if err != nil {
		return
	}
	defer file.Close()

	for _, container := range pod.Spec.InitContainers {
		k8sh.appendContainerLogs(file, pod, container.Name, previousLog, true)
	}
	for _, container := range pod.Spec.Containers {
		k8sh.appendContainerLogs(file, pod, container.Name, previousLog, false)
	}
}

func (k8sh *K8sHelper) appendContainerLogs(file *os.File, pod v1.Pod, containerName string, previousLog, initContainer bool) {
	message := fmt.Sprintf("CONTAINER: %s", containerName)
	if initContainer {
		message = "INIT " + message
	}
	writeHeader(file, message) //nolint // ok to ignore this test logging
	ctx := context.TODO()
	logOpts := &v1.PodLogOptions{Previous: previousLog}
	if containerName != "" {
		logOpts.Container = containerName
	}
	res := k8sh.kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts).Do(ctx)
	rawData, err := res.Raw()
	if err != nil {
		// Sometimes we fail to get logs for pods using this method, notably the operator pod. It is
		// unknown why this happens. Pod logs are VERY important, so try again using kubectl.
		l, err := k8sh.Kubectl("-n", pod.Namespace, "logs", pod.Name, "-c", containerName)
		if err != nil {
			zap.S().Errorf("Cannot get logs for pod %s and container %s. %v", pod.Name, containerName, err)
			return
		}
		rawData = []byte(l)
	}
	if _, err := file.Write(rawData); err != nil {
		zap.S().Errorf("Errors while writing logs for pod %s and container %s. %v", pod.Name, containerName, err)
	}
}

func writeHeader(file *os.File, message string) error {
	file.WriteString("\n-----------------------------------------\n") //nolint // ok to ignore this test logging
	file.WriteString(message)                                         //nolint // ok to ignore this test logging
	file.WriteString("\n-----------------------------------------\n") //nolint // ok to ignore this test logging

	return nil
}
