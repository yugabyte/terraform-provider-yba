default: documents testacc

# Generate documents
.PHONY: documents
documents:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs
# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m
