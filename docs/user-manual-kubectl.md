# Install Mondoo Operator with kubectl

The following steps sets up the Mondoo operator using `kubectl` and a manifest file.

## Preconditions:

- `kubectl` and cluster with admin role

## Deployment of Operator using Manifests

1. GET operator manifests

```bash
kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
```

or

```bash
curl -sSL https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml > mondoo-operator-manifests.yaml
kubectl apply -f mondoo-operator-manifests.yaml
```

2. Configure the Mondoo Secret:

- Create a new Mondoo service account to report assessments to [Mondoo Platform](https://mondoo.com/docs/platform/service_accounts)
- Store the service account json into a local file `creds.json`
- Store service account as a secret in the mondoo namespace via:

```bash
kubectl create secret generic mondoo-client --namespace mondoo-operator --from-file=config=creds.json
```

Once the secret is configured, we configure the operator to define the scan targets:

3. Create `mondoo-config.yaml`

```yaml
apiVersion: k8s.mondoo.com/v1alpha1
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: mondoo-operator
spec:
  workloads:
    enable: true
    serviceAccount: mondoo-operator-workload
  nodes:
    enable: true
  mondooSecretRef: mondoo-client
```

4. Apply the configuration via:

```bash
kubectl apply -f mondoo-config.yaml
```

## Deploying the Validating webhook

K8s webhooks require TLS certs to establish the trust between the Certificate Authority listed in the ValidatingWebhookConfiguration.Webhooks[].ClientConfig.CABundle and the TLS certificates presented when connecting to the HTTPS endpoint specified in the webhook.

You can choose to install and use cert-manager to automate the creation and updating of the TLS certs, or you can create (and rotate) your own TLS certificates manually.

A working setup should show the Pods being created/modified/deleted being processed by the webhook Pod.

Display the logs in one window:
`kubectl logs -f deployment/mondoo-operator/mondoo-operator-webhook-manager -n mondoo-operator`

And create/modify/delete a Pod in another window:
`kubectl delete pod -n mondoo-operator --selector control-plane=controller-manager`

### Using cert-manager

1. Install cert-manger on the cluster if it hasn't already been installed https://cert-manager.io/docs/installation/#default-static-install:

2. Update MondooAuditConfig so that the webhook section requests TLS certificates from cert-manager:

```yaml
apiVersion: k8s.mondoo.com/v1alpha1
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: mondoo-operator
spec:
  workloads:
    enable: true
    serviceAccount: mondoo-operator-workload
  webhooks:
    enable: true
    certificateConfig:
      injectionStyle: cert-manager
  nodes:
    enable: true
  mondooSecretRef: mondoo-client
```

3. The webhook Deployment should start up, and the ValidatingWebhookConfiguration should be annotated to insert the Certificate Authority data along with the Secret containing the TLS certs will be created by cert-manager (named webhook-serving-cert).3. The webhook Deployment should start up, and the ValidatingWebhookConfiguration should be annotated to insert the Certificate Authority data along with the Secret containing the TLS certs will be created by cert-manager (named webhook-serving-cert).

### Manually creating TLS certificates using OpenSSL

1. Create key for your CA.
   `openssl genrsa -out ca.key 2048`

2. Generate certificate for your CA.
   `openssl req -x509 -new -nodes -key ca.key -subj "/CN=Webhook Issuer" -days 10000 -out ca.crt`

3. Generate key for webhook server pod.
   `openssl genrsa -out server.key 2048`

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
   `openssl req -new -key server.key -out server.csr -config csr.conf`

6. Generate the certificate for the webhook server.
   `openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 10000 -extensions v3_ext -extfile csr.conf`

7. Create the Secret holding the TLS certificate data.
   `kubectl create secret tls -n mondoo-operator webhook-server-cert --cert ./server.crt --key ./server.key`

8. Add the certificate authority as bas64 encoded CA data (`base64 ./ca.crt`) to the ValidatingWebhookConfiguration under the webhooks[].clientConfig.caBundle field. The end result should look something like this after a `kubectl edit validatingwebhookconfiguration mondoo-operator-mondoo-webhook`:

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

## FAQ

**I do not see the service running, only the operator?**

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
