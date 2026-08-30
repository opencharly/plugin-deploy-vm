package deployvm

import "testing"

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
