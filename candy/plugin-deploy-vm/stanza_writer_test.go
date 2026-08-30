package deployvm

import (
	"os"
	"path/filepath"
	"testing"
)

// The stanza has ONE authority: `vm create`. prepare-venue may write it only when `vm create`
// is not going to — never after it.
//
// The condition this replaces read as "skip when auto-boot will write it" and actually meant
// "skip when no port is persisted yet". Those two agree on a FRESH domain and diverge on every
// REBUILD, which is why the resulting bug looked intermittent: the first add of a domain was
// correct and a later rebuild of the same domain silently clobbered it.
func TestShouldPublishStanza(t *testing.T) {
	cases := []struct {
		name                           string
		deferToAutoBoot, createdByAuto bool
		want                           bool
		why                            string
	}{
		{
			name:            "fresh domain, vm create will allocate and publish",
			deferToAutoBoot: true, createdByAuto: true, want: false,
			why: "vm create owns the port AND the stanza; writing here would race its allocation",
		},
		{
			name:            "REBUILD — port persisted, and auto-boot ran vm create",
			deferToAutoBoot: false, createdByAuto: true, want: false,
			why: "THE REGRESSION: vm create just published the correct stanza; writing now " +
				"replaces its source-kind-dependent known-hosts policy and strands the guest",
		},
		{
			name:            "port persisted and the guest was already reachable — no vm create ran",
			deferToAutoBoot: false, createdByAuto: false, want: true,
			why: "nobody else will publish it, so prepare-venue must",
		},
		{
			name:            "auto-boot disabled entirely (DryRun / CHARLY_DEPLOY_NO_AUTOBOOT)",
			deferToAutoBoot: false, createdByAuto: false, want: true,
			why: "no vm create exists on this path at all",
		},
	}
	for _, c := range cases {
		if got := shouldPublishStanza(c.deferToAutoBoot, c.createdByAuto); got != c.want {
			t.Errorf("%s: got %v, want %v — %s", c.name, got, c.want, c.why)
		}
	}
}

// NEGATIVE CONTROL, expressed as an invariant rather than a case: whenever `vm create` ran,
// prepare-venue must NOT write, for every combination of the other input. This is the single
// property the defect violated, and it is the one a future edit is most likely to lose.
func TestPrepareVenueNeverWritesAfterVmCreate(t *testing.T) {
	for _, deferToAutoBoot := range []bool{true, false} {
		if shouldPublishStanza(deferToAutoBoot, true) {
			t.Errorf("deferToAutoBoot=%v: prepare-venue would write the stanza AFTER `vm create` "+
				"published it, discarding the known-hosts policy only vm create knows "+
				"(an iso guest must record no host key). That is the measured defect.",
				deferToAutoBoot)
		}
	}
}

// The complementary invariant, so the fix cannot be "never write": when `vm create` did NOT
// run and nothing is deferred to it, prepare-venue is the only writer and must act.
func TestPrepareVenueWritesWhenNothingElseWill(t *testing.T) {
	if !shouldPublishStanza(false, false) {
		t.Fatal("no `vm create` ran and nothing was deferred to it, so prepare-venue is the only " +
			"writer — skipping here would leave `ssh <alias>` unresolvable")
	}
}

// The known-hosts policy, on the path where prepare-venue is the LEGITIMATE sole writer:
// a domain adopted by `charly vm import` has no stanza at all (import publishes none), so
// something must write one.
//
// THE OTHER COPY OF THIS RULE lives in plugin-vm's publishVmSshAlias
// (candy/plugin-vm/vm_create_orchestrate.go). Both are writers of the same stanza and must
// agree. If you change one, change the other — the single-owner version was blocked as a
// partial cutover because a shared function necessarily lands with zero callers.
func TestKnownHostsPathFor(t *testing.T) {
	if got := knownHostsPathFor("iso", "/state/charly-omarchy-vm"); got != os.DevNull {
		t.Fatalf("an iso guest must record no host key, got %q", got)
	}
	// NEGATIVE CONTROL. The tempting simplification is to return /dev/null for everything:
	// it makes the iso case work and nothing visibly breaks. It also silently removes
	// host-key continuity from every guest whose identity is stable from first boot, which
	// is a security property, not a convenience.
	for _, kind := range []string{"cloud_image", "bootc", "bootstrap", "clone", ""} {
		want := filepath.Join("/state/charly-vm", "known_hosts")
		if got := knownHostsPathFor(kind, "/state/charly-vm"); got != want {
			t.Errorf("kind %q: host-key recording was disabled for a VM with ONE stable identity: got %q, want %q",
				kind, got, want)
		}
	}
	// Per-DOMAIN, so two beds sharing one kind:vm entity never check one guest's host key
	// against another's.
	if knownHostsPathFor("cloud_image", "/state/a") == knownHostsPathFor("cloud_image", "/state/b") {
		t.Fatal("two domains got the same known_hosts path")
	}
}
