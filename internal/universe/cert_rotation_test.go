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
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	caX = "11111111-1111-1111-1111-111111111111"
	caY = "22222222-2222-2222-2222-222222222222"
	caZ = "33333333-3333-3333-3333-333333333333"
)

func TestCertRotationSchemaSanity(t *testing.T) {
	s := ResourceUniverse().Schema
	block, ok := s["cert_rotation"]
	if !ok {
		t.Fatal("cert_rotation block missing from universe schema")
	}
	if block.MaxItems != 1 {
		t.Error("cert_rotation must have MaxItems = 1")
	}
	elem := block.Elem.(*schema.Resource).Schema
	for _, trigger := range []string{"server_cert_trigger", "client_cert_trigger"} {
		f, ok := elem[trigger]
		if !ok {
			t.Fatalf("cert_rotation.%s missing", trigger)
		}
		if f.Required || f.Computed {
			t.Errorf("cert_rotation.%s must be plain Optional client-side bookkeeping",
				trigger)
		}
		if f.ForceNew {
			t.Errorf("cert_rotation.%s must not be ForceNew", trigger)
		}
	}
}

func TestPlanCertRotationsRootCAChange(t *testing.T) {
	// root_ca changed X -> Y on a same-CA universe: RootCert rotation for both
	// channels via the single root.
	p := planCertRotations(caY, "", false, false, liveCertState{
		rootCA: caX, clientRootCA: caX, sameRootCA: true,
		n2nEnabled: true, c2nEnabled: true,
	})
	if !p.caChange {
		t.Fatal("expected CA change")
	}
	if p.rootCA != caY || p.clientRootCA != caX {
		t.Errorf("effective CAs = (%s, %s): plan root must win, absent client must "+
			"resend the live value (absent == null mints a new CA in YBA)",
			p.rootCA, p.clientRootCA)
	}
	if p.sameRootCA {
		t.Error("root Y with client still X must derive rootAndClientRootCASame=false")
	}
	if p.rotateServerCerts || p.rotateClientCerts {
		t.Error("CA change alone must not set the selfSigned*CertRotate flags")
	}
}

func TestPlanCertRotationsClientOnlyChange(t *testing.T) {
	// client_root_ca repointed to a new CustomServerCert config; root untouched.
	p := planCertRotations("", caZ, false, false, liveCertState{
		rootCA: caX, clientRootCA: caY,
		n2nEnabled: true, c2nEnabled: true,
	})
	if !p.caChange {
		t.Fatal("expected CA change")
	}
	if p.rootCA != caX || p.clientRootCA != caZ {
		t.Errorf("effective CAs = (%s, %s), want unchanged root %s and new client %s",
			p.rootCA, p.clientRootCA, caX, caZ)
	}
	if p.sameRootCA {
		t.Error("distinct client CA must derive rootAndClientRootCASame=false")
	}
}

func TestPlanCertRotationsNoOpWhenLiveAlreadyMatches(t *testing.T) {
	// A TLS toggle earlier in the same update already carried the CA: the
	// rotation pass must become a no-op instead of dispatching an upgrade
	// YBA would 400 ("No changes in rootCA or clientRootCA.").
	p := planCertRotations(caX, caX, false, false, liveCertState{
		rootCA: caX, clientRootCA: caX, sameRootCA: true,
		n2nEnabled: true, c2nEnabled: true,
	})
	if p.caChange {
		t.Error("no-op plan must not dispatch a CA rotation")
	}
}

func TestPlanCertRotationsDisabledChannelsSendNull(t *testing.T) {
	// Client-to-node-only universe: rootCA must stay null (YBA rejects any
	// value when node-to-node encryption is off).
	p := planCertRotations("", caZ, false, false, liveCertState{
		clientRootCA: caY, c2nEnabled: true,
	})
	if p.rootCA != "" {
		t.Errorf("rootCA = %q, must stay empty (nil pointer) when n2n is disabled",
			p.rootCA)
	}
	if !p.caChange || p.clientRootCA != caZ {
		t.Errorf("client rotation must proceed, got %+v", p)
	}
	if p.sameRootCA {
		t.Error("client-only universe must derive rootAndClientRootCASame=false")
	}
}

func TestPlanCertRotationsServerTriggerSameCA(t *testing.T) {
	// Same-CA universe: a server trigger must also rotate the client-to-node
	// server certificates (YBA UI/CLI parity) so channels don't half-refresh.
	p := planCertRotations("", "", true, false, liveCertState{
		rootCA: caX, clientRootCA: caX, sameRootCA: true,
		n2nEnabled: true, c2nEnabled: true,
	})
	if p.caChange {
		t.Error("trigger firing must not dispatch a CA change")
	}
	if !p.rotateServerCerts || !p.rotateClientCerts {
		t.Errorf("same-CA server trigger must set both rotate flags, got %+v", p)
	}
}

func TestPlanCertRotationsServerTriggerSplitCA(t *testing.T) {
	// Split-CA universe: the server trigger only touches the root side.
	p := planCertRotations("", "", true, false, liveCertState{
		rootCA: caX, clientRootCA: caY,
		n2nEnabled: true, c2nEnabled: true,
	})
	if !p.rotateServerCerts || p.rotateClientCerts {
		t.Errorf("split-CA server trigger must rotate the server side only, got %+v", p)
	}
}

func TestPlanCertRotationsClientTrigger(t *testing.T) {
	p := planCertRotations("", "", false, true, liveCertState{
		rootCA: caX, clientRootCA: caY,
		n2nEnabled: true, c2nEnabled: true,
	})
	if p.rotateServerCerts || !p.rotateClientCerts {
		t.Errorf("client trigger must set only selfSignedClientCertRotate, got %+v", p)
	}
	if p.caChange {
		t.Error("client trigger must not dispatch a CA change")
	}
}

func TestPlanCertRotationsCombinedChangeAndTrigger(t *testing.T) {
	// CA swap and trigger bump in one apply: both operations planned; the
	// caller dispatches them sequentially (YBA cannot combine them).
	p := planCertRotations(caY, "", true, false, liveCertState{
		rootCA: caX, clientRootCA: caX, sameRootCA: true,
		n2nEnabled: true, c2nEnabled: true,
	})
	if !p.caChange || !p.rotateServerCerts {
		t.Errorf("combined edit must plan both dispatches, got %+v", p)
	}
}

func TestPlanCertRotationsLegacyRootCAOnClientOnlyUniverse(t *testing.T) {
	// Universes created before YBA scoped rootCA to node-to-node TLS report a
	// rootCA (shared with the client channel) even though n2n is off. The
	// zeroed root must not register as a CA change: the spurious dispatch
	// either 400s ("clientRootCA is required with the current TLS parameters")
	// or rolls the universe for nothing.
	legacy := liveCertState{
		rootCA: caX, clientRootCA: caX, sameRootCA: true, c2nEnabled: true,
	}

	p := planCertRotations(caX, caX, false, true, legacy)
	if p.caChange {
		t.Error("dead rootCA on a client-only universe must not plan a CA change")
	}
	if !p.rotateClientCerts {
		t.Error("client trigger must still fire")
	}

	// Repointing the client CA on the same universe must dispatch with the
	// root left null so YBA resets the dead rootCA (its documented handling).
	p = planCertRotations(caX, caZ, false, false, legacy)
	if !p.caChange || p.rootCA != "" || p.clientRootCA != caZ {
		t.Errorf("client CA change must rotate with a null root, got %+v", p)
	}
}
