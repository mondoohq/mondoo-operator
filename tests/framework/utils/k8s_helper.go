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

	cnquery_k8s "go.mondoo.com/cnquery/v12/providers/k8s/resources"
	api "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	cmd                    = "kubectl"
	RetryInterval          = 2
	RetryLoop              = 100
	CronJobRetryLoop       = 300 // CronJobs need more time: schedule delay + scan execution (600s)
	SkipVersionCheckEnvVar = "SKIP_VERSION_CHECK"
)

var (
	CreateArgs                        = []string{"create", "-f"}
	CreateFromStdinArgs               = append(CreateArgs, "-")
	ApplyArgs                         = []string{"apply", "-f"}
	ApplyFromStdinArgs                = append(ApplyArgs, "-")
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
	dynamicClient    dynamic.Interface
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

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes dynamic client. %+v", err)
	}

	h := &K8sHelper{executor: executor, Clientset: clientset, kubeClient: kubeClient, dynamicClient: dynamicClient}
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

// IsPodReady waits for a pod to be in a Ready state
// If the pod is in ready state within the time retry limit true is returned, if not false
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

// ListPods returns a list of pods matching the label selector in the given namespace
func (k8sh *K8sHelper) ListPods(namespace, labelSelector string) (*v1.PodList, error) {
	listOpts, err := LabelSelectorListOptions(labelSelector)
	if err != nil {
		return nil, err
	}
	listOpts.Namespace = namespace
	podList := &v1.PodList{}
	err = k8sh.Clientset.List(context.Background(), podList, listOpts)
	return podList, err
}

// IsPodInExpectedState waits for a pod to be in a Ready state
// If the pod is in expected state within the time retry limit true is returned, if not false
func (k8sh *K8sHelper) EnsureNoPodsPresent(listOpts *client.ListOptions) error {
	ctx := context.Background()
	podList := &v1.PodList{}

	return k8sh.ExecuteWithRetries(func() (bool, error) {
		err := k8sh.Clientset.List(ctx, podList, listOpts)
		if err == nil {
			if len(podList.Items) == 0 {
				return true, nil
			}
		}
		return false, nil
	})
}

func (k8sh *K8sHelper) WaitUntilMondooClientSecretExists(ctx context.Context, ns string) bool {
	// Wait until token has been exchanged for a Mondoo service account
	err := k8sh.ExecuteWithRetries(func() (bool, error) {
		secret := &v1.Secret{}
		if err := k8sh.Clientset.Get(ctx, types.NamespacedName{Namespace: ns, Name: MondooClientSecret}, secret); err != nil {
			if kerrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	return err == nil
}

// WaitUntilCronJobsSuccessful waits for the CronJobs with the specified selector to have at least
// one successful run. Uses CronJobRetryLoop for a longer timeout since CronJobs need time for
// schedule delay plus scan execution.
func (k8sh *K8sHelper) WaitUntilCronJobsSuccessful(labelSelector, namespace string) bool {
	listOpts, err := LabelSelectorListOptions(labelSelector)
	if err != nil {
		return false
	}
	listOpts.Namespace = namespace
	ctx := context.Background()
	cronJobs := &batchv1.CronJobList{}

	for i := 0; i < CronJobRetryLoop; i++ {
		if err := k8sh.Clientset.List(ctx, cronJobs, listOpts); err != nil {
			zap.S().Errorf("Failed to list CronJobs. %v", err)
			return false
		}

		allReady := true
		for _, c := range cronJobs.Items {
			if c.Status.LastScheduleTime == nil || len(c.Status.Active) > 0 {
				allReady = false
				break
			}
		}

		if allReady && k8s.AreCronJobsSuccessful(cronJobs.Items) {
			return true
		}
		time.Sleep(RetryInterval * time.Second)
	}
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
func (k8sh *K8sHelper) GetLogsFromNamespace(namespace, suiteName, testName string) {
	ctx := context.TODO()
	zap.S().Infof("Gathering logs for all pods in namespace %s", namespace)
	pods, err := k8sh.kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		zap.S().Errorf("failed to list pods in namespace %s. %+v", namespace, err)
		return
	}
	k8sh.getPodsLogs(pods, namespace, suiteName, testName)
}

func (k8sh *K8sHelper) GetDescribeFromNamespace(namespace, suiteName, testName string) {
	ctx := context.TODO()
	zap.S().Infof("Gathering pod describe for all pods in namespace %s", namespace)
	pods, err := k8sh.kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		zap.S().Errorf("failed to list pods in namespace %s. %+v", namespace, err)
		return
	}

	file, err := k8sh.createTestLogFile("", "describe", suiteName, testName, namespace)
	if err != nil {
		return
	}
	defer file.Close() //nolint:errcheck

	for _, p := range pods.Items {
		k8sh.appendPodDescribe(file, namespace, p.Name)
	}

	deployments, err := k8sh.kubeClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		zap.S().Errorf("failed to list pods in namespace %s. %+v", namespace, err)
		return
	}

	for _, p := range deployments.Items {
		k8sh.appendDeploymentDescribe(file, namespace, p.Name)
	}

	cronjobs, err := k8sh.kubeClient.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		zap.S().Errorf("failed to list cronjobs in namespace %s. %+v", namespace, err)
		return
	}

	for _, p := range cronjobs.Items {
		k8sh.appendCronJobDescribe(file, namespace, p.Name)
	}

	auditConfigs, err := k8sh.dynamicClient.Resource(api.GroupVersion.WithResource("mondooauditconfigs")).
		Namespace(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		zap.S().Errorf("failed to list mondooauditconfig in namespace %s. %+v", namespace, err)
		return
	}

	for _, a := range auditConfigs.Items {
		k8sh.appendAuditConfigDescribe(file, namespace, a.GetName())
	}
}

func (k8sh *K8sHelper) GetEventsFromNamespace(namespace, suiteName, testName string) {
	zap.S().Infof("Gathering events in namespace %q", namespace)

	file, err := k8sh.createTestLogFile("", "events", suiteName, testName, namespace)
	if err != nil {
		zap.S().Errorf("failed to create event file. %v", err)
		return
	}
	defer file.Close() //nolint:errcheck

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

func (k8sh *K8sHelper) UpdateAuditConfigWithRetries(name, namespace string, update func(auditConfig *api.MondooAuditConfig)) error {
	err := k8sh.ExecuteWithRetries(func() (bool, error) {
		auditConfig, err := k8sh.GetMondooAuditConfigFromCluster(name, namespace)
		if err != nil {
			return false, err
		}
		update(auditConfig)
		err = k8sh.Clientset.Update(context.Background(), auditConfig)
		if err != nil {
			return false, nil // retry
		}
		return true, nil // success
	})
	return err
}

func (k8sh *K8sHelper) UpdateDeploymentWithRetries(ctx context.Context, listOpts client.ListOption, update func(*appsv1.Deployment)) error {
	deployments := &appsv1.DeploymentList{}
	return k8sh.ExecuteWithRetries(func() (bool, error) {
		if err := k8sh.Clientset.List(ctx, deployments, listOpts); err != nil {
			return false, err
		}
		if len(deployments.Items) != 1 {
			return false, nil
		}

		// update the deployment
		dep := &deployments.Items[0]
		update(dep)
		if err := k8sh.Clientset.Update(ctx, dep); err != nil {
			return false, nil // retry
		}
		return true, nil
	})
}

func (k8sh *K8sHelper) UpdateDaemonSetWithRetries(ctx context.Context, key types.NamespacedName, update func(*appsv1.DaemonSet)) error {
	ds := &appsv1.DaemonSet{}
	return k8sh.ExecuteWithRetries(func() (bool, error) {
		if err := k8sh.Clientset.Get(ctx, key, ds); err != nil {
			return false, err
		}

		// update the daemonset
		update(ds)
		if err := k8sh.Clientset.Update(ctx, ds); err != nil {
			return false, nil // retry
		}
		return true, nil
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

func (k8sh *K8sHelper) appendDeploymentDescribe(file *os.File, namespace, name string) {
	description := k8sh.getDeploymentDescribe(namespace, name)
	if description == "" {
		return
	}
	writeHeader(file, fmt.Sprintf("Deployment: %s\n", name)) //nolint // ok to ignore this test logging
	file.WriteString(description)                            //nolint // ok to ignore this test logging
	file.WriteString("\n")                                   //nolint // ok to ignore this test logging
}

func (k8sh *K8sHelper) appendCronJobDescribe(file *os.File, namespace, name string) {
	description := k8sh.getCronJobDescribe(namespace, name)
	if description == "" {
		return
	}
	writeHeader(file, fmt.Sprintf("CronJob: %s\n", name)) //nolint // ok to ignore this test logging
	file.WriteString(description)                         //nolint // ok to ignore this test logging
	file.WriteString("\n")                                //nolint // ok to ignore this test logging
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

func (k8sh *K8sHelper) appendAuditConfigDescribe(file *os.File, namespace, name string) {
	description := k8sh.getAuditConfigDescribe(namespace, name)
	if description == "" {
		return
	}
	writeHeader(file, fmt.Sprintf("MondooAuditConfig: %s\n", name)) //nolint // ok to ignore this test logging
	file.WriteString(description)                                   //nolint // ok to ignore this test logging
	file.WriteString("\n")                                          //nolint // ok to ignore this test logging
}

func (k8sh *K8sHelper) PrintPodDescribe(namespace string, args ...string) {
	description := k8sh.getPodDescribe(namespace, args...)
	if description == "" {
		return
	}
	zap.S().Infof("POD Description:\n%s", description)
}

func (k8sh *K8sHelper) getDeploymentDescribe(namespace string, args ...string) string {
	args = append([]string{"get", "deployment", "-o", "yaml", "-n", namespace}, args...)
	description, err := k8sh.Kubectl(args...)
	if err != nil {
		zap.S().Errorf("failed to describe deployment. %v %+v", args, err)
		return ""
	}
	return description
}

func (k8sh *K8sHelper) getCronJobDescribe(namespace string, args ...string) string {
	args = append([]string{"get", "cronjob", "-o", "yaml", "-n", namespace}, args...)
	description, err := k8sh.Kubectl(args...)
	if err != nil {
		zap.S().Errorf("failed to describe cronjob. %v %+v", args, err)
		return ""
	}
	return description
}

func (k8sh *K8sHelper) getPodDescribe(namespace string, args ...string) string {
	args = append([]string{"get", "pod", "-o", "yaml", "-n", namespace}, args...)
	description, err := k8sh.Kubectl(args...)
	if err != nil {
		// Completed CronJob pods may be cleaned up before we can describe them
		zap.S().Warnf("failed to describe pod. %v %+v", args, err)
		return ""
	}
	return description
}

func (k8sh *K8sHelper) getAuditConfigDescribe(namespace string, args ...string) string {
	args = append([]string{"get", "mondooauditconfig", "-o", "yaml", "-n", namespace}, args...)
	description, err := k8sh.Kubectl(args...)
	if err != nil {
		zap.S().Errorf("failed to describe mondooauditconfig. %v %+v", args, err)
		return ""
	}
	return description
}

func (k8sh *K8sHelper) getPodsLogs(pods *v1.PodList, namespace, suiteName, testName string) {
	for _, p := range pods.Items {
		k8sh.getPodLogs(p, namespace, suiteName, testName, false)
		if strings.Contains(p.Name, "operator") {
			// get the previous logs for the operator
			k8sh.getPodLogs(p, namespace, suiteName, testName, true)
		}
	}
}

func (k8sh *K8sHelper) createTestLogFile(name, namespace, suiteName, testName, suffix string) (*os.File, error) {
	dir, _ := os.Getwd()
	logDir := path.Join(dir, "_output/tests/")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		err := os.MkdirAll(logDir, 0o777) //nolint:gosec
		if err != nil {
			zap.S().Errorf("Cannot get logs files dir for app : %v in namespace %v, err: %v", name, namespace, err)
			return nil, err
		}
	}

	suiteDir := path.Join(logDir, suiteName)
	if _, err := os.Stat(suiteDir); os.IsNotExist(err) {
		err := os.MkdirAll(suiteDir, 0o777) //nolint:gosec
		if err != nil {
			zap.S().Errorf("Cannot get suite files dir for app : %v in namespace %v, err: %v", name, namespace, err)
			return nil, err
		}
	}

	testDir := path.Join(suiteDir, testName)
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		err := os.MkdirAll(testDir, 0o777) //nolint:gosec
		if err != nil {
			zap.S().Errorf("Cannot get test files dir for app : %v in namespace %v, err: %v", name, namespace, err)
			return nil, err
		}
	}

	fileName := fmt.Sprintf("%s_%s%s_%d.log", namespace, name, suffix, time.Now().Unix())
	fileName = strings.ReplaceAll(fileName, "/", "")
	filePath := path.Join(testDir, fileName)
	file, err := os.Create(filePath) //nolint:gosec
	if err != nil {
		zap.S().Errorf("Cannot create file %s. %v", filePath, err)
		return nil, err
	}

	zap.S().Debugf("created log file: %s", filePath)
	return file, nil
}

func (k8sh *K8sHelper) getPodLogs(pod v1.Pod, namespace, suiteName, testName string, previousLog bool) {
	suffix := ""
	if previousLog {
		suffix = "_previous"
	}
	file, err := k8sh.createTestLogFile(pod.Name, namespace, suiteName, testName, suffix)
	if err != nil {
		return
	}
	defer file.Close() //nolint:errcheck

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
		// We have test cases where containers intentionally will not start.
		// These cases create disturbing stack traces, so ignore them
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
func (k8sh *K8sHelper) CheckForDegradedCondition(
	auditConfig *api.MondooAuditConfig, conditionType api.MondooAuditConfigConditionType, conditionStatus v1.ConditionStatus, msg string,
) error {
	err := k8sh.ExecuteWithRetries(func() (bool, error) {
		// Condition of MondooAuditConfig should be updated
		foundMondooAuditConfig, err := k8sh.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
		if err != nil {
			return false, err
		}
		condition, err := k8sh.GetMondooAuditConfigConditionByType(foundMondooAuditConfig, conditionType)
		if err != nil {
			return false, nil // The condition might not exist yet. This doesn't mean we should stop trying.
		}
		// If there is a msg specified then test for message too
		if condition.Status == conditionStatus && (msg == "" || (msg != "" && strings.Contains(condition.Message, msg))) {
			return true, nil
		}
		return false, nil
	})

	return err
}

// CheckForDegradedCondition Check whether specified Condition is in degraded state in a MondooAuditConfig with retries.
func (k8sh *K8sHelper) WaitForGoodCondition(auditConfig *api.MondooAuditConfig, conditionType api.MondooAuditConfigConditionType) error {
	err := k8sh.ExecuteWithRetries(func() (bool, error) {
		// Condition of MondooAuditConfig should be updated
		foundMondooAuditConfig, err := k8sh.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
		if err != nil {
			return false, err
		}
		condition, err := k8sh.GetMondooAuditConfigConditionByType(foundMondooAuditConfig, conditionType)
		if err != nil {
			return false, nil
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
func (k8sh *K8sHelper) CheckForReconciledOperatorVersion(auditConfig *api.MondooAuditConfig, version string) error {
	val, ok := os.LookupEnv(SkipVersionCheckEnvVar)
	if ok && val == "true" {
		zap.S().Warnf("Skipping version check for MondooAuditConfig reconciliation because %s env var is set", SkipVersionCheckEnvVar)
		return nil
	}

	err := k8sh.ExecuteWithRetries(func() (bool, error) {
		// Condition of MondooAuditConfig should be updated
		foundMondooAuditConfig, err := k8sh.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
		if err != nil {
			return false, err
		}
		reconciledVersion := foundMondooAuditConfig.Status.ReconciledByOperatorVersion
		if reconciledVersion == version {
			return true, nil
		}
		return false, nil
	})

	return err
}

func (k8sh *K8sHelper) GetWorkloadNames(ctx context.Context) ([]string, error) {
	var names []string

	nss := &v1.NamespaceList{}
	if err := k8sh.Clientset.List(ctx, nss); err != nil {
		return nil, err
	}

	for _, ns := range nss.Items {
		names = append(names, ns.Name)
	}

	// pods
	pods := &v1.PodList{}
	if err := k8sh.Clientset.List(ctx, pods); err != nil {
		return nil, err
	}

	for _, w := range pods.Items {
		if cnquery_k8s.PodOwnerReferencesFilter(w.GetOwnerReferences()) {
			continue
		}
		names = append(names, fmt.Sprintf("%s/%s", w.GetNamespace(), w.GetName()))
	}

	jobs := &batchv1.JobList{}
	if err := k8sh.Clientset.List(ctx, jobs); err != nil {
		return nil, err
	}

	for _, w := range jobs.Items {
		if cnquery_k8s.JobOwnerReferencesFilter(w.GetOwnerReferences()) {
			continue
		}
		names = append(names, fmt.Sprintf("%s/%s", w.Namespace, w.Name))
	}

	// cronjobs
	cronjobs := &batchv1.CronJobList{}
	if err := k8sh.Clientset.List(ctx, cronjobs); err != nil {
		return nil, err
	}

	for _, w := range cronjobs.Items {
		names = append(names, fmt.Sprintf("%s/%s", w.Namespace, w.Name))
	}

	// statefulsets
	statefulsets := &appsv1.StatefulSetList{}
	if err := k8sh.Clientset.List(ctx, statefulsets); err != nil {
		return nil, err
	}

	for _, w := range statefulsets.Items {
		names = append(names, fmt.Sprintf("%s/%s", w.Namespace, w.Name))
	}

	// deployments
	deployments := &appsv1.DeploymentList{}
	if err := k8sh.Clientset.List(ctx, deployments); err != nil {
		return nil, err
	}

	for _, w := range deployments.Items {
		names = append(names, fmt.Sprintf("%s/%s", w.Namespace, w.Name))
	}

	// daemonsets
	daemonsets := &appsv1.DaemonSetList{}
	if err := k8sh.Clientset.List(ctx, daemonsets); err != nil {
		return nil, err
	}

	for _, w := range daemonsets.Items {
		names = append(names, fmt.Sprintf("%s/%s", w.Namespace, w.Name))
	}

	// replicasets
	replicasets := &appsv1.ReplicaSetList{}
	if err := k8sh.Clientset.List(ctx, replicasets); err != nil {
		return nil, err
	}

	for _, w := range replicasets.Items {
		if cnquery_k8s.ReplicaSetOwnerReferencesFilter(w.GetOwnerReferences()) {
			continue
		}
		names = append(names, fmt.Sprintf("%s/%s", w.Namespace, w.Name))
	}

	return names, nil
}
