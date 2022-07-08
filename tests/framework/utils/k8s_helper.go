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

	api "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/version"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
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
	RetryInterval = 5
	RetryLoop     = 30
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
		return nil, fmt.Errorf("failed to get clientset. %+v", err)
	}
	if err := api.AddToScheme(clientset.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to register schemes for Kube client. %+v", err)
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
		zap.S().Errorf("failed to execute: %s %+v : %+v. %s", cmd, args, err, result)
		if args[0] == "delete" {
			// allow the tests to continue if we were deleting a resource that timed out
			return result, nil
		}
		return result, fmt.Errorf("failed to run: %s %v : %v", cmd, args, err)
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
		return cmdOut.StdErr, fmt.Errorf("failed to run stdin: %s %v : %v", cmd, args, cmdOut.StdErr)
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
			"failed to delete %s %s/%s. %v",
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

	err = k8sh.ExecuteWithRetries(func() (bool, error) {
		err := k8sh.Clientset.List(ctx, podList, listOpts)
		if err == nil {
			if len(podList.Items) >= 1 {
				for _, pod := range podList.Items {
					for _, c := range pod.Status.Conditions {
						if c.Type == v1.PodReady && c.Status == v1.ConditionTrue {
							return true, nil
						}
					}
				}
			}
		} else {
			zap.S().Errorf("Error listing pods: %v", err)
		}
		return false, nil
	})
	return err == nil
}

// WaitUntilCronJobsSuccessful waits for the CronJobs with the specified selector to have at least
// one successful run.
func (k8sh *K8sHelper) WaitUntilCronJobsSuccessful(labelSelector, namespace string) bool {
	listOpts, err := LabelSelectorListOptions(labelSelector)
	if err != nil {
		return false
	}
	listOpts.Namespace = namespace
	ctx := context.Background()
	cronJobs := &batchv1.CronJobList{}

	err = k8sh.ExecuteWithRetries(func() (bool, error) {
		if err := k8sh.Clientset.List(ctx, cronJobs, listOpts); err != nil {
			zap.S().Errorf("Failed to list CronJobs. %v", err)
			return false, err
		}
		for _, c := range cronJobs.Items {
			// Make sure the job has been scheduled
			if c.Status.LastScheduleTime == nil {
				return false, nil
			}
		}

		// Make sure all jobs have succeeded
		if k8s.AreCronJobsSuccessful(cronJobs.Items) {
			return true, nil
		}
		return false, nil
	})
	return err == nil
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

	return k8sh.ExecuteWithRetries(func() (bool, error) {
		err := k8sh.Clientset.Get(ctx, key, r)
		if err == nil {
			zap.S().Infof("Resource %s %s/%s still exists.", kind, key.Namespace, key.Name)
			return false, nil
		}
		if kerrors.IsNotFound(err) {
			zap.S().Infof("Resource %s %s/%s deleted.", kind, key.Namespace, key.Name)
			return true, nil
		}
		return false, fmt.Errorf("gave up deleting %s %s/%s. %v", kind, key.Namespace, key.Name, err)
	})
}

func (k8sh *K8sHelper) ExecuteWithRetries(f func() (bool, error)) error {
	for i := 0; i < RetryLoop; i++ {
		success, err := f()
		if success {
			return nil
		}

		if err != nil {
			return err
		}
		time.Sleep(RetryInterval * time.Second)
	}
	return fmt.Errorf("test did not succeed after %d retries", RetryLoop)
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
	args = append([]string{"get", "pod", "-o", "yaml", "-n", namespace}, args...)
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
		// We have test cases where containers intentionally will not start, e.g. the webhook
		// These cases creating disturbing stack traces, so ignore them
		if strings.Contains(res.Error().Error(), "ContainerCreating") {
			zap.S().Infof("Cannot get logs for pod %s and container %s. Container is in state ContainerCreating", pod.Name, containerName)
			return
		}
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

// GetMondooAuditConfigFromCluster Fetches current MondooAuditConfig from Cluster
func (k8sh *K8sHelper) GetMondooAuditConfigFromCluster(auditConfigName, auditConfigNamespace string) (*api.MondooAuditConfig, error) {
	foundMondooAuditConfig := &api.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      auditConfigName,
			Namespace: auditConfigNamespace,
		},
	}
	err := k8sh.Clientset.Get(context.Background(), client.ObjectKeyFromObject(foundMondooAuditConfig), foundMondooAuditConfig)
	if err != nil {
		return &api.MondooAuditConfig{}, err
	}

	return foundMondooAuditConfig, nil
}

// GetMondooAuditConfigConditionByType Fetches Condition from MondooAuditConfig Status for the specified Type.
func (k8sh *K8sHelper) GetMondooAuditConfigConditionByType(auditConfig *api.MondooAuditConfig, conditionType api.MondooAuditConfigConditionType) (api.MondooAuditConfigCondition, error) {
	conditions := auditConfig.Status.Conditions
	if len(conditions) == 0 {
		return api.MondooAuditConfigCondition{}, fmt.Errorf("Status.Conditions is empty")
	}
	searchedForCondition := api.MondooAuditConfigCondition{}
	for _, condition := range conditions {
		if condition.Type == conditionType {
			searchedForCondition = condition
			return searchedForCondition, nil
		}
	}

	return searchedForCondition, fmt.Errorf("couldn't find condition of type %v", conditionType)
}

// CheckForDegradedCondition Check whether specified Condition is in degraded state in a MondooAuditConfig with retries.
func (k8sh *K8sHelper) CheckForDegradedCondition(auditConfig *api.MondooAuditConfig, conditionType api.MondooAuditConfigConditionType) error {
	err := k8sh.ExecuteWithRetries(func() (bool, error) {
		// Condition of MondooAuditConfig should be updated
		foundMondooAuditConfig, err := k8sh.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
		if err != nil {
			return false, err
		}
		condition, err := k8sh.GetMondooAuditConfigConditionByType(foundMondooAuditConfig, conditionType)
		if err != nil {
			return false, err
		}
		if condition.Status == v1.ConditionFalse {
			return true, nil
		}
		return false, nil
	})

	return err
}

// CheckForPodInStatus Check whether a give PodName is an element of the PodList saved in the Status part of MondooAuditConfig
func (k8sh *K8sHelper) CheckForPodInStatus(auditConfig *api.MondooAuditConfig, podName string) error {
	err := k8sh.ExecuteWithRetries(func() (bool, error) {
		// Condition of MondooAuditConfig should be updated
		foundMondooAuditConfig, err := k8sh.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
		if err != nil {
			return false, err
		}
		for _, currentPodName := range foundMondooAuditConfig.Status.Pods {
			if strings.Contains(currentPodName, podName) {
				return true, nil
			}
		}
		return false, nil
	})

	return err
}

// CheckForReconciledOperatorVersion Check whether the MondooAuditConfig Status contains the current operator Version after Reconcile.
func (k8sh *K8sHelper) CheckForReconciledOperatorVersion(auditConfig *api.MondooAuditConfig) error {
	err := k8sh.ExecuteWithRetries(func() (bool, error) {
		// Condition of MondooAuditConfig should be updated
		foundMondooAuditConfig, err := k8sh.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
		if err != nil {
			return false, err
		}
		reconciledVersion := foundMondooAuditConfig.Status.ReconciledByOperatorVersion
		if reconciledVersion == version.Version {
			return true, nil
		}
		return false, nil
	})

	return err
}
