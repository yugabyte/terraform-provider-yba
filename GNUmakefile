HOSTNAME=registry.terraform.io
NAMESPACE=yugabyte
NAME=yba
VERSION=0.1.0-dev

OS := $(if $(GOOS),$(GOOS),$(shell go env GOOS))
ARCH := $(if $(GOARCH),$(GOARCH),$(shell go env GOARCH))

BINARY=terraform-provider-${NAME}
export GOPRIVATE := github.com/yugabyte

default: fmtTf documents arclint

# Lint test
.PHONY: arclint
arclint:
	arc lint --lintall
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
	go test ./internal/provider/... -v -run "^Test[^Acc]" $(TESTARGS)

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

.PHONY: fmt
fmt:
	go clean -modcache
	GOBIN=$(PWD)/bin go install github.com/segmentio/golines@latest
	$(PWD)/bin/golines --max-len=100 -w ./internal

#Install the provider localu useful for testing local changes
.PHONY: install
install: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$(OS)_$(ARCH)/
	mv ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$(OS)_$(ARCH)/