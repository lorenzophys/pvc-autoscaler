SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

PLATFORM ?= linux/amd64
IMG ?= lorenzophys/pvc-autoscaler:dev

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: fmt
fmt: ## Run go fmt against code.
	@go fmt $$(go list ./... | grep -v 'mock_*')

.PHONY: vet
vet: fmt ## Run go vet against code.
	@go vet $$(go list ./... | grep -v 'mock_*')

.PHONY: test
test: fmt vet ## Run go test against code.
	@go test $$(go list ./... | grep -v 'mock_*') -v

.PHONY: cov
cov: fmt vet ## Run go test with coverage against code.
	@go test $$(go list ./... | grep -v 'mock_*') -coverprofile=coverage.out

.PHONY: cov-html
cov-html: cov ## Display the coverage.out in html form.
	@go tool cover -html=coverage.out

.PHONY: build
build: fmt vet test ## Build the autoscaler binary.
	@go build -o bin/pvc-autoscaler ./cmd

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
