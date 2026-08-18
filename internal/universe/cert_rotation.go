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

package universe

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// certRotationPlan captures everything performCertRotations decided to do, in
// a form unit tests can assert without a live server.
type certRotationPlan struct {
	// RootCert rotation (CA change): dispatched first.
	caChange     bool
	rootCA       string // effective value; "" means send null (channel disabled)
	clientRootCA string
	sameRootCA   bool

	// ServerCert rotation (same-CA refresh of per-node server certificates):
	// dispatched after the CA change completes.
	rotateServerCerts bool // selfSignedServerCertRotate
	rotateClientCerts bool // selfSignedClientCertRotate
}

// liveCertState is the certificate-relevant slice of the live universe:
// current CAs (as reported by the API) and encryption-in-transit toggles.
type liveCertState struct {
	rootCA       string
	clientRootCA string
	sameRootCA   bool
	n2nEnabled   bool
	c2nEnabled   bool
}

// planCertRotations derives the rotation work from the Terraform plan and the
// live universe state.
//
//   - Effective CA per channel = plan value when set, else the universe's
//     current value. The pointers are ALWAYS sent for enabled channels:
//     omitting a CA in CertsRotateParams means "explicitly null", which makes
//     YBA silently mint a brand-new root certificate.
//   - A RootCert rotation is needed when an effective CA differs from the
//     live one, compared per enabled channel only: universes created before
//     YBA scoped rootCA to node-to-node TLS report a rootCA even with n2n
//     off, and that dead value must not read as a pending CA change.
//     Comparing against live (not just d.HasChange) keeps this a no-op when
//     an earlier step of the same update (e.g. a TLS toggle that carried the
//     new CA) already applied it.
//   - On universes where one root certificate serves both channels
//     (rootAndClientRootCASame), the client channel follows the root unless
//     client_root_ca is explicitly written in config — the state echo of the
//     shared CA must not pin client-to-node to the old certificate.
//   - Trigger semantics: a ServerCert rotation fires when a cert_rotation
//     trigger changed to a non-empty value. Unsetting a trigger never fires.
//     On shared-CA universes a trigger on either side rotates both channels
//     (matching YBA CLI/UI behaviour, and required on Kubernetes) so they
//     never end up half-refreshed.
func planCertRotations(
	planRootCA, planClientRootCA string,
	clientCASetInConfig bool,
	serverTriggerFired, clientTriggerFired bool,
	live liveCertState,
) certRotationPlan {
	plan := certRotationPlan{}

	effRoot := planRootCA
	if effRoot == "" {
		effRoot = live.rootCA
	}
	effClient := planClientRootCA
	if effClient == "" {
		effClient = live.clientRootCA
	}
	// On a universe whose channels share one CA (rootAndClientRootCASame), the
	// client channel follows the root unless client_root_ca is explicitly
	// written in config: YBA mirrors clientRootCA = rootCA into the stored
	// universe at create and Read copies that mirror into state, so the plan
	// value is a state echo, not user intent. Without this, a root_ca change
	// dispatches {new root, old client, same=false} — a request YBA reads as
	// "split the channels and pin client-to-node to the old certificate" and
	// executes without error, leaving the client channel on the expiring CA.
	if !clientCASetInConfig && live.sameRootCA && live.n2nEnabled {
		effClient = effRoot
	}
	// A disabled channel must keep its CA null: YBA rejects any non-current
	// value ("rootCA is not required with the current TLS parameters").
	if !live.n2nEnabled {
		effRoot = ""
	}
	if !live.c2nEnabled {
		effClient = ""
	}

	plan.rootCA = effRoot
	plan.clientRootCA = effClient
	plan.sameRootCA = effClient == "" || effClient == effRoot
	plan.caChange = (live.n2nEnabled && effRoot != live.rootCA) ||
		(live.c2nEnabled && effClient != live.clientRootCA)

	// On universes where one root certificate serves both channels, a trigger
	// on either side rotates both: the flags ride a single task (no extra
	// restarts), the channels never end up half-refreshed, and Kubernetes
	// outright rejects one-sided rotations ("Cannot rotate only ... when ...
	// encryption is enabled").
	plan.rotateServerCerts = serverTriggerFired
	plan.rotateClientCerts = clientTriggerFired
	if serverTriggerFired && live.sameRootCA && live.c2nEnabled {
		plan.rotateClientCerts = true
	}
	if clientTriggerFired && live.sameRootCA && live.n2nEnabled {
		plan.rotateServerCerts = true
	}

	return plan
}

// triggerFired reports whether the given cert_rotation trigger changed to a
// non-empty value in this update. First-time set fires; clearing does not.
func triggerFired(d *schema.ResourceData, key string) bool {
	if !d.HasChange(key) {
		return false
	}
	return d.Get(key).(string) != ""
}

// performCertRotations runs at the tail of resourceUniverseUpdate — after all
// cluster-level operations including the TLS toggle — and dispatches up to two
// sequential upgrade/certs tasks: a RootCert rotation for root_ca /
// client_root_ca changes, then a ServerCert rotation for fired cert_rotation
// triggers. YBA cannot combine a CA change with a server-certificate refresh
// in a single task, so combined edits are dispatched in that order.
func performCertRotations(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
	upgradeOption string,
	sleepAfterMasterMs int32,
	sleepAfterTServerMs int32,
) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	serverTrigger := triggerFired(d, "cert_rotation.0.server_cert_trigger")
	clientTrigger := triggerFired(d, "cert_rotation.0.client_cert_trigger")

	// Cheap exit before any API call when nothing cert-related is in the plan.
	if !d.HasChange("root_ca") && !d.HasChange("client_root_ca") &&
		!serverTrigger && !clientTrigger {
		return nil
	}

	liveUni, response, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Universe", "Update - Fetch universe for cert rotation")
		return diag.FromErr(errMessage)
	}
	details := liveUni.UniverseDetails
	n2nEnabled, c2nEnabled := liveEncryptionFlags(details)

	if serverTrigger && !n2nEnabled {
		return diag.Errorf(
			"cert_rotation.server_cert_trigger requires node-to-node encryption: " +
				"enable_node_to_node_encrypt is false on this universe")
	}
	if clientTrigger && !c2nEnabled {
		return diag.Errorf(
			"cert_rotation.client_cert_trigger requires client-to-node encryption: " +
				"enable_client_to_node_encrypt is false on this universe")
	}

	plan := planCertRotations(
		d.Get("root_ca").(string), d.Get("client_root_ca").(string),
		clientRootCASetInConfig(d),
		serverTrigger, clientTrigger,
		liveCertState{
			rootCA:       details.GetRootCA(),
			clientRootCA: details.GetClientRootCA(),
			sameRootCA:   details.GetRootAndClientRootCASame(),
			n2nEnabled:   n2nEnabled,
			c2nEnabled:   c2nEnabled,
		},
	)

	if plan.caChange {
		req := newCertsRotateParams(details.GetClusters(), upgradeOption,
			sleepAfterMasterMs, sleepAfterTServerMs)
		req.RootCA = utils.GetStringPointer(plan.rootCA)
		req.ClientRootCA = utils.GetStringPointer(plan.clientRootCA)
		req.RootAndClientRootCASame = utils.GetBoolPointer(plan.sameRootCA)
		if diags := dispatchCertsRotate(ctx, d, meta, "Certs Rotate", req); diags != nil {
			return diags
		}
	}

	if plan.rotateServerCerts || plan.rotateClientCerts {
		// Re-fetch after a preceding CA rotation so the refresh references the
		// universe's current certificates.
		liveUni, response, err = c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).
			Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Universe", "Update - Fetch universe for server cert rotation")
			return diag.FromErr(errMessage)
		}
		details = liveUni.UniverseDetails

		req := newCertsRotateParams(details.GetClusters(), upgradeOption,
			sleepAfterMasterMs, sleepAfterTServerMs)
		req.RootCA = utils.GetStringPointer(details.GetRootCA())
		req.ClientRootCA = utils.GetStringPointer(details.GetClientRootCA())
		req.RootAndClientRootCASame = utils.GetBoolPointer(
			details.GetRootAndClientRootCASame())
		req.SelfSignedServerCertRotate = plan.rotateServerCerts
		req.SelfSignedClientCertRotate = plan.rotateClientCerts
		if diags := dispatchCertsRotate(
			ctx, d, meta, "Server Certs Rotate", req); diags != nil {
			return diags
		}
	}

	return nil
}

// clientRootCASetInConfig reports whether client_root_ca is explicitly
// written in the user's HCL config. d.Get cannot answer this: the attribute
// is Optional+Computed, so on shared-CA universes it returns the state echo
// of YBA's clientRootCA = rootCA mirror whether or not the user set the
// field. Unknown values (references not yet resolved) count as set — Update
// runs at apply time, when they carry the user's value.
func clientRootCASetInConfig(d *schema.ResourceData) bool {
	rawConfig := d.GetRawConfig()
	if rawConfig == cty.NilVal || !rawConfig.IsKnown() || rawConfig.IsNull() {
		return false
	}
	return !rawConfig.GetAttr("client_root_ca").IsNull()
}

// liveEncryptionFlags reads the encryption-in-transit toggles from the live
// primary cluster's user intent.
func liveEncryptionFlags(details *client.UniverseDefinitionTaskParamsResp) (bool, bool) {
	for _, cl := range details.GetClusters() {
		if cl.ClusterType != "PRIMARY" {
			continue
		}
		return cl.UserIntent.GetEnableNodeToNodeEncrypt(),
			cl.UserIntent.GetEnableClientToNodeEncrypt()
	}
	return false, false
}

// newCertsRotateParams builds the shared skeleton of an upgrade/certs request.
// CreatingUser is overwritten server-side from the session and
// KubernetesUpgradeSupported is ignored by YBA, so their zero values are fine
// (same as the shipped TLS-toggle dispatch). Clusters must never be nil — the
// backend NPEs on a JSON null — so the live clusters are always passed.
func newCertsRotateParams(
	clusters []client.Cluster,
	upgradeOption string,
	sleepAfterMasterMs int32,
	sleepAfterTServerMs int32,
) client.CertsRotateParams {
	return client.CertsRotateParams{
		Clusters:                       clusters,
		UpgradeOption:                  upgradeOption,
		SleepAfterMasterRestartMillis:  sleepAfterMasterMs,
		SleepAfterTServerRestartMillis: sleepAfterTServerMs,
	}
}

func dispatchCertsRotate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
	label string,
	req client.CertsRotateParams,
) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	return utils.DispatchAndWait(ctx, label, cUUID, c,
		d.Timeout(schema.TimeoutUpdate),
		utils.ResourceEntity, "Universe", fmt.Sprintf("Update - %s", label),
		func() (string, *http.Response, error) {
			r, resp, e := c.UniverseUpgradesManagementAPI.UpgradeCerts(
				ctx, cUUID, d.Id()).CertsRotateParams(req).Execute()
			if e != nil {
				return "", resp, e
			}
			return r.GetTaskUUID(), resp, nil
		},
	)
}
