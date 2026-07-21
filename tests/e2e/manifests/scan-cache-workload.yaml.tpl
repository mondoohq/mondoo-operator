# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deployment referencing a mutable tag (like :latest).
# The same tag is re-pushed with different content to change the digest,
# simulating the real-world cache invalidation scenario.

apiVersion: apps/v1
kind: Deployment
metadata:
  name: cache-test
  namespace: default
  labels:
    app: cache-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: cache-test
  template:
    metadata:
      labels:
        app: cache-test
    spec:
      containers:
      - name: app
        image: ${CACHE_TEST_IMAGE}
        imagePullPolicy: Always
        command: ["sleep", "infinity"]
