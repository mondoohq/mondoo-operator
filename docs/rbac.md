[Role-based access control](https://en.wikipedia.org/wiki/Role-based_access_control) (RBAC) for the Mondoo Operator involves two parts, RBAC rules for the Operator itself and RBAC rules for the Mondoo Pods themselves created by the Operator as Mondoo requires access to the Kubernetes API for resource discovery.

## Mondoo Operator RBAC

In order for the Mondoo Operator to work in an RBAC based authorization environment, a `ClusterRole` with access to all the resources the Operator requires for the Kubernetes API needs to be created.

Here is a ready to use manifest of a `ClusterRole` that can be used to start the Mondoo Operator:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: operator-role
rules:
- apiGroups:
  - '*'
  resources:
  - '*'
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - admissionregistration.k8s.io
  resources:
  - validatingwebhookconfigurations
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
  - daemonsets
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
  - cert-manager.io
  resources:
  - certificates
  - issuers
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
  - configmaps
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
  - pods
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
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
  - k8s.mondoo.com
  resources:
  - mondooauditconfigs
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - k8s.mondoo.com
  resources:
  - mondooauditconfigs/finalizers
  verbs:
  - update
- apiGroups:
  - k8s.mondoo.com
  resources:
  - mondooauditconfigs/status
  verbs:
  - get
  - patch
  - update
```

## Workload RBAC

The Mondoo workload-scanner itself accesses the Kubernetes API to discover resources. Therefore a separate `ClusterRole` for the workload-scanner needs to exist.

As Mondoo does not modify any Objects in the Kubernetes API, but just reads them it simply requires the `get`, `list`, and `watch` actions.


```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: workload
rules:
- apiGroups:
  - '*'
  resources:
  - '*'
  verbs:
  - get
  - watch
  - list
```
> When `MondooAuditConfig` is created in the same namespace as the operator a service account is added by default. If `MondooAuditConfig` is created in any other namespace a  `Clusterrolebinding` or `Rolebinding` needs to be created. The subject serviceaccout name needs to be added to the `MondooAuditConfig` object.
> Note: A cluster admin is required to create this `ClusterRole` and create a `ClusterRoleBinding` or `RoleBinding` to the `ServiceAccount` used by the mondoo-scanner `Pod`s. The `ServiceAccount` used by the workload `Pod`s can be specified in the `MondooAuditConfig` object.


## Node RBAC

The Mondoo node-scanner does not require access to the Kubernetes API server, thus a default service account with no permissions should suffice.


> The `ServiceAccount` used by the node-scanner `Pod`s can be specified in the `MondooAuditConfig` object.



> See [Using Authorization Plugins](https://kubernetes.io/docs/reference/access-authn-authz/authorization/) for further usage information on RBAC components.