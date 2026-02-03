# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Use mondoo/cnspec as base image to have cnspec available for k8s scanning
ARG CNSPEC_VERSION=latest
FROM mondoo/cnspec:${CNSPEC_VERSION}

# Install the k8s provider needed for k8s resource scanning
RUN cnspec providers install k8s

WORKDIR /
COPY bin/mondoo-operator .

# Use same user as cnspec base image
USER 100:101

ENTRYPOINT ["/mondoo-operator"]
