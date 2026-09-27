package deployvm

import (
	"strings"
	"testing"
)

// TestVmTeardownRemoveEntries_IdentityKeys is the regression for the sdk#301 consumer leg: a vm
// teardown must remove the per-host entries by the deploy IDENTITY (the deploy name and the
// domain identity), NEVER the retired `vm:<domain>` key — a `vm:`-prefixed key names no existing
// entry, so the identity-keyed entry would be left behind (entries accumulate). Fails pre-fix
// (the old code returned `"vm:" + domain`).
func TestVmTeardownRemoveEntries_IdentityKeys(t *testing.T) {
	// The common bed shape: the deploy name IS the domain identity → one entry, no duplicate.
	if got := vmTeardownRemoveEntries("check-r10-two-vm", "check-r10-two-vm"); len(got) != 1 || got[0] != "check-r10-two-vm" {
		t.Fatalf("name==domain: got %v, want [check-r10-two-vm]", got)
	}
	// name != domain → both identity keys, in order.
	got := vmTeardownRemoveEntries("check-k3s-vm", "check-k3s-vm-x")
	if len(got) != 2 || got[0] != "check-k3s-vm" || got[1] != "check-k3s-vm-x" {
		t.Fatalf("name!=domain: got %v, want [check-k3s-vm check-k3s-vm-x]", got)
	}
	for _, k := range got {
		if strings.HasPrefix(k, "vm:") {
			t.Fatalf("teardown entry %q carries the retired `vm:` prefix — the identity-keyed entry would be left behind", k)
		}
	}
}
