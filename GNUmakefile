HOSTNAME=registry.terraform.io
NAMESPACE=yugabyte
NAME=yba
VERSION=0.1.0-dev

OS := $(if $(GOOS),$(GOOS),$(shell go env GOOS))
ARCH := $(if $(GOARCH),$(GOARCH),$(shell go env GOARCH))

BINARY=terraform-provider-${NAME}
export GOPRIVATE := github.com/yugabyte

default: fmtTf documents testacc arclint

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
# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

.PHONY: updateclient
updateclient:
	go get github.com/yugabyte/platform-go-client

#Build localy the provider
.PHONY: build
build:
	go build -ldflags="-X 'main.version=v${VERSION}'" -o ${BINARY}

#Install the provider localu useful for testing local changes
.PHONY: install
install: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$(OS)_$(ARCH)/
	mv ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$(OS)_$(ARCH)/