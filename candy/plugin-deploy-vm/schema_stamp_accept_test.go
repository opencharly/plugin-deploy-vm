package deployvm

// schema_stamp_accept_test.go — the Cutover C pin-bump proof for THIS plugin's
// config-parse surface. The plugin's embedded spec pin (go.mod: spec v0.2026249.2129,
// the version-bump superset on top of the 2106 group-kind removal) carries
// spec.SchemaVersion HEAD 2026.249.2125. The pre-wave failure was:
// "config schema 2026.249.2125 is newer than this charly supports" — the embedded
// spec v0.2026249.2106 cap (HEAD 2026.248.1030) rejected migrated (group-unrolled)
// trees. This test pins the NEW world: the embedded cap accepts the migrated stamp
// 2026.249.2125 (mirrors the deploy-pod member_tree_accept_test stamp-acceptance half;
// the member-tree folding half is deploy-pod's fedora-substrate shape and is covered
// by that plugin's own test).
import (
	"testing"

	calverpkg "github.com/opencharly/spec/calver"
	"github.com/opencharly/spec/spec"
)

func TestMigratedTreeStampAcceptedByEmbeddedParser(t *testing.T) {
	head := calverpkg.MustCalVer(spec.SchemaVersion)
	stamp := calverpkg.MustCalVer("2026.249.2125")
	if head.Less(stamp) {
		t.Fatalf("embedded spec cap %s rejects the migrated stamp 2026.249.2125 — the pre-wave failure is NOT fixed", spec.SchemaVersion)
	}
}
