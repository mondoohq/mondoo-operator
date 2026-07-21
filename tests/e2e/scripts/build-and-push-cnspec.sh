#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Build a dev cnspec image from a local repo and push to the cloud registry.
# Sets CNSPEC_IMAGE_NAME and CNSPEC_IMAGE_TAG for use in manifest templates.
#
# By default, builds cnspec on top of the upstream mondoo/mql image.
# When MQL_REPO is set, providers are built from that repo and baked into the
# image for testing local mql changes.
#
# Requires:
#   CNSPEC_REPO    — path to local cnspec repo (default: ~/repos/cnspec)
#   REGISTRY_REPO  — cloud container registry (from Terraform outputs)
#   REGION         — cloud region (for Docker auth)
#
# Optional:
#   MQL_REPO       — path to local mql repo. When set, providers are built
#                    from it and baked into the image.
#   MQL_PROVIDERS  — space-separated provider names to build from MQL_REPO
#                    (default: "k8s os")

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${REGISTRY_REPO:?REGISTRY_REPO must be set}"
: "${REGION:?REGION must be set}"

CNSPEC_REPO="${CNSPEC_REPO:-${HOME}/repos/cnspec}"
if [[ ! -d "${CNSPEC_REPO}" ]]; then
  die "cnspec repo not found at ${CNSPEC_REPO}. Set CNSPEC_REPO to the correct path."
fi

CNSPEC_DEV_TAG="dev-$(git -C "${CNSPEC_REPO}" rev-parse --short HEAD)-$(date +%s)"

export CNSPEC_IMAGE_NAME="${REGISTRY_REPO}/cnspec-dev"
export CNSPEC_IMAGE_TAG="${CNSPEC_DEV_TAG}"
CNSPEC_FULL_IMG="${CNSPEC_IMAGE_NAME}:${CNSPEC_IMAGE_TAG}"

info "Building cnspec from ${CNSPEC_REPO}..."
cd "${CNSPEC_REPO}"
make cnspec/build/linux

if [[ -n "${MQL_REPO:-}" ]]; then
  if [[ ! -d "${MQL_REPO}" ]]; then
    die "mql repo not found at ${MQL_REPO}. Set MQL_REPO to the correct path."
  fi

  MQL_PROVIDERS="${MQL_PROVIDERS:-k8s os}"
  PROVIDERS_STAGING="$(mktemp -d)"
  trap 'rm -rf "${PROVIDERS_STAGING}"' EXIT

  for prov in ${MQL_PROVIDERS}; do
    info "Building provider '${prov}' from ${MQL_REPO}..."
    TARGETOS=linux TARGETARCH=amd64 make -C "${MQL_REPO}" "providers/build/${prov}"

    prov_dist="${MQL_REPO}/providers/${prov}/dist"
    if [[ ! -f "${prov_dist}/${prov}" ]]; then
      die "Provider binary not found at ${prov_dist}/${prov} after build"
    fi

    mkdir -p "${PROVIDERS_STAGING}/${prov}"
    cp "${prov_dist}/${prov}" "${prov_dist}/${prov}.json" "${prov_dist}/${prov}.resources.json" \
       "${PROVIDERS_STAGING}/${prov}/"
    info "Staged provider '${prov}'"
  done

  info "Building dev cnspec image: ${CNSPEC_FULL_IMG} (with local providers)"

  cp -r "${PROVIDERS_STAGING}" "${CNSPEC_REPO}/_providers_staging"
  trap 'rm -rf "${CNSPEC_REPO}/_providers_staging" "${PROVIDERS_STAGING}"' EXIT

  cat > "${CNSPEC_REPO}/_Dockerfile.dev" <<'DOCKERFILE'
ARG VERSION
FROM mondoo/mql:${VERSION}
COPY cnspec /usr/local/bin
COPY _providers_staging/ /opt/mondoo/providers/
ENTRYPOINT ["cnspec"]
CMD ["help"]
DOCKERFILE

  docker build --platform=linux/amd64 \
    -f "${CNSPEC_REPO}/_Dockerfile.dev" \
    --build-arg VERSION=latest \
    -t "${CNSPEC_FULL_IMG}" "${CNSPEC_REPO}"

  rm -f "${CNSPEC_REPO}/_Dockerfile.dev"
  rm -rf "${CNSPEC_REPO}/_providers_staging"
else
  info "Building dev cnspec image: ${CNSPEC_FULL_IMG} (base: mondoo/cnspec:latest)"

  cat > "${CNSPEC_REPO}/_Dockerfile.dev" <<'DOCKERFILE'
FROM mondoo/cnspec:latest
COPY cnspec /usr/local/bin/
RUN cnspec providers install os
RUN cnspec providers install network
RUN cnspec providers install k8s
USER 100:101
ENTRYPOINT ["cnspec"]
CMD ["help"]
DOCKERFILE

  docker build --platform=linux/amd64 \
    -f "${CNSPEC_REPO}/_Dockerfile.dev" \
    -t "${CNSPEC_FULL_IMG}" "${CNSPEC_REPO}"

  rm -f "${CNSPEC_REPO}/_Dockerfile.dev"
fi

cd "${REPO_ROOT}"

info "Configuring Docker for Artifact Registry..."
gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet

info "Pushing cnspec image..."
docker push "${CNSPEC_FULL_IMG}"

info "Dev cnspec image pushed: ${CNSPEC_FULL_IMG}"
