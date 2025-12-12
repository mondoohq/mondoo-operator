ifneq (,$(wildcard ./.env))
	include .env
	export
endif

# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= sha256-$(shell git rev-parse HEAD).sig

COMMIT_SHA ?= $(shell git rev-parse HEAD)

PROTO_VERSION ?= 21.7

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# mondoo.com/mondoo-operator-bundle:$VERSION and mondoo.com/mondoo-operator-catalog:$VERSION.
IMAGE_TAG_BASE ?= ghcr.io/mondoohq/mondoo-operator

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:$(VERSION)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION:v%=%) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):$(VERSION)

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.26

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# List all packages that contain unit tests. Ignore the integration tests folder.
UNIT_TEST_PACKAGES=$(shell go list ./... | grep -v /tests/integration)

# The target architecture for the binaries to be built.
TARGET_ARCH?=$(shell go env GOARCH)
TARGET_OS?=$(shell go env GOOS)

# Linker flags to build the operator binary
LDFLAGS="-s -w -X go.mondoo.com/mondoo-operator/pkg/version.Version=$(VERSION) -X go.mondoo.com/mondoo-operator/pkg/version.Commit=$(COMMIT_SHA)"

all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role paths="./controllers/..."
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd paths="./api/..." output:crd:artifacts:config=config/crd/bases
	$(CONTROLLER_GEN) rbac:roleName=manager-role webhook paths="./pkg/webhooks/..."

generate: controller-gen gomockgen prep/tools ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	echo "Running generate"
	go mod tidy
	echo "Running controller-gen"
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	echo "Running go generate"
	go generate ./controllers/... ./pkg/...

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code. 
	go vet ./...

lint: golangci-lint generate
	$(GOLANGCI_LINT) run

test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) --arch=amd64 use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $(UNIT_TEST_PACKAGES) -coverprofile cover.out

test/ci: manifests generate fmt vet envtest gotestsum
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) --arch=amd64 use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GOTESTSUM) --junitfile unit-tests.xml -- $(UNIT_TEST_PACKAGES) -coverprofile cover.out

# Integration tests are run synchronously to avoid race conditions
ifeq ($(K8S_DISTRO),gke)
test/integration: manifests generate generate-manifests
else ifeq ($(K8S_DISTRO),aks)
test/integration: manifests generate generate-manifests
else ifeq ($(K8S_DISTRO),eks)
test/integration: manifests generate generate-manifests
else ifeq ($(K8S_DISTRO),k3d)
test/integration: manifests generate generate-manifests load-k3d
else
test/integration: manifests generate generate-manifests load-minikube
endif
	go test -ldflags $(LDFLAGS) -v -timeout 45m -p 1 ./tests/integration/...

ifeq ($(K8S_DISTRO),gke)
test/integration/ci: manifests generate generate-manifests gotestsum
else ifeq ($(K8S_DISTRO),aks)
test/integration/ci: manifests generate generate-manifests gotestsum
else ifeq ($(K8S_DISTRO),eks)
test/integration/ci: manifests generate generate-manifests gotestsum
else ifeq ($(K8S_DISTRO),k3d)
test/integration/ci: manifests generate generate-manifests gotestsum load-k3d
else
test/integration/ci: manifests generate generate-manifests gotestsum load-minikube
endif
	$(GOTESTSUM) --junitfile integration-tests.xml -- ./tests/integration/... -ldflags $(LDFLAGS) -v -timeout 45m -p 1

##@ Build

build: manifests generate fmt vet ## Build manager binary.
	CGO_ENABLED=0 GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -o bin/mondoo-operator -ldflags $(LDFLAGS) cmd/mondoo-operator/main.go

run: manifests generate fmt vet ## Run a controller from your host.
	MONDOO_NAMESPACE_OVERRIDE=mondoo-operator go run ./cmd/mondoo-operator/main.go operator

docker-build: TARGET_OS=linux
docker-build: build ## Build docker image with the manager.
	docker build --platform=linux/$(TARGET_ARCH) -t ${IMG} .

load-minikube: docker-build ## Build docker image with the manager and load it into minikube.
	minikube image load ${IMG}

load-k3d: docker-build
	k3d images import ${IMG}

buildah-build: build ## Build container image
	buildah build --platform=${TARGET_OS}/${TARGET_ARCH} -t ${IMG} .

docker-push: ## Push docker image with the manager.
	docker push ${IMG}

# PLATFORMS defines the target platforms for  the manager image be build to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - able to use docker buildx . More info: https://docs.docker.com/build/buildx/
# - have enable BuildKit, More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image for your registry (i.e. if you do not inform a valid value via IMG=<myregistry/image:<tag>> than the export will fail)
# To properly provided solutions that supports more than one platform you should use this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	- docker buildx create --name project-v3-builder
	docker buildx use project-v3-builder
	- docker buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile .
	- docker buildx rm project-v3-builder

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cp config/manager/kustomization.yaml config/manager/kustomization.yaml.before_kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -
	mv config/manager/kustomization.yaml.before_kustomize config/manager/kustomization.yaml

.PHONY: generate-manifests
generate-manifests: manifests kustomize ## Generates manifests and pipes into a yaml file
	echo "Running generate-manifests"
	cp config/manager/kustomization.yaml config/manager/kustomization.yaml.before_kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > mondoo-operator-manifests.yaml
	cp config/webhook/kustomization.yaml config/webhook/kustomization.yaml.before_kustomize
	cd config/webhook && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/webhook > controllers/admission/webhook-manifests.yaml
	mv config/webhook/kustomization.yaml.before_kustomize config/webhook/kustomization.yaml
	mv config/manager/kustomization.yaml.before_kustomize config/manager/kustomization.yaml

.PHONY: deploy-olm
deploy-olm: manifests kustomize ## Deploy using operator-sdk OLM 
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | operator-sdk run bundle --index-image=quay.io/operator-framework/opm:v1.23.0 ${BUNDLE_IMG}

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v4.5.7
CONTROLLER_TOOLS_VERSION ?= v0.19.0

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
## we intentionally install kustomize without the script to prevent GitHub API rate limits
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
ifeq (,$(wildcard $(LOCALBIN)/kustomize))
	curl -sLO https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F$(KUSTOMIZE_VERSION)/kustomize_$(KUSTOMIZE_VERSION)_linux_amd64.tar.gz && \
	tar xzf kustomize_$(KUSTOMIZE_VERSION)_linux_amd64.tar.gz -C $(LOCALBIN)/ && \
	rm kustomize_$(KUSTOMIZE_VERSION)_linux_amd64.tar.gz
endif

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	echo "Installing controller-gen"
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.22

GOTESTSUM = $(LOCALBIN)/gotestsum
gotestsum: $(GOTESTSUM) ## Download gotestsum locally if necessary.
$(GOTESTSUM): $(LOCALBIN)
	echo "Installing gotestsum"
	test -s $(LOCALBIN)/gotestsum || GOBIN=$(LOCALBIN) go install gotest.tools/gotestsum@latest

GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	test -s $(LOCALBIN)/golangci-lint || GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.54

GOMOCKGEN = $(LOCALBIN)/mockgen
.PHONY: gomockgen
gomockgen: $(GOMOCKGEN) ## Download go mockgen locally if necessary.
$(GOMOCKGEN): $(LOCALBIN)
	test -s $(LOCALBIN)/mockgen || GOBIN=$(LOCALBIN) go install github.com/golang/mock/mockgen@v1.6.0

PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

.PHONY: bundle
bundle: manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	cd config/webhook && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/manifests | operator-sdk generate --channels "stable-v1" bundle $(BUNDLE_GEN_FLAGS)
	sed -i -e 's|containerImage: .*|containerImage: $(IMG)|' bundle/manifests/*.clusterserviceversion.yaml
	# TODO: find a portable way to in-place sed edit a file between Linux/MacOS
	# MacOS sed requires a '-i""' to avoid making backup files when doing in-place edits, but that
	# causes trouble for GNU sed which only needs '-i'.
	# Just remove the MacOS-generated backup file in a way that doesn't error on Linux so that the
	# 'validate' step below doesn't complain about multiple CSV files in the generated bundle.
	rm -f bundle/manifests/*-e
	operator-sdk bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM = ./bin/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.23.0/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

HELMIFY = $(LOCALBIN)/helmify
helmify: $(LOCALBIN) ## Download helmify locally if necessary.
	GOBIN=$(LOCALBIN) go install github.com/arttor/helmify/cmd/helmify@v0.4.3

helm: manifests kustomize helmify
	$(KUSTOMIZE) build config/default | $(HELMIFY) $(CHART_NAME)
	# The above command creates a helm chart, which has duplicate labels after templating
	# We can remove the static doublicate labels here
	sed -i -z 's#\(\n[[:blank:]]*selector:\)\n[[:blank:]]*app.kubernetes.io/name: mondoo-operator#\1#' charts/mondoo-operator/templates/metrics-service.yaml
	sed -i -z 's#\([[:blank:]]*selector:\n[[:blank:]]*matchLabels:\)\n[[:blank:]]*app.kubernetes.io/name: mondoo-operator#\1#' charts/mondoo-operator/templates/deployment.yaml
	sed -i -z 's#\([[:blank:]]*template:\n[[:blank:]]*metadata:\n[[:blank:]]*labels:\)\n[[:blank:]]*app.kubernetes.io/name: mondoo-operator#\1#' charts/mondoo-operator/templates/deployment.yaml

# Install prettier gloablly via
# yarn global add prettier --prefix /usr/local
.PHONY: fmt/docs
fmt/docs:
	prettier --write docs/
	prettier --write README.md
	prettier --write RELEASE.md

.PHONY: test/fmt
test/fmt:
	prettier --check docs/
	prettier --check README.md
	prettier --check RELEASE.md

.PHONY: test/github-actions
test/github-actions:
	$(eval TMP_ACT_JSON := $(shell mktemp -t act-json.XXXXX))
	echo '{ "comment": { "body": "not_nil" } }' > $(TMP_ACT_JSON)
	act -n --container-architecture linux/amd64 --eventpath $(TMP_ACT_JSON)
	rm /tmp/act-json.*

.PHONY: test/spell-check
test/spell-check:
	$(eval TMP_ACT_JSON := $(shell mktemp -t act-json.XXXXX))
	echo '{ "comment": { "body": "not_nil" } }' > $(TMP_ACT_JSON)
	act -j spelling --container-architecture linux/amd64 --eventpath $(TMP_ACT_JSON)
	rm /tmp/act-json.*

# we need cnquery due to a few proto files requiring it. proto doesn't resolve dependencies for us
# or download them from the internet, so we are making sure the repo exists this way.
# An alternative (especially for local development) is to soft-link a local copy of the repo
# yourself. We don't pin submodules at this time, but we may want to check if they are up to date here.
prep/tools: prep/tools/ranger
	echo "Running prep/tools"
	command -v protoc-gen-go || go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	command -v protoc-gen-rangerrpc-swagger || go install go.mondoo.com/ranger-rpc/protoc-gen-rangerrpc-swagger@latest

prep/tools/ranger:
	echo "prep/tools/ranger"
	go install go.mondoo.com/ranger-rpc/protoc-gen-rangerrpc@latest

prep/ci/protoc:
	curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v${PROTO_VERSION}/protoc-${PROTO_VERSION}-linux-x86_64.zip
	mkdir tools
	unzip protoc-${PROTO_VERSION}-linux-x86_64.zip -d ./tools
	rm protoc-${PROTO_VERSION}-linux-x86_64.zip

# Copywrite Check Tool: https://github.com/hashicorp/copywrite
license: license/headers/check

license/headers/check:
	copywrite headers --plan

license/headers/apply:
	copywrite headers
