# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 0.0.1

GIT_COMMIT ?= $(shell git rev-parse --short HEAD)

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
# quay.io/edge-infrastructure/kernel-module-management-bundle:$VERSION and quay.io/edge-infrastructure/kernel-module-management-catalog:$VERSION.
IMAGE_TAG_BASE ?= quay.io/edge-infrastructure/kernel-module-management-operator

# This is the default tag of all images made by this Makefile.
IMAGE_TAG ?= latest

SIGNER_IMAGE_TAG_BASE ?= quay.io/edge-infrastructure/kernel-module-management-signimage
SIGNER_IMG ?= $(SIGNER_IMAGE_TAG_BASE):$(IMAGE_TAG)

WEBHOOK_IMAGE_TAG_BASE ?= quay.io/edge-infrastructure/kernel-module-management-webhook-server
WEBHOOK_IMG ?= $(WEBHOOK_IMAGE_TAG_BASE):$(IMAGE_TAG)

WORKER_IMAGE_TAG_BASE ?= quay.io/edge-infrastructure/kernel-module-management-worker
WORKER_IMG ?= $(WORKER_IMAGE_TAG_BASE):$(IMAGE_TAG)

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# GATHER_IMG define the must-gather image used to get all the information for debugging purpose.
GATHER_IMG ?= quay.io/edge-infrastructure/kernel-module-management-must-gather:$(IMAGE_TAG)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):$(IMAGE_TAG)
HUB_IMG ?= $(IMAGE_TAG_BASE)-hub:$(IMAGE_TAG)

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

.PHONY: all
all: generate manager manager-hub manifests

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

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:artifacts:config=config/crd/bases
	$(CONTROLLER_GEN) webhook paths="./internal/webhook" output:webhook:artifacts:config=config/webhook
	$(CONTROLLER_GEN) rbac:roleName=manager-role paths="./internal/controllers" output:rbac:artifacts:config=config/rbac
	# Hub
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:artifacts:config=config/crd-hub/bases
	$(CONTROLLER_GEN) crd paths="./api-hub/..." output:crd:artifacts:config=config/crd-hub/bases
	$(CONTROLLER_GEN) webhook paths="./internal/webhook/hub" output:webhook:artifacts:config=config/webhook-hub
	$(CONTROLLER_GEN) \
		rbac:roleName=manager-role \
		paths="./internal/controllers/hub" \
		paths="internal/controllers/imagestream_reconciler.go" \
		paths="internal/controllers/kernel_dtk_reconciler.go" \
		output:rbac:artifacts:config=config/rbac-hub

.PHONY: generate
generate: controller-gen mockgen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	go generate ./...

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

TEST ?= ./...

.PHONY: unit-test
unit-test: vet ## Run tests.
	go test $(TEST) -coverprofile cover.out

GOFILES_NO_VENDOR = $(shell find . -type f -name '*.go' -not -path "./vendor/*")
.PHONY: lint
lint: golangci-lint ## Run golangci-lint against code.
	@if [ `gofmt -l $(GOFILES_NO_VENDOR) | wc -l` -ne 0 ]; then \
		echo There are some malformed files, please make sure to run \'make fmt\'; \
		gofmt -l $(GOFILES_NO_VENDOR); \
		exit 1; \
	fi
	$(GOLANGCI_LINT) run -v --timeout 10m

##@ Build

manager: $(shell find -name "*.go") go.mod go.sum  ## Build manager binary.
	go build -ldflags="-X main.Version=$(VERSION) -X main.GitCommit=$(GIT_COMMIT)" -o $@ ./cmd/manager

manager-hub: $(shell find -name "*.go") go.mod go.sum  ## Build manager-hub binary.
	go build -ldflags="-X main.Version=$(VERSION) -X main.GitCommit=$(GIT_COMMIT)" -o $@ ./cmd/manager-hub

webhook-server: $(shell find -name "*.go") go.mod go.sum  ## Build webhook-server binary.
	go build -ldflags="-X main.Version=$(VERSION)" -o $@ ./cmd/webhook-server

worker: $(shell find -name "*.go") go.mod go.sum  ## Build worker binary.
	go build -ldflags="-X main.Version=$(VERSION) -X main.GitCommit=$(GIT_COMMIT)" -o $@ ./cmd/worker

day1-utility: $(shell find -name "*.go") go.mod go.sum  ## Build day1 binary
	go build -ldflags="-X main.Version=$(VERSION) -X main.GitCommit=$(GIT_COMMIT)" -o $@ ./cmd/day1-utility

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	docker build -t $(IMG) --build-arg TARGET=manager .

.PHONY: docker-build-hub
docker-build-hub: ## Build docker image with the hub manager.
	docker build -t $(HUB_IMG) --build-arg TARGET=manager-hub .

.PHONY: docker-build-must-gather
docker-build-must-gather: ## Build the must-gather image.
	docker build -t ${GATHER_IMG} -f Dockerfile.must-gather .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push $(IMG)

.PHONY: docker-push-must-gather
docker-push-must-gather: ## Push the must-gather docker image.
	docker push ${GATHER_IMG}

.PHONY: docker-push-hub
docker-push-hub: ## Push docker image with the hub manager.
	docker push $(HUB_IMG)

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

KUSTOMIZE_CONFIG_CRD ?= config/crd

.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	oc apply -k $(KUSTOMIZE_CONFIG_CRD)

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	oc delete -k $(KUSTOMIZE_CONFIG_CRD) --ignore-not-found=$(ignore-not-found)

KUSTOMIZE_CONFIG_DEFAULT ?= config/default
KUSTOMIZE_CONFIG_HUB_DEFAULT ?= config/default-hub

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG) worker=$(WORKER_IMG)
	cd config/manager-base && $(KUSTOMIZE) edit set image must-gather=$(GATHER_IMG) signer=$(SIGNER_IMG)
	cd config/webhook-server && $(KUSTOMIZE) edit set image webhook-server=$(WEBHOOK_IMG)
	oc apply -k $(KUSTOMIZE_CONFIG_DEFAULT)

.PHONY: deploy-hub
deploy-hub: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager-hub && $(KUSTOMIZE) edit set image controller=$(HUB_IMG)
	cd config/manager-base && $(KUSTOMIZE) edit set image must-gather=$(GATHER_IMG) signer=$(SIGNER_IMG)
	cd config/webhook-server && $(KUSTOMIZE) edit set image webhook-server=$(WEBHOOK_IMG)
	oc apply -k $(KUSTOMIZE_CONFIG_HUB_DEFAULT)

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	oc delete -k $(KUSTOMIZE_CONFIG_DEFAULT) --ignore-not-found=$(ignore-not-found)

.PHONY: undeploy-hub
undeploy-hub: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	oc delete -k $(KUSTOMIZE_CONFIG_HUB_DEFAULT) --ignore-not-found=$(ignore-not-found)

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.1)

GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
.PHONY: golangci-lint
golangci-lint: ## Download golangci-lint locally if necessary.
	$(call go-get-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.2)

.PHONY: mockgen
mockgen: ## Install mockgen locally.
	go install go.uber.org/mock/mockgen@v0.3.0

KUSTOMIZE = $(shell pwd)/bin/kustomize
.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	@if [ ! -f ${KUSTOMIZE} ]; then \
		BINDIR=$(shell pwd)/bin ./hack/download-kustomize; \
	fi


# go-get-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
}
endef

OPERATOR_SDK = $(shell pwd)/bin/operator-sdk
.PHONY: operator-sdk
operator-sdk:
	@if [ ! -f ${OPERATOR_SDK} ]; then \
		set -e ;\
		echo "Downloading ${OPERATOR_SDK}"; \
		mkdir -p $(dir ${OPERATOR_SDK}) ;\
		curl -Lo ${OPERATOR_SDK} 'https://github.com/operator-framework/operator-sdk/releases/download/v1.32.0/operator-sdk_linux_amd64'; \
		chmod +x ${OPERATOR_SDK}; \
	fi

.PHONY: bundle
bundle: operator-sdk manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	rm -fr ./bundle
	${OPERATOR_SDK} generate kustomize manifests --apis-dir api
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG) worker=$(WORKER_IMG)
	cd config/manager-base && $(KUSTOMIZE) edit set image must-gather=$(GATHER_IMG) signer=$(SIGNER_IMG)
	cd config/webhook-server && $(KUSTOMIZE) edit set image webhook-server=$(WEBHOOK_IMG)

	OPERATOR_SDK="${OPERATOR_SDK}" \
	BUNDLE_GEN_FLAGS="${BUNDLE_GEN_FLAGS} --extra-service-accounts kmm-operator-module-loader,kmm-operator-device-plugin" \
	PKG=kernel-module-management \
	SOURCE_DIR=$(dir $(realpath $(lastword $(MAKEFILE_LIST)))) \
	./hack/generate-bundle

	${OPERATOR_SDK} bundle validate ./bundle

.PHONY: bundle-hub
bundle-hub: operator-sdk manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	rm -fr bundle-hub

	${OPERATOR_SDK} generate kustomize manifests \
		--apis-dir api-hub \
		--output-dir config/manifests-hub \
		--package kernel-module-management-hub \
		--input-dir config/manifests-hub
	cd config/manager-hub && $(KUSTOMIZE) edit set image controller=$(HUB_IMG)
	cd config/manager-base && $(KUSTOMIZE) edit set image must-gather=$(GATHER_IMG) signer=$(SIGNER_IMG)
	cd config/webhook-server && $(KUSTOMIZE) edit set image webhook-server=$(WEBHOOK_IMG)

	OPERATOR_SDK="${OPERATOR_SDK}" \
	BUNDLE_GEN_FLAGS="${BUNDLE_GEN_FLAGS}" \
	MANIFESTS_DIR=config/manifests-hub \
	PKG=kernel-module-management-hub \
	SOURCE_DIR=$(dir $(realpath $(lastword $(MAKEFILE_LIST)))) \
	SUFFIX="-hub" \
	./hack/generate-bundle

	${OPERATOR_SDK} bundle validate ./bundle-hub

.PHONY: bundle-build-hub
bundle-build-hub: ## Build the bundle-hub image.
	docker build -f bundle-hub.Dockerfile -t $(BUNDLE_IMG) .

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
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.31.0/$${OS}-$${ARCH}-opm ;\
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

.PHONY: signimage-build
signimage-build: ## Build docker image with the signer.
	docker build -f Dockerfile.signimage -t $(SIGNER_IMG) .

.PHONY: webhookimage-build
webhookimage-build: ## Build docker image for the webhook server.
	docker build -f Dockerfile.webhook -t $(WEBHOOK_IMG) .

.PHONY: workerimage-build 
workerimage-build: ## Build docker image for worker app.
	docker build -f Dockerfile.worker -t $(WORKER_IMG) .

include docs.mk
