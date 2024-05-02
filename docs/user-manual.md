# User manual

This user manual describes how to install and use the Mondoo Operator.

- [User manual](#user-manual)
  - [Mondoo Operator Installation](#mondoo-operator-installation)
    - [Installing with kubectl](#installing-with-kubectl)
    - [Installing with Helm](#installing-with-helm)
      - [Customization](#customization)
    - [Installing with Operator Lifecycle Manager (OLM)](#installing-with-operator-lifecycle-manager-olm)
  - [Configuring the Mondoo Secret](#configuring-the-mondoo-secret)
  - [Creating a MondooAuditConfig](#creating-a-mondooauditconfig)
    - [Filter Kubernetes objects based on namespace](#filter-kubernetes-objects-based-on-namespace)
  - [Deploying the admission controller](#deploying-the-admission-controller)
    - [Scanned workload types](#scanned-workload-types)
    - [Different modes of operation](#different-modes-of-operation)
    - [Deploying the admission controller using cert-manager](#deploying-the-admission-controller-using-cert-manager)
    - [Manually creating TLS certificates using OpenSSL](#manually-creating-tls-certificates-using-openssl)
    - [Firewall rules for the webhook](#firewall-rules-for-the-webhook)
  - [Creating a secret for private image scanning](#creating-a-secret-for-private-image-scanning)
  - [Installing Mondoo into multiple namespaces](#installing-mondoo-into-multiple-namespaces)
  - [Adjust the scan interval](#adjust-the-scan-interval)
  - [Configure resources for the operator and its components](#configure-resources-for-the-operator-and-its-components)
    - [Configure resources for the operator-controller](#configure-resources-for-the-operator-controller)
    - [Configure resources for the different scanning components](#configure-resources-for-the-different-scanning-components)
  - [Uninstalling the Mondoo operator](#uninstalling-the-mondoo-operator)
    - [Uninstalling the operator with kubectl](#uninstalling-the-operator-with-kubectl)
    - [Uninstalling the operator with Helm](#uninstalling-the-operator-with-helm)
    - [Uninstalling the operator with Operator Lifecycle Manager (OLM)](#uninstalling-the-operator-with-operator-lifecycle-manager-olm)
    - [Cleanup failed uninstalls](#cleanup-failed-uninstalls)
  - [FAQ](#faq)
    - [I do not see the service running, only the operator. What should I do?](#i-do-not-see-the-service-running-only-the-operator-what-should-i-do)
    - [How do I edit an existing operator configuration?](#how-do-i-edit-an-existing-operator-configuration)
    - [How do I run asset garbage collection manually?](#how-do-i-run-asset-garbage-collection-manually)
    - [Why is there a deployment marked as unschedulable?](#why-is-there-a-deployment-marked-as-unschedulable)
    - [Why are (some of) my nodes unscored?](#why-are-some-of-my-nodes-unscored)
    - [How can I trigger a new scan?](#how-can-i-trigger-a-new-scan)

## Mondoo Operator Installation

Install the Mondoo Operator using kubectl, Helm, or Operator Lifecycle Manager.

### Installing with kubectl

Follow this step to set up the Mondoo Operator using kubectl and a manifest file.

Precondition: kubectl with cluster admin access

To install with kubectl, apply the operator manifests:

```bash
kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
```

or

```bash
curl -sSL https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml > mondoo-operator-manifests.yaml
kubectl apply -f mondoo-operator-manifests.yaml
```

### Installing with Helm

Follow these steps to set up the Mondoo Operator using [Helm](https://helm.sh/).

Preconditions:

- `kubectl` with cluster admin access
- `helm 3`

1. Add the Helm repo:

   ```bash
   helm repo add mondoo https://mondoohq.github.io/mondoo-operator
   helm repo update
   ```

2. Deploy the operator using Helm:

   ```bash
   helm install mondoo-operator mondoo/mondoo-operator --namespace mondoo-operator --create-namespace
   ```

#### Customization

In case you set the Chart name to a different name, this will break parts of the operator, unless you also set:
```
fullnameOverride=mondoo-operator
```

### Installing with Operator Lifecycle Manager (OLM)

Follow these steps to set up the Mondoo Operator using [Operator Lifecycle Manager (OLM)](https://olm.operatorframework.io/):

Preconditions:

- kubectl with cluster admin access
- [`operator-lifecycle-manager`](https://olm.operatorframework.io/) installed in the cluster (see the [OLM QuickStart](https://olm.operatorframework.io/docs/getting-started/))
- [`operator-sdk`](https://sdk.operatorframework.io/docs/installation/) installed locally

1. Verify that operator-lifecycle-manager is up:

   ```bash
   operator-sdk olm status | echo $?
   0
   INFO[0000] Fetching CRDs for version "v0.20.0"
   INFO[0000] Fetching resources for resolved version "v0.20.0"
   INFO[0001] Successfully got OLM status for version "v0.20.0"
   ```

2. Install the Mondoo Operator bundle:

   ```bash
   kubectl create namespace mondoo-operator
   operator-sdk run bundle ghcr.io/mondoohq/mondoo-operator-bundle:latest --namespace=mondoo-operator
   ```

3. Verify that the operator is properly installed:

   ```bash
   kubectl get csv -n operators
   ```

## Configuring the Mondoo Secret

Follow these steps to configure the Mondoo Secret:

1. Create a new Mondoo service account to report assessments to [Mondoo Platform](https://mondoo.com/docs/platform/maintain/access/service_accounts/).
2. Store the service account json into a local file `creds.json`. The `creds.json` file should look like this:

   ```json
   {
     "mrn": "//agents.api.mondoo.app/spaces/<space name>/serviceaccounts/<Key ID>",
     "space_mrn": "//captain.api.mondoo.app/spaces/<space name>",
     "private_key": "-----BEGIN PRIVATE KEY-----\n....\n-----END PRIVATE KEY-----\n",
     "certificate": "-----BEGIN CERTIFICATE-----\n....\n-----END CERTIFICATE-----\n",
     "api_endpoint": "https://api.mondoo.com"
   }
   ```

3. Store the service account as a Secret in the Mondoo namespace:

   ```bash
   kubectl create secret generic mondoo-client --namespace mondoo-operator --from-file=config=creds.json
   ```

## Creating a MondooAuditConfig

Once the Secret is configured, configure the operator to define the scan targets:

1. Create `mondoo-config.yaml`:

   ```yaml
   apiVersion: k8s.mondoo.com/v1alpha2
   kind: MondooAuditConfig
   metadata:
     name: mondoo-client
     namespace: mondoo-operator
   spec:
     mondooCredsSecretRef:
       name: mondoo-client
     kubernetesResources:
       enable: true
     nodes:
       enable: true
   ```

2. Apply the configuration:

   ```bash
   kubectl apply -f mondoo-config.yaml
   ```

### Filter Kubernetes objects based on namespace

To exclude specific namespaces add this to your `MondooAuditConfig`:

```
...
spec:
...
  filtering:
    namespaces:
      exclude:
        - kube-system
```

When you only want to scan specific namespaces:

```
...
spec:
...
  filtering:
    namespaces:
      include:
        - app1
        - backend2
        - ...
```

## Deploying the admission controller

Kubernetes webhooks require TLS certs to establish the trust between the certificate authority listed in `ValidatingWebhookConfiguration.Webhooks[].ClientConfig.CABundle` and the TLS certificates presented when connecting to the HTTPS endpoint specified in the webhook.

You can choose one of three approaches:

- Install and use cert-manager to automate the creation and update of the TLS certs
- Use the OpenShift certificate creation/rotation features
- Create (and rotate) your own TLS certificates manually

A working setup shows the webhook Pod processing the created/modified Pods.

Display the logs in one window:

```bash
kubectl logs -f deployment/mondoo-client-webhook-manager -n mondoo-operator
```

And delete a Pod in another window (which will cause a new one to be created):

```bash
kubectl delete pod -n mondoo-operator --selector app.kubernetes.io/name=mondoo-operator
```

### Scanned workload types

Currently, the admission controller can scan these workload types:

- Pods
- Deployments
- DaemonSets
- StatefulSets
- Jobs
- CronJobs

If a workload is dependent on another workload, the admission controller only scans the owner workload.
For example, if a Deployment creates a Pod, the admission controller skips the Pod and scans the Deployment.
The owner workload is the definition where you can fix issues permanently.

For more information on how you can configure this, have a look at [this tutorial](https://mondoo.com/docs/platform/infra/cloud/kubernetes/scan-kubernetes-with-operator/).

### Different modes of operation

You can run the admission controller in two modes: permissive and enforcing.
You configure the mode via the `MondooAuditConfig`:

```yaml
    apiVersion: k8s.mondoo.com/v1alpha2
    kind: MondooAuditConfig
    ...
    spec:
      ...
      admission:
        enable: true
        mode: permissive
        replicas: 1
      scanner:
        replicas: 1
```

When admission is enabled, the default mode is `permissive` with one replica.
In permissive mode, the webhook checks objects like Deployments or Pods against policies and reports problems to the Mondoo Backend.
Mondoo shows the results in the CI/CD view.
For more details, have a look at the [docs](https://mondoo.com/docs/platform/infra/supply/overview/).
In enforcing mode, the operator automatically sets the `failurePolicy` of the `ValidatingWebhookConfiguration` to `Fail`.
The webhook then will deny objects not passing the policy.
The details are reported to the Mondoo Backend.

With kubectl, this looks similar to this example:

```bash
$ kubectl apply -f ubuntu-privileged.yaml
Error from server (FAILED MONDOO SCAN): error when creating "ubuntu-privileged.yaml": admission webhook "policy.k8s.mondoo.com" denied the request: FAILED MONDOO SCAN
```

> :warning: The default replica count of one is not meant for production usage in enforcing mode.
>
> Increase replicas for webhook **and** scanner to at least two.

The Deployments are configured with [`PodAntiAffinity`](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#inter-pod-affinity-and-anti-affinity).
This, with a replica count of two, helps to prevent outages because of single Pod or Node failures.
Please increase the replicas count according to your needs.

### Deploying the admission controller using cert-manager

[cert-manager](https://cert-manager.io/) is the easiest way to bootstrap the admission controller TLS certificate:

1. Install cert-manger on the cluster if it isn't already installed. ([See instructions](https://cert-manager.io/docs/installation/).)

2. Update MondooAuditConfig so that the webhook section requests TLS certificates from cert-manager:

   ```yaml
   apiVersion: k8s.mondoo.com/v1alpha2
   kind: MondooAuditConfig
   metadata:
     name: mondoo-client
     namespace: mondoo-operator
   spec:
     mondooCredsSecretRef:
       name: mondoo-client
     kubernetesResources:
       enable: true
     nodes:
       enable: true
     admission:
       enable: true
       certificateProvisioning:
         mode: cert-manager
   ```

The admission controller `Deployment` should start. The`ValidatingWebhookConfiguration` should be annotated to insert the certificate authority data. cert-manager creates a Secret named `webhook-serving-cert` that contains the TLS certificates.

### Manually creating TLS certificates using OpenSSL

You can manually create the TLS certificate required for the admission controller. These steps show one method:

1. Create a key for your certificate authority:

   ```bash
   openssl genrsa -out ca.key 2048
   ```

2. Generate a certificate for your certificate authority:

   ```bash
   openssl req -x509 -new -nodes -key ca.key -subj "/CN=Webhook Issuer" -days 10000 -out ca.crt
   ```

3. Generate a key for the webhook server Pod:

   ```bash
   openssl genrsa -out server.key 2048
   ```

4. Create a config file named `csr.conf` to specify the options for the webhook server certificate. Substitute NAMESPACE with the namespace where the MondooAuditConfig resource was created (ie. mondoo-operator):

   ```
   [ req ]
   default_bits = 2048
   prompt = no
   default_md = sha256
   req_extensions = req_ext
   distinguished_name = dn
   [ dn ]
   C = US
   ST = NY
   L = NYC
   O = My Company
   OU = My Organization
   CN = Webhook Server
   [ req_ext ]
   subjectAltName = @alt_names
   [ alt_names ]
   DNS.1 = mondoo-operator-webhook-service.NAMESPACE.svc
   DNS.2 = mondoo-operator-webhook-service.NAMESPACE.svc.cluster.local
   [ v3_ext ]
   authorityKeyIdentifier=keyid,issuer:always
   basicConstraints=CA:FALSE
   keyUsage=keyEncipherment,dataEncipherment
   extendedKeyUsage=serverAuth,clientAuth
   subjectAltName=@alt_names
   ```

5. Generate the certificate signing request:

   ```bash
   openssl req -new -key server.key -out server.csr -config csr.conf
   ```

6. Generate the certificate for the webhook server:

   ```bash
   openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 10000 -extensions v3_ext -extfile csr.conf
   ```

7. Create the Secret holding the TLS certificate data:

   ```bash
   kubectl create secret tls -n mondoo-operator webhook-server-cert --cert ./server.crt --key ./server.key
   ```

8. Add the certificate authority as base64 encoded CA data (`base64 ./ca.crt`) to the ValidatingWebhookConfiguration under the `webhooks[].clientConfig.caBundle` field:

   ```bash
   kubectl edit validatingwebhookconfiguration mondoo-operator-mondoo-webhook
   ```

The end result should resemble this:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  annotations:
    mondoo.com/tls-mode: manual
  name: mondoo-operator-mondoo-webhook
webhooks:
  - admissionReviewVersions:
      - v1
    clientConfig:
      caBundle: BASE64-ENCODED-DATA-HERE
      service:
        name: mondoo-operator-webhook-service
        namespace: mondoo-operator
        path: /validate-k8s-mondoo-com-core
        port: 443
```

### Firewall rules for the webhook

Make sure your Kubernetes API servers can connect to the webhook.
Otherwise, you would see connection timeout errors in your API server logs.
An example for an EKS basic firewall rule can be found [here](../.github/terraform/aws/main.tf#L110)

## Creating a secret for private image scanning

To allow the Mondoo operator to scan private images, it needs access to image pull secrets for these private registries.
Please create a secret with the name `mondoo-private-registries-secrets` within the same namespace you created your `MondooAuditConfig`.
The Mondoo operator will only read a secret with this exact name so that we can limit required RBAC permissions.
Please ensure the secret contains access data to all the registries you want to scan.
Now add the name to your `MondooAuditConfig` so that the operator knows you want to scan private images.

```yaml
    apiVersion: k8s.mondoo.com/v1alpha2
    kind: MondooAuditConfig
    ...
    spec:
      ...
      kubernetesResources:
        enable: true
        containerImageScanning: true
        privateRegistriesPullSecretRef:
          name: "mondoo-private-registries-secrets"
      ...
```

You can find examples of creating such a secret [here](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/).

It is also possible to create a secret with a different name, but by default the operator isn't allowed to read the secret.
Please extend RBAC in a way, that the `ServiceAccount` `mondoo-operator-k8s-resources-scanning` has the privilege to get the secret.

## Installing Mondoo into multiple namespaces

You can deploy the mondoo client into multiple namespaces with just a single operator running inside the cluster.

We assume you already have the operator running inside the default namespace.
Now you want to send the data from a different namespace into another Mondoo Space.
To do so, follow these steps:

1. Create an additional [Space in Mondoo](https://mondoo.com/docs/platform/start/organize/spaces/)
2. Create a [Mondoo Service Account](https://mondoo.com/docs/platform/maintain/access/service_accounts/) for this space
3. Create the new namespace in Kubernetes:

  ```bash
  kubectl create namespace 2nd-namespace
  ```

4. Create a Kubernetes Service Account in this namespace:

  ```yaml
  apiVersion: v1
  kind: ServiceAccount
  metadata:
    name: mondoo-operator-k8s-resources-scanning
    namespace: 2nd-namespace
  ```

5. Bind this Service Account to a Cluster Role which was created during the installation of the operator:

  ```yaml
  apiVersion: rbac.authorization.k8s.io/v1
  kind: ClusterRoleBinding
  metadata:
    name: k8s-resources-scanning
  roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: ClusterRole
    name: mondoo-operator-k8s-resources-scanning
  subjects:
    - kind: ServiceAccount
      name: mondoo-operator-k8s-resources-scanning
      namespace: 2nd-namespace
  ```

6. Add the Mondoo Service Account as a secret to the namespace as described [here](https://github.com/mondoohq/mondoo-operator/blob/main/docs/user-manual.md#configuring-the-mondoo-secret)
7. Create a `MondooAuditConfig` in `2nd-namespace` as described [here](https://github.com/mondoohq/mondoo-operator/blob/main/docs/user-manual.md#creating-a-mondooauditconfig)
8. (Optional) In case you want to separate which Kubernetes namespaces show up in which Mondoo Space, you can add [filtering](https://github.com/mondoohq/mondoo-operator/blob/main/docs/user-manual.md#filter-kubernetes-objects-based-on-namespace).

After some seconds, you should see that the operator picked up the new `MondooAuditConfig` and starts creating objects.

## Adjust the scan interval

You can adjust the interval for scans triggered via a CronJob.
Edit the `MondooAuditConfig` to adjust the interval:

```bash
kubectl -n mondoo-operator edit mondooauditconfigs.k8s.mondoo.com mondoo-client
```

```
  kubernetesResources:
    enable: true
    schedule: 41 * * * *
```

You can adjust the schedule for the following components:
- Kubernetes Resources Scanning
- Container Image Scanning
- Node Scanning

## Configure resources for the operator and its components

### Configure resources for the operator-controller

To change resources for the `mondoo-operator-controller-manager`, you need to change the Deployment:

```
kubectl -n mondoo-operator edit deployment mondoo-operator-controller-manager
```

The `mondoo-operator-controller-manager` has predefined `requests` and `limits`.
Depending on your cluster size, something other than these might work better.
During editing, search for the defaults in the manifest:
```
        resources:
          limits:
            cpu: 200m
            memory: 140Mi
          requests:
            cpu: 100m
            memory: 70Mi

```
Increase them as required.

### Configure resources for the different scanning components

The `mondoo-operator-controller-manager` manages the other Deployments and CronJobs needed to scan your cluster.
If the provided `requests` and `limits` do not match your cluster size, increase them as needed.
For components, do **not** edit the Deployments or CronJobs directly.
The `mondoo-operator-controller-manager` will revert your changes.
Instead, edit the `MondooAuditConfig`:
```
kubectl -n mondoo-operator edit mondooauditconfigs.k8s.mondoo.com mondoo-client
```
You can change the resources for different components in the config:
```
spec:
...
  containers:
    resources: {}
...
  nodes:
    enable: true
    resources: {}
  scanner:
    image: {}
    privateRegistriesPullSecretRef: {}
    replicas: 1
    resources: {}
    serviceAccountName: mondoo-operator-k8s-resources-scanning
...
```

The `resources` field accepts the [Kubernetes resource definitions](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/):
```
spec:
...
  containers:
    resources: {}
...
  nodes:
    enable: true
    resources:
      limits:
        cpu: 1
        memory: 1Gi
      requests:
        cpu: 500m
        memory: 200Mi
  scanner:
    image: {}
    privateRegistriesPullSecretRef: {}
    replicas: 1
    resources: {}
    serviceAccountName: mondoo-operator-k8s-resources-scanning
...
```
After you saved the changes, the `mondoo-operator-controller-manager` will adjust the corresponding Deployment or CronJob.

## Uninstalling the Mondoo operator

Before uninstalling the Mondoo operator, be sure to delete all `MondooAuditConfig` and `MondooOperatorConfig` objects. You can find any in your cluster by running:

```bash
kubectl get mondooauditconfigs.k8s.mondoo.com,mondoooperatorconfigs.k8s.mondoo.com -A
```

### Uninstalling the operator with kubectl

Run:

```bash
kubectl delete -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
```

### Uninstalling the operator with Helm

Run:

```bash
helm uninstall mondoo-operator
```

### Uninstalling the operator with Operator Lifecycle Manager (OLM)

Run:

```bash
operator-sdk olm uninstall mondoo-operator
```

### Cleanup failed uninstalls

Under normal circumstances, the above steps clean up the complete operator installation.
In rare cases, the namespace gets stuck in `terminating` state.
This most likely happens because of [finalizers](https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/).

This can happen when the operator-controller isn't running correctly during uninstall.

To clean up the namespace:
- Find objects that are still present in the namespace:
  ```
  kubectl api-resources --verbs=list --namespaced -o name | xargs -n 1 kubectl get --show-kind --ignore-not-found -n mondoo-operator
  ```
- Check the remaining objects for `finalizers`, e.g., the `MondooAuditConfig`:
  ```
  kubectl -n mondoo-operator get mondooauditconfigs.k8s.mondoo.com mondoo-client -o jsonpath='{.metadata.finalizers}'
  ```
- Remove the `finalizers` from the object:
  ```
  kubectl -n mondoo-operator patch --type=merge mondooauditconfigs.k8s.mondoo.com mondoo-client -p '{"metadata":{"finalizers":null}}'
  ```

The namespace should now automatically clean up after a short time.


## FAQ

### I do not see the service running, only the operator. What should I do?

1. Check that the CRD is properly registered with the operator:

```bash
kubectl get crd
NAME                           CREATED AT
mondooauditconfigs.k8s.mondoo.com   2022-01-14T14:07:28Z
```

2. Make sure a configuration for the Mondoo Client is deployed:

```bash
kubectl get mondooauditconfigs -A
NAME                  AGE
mondoo-client        2m44s
```

### How do I edit an existing operator configuration?

Run:

```bash
kubectl edit  mondooauditconfigs -n mondoo-operator
```

### How do I run asset garbage collection manually?

The operator will trigger garbage collection after each successful cluster scan. This will delete all old assets that were created
by the operator but are no longer present in the cluster. It is possible to trigger garbage collection manually. To do this it is required
to have the Mondoo Client API running locally:

```bash
mondoo serve --api --token abcdefgh
```

Retrieve the cluster UID:

```bash
kubectl get ns kube-system -o yaml | grep uid
```

Then you can trigger asset garbage collection for workloads by the following command:

```bash
mondoo-operator garbage-collect --filter-managed-by mondoo-operator-<<cluster UID>> --filter-older-than 2h --filter-platform-runtime k8s-cluster --scan-api-url http://127.0.0.1:8989 --token abcdefgh
```

For container images use:

```bash
mondoo-operator garbage-collect --filter-managed-by mondoo-operator-<<cluster UID>> --filter-older-than 48h --filter-platform-runtime docker-registry --scan-api-url http://127.0.0.1:8989 --token abcdefgh
```

For different use-cases adjust the CLI arguments.

### Why is there a deployment marked as unschedulable?

For development testing, you can see the allocated resources for the Mondoo Client:

```yaml
spec:
  mondooCredsSecretRef: mondoo-client
  scanner:
    resources:
      limits:
        cpu: 500m
        memory: 900Mi
      requests:
        cpu: 100m
        memory: 20Mi
  kubernetesResources:
    enable: true
  nodes:
    enable: true
```

### Why are (some of) my nodes unscored?

In some cases a node scan can require more memory than initially allotted. You can check whether that is the case by running:

```bash
kubectl get pods -n mondoo-operator
```

Look for pods in the form of `<mondooauditconfig-name>-node-<node-name>-hash`. For example, if your `MondooAuditConfig` is called `mondoo-client` and you have a node called `node01`, you should be able to find a pod `mondoo-client-node-node01-<hash>`.

If the pod is crashing and restarting, it's most probably running out of memory and terminating. You can verify that by looking into the pod's status:

```bash
kubectl get pods -n mondoo-operator mondoo-client-node-node01-<hash> -o yaml
```

If you need to increase the resource limits for node scanning, change your `MondooAuditConfig`:

```bash
kubectl edit -n mondoo-operator mondooauditconfig mondoo-client
```

Search for the `nodes:` section and specify the new limits there. It should look like this:

```yaml
spec:
  nodes:
    enable: true
    resources:
      limits:
        cpu: 200m
        memory: 200Mi
```

### How can I trigger a new scan?

The operator runs a full cluster scan and node scans hourly. If you need to manually trigger those scans there are two options:

Option A: Create a job from the existing cron job

1. Locate the cron job you want to trigger:

```bash
kubectl get cronjobs -n mondoo-operator
```

2. Create a new job from the existing cron job. To trigger a new cluster scan, the command is:

```bash
kubectl create job -n mondoo-operator my-job --from=cronjob/mondoo-client-k8s-scan
```

3. A job called `my-job` starts the scan immediately.

Option B: Turn scanning off and then on again

1. Edit the `MondooAuditConfig`:

```bash
kubectl edit -n mondoo-operator mondooauditconfig mondoo-client
```

2. Disable scanning by changing `enable: true` to `enable: false`:

```yaml
spec:
  kubernetesResources:
    enable: false
  nodes:
    enable: false
```

3. Make sure the scan cron jobs are deleted before proceeding:

```bash
kubectl get cronjobs -n mondoo-operator
```

4. Edit the `MondooAuditConfig` again and re-enable scanning:

```yaml
spec:
  kubernetesResources:
    enable: true
  nodes:
    enable: true
```

5. The scan cron jobs will be re-created and their initial run will occur within the next minute.
