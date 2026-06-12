// +build tools

package tools

import (
	// document generation
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
	// docs spell checking
	_ "github.com/client9/misspell/cmd/misspell"
)
