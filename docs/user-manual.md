# User manual
This user manual describes how to install and use the Mondoo Operator.

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
1. Create a new Mondoo service account to report assessments to [Mondoo Platform](https://mondoo.com/docs/platform/service_accounts).
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
Kubernetes webhooks require TLS certs to establish the trust between the certificate authority listed in  `ValidatingWebhookConfiguration.Webhooks[].ClientConfig.CABundle` and the TLS certificates presented when connecting to the HTTPS endpoint specified in the webhook.

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

For more information on how you can configure this, have a look at [this tutorial](https://mondoo.com/docs/tutorials/kubernetes/scan-kubernetes-with-operator/).

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
For more details, have a look at the [docs](https://mondoo.com/docs/supplychain/overview/).
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

The admission controller `Deployment` should start.  The`ValidatingWebhookConfiguration` should be annotated to insert the certificate authority data. cert-manager creates a Secret named `webhook-serving-cert` that contains the TLS certificates.

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

1. Create an additional [Space in Mondoo](https://mondoo.com/docs/platform/spaces/)
2. Create a [Mondoo Service Account](https://mondoo.com/docs/platform/service_accounts/) for this space
3. Create the new namespace in Kubernetes:
```
kubectl create namespace 2nd-namespace
```
4. Create a Kubernetes Service Account in this namespace:
```
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mondoo-operator-k8s-resources-scanning
  namespace: 2nd-namespace
```
5. Bind this Service Account to a Cluster Role which was created during the installation of the operator:
```
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

### I had a `MondooAuditConfig` in my cluster with version `v1alpha1` and now I can no longer access it. What should I do?
Mondoo recently upgraded our CRDs version to `v1alpha2`. You need to manually migrate to the new version. You can list the CRDs with the old version by running:
```bash
kubectl get mondooauditconfigs.v1alpha1.k8s.mondoo.com -A
```

Manually edit each of the CRDs in the list to map it to the new version. 

Note: This is not possible immediately after performing the operator upgrade. 

1. Back up your old `MondooAuditConfig`:
    ```bash
    kubectl get mondooauditconfigs.v1alpha1.k8s.mondoo.com mondoo-client -n mondoo-operator -o yaml > audit-config.yaml
    ```

2. Map the old `v1alpha1` config to the new `v1alpha2` and save the new `MondooAuditConfig`. Find the mapping from `v1alpha1` to `v1alpha2` [here](../api/v1alpha1/mondooauditconfig_types.go#L155-L199).

3. Disable the `webhook` conversion for the `MondooAuditConfig` CRD:
    ```bash
    kubectl edit crd mondooauditconfigs.k8s.mondoo.com
    ```

    Delete or comment out this section:
    ```yaml
    spec:
      # conversion:
      #   strategy: Webhook
      #   webhook:
      #     clientConfig:
      #       service:
      #         name: webhook-service
      #         namespace: mondoo-operator
      #         path: /convert
      #     conversionReviewVersions:
      #     - v1
      group: k8s.mondoo.com
      names:
        kind: MondooAuditConfig
        listKind: MondooAuditConfigList
        plural: mondooauditconfigs
        singular: mondooauditconfig
    ```

4. Apply the updated `MondooAuditConfig`:
    ```bash
    kubectl apply -f audit-config.yaml
    ```

5. Restore the original CRD definition. The easiest way to do that is to apply the manifests from our latest release:
    ```bash
    kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
    ```
