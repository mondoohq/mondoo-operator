[Role-based access control](https://en.wikipedia.org/wiki/Role-based_access_control) (RBAC) for the Mondoo Operator involves two parts, RBAC rules for the Operator itself and RBAC rules for the Mondoo Pods themselves created by the Operator as Mondoo requires access to the Kubernetes API for resource discovery.

## Mondoo Operator RBAC

In order for the Mondoo Operator to work in an RBAC based authorization environment, a `ClusterRole` with access to all the resources the Operator requires for the Kubernetes API needs to be created.

Here is a ready to use manifest of a `ClusterRole` that can be used to start the Mondoo Operator:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: manager-role
rules:
  - apiGroups:
      - ""
    resources:
      - configmaps
      - services
    verbs:
      - create
      - delete
      - get
      - list
      - patch
      - update
      - watch
  - apiGroups:
      - ""
    resources:
      - namespaces
      - nodes
      - pods
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - ""
    resources:
      - secrets
    verbs:
      - create
      - delete
      - get
  - apiGroups:
      - apps
    resources:
      - daemonsets
      - deployments
    verbs:
      - create
      - delete
      - get
      - list
      - patch
      - update
      - watch
  - apiGroups:
      - apps
    resources:
      - replicasets
      - statefulsets
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - batch
    resources:
      - cronjobs
    verbs:
      - create
      - delete
      - get
      - list
      - patch
      - update
      - watch
  - apiGroups:
      - batch
    resources:
      - jobs
    verbs:
      - deletecollection
      - get
      - list
      - watch
  - apiGroups:
      - k8s.mondoo.com
    resources:
      - mondooauditconfigs
    verbs:
      - get
      - list
      - update
      - watch
  - apiGroups:
      - k8s.mondoo.com
    resources:
      - mondooauditconfigs/finalizers
      - mondoooperatorconfigs/finalizers
    verbs:
      - update
  - apiGroups:
      - k8s.mondoo.com
    resources:
      - mondooauditconfigs/status
      - mondoooperatorconfigs/status
    verbs:
      - get
      - patch
      - update
  - apiGroups:
      - k8s.mondoo.com
    resources:
      - mondoooperatorconfigs
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - monitoring.coreos.com
    resources:
      - servicemonitors
    verbs:
      - create
      - delete
      - get
      - list
      - patch
      - update
      - watch
```

## RBAC rules for Mondoo Workload Scanning

To scan the Kubernetes resources, the mondoo-client needs access to the Kubernetes API. Therefore a separate `ClusterRole` for accessing the data needs to exist.

As Mondoo does not modify any Objects in the Kubernetes API, but just reads them it simply requires the `get`, `list`, and `watch` actions.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: workload
rules:
  - apiGroups:
      - "*"
    resources:
      - "*"
    verbs:
      - get
      - watch
      - list
```

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: workload
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: workload
subjects:
  - kind: ServiceAccount
    name: workload
    namespace: system
```

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: workload
  namespace: system
```

> When `MondooAuditConfig` is created in the same namespace as the operator a service account named `mondoo-operator-k8s-resources-scanning` is added by default. If `MondooAuditConfig` is created in any other namespace create a ServiceAccount in that other namespace and add the ServiceAccount to the `ClusterRoleBinding` named `mondoo-operator-k8s-resources-scanning` that was created during installation of the mondoo-operator. The ServiceAccount needs to be specified in the `MondooAuditConfig` object at `.spec.workload.serviceAccount`.

> Additionally, when defining a `MondooAuditConfig` in a different namespace, a ServiceAccount with no permissions is needed for the node scanning. Create a ServiceAccount named `mondoo-operator-nodes` that will be used by the DaemonSet for node scanning.

> Note: A cluster admin is required to create this `ClusterRole` and create a `ClusterRoleBinding` or `RoleBinding` to the `ServiceAccount` used by the mondoo-client `Pod`s. The `ServiceAccount` used by the workload `Pod`s can be specified in the `MondooAuditConfig` object.

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: mondoo-operator
spec:
  scanner:
    serviceAccountName: workload
  kubernetesResources:
    enable: true
  nodes:
    enable: true
  mondooCredsSecretRef: mondoo-client
```

## RBAC rules for Mondoo Node Scanning

To scan the Kubernetes nodes, Mondoo does not does not require access to the Kubernetes API server, thus a default service account with no permissions should suffice.

> The `ServiceAccount` used by the node-scanner `Pod`s can be specified in the `MondooAuditConfig` object.

> See [Using Authorization Plugins](https://kubernetes.io/docs/reference/access-authn-authz/authorization/) for further usage information on RBAC components.
