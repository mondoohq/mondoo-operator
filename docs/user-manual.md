# User manual
This user manual describes the installation and usage of the Mondoo Operator.

## Mondoo Operator Installation
The following section describes the different options for installing the Mondoo Operator on a Kubernetes cluster.

### Installing with kubectl
The following steps set up the Mondoo operator using `kubectl` and a manifest file.

__Preconditions__:
- `kubectl` with cluster admin access

1. Apply the operator manifests
    ```bash
    kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
    ```

    or

    ```bash
    curl -sSL https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml > mondoo-operator-manifests.yaml
    kubectl apply -f mondoo-operator-manifests.yaml
    ```

### Installation with helm
The following steps setup a development Kubernetes to test the operator using [helm](https://helm.sh/).

__Preconditions__:
- `kubectl` with cluster admin access
- `helm 3`

1. Add the helm repo
    ```bash
    helm repo add mondoo https://mondoohq.github.io/mondoo-operator
    helm repo update
    ```

2. Deploy the operator using helm:
    ```bash
    helm install mondoo-operator mondoo/mondoo-operator --namespace mondoo-operator --create-namespace
    ```

### Installation with Operator Lifecycle Manager (OLM)
The following steps sets up the Mondoo operator using [Operator Lifecycle Manager (OLM)](https://olm.operatorframework.io/).

__Preconditions__:
- `kubectl` with cluster admin access
- [`operator-lifecycle-manager`](https://olm.operatorframework.io/) installed in the cluster (see [here](https://olm.operatorframework.io/docs/getting-started/))
- [`operator-sdk`](https://sdk.operatorframework.io/docs/installation/) installed locally


1. Verify that operator-lifecycle-manager is up
    ```bash
    operator-sdk olm status | echo $?
    0
    INFO[0000] Fetching CRDs for version "v0.20.0"
    INFO[0000] Fetching resources for resolved version "v0.20.0"
    INFO[0001] Successfully got OLM status for version "v0.20.0"
    ```

2. Install the Mondoo Operator Bundle
    ```bash
    kubectl create namespace mondoo-operator
    operator-sdk run bundle ghcr.io/mondoohq/mondoo-operator-bundle:latest --namespace=mondoo-operator
    ```

3. Verify that the operator is properly installed
    ```bash
    kubectl get csv -n operators
    ```

## Configuring the Mondoo client secret
To configure the Mondoo client secret to the following steps:
1. Create a new Mondoo service account to report assessments to [Mondoo Platform](https://mondoo.com/docs/platform/service_accounts)
2. Store the service account json into a local file `creds.json`. The `creds.json` should look like the following example:
    ```json
    {
      "mrn": "//agents.api.mondoo.app/spaces/<space name>/serviceaccounts/<Key ID>",
      "space_mrn": "//captain.api.mondoo.app/spaces/<space name>",
      "private_key": "-----BEGIN PRIVATE KEY-----\n....\n-----END PRIVATE KEY-----\n",
      "certificate": "-----BEGIN CERTIFICATE-----\n....\n-----END CERTIFICATE-----\n",
      "api_endpoint": "https://api.mondoo.com"
    }
    ```

3. Store service account as a secret in the mondoo namespace via:
    ```bash
    kubectl create secret generic mondoo-client --namespace mondoo-operator --from-file=config=creds.json
    ```

## Creating the MondooAuditConfig
Once the secret is configured, we configure the operator to define the scan targets:

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

2. Apply the configuration via:
    ```bash
    kubectl apply -f mondoo-config.yaml
    ```

## Deploying the Admission controller
K8s webhooks require TLS certs to establish the trust between the Certificate Authority listed in the `ValidatingWebhookConfiguration.Webhooks[].ClientConfig.CABundle` and the TLS certificates presented when connecting to the HTTPS endpoint specified in the webhook.

You can choose to install and use cert-manager to automate the creation and updating of the TLS certs, or you can use the OpenShift certificate creation/rotation features, or you can create (and rotate) your own TLS certificates manually.

A working setup should show the Pods being created/modified/deleted being processed by the webhook Pod.

Display the logs in one window:
```bash
kubectl logs -f deployment/mondoo-operator/mondoo-operator-webhook-manager -n mondoo-operator
```

And create/modify/delete a Pod in another window:
```bash
kubectl delete pod -n mondoo-operator --selector control-plane=controller-manager
```

### Using cert-manager
The easiest way to bootstrap the admission controller TLS certificate is by using [cert-manager](https://cert-manager.io/). The steps to configure it are described below.

1. Install cert-manger on the cluster if it hasn't already been installed https://cert-manager.io/docs/installation/#default-static-install:

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

3. The admission controller `Deployment` should start up, and the `ValidatingWebhookConfiguration` should be annotated to insert the Certificate Authority data along with the Secret containing the TLS certs will be created by cert-manager (named webhook-serving-cert).

### Manually creating TLS certificates using OpenSSL
It is possible to manually create the TLS certificate required for the admission controller. One way of doing that is described below.

1. Create key for your CA.
   ```bash
   openssl genrsa -out ca.key 2048
   ```

2. Generate certificate for your CA.
   ```bash
   openssl req -x509 -new -nodes -key ca.key -subj "/CN=Webhook Issuer" -days 10000 -out ca.crt
   ```

3. Generate key for webhook server pod.
   ```bash
   openssl genrsa -out server.key 2048
   ```

4. Create a config file csr.conf to specify the options for the webhook server certificate. Substitute NAMESPACE with the namespace where the MondooAuditConfig resource was created (ie. mondoo-operator).
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

5. Generate the certificate signing request.
   ```bash
   openssl req -new -key server.key -out server.csr -config csr.conf
   ```

6. Generate the certificate for the webhook server.
   ```bash
   openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 10000 -extensions v3_ext -extfile csr.conf
   ```

7. Create the Secret holding the TLS certificate data.
   ```bash
   kubectl create secret tls -n mondoo-operator webhook-server-cert --cert ./server.crt --key ./server.key
   ```

8. Add the certificate authority as base64 encoded CA data (`base64 ./ca.crt`) to the ValidatingWebhookConfiguration under the `webhooks[].clientConfig.caBundle` field.
    ```bash
    kubectl edit validatingwebhookconfiguration mondoo-operator-mondoo-webhook
    ```

9.  The end result should look something like this:
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

## Uninstalling operator
Before uninstalling the operator make sure all `MondooAuditConfig` and `MondooOperatorConfig` objects are deleted. You can verify if there are any of them in your cluster by running:
```bash
kubectl get mondooauditconfigs.k8s.mondoo.com,mondoooperatorconfigs.k8s.mondoo.com -A
```

### Uninstalling the operator with kubectl
To uninstall the operator run: 
```bash
kubectl delete -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
```

### Uninstalling the operator with helm
To uninstall the operator run: 
```bash
helm uninstall mondoo-operator
```

### Uninstalling the operator with Operator Lifecycle Manager (OLM)
To uninstall the operator run: 
```bash
operator-sdk olm uninstall mondoo-operator
```

## FAQ

### I do not see the service running, only the operator?
First check that the CRD is properly registered with the operator:

```bash
kubectl get crd
NAME                           CREATED AT
mondooauditconfigs.k8s.mondoo.com   2022-01-14T14:07:28Z
```

Then make sure a configuration for the Mondoo Client is deployed:

```bash
kubectl get mondooauditconfigs -A
NAME                  AGE
mondoo-client        2m44s
```

### How do I edit an existing operator configuration?
```bash
kubectl edit  mondooauditconfigs -n mondoo-operator
```

### Why is there a deployment marked as unschedulable?
For development testing you can resources the allocated resources for the Mondoo Client:

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

### I had a `MondooAuditConfig` in my cluster with version `v1alpha1` and now I can no longer access it?
We recently upgraded our CRDs version to `v1alpha2` and currently manual migration steps need to be executed. You can list the CRDs with the old version by running:
```bash
kubectl get mondooauditconfigs.v1alpha1.k8s.mondoo.com -A
```

Each of the CRDs in the list needs to be manually edited and mapped to the new version. This is not immediately possible after performing the operator upgrade. You can do that by following these steps:

1. Backup your old `MondooAuditConfig`
    ```bash
    kubectl get mondooauditconfigs.v1alpha1.k8s.mondoo.com mondoo-client -n mondoo-operator -o yaml > audit-config.yaml
    ```

2. Map the old `v1alpha1` config to the new `v1alpha2` and save the new `MondooAuditConfig`. The mapping from `v1alpha1` to `v1alpha2` can be found [here](../api/v1alpha1/mondooauditconfig_types.go#L155-L199).

3. Disable the `webhook` conversion for the `MondooAuditConfig` CRD
    ```bash
    kubectl edit crd mondooauditconfigs.k8s.mondoo.com
    ```

    You need to delete or comment out this section:
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

4. Apply the updated `MondooAuditConfig`
    ```bash
    kubectl apply -f audit-config.yaml
    ```

5. Restore the original CRD definition. The easiest way to do that is to just apply the manifests from our latest release:
    ```bash
    kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
    ```
