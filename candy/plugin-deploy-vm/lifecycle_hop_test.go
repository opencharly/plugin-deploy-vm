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
