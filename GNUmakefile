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

# gotestsum reads GOTESTSUM_FORMAT from the environment, so we export a default
# of "testname" for readable local runs; CI overrides it to "github-actions"
# for collapsible groups and annotations.
export GOTESTSUM_FORMAT ?= testname

# Resolve user's Go bin directory (GOBIN, else GOPATH/bin)
GO_BIN_DIR := $(shell go env GOBIN)
ifeq ($(GO_BIN_DIR),)
GO_BIN_DIR := $(shell go env GOPATH)/bin
endif
GOLANGCI_LINT := $(GO_BIN_DIR)/golangci-lint
GOTESTSUM := $(GO_BIN_DIR)/gotestsum

# Delete a target file if its recipe fails, so a partial write (e.g. acctest/env
# when terraform output errors) is not left behind.
.DELETE_ON_ERROR:

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

# Install gotestsum (used by the test/acctest targets for live, per-test output)
# pinned to GOTESTSUM_VERSION.
$(GOTESTSUM):
	go install gotest.tools/gotestsum@$(GOTESTSUM_VERSION)

.PHONY: gotestsum-install
gotestsum-install: $(GOTESTSUM)

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
lint: lint-go lint-docs

.PHONY: lint-fix fmt
lint-fix fmt: lint-go-fix

# Format terraform files
.PHONY: fmtTf
fmtTf:
	terraform fmt -recursive

# Generate docs (alias: docs) from templates + the provider schema, then
# markdownlint --fix so the committed docs satisfy lint-docs (config in
# .markdownlint.yaml; markdownlint-cli2 pinned via MARKDOWNLINT_CLI2_VERSION).
.PHONY: documents docs
documents docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --rendered-provider-name 'YugabyteDB Anywhere' --provider-name yba
	npx --yes markdownlint-cli2@$(MARKDOWNLINT_CLI2_VERSION) --fix "docs/**/*.md"

# Lint the generated docs:
#   - regen reproduces the committed docs/ (no stale docs / hand-edits)
#   - tfplugindocs validate (Terraform Registry doc rules)
#   - markdownlint (markdown hygiene; config in .markdownlint.yaml)
#   - misspell (common English typos in docs/, templates/, and examples/)
.PHONY: lint-docs
lint-docs:
	$(MAKE) documents
	@test -z "$$(git status --porcelain -- docs/)" || { git status --porcelain -- docs/; echo "docs/ out of sync (incl. new/untracked) - run 'make docs' and commit"; exit 1; }
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs validate
	npx --yes markdownlint-cli2@$(MARKDOWNLINT_CLI2_VERSION) "docs/**/*.md"
	go run github.com/client9/misspell/cmd/misspell -error docs templates examples

# Run unit tests (no YBA or cloud credentials required)
.PHONY: test
test: $(GOTESTSUM)
	$(GOTESTSUM) -- ./... -skip '^TestAcc' $(TESTARGS)

# Run the short acceptance tier (all TestAcc* except TestAccLong*). Sources
# acctest/env, generated locally by `make -C acctest env` or written by CI from
# the ACCTEST_ENV secret. Set TF_ACCTEST_PREFIX to override the resource prefix.
.PHONY: acctest
acctest: install $(GOTESTSUM) acctest/env
	@set -a; . ./acctest/env; set +a; \
	TF_ACC=1 $(GOTESTSUM) -- -timeout 20m ./... -run '^TestAcc' -skip '^TestAccLong'

# Run the long acceptance tier (TestAccLong*). These deploy real multi-node
# universes and take ~15 min each, so they stay out of `make acctest`. Same env
# handling as acctest.
#
# For now this runs the GCP universe test only. The AWS/Azure long tests are
# skipped until each cloud's universe test targets its own standing YBA (the
# per-cloud-YBA wiring is a separate change); running them against the GCP YBA
# is not a meaningful test.
.PHONY: acctest-long
acctest-long: install $(GOTESTSUM) acctest/env
	@set -a; . ./acctest/env; set +a; \
	TF_ACC=1 $(GOTESTSUM) -- -timeout 160m ./... -run '^TestAccLong' -skip '_(AWS|Azure)_'

acctest/env:
	$(MAKE) -C acctest env

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

PLUGIN_DIR := $(HOME)/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$(OS)_$(ARCH)
TERRAFORMRC := $(HOME)/.terraformrc

# Install the locally-built provider so `terraform` picks it up for tests.
#
# Two things are needed:
#   1. The binary at the standard plugin path (so plain `terraform init`
#      can find it without dev_overrides).
#   2. A ~/.terraformrc with dev_overrides for ${NAMESPACE}/${NAME}, which
#      makes terraform use this binary regardless of the version constraint
#      in `required_providers`. Without (2), modules pinning specific
#      released versions (e.g. ">= 0.1.12") won't see this -dev build.
.PHONY: install
install: build
	mkdir -p $(PLUGIN_DIR)
	mv ${BINARY} $(PLUGIN_DIR)/
	@echo "Writing dev_overrides for ${NAMESPACE}/${NAME} to $(TERRAFORMRC)"
	@printf 'provider_installation {\n  dev_overrides {\n    "${NAMESPACE}/${NAME}" = "%s"\n  }\n  direct {}\n}\n' "$(PLUGIN_DIR)" > $(TERRAFORMRC)
