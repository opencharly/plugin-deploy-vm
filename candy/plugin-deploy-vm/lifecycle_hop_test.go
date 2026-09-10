package deployvm

import (
	"encoding/json"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestVmPrepareTargetEntityFromUf gates the PURE deploy-hop decision (Phase 3): the
// derived entity may be the clone-base BED (a deploy whose from: names the terminal
// kind:vm template) — the ONE chain resolver hops it. The test exercises the pure
// decision function (vmPrepareTargetEntityFromUf) that vmPrepareVenue consumes;
// removing the hop from the pure function fails this test (a plain passthrough would
// return the bed name).
func TestVmPrepareTargetEntityFromUf(t *testing.T) {
	uf := &spec.UnifiedFile{
		Deploy: map[string]spec.DeployNode{
			"check-vm-clone-base": {From: "cachyos-vm"},
		},
		PluginKinds: map[string]map[string]json.RawMessage{
			"vm": {"cachyos-vm": json.RawMessage("{}")},
		},
	}
	if got := vmPrepareTargetEntityFromUf(uf, "check-vm-clone-base"); got != "cachyos-vm" {
		t.Fatalf("deploy hop: vmPrepareTargetEntityFromUf(%q) = %q, want cachyos-vm", "check-vm-clone-base", got)
	}
	if got := vmPrepareTargetEntityFromUf(uf, "cachyos-vm"); got != "cachyos-vm" {
		t.Fatalf("plain entity: got %q, want cachyos-vm", got)
	}
	if got := vmPrepareTargetEntityFromUf(nil, "any"); got != "any" {
		t.Fatalf("nil uf must pass through: got %q", got)
	}
}

// TestVmPrepareTargetEntityFromUf_QualifiedFromHop gates the NAMESPACE-QUALIFIED
// deploy-hop (the clone-base bed hop through a git-linked import): a bed whose
// from: is a qualified template (ns.template) must hop to it. FAILS with sdk
// < v0.2026253.1947 (DeployTargetEntity's hop resolved the local vm map only —
// the check-omarchy-eval-edge-inst deploy-add regression).
func TestVmPrepareTargetEntityFromUf_QualifiedFromHop(t *testing.T) {
	ns := &spec.UnifiedFile{
		PluginKinds: map[string]map[string]json.RawMessage{
			"vm": {"omarchy-vm": json.RawMessage("{}")},
		},
	}
	uf := &spec.UnifiedFile{
		Deploy: map[string]spec.DeployNode{
			"check-omarchy-eval-edge-inst": {From: "omarchy.omarchy-vm"},
		},
		Namespaces: map[string]*spec.UnifiedFile{"omarchy": ns},
	}
	if got := vmPrepareTargetEntityFromUf(uf, "check-omarchy-eval-edge-inst"); got != "omarchy.omarchy-vm" {
		t.Fatalf("qualified deploy hop: vmPrepareTargetEntityFromUf(%q) = %q, want omarchy.omarchy-vm", "check-omarchy-eval-edge-inst", got)
	}
}
