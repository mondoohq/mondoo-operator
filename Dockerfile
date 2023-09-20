# Copyright (c) Mondoo, Inc.
# SPDX-License-Identifier: BUSL-1.1

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
ADD bin/mondoo-operator .
USER 65532:65532

ENTRYPOINT ["/mondoo-operator"]
