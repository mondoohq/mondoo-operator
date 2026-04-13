# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deploys nginx from a private cloud registry (no imagePullSecrets).
# The container image scanner must use WIF to authenticate and pull this image.
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-private-workload
  namespace: default
  labels:
    app: nginx-private-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx-private-test
  template:
    metadata:
      labels:
        app: nginx-private-test
    spec:
      containers:
      - name: nginx
        image: ${PRIVATE_IMAGE}
        ports:
        - containerPort: 80
