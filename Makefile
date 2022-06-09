
# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= latest
REPO ?= yndd

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for ndd packages.
IMAGE_TAG_BASE ?= $(REPO)/config-srl

# Image URL to use all building/pushing image targets
IMG_RECONCILER ?= $(IMAGE_TAG_BASE)-reconciler-controller:$(VERSION)
IMG_WORKER ?= $(IMAGE_TAG_BASE)-worker-controller:$(VERSION)
# Package
PKG_RECONCILER ?= $(IMAGE_TAG_BASE)-reconciler
PKG_WORKER ?= $(IMAGE_TAG_BASE)-worker

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

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	rm -rf package/reconciler/crds/*
	$(CONTROLLER_GEN) crd webhook paths="./..." output:crd:artifacts:config=package/reconciler/crds
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
    @CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o ./bin/manager ./cmd/workercmd/main.go

.PHONY: run
run: generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	docker build -f DockerfileReconciler -t ${IMG_RECONCILER} .
	docker build -f DockerfileWorker -t ${IMG_WORKER} .

.PHONY: docker-build-reconciler
docker-build-reconciler: test ## Build docker images.
	docker build -f DockerfileReconciler -t ${IMG_RECONCILER} .

.PHONY: docker-build-worker
docker-build-worker: test ## Build docker images.
	docker build -f DockerfileWorker -t ${IMG_WORKER} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG_RECONCILER}
	docker push ${IMG_WORKER}

.PHONY: docker-push-reconciler
docker-push-reconciler: ## Push docker images.
	docker push ${IMG_RECONCILER}

.PHONY: docker-push-worker
docker-push-worker: ## Push docker images.
	docker push ${IMG_WORKER}

.PHONY: package-build
package-build: kubectl-ndd ## build ndd package.
	rm -rf package/reconciler/*.nddpkg
	gomplate -d repo=env:REPO -f package/reconciler/ndd.gotmpl > package/reconciler/ndd.yaml
	cd package/reconciler;PATH=$$PATH:$(LOCALBIN) kubectl ndd package build -t provider;cd ../..
	rm -rf package/worker/*.nddpkg
	gomplate -d repo=env:REPO -f package/worker/ndd.gotmpl > package/worker/ndd.yaml
	cd package/worker;PATH=$$PATH:$(LOCALBIN) kubectl ndd package build -t provider;cd ../..

.PHONY: package-push
package-push: kubectl-ndd ## build ndd package.
	cd package/reconciler;ls;PATH=$$PATH:$(LOCALBIN) kubectl ndd package push ${PKG_RECONCILER};cd ../..
	cd package/worker;ls;PATH=$$PATH:$(LOCALBIN) kubectl ndd package push ${PKG_WORKER};cd ../..

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
KUBECTL_NDD ?= $(LOCALBIN)/kubectl-ndd

## Tool Versions
KUSTOMIZE_VERSION ?= v3.8.7
CONTROLLER_TOOLS_VERSION ?= v0.9.0
KUBECTL_NDD_VERSION ?= v0.2.20

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: kubectl-ndd
kubectl-ndd: $(KUBECTL_NDD) ## Download kubectl-ndd locally if necessary.
$(KUBECTL_NDD): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/yndd/ndd-core/cmd/kubectl-ndd@$(KUBECTL_NDD_VERSION)  ;\
