
# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= latest

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for ndd packages.
IMAGE_TAG_BASE ?= yndd/ndd-config-srl

# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE)-controller:$(VERSION)
IMG_PROVIDER ?= $(IMAGE_TAG_BASE)-provider:$(VERSION)
IMG_WORKER ?= $(IMAGE_TAG_BASE)-worker:$(VERSION)
# Package
PKG ?= $(IMAGE_TAG_BASE)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

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

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	rm -rf package/crds/
	$(CONTROLLER_GEN) $(CRD_OPTIONS) webhook paths="./..." output:crd:artifacts:config=package/crds
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
test: generate fmt vet ## Run tests.
	mkdir -p ${ENVTEST_ASSETS_DIR}
	test -f ${ENVTEST_ASSETS_DIR}/setup-envtest.sh || curl -sSLo ${ENVTEST_ASSETS_DIR}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.9.3/hack/setup-envtest.sh
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out

##@ Build

build: generate fmt vet ## Build manager binary.
    @CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o ./bin/manager cmd/combinedcmd/main.go

run: generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

docker-build: test ## Build docker image with the manager.
	docker build -f DockerfileController -t ${IMG} .
	docker build -f DockerfileProvider -t ${IMG_PROVIDER} .
	docker build -f DockerfileWorker -t ${IMG_WORKER} .

docker-build-controller: test ## Build docker images.
	docker build -f DockerfileController -t ${IMG} .

docker-build-provider: test ## Build docker images.
	docker build -f DockerfileProvider -t ${IMG_PROVIDER} .

docker-build-worker: test ## Build docker images.
	docker build -f DockerfileWorker -t ${IMG_WORKER} .

docker-push: ## Push docker image with the manager.
	docker push ${IMG}
	docker push ${IMG_PROVIDER}
	docker push ${IMG_WORKER}

docker-push-controller: ## Push docker images.
	docker push ${IMG}

docker-push-provider: ## Push docker images.
	docker push ${IMG_PROVIDER}

docker-push-worker: ## Push docker images.
	docker push ${IMG_WORKER}

package-build: ## build ndd package.
	rm -rf package/ndd-*
	cd package;kubectl ndd package build -t provider;cd ..

package-push: ## build ndd package.
	cd package;kubectl ndd package push ${PKG};cd ..


CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

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