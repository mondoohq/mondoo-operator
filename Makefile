# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= sha256-$(shell git rev-parse HEAD).sig

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

# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):$(VERSION)

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.23

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
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

generate: controller-gen gomockgen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	go generate ./pkg/...

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

lint: golangci-lint
	$(GOLANGCI_LINT) run

test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) --arch=amd64 use $(ENVTEST_K8S_VERSION) -p path)" go test $(UNIT_TEST_PACKAGES) -coverprofile cover.out

test/ci: manifests generate fmt vet envtest gotestsum
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) --arch=amd64 use $(ENVTEST_K8S_VERSION) -p path)" $(GOTESTSUM) --junitfile unit-tests.xml -- $(UNIT_TEST_PACKAGES) -coverprofile cover.out

# Integration tests are run synchronously to avoid race conditions
test/integration: manifests generate generate-manifests load-minikube
	go test -v -timeout 900s -p 1 ./tests/integration/...

test/integration/ci: manifests generate generate-manifests load-minikube gotestsum
	$(GOTESTSUM) --junitfile integration-tests.xml -- ./tests/integration/... -v -timeout 900s -p 1

##@ Build

build: generate fmt vet ## Build manager binary.
	CGO_ENABLED=0 GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -o bin/manager -ldflags "-s -w -X go.mondoo.com/mondoo-operator/pkg/version.Version=${VERSION}" main.go
	CGO_ENABLED=0 GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -o bin/webhook pkg/webhooks/main.go

run: manifests generate fmt vet ## Run a controller from your host.
	MONDOO_OPERATOR_NAMESPACE=mondoo-operator go run ./main.go

docker-build: TARGET_OS=linux
docker-build: build ## Build docker image with the manager.
	docker build --platform=$(TARGET_ARCH) -t ${IMG} .

load-minikube: docker-build ## Build docker image with the manager and load it into minikube.
	minikube image load ${IMG}

buildah-build: test ## Build container image
	buildah build -t ${IMG} .

docker-push: ## Push docker image with the manager.
	docker push ${IMG}

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
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
generate-manifests: manifests kustomize ## Generates manifests and pipes into a yaml file
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > mondoo-operator-manifests.yaml
	cd config/webhook && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/webhook > controllers/admission/webhook-manifests.yaml

.PHONY: deploy-olm
deploy-olm: manifests kustomize ## Deploy using operator-sdk OLM 
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | operator-sdk run bundle ${BUNDLE_IMG}

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -


CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.8.0)

KUSTOMIZE = $(shell pwd)/bin/kustomize
.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

ENVTEST = $(shell pwd)/bin/setup-envtest
.PHONY: envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

GOTESTSUM = $(shell pwd)/bin/gotestsum
gotestsum: ## Download gotestsum locally if necessary.
	$(call go-get-tool,$(GOTESTSUM),gotest.tools/gotestsum@latest)

GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
golangci-lint: ## Download golangci-lint locally if necessary.
	$(call go-get-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@v1.45)

GOMOCKGEN = $(shell pwd)/bin/mockgen
.PHONY: gomockgen
gomockgen: ## Download go mockgen locally if necessary.
	$(call go-get-tool,$(GOMOCKGEN),github.com/golang/mock/mockgen@v1.6.0)
	# mockgen binary needs to be in $PATH
	cp $(GOMOCKGEN) /usr/local/bin/

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: bundle
bundle: manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle $(BUNDLE_GEN_FLAGS)
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
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.19.0/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION:v%=%) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
    BUNDLE_GEN_FLAGS += --use-image-digests
endif

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

HELMIFY = $(shell pwd)/bin/helmify
helmify:
	$(call go-get-tool,$(HELMIFY),github.com/arttor/helmify/cmd/helmify@latest)

helm: manifests kustomize helmify
	$(KUSTOMIZE) build config/default | $(HELMIFY) $(CHART_NAME)

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
