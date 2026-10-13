// Licensed to YugabyteDB, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Mozilla License, Version 2.0
// (the "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
// http://mozilla.org/MPL/2.0/.
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package acctest

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// ImportPlanOptions adjusts ImportPlanSteps.
type ImportPlanOptions struct {
	// IDFunc returns the import ID from the created resource. By default the
	// import ID is the resource ID.
	IDFunc func(created *terraform.InstanceState) (string, error)
	// VerifyIgnore lists the attributes that the import comparison skips. Add
	// only a value that YBA cannot return, with a comment that cites the YBA
	// evidence.
	VerifyIgnore []string
	// ApplyCheck, when set, applies config once after the import, before the
	// plan, and runs this check. Set it only for node_restart_settings, which
	// YBA does not store, so the first plan after an import adds it. The check
	// shows that the apply changed nothing in YBA (UniverseTaskWatch.NoNewTask).
	ApplyCheck resource.TestCheckFunc
}

// ImportPlanSteps checks that `terraform import` of the resource that config
// created gives a clean plan with config. Terraform does not import into an
// address that is in the state, so the steps import into a copy of the
// resource block at <name>_imported, compare its state with the created
// state, plan config with the copy, and then remove the copy from the state
// without a destroy.
func ImportPlanSteps(
	t *testing.T, resourceName, config string, opts ImportPlanOptions,
) []resource.TestStep {
	t.Helper()
	copyName := resourceName + "_imported"
	withCopy := config + resourceBlockCopy(t, config, resourceName, copyName)
	steps := []resource.TestStep{{
		Config:             withCopy,
		ResourceName:       copyName,
		ImportState:        true,
		ImportStatePersist: true,
		ImportStateIdFunc: func(s *terraform.State) (string, error) {
			created, ok := s.RootModule().Resources[resourceName]
			if !ok {
				return "", fmt.Errorf("%s is not in the state", resourceName)
			}
			if opts.IDFunc != nil {
				return opts.IDFunc(created.Primary)
			}
			return created.Primary.ID, nil
		},
		ImportStateVerify:       true,
		ImportStateVerifyIgnore: opts.VerifyIgnore,
	}}
	if opts.ApplyCheck != nil {
		steps = append(steps, resource.TestStep{Config: withCopy, Check: opts.ApplyCheck})
	}
	return append(steps,
		resource.TestStep{
			Config:   withCopy,
			PlanOnly: true,
		},
		resource.TestStep{
			Config: config + fmt.Sprintf(`
removed {
  from = %s
  lifecycle {
    destroy = false
  }
}
`, copyName),
		},
	)
}

// resourceBlockCopy returns the HCL of resourceName's block in config, with
// its name label changed to copyName's. The copy depends on the original, so
// a destroy after a failed step deletes the shared object in sequence.
func resourceBlockCopy(t *testing.T, config, resourceName, copyName string) string {
	t.Helper()
	f, diags := hclwrite.ParseConfig([]byte(config), "config.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse test config: %s", diags.Error())
	}
	labels := strings.SplitN(resourceName, ".", 2)
	for _, b := range f.Body().Blocks() {
		if b.Type() != "resource" || !slices.Equal(b.Labels(), labels) {
			continue
		}
		out, _ := hclwrite.ParseConfig(b.BuildTokens(nil).Bytes(), "copy.tf", hcl.InitialPos)
		cp := out.Body().Blocks()[0]
		cp.SetLabels([]string{labels[0], strings.SplitN(copyName, ".", 2)[1]})
		deps := hclwrite.TokensForTraversal(hcl.Traversal{
			hcl.TraverseRoot{Name: labels[0]}, hcl.TraverseAttr{Name: labels[1]},
		})
		if attr := cp.Body().GetAttribute("depends_on"); attr != nil {
			// Keep the block's own edges: insert the original before the "]".
			toks := attr.Expr().BuildTokens(nil)
			end := len(toks) - 1
			for toks[end].Type != hclsyntax.TokenCBrack {
				end--
			}
			deps = append(append(append(hclwrite.Tokens{}, toks[:end]...),
				&hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")}),
				append(deps, toks[end:]...)...)
		} else {
			deps = hclwrite.TokensForTuple([]hclwrite.Tokens{deps})
		}
		cp.Body().SetAttributeRaw("depends_on", deps)
		return "\n" + string(out.Bytes())
	}
	t.Fatalf("resource %s is not in the test config", resourceName)
	return ""
}
