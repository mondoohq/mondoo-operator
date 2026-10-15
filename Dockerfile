# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Use mondoo/cnspec as base image to have cnspec available for k8s scanning
ARG CNSPEC_VERSION=14
FROM mondoo/cnspec:${CNSPEC_VERSION}

# Install the k8s provider needed for k8s resource scanning. Since v14 the k8s provider
# expects os and network as peer providers, and they are not pulled in automatically.
RUN cnspec providers install os && \
    cnspec providers install network && \
    cnspec providers install k8s

WORKDIR /
COPY bin/mondoo-operator .

# Use same user as cnspec base image
USER 100:101

ENTRYPOINT ["/mondoo-operator"]
