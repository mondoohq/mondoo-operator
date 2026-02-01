# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

ARG VERSION

FROM mondoo/cnspec:$VERSION

RUN cnspec providers install os
RUN cnspec providers install network
RUN cnspec providers install k8s

USER 100:101