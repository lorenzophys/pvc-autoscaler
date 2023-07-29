SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

PLATFORM ?= linux/amd64
IMG ?= lorenzophys/pvc-autoscaler:dev

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: fmt
fmt: ## Run go fmt against code.
	@go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	@go vet ./...

.PHONY: build
build: fmt vet ## Build the autoscaler binary.
	@go build -o bin/pvc-autoscaler ./...

.PHONY: run
run: fmt vet ## Run the autoscaler locally.
	@go run ./main.go

.PHONY: docker-build
docker-build: fmt vet ## Build the docker image with the autoscaler.
	@docker build -t ${IMG} --platform ${PLATFORM} .

.PHONY: docker-push
docker-push: ## Push the docker image with the autoscaler.
	@docker push ${IMG}

.PHONY: docker-run
docker-run: ## Run the docker image with the autoscaler.
	@docker run --rm ${IMG}
