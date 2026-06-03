HOSTNAME=registry.terraform.io
NAMESPACE=yugabyte
NAME=yba
VERSION=0.1.0-dev

OS := $(if $(GOOS),$(GOOS),$(shell go env GOOS))
ARCH := $(if $(GOARCH),$(GOARCH),$(shell go env GOARCH))

BINARY=terraform-provider-${NAME}
export GOPRIVATE := github.com/yugabyte

# Pinned tool versions
# Tool versions are sourced from versions.env so CI and local builds stay aligned.
include versions.env

# Resolve user's Go bin directory (GOBIN, else GOPATH/bin)
GO_BIN_DIR := $(shell go env GOBIN)
ifeq ($(GO_BIN_DIR),)
GO_BIN_DIR := $(shell go env GOPATH)/bin
endif
GOLANGCI_LINT := $(GO_BIN_DIR)/golangci-lint

default: fmtTf documents lint arclint

# Lint test
.PHONY: arclint
arclint:
	arc lint --lintall

# Install golangci-lint to the user's Go bin dir pinned to GOLANGCI_LINT_VERSION
$(GOLANGCI_LINT):
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: lint-go-install
lint-go-install: $(GOLANGCI_LINT)

# Run golangci-lint over the module
.PHONY: lint-go
lint-go: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

# Run golangci-lint with --fix
.PHONY: lint-go-fix
lint-go-fix: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --fix ./...

# Aggregate lint targets (add more language-specific lints here as needed)
.PHONY: lint
lint: lint-go

.PHONY: lint-fix fmt
lint-fix fmt: lint-go-fix

# Format terraform files
.PHONY: fmtTf
fmtTf:
	terraform fmt -recursive
# Generate documents
.PHONY: documents
documents:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --rendered-provider-name 'YugabyteDB Anywhere' --provider-name yba

# Run unit tests (no YBA or cloud credentials required)
.PHONY: test
test:
	go test ./... -skip '^TestAcc' $(TESTARGS)

# Run acceptance tests (requires YBA and cloud credentials - see CONTRIBUTING.md)
.PHONY: testacc acctest
testacc acctest:
	TF_ACC=1 go test ./... -v -run "^TestAcc" $(TESTARGS) -timeout 120m

.PHONY: updateclient
updateclient: updatev1client updatev2client

.PHONY: updatev1client
updatev1client:
	go get github.com/yugabyte/platform-go-client

.PHONY: updatev2client
updatev2client:
	go get github.com/yugabyte/platform-go-client/v2

#Build localy the provider
.PHONY: build
build:
	go build -ldflags="-X 'main.version=v${VERSION}'" -o ${BINARY}

#Install the provider localu useful for testing local changes
.PHONY: install
install: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$(OS)_$(ARCH)/
	mv ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$(OS)_$(ARCH)/