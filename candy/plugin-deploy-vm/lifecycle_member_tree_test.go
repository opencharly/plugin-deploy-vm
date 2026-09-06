package deployvm

import (
	"encoding/json"
	"testing"

	"github.com/opencharly/spec/spec"
)

// memberTreeNodeJSON is a converted-tree node in the Cutover C shape (the primary
// substrate at the kind key + the ONE ordered member list), the shape the charly
// CC-1 migration stamps onto every former group:/member surface — here the
// distro-fedora silhouette: a vm: primary with a deploy-level pod MEMBER (brought
// up alongside on the shared network) and an in-substrate pod MEMBER (deployed
// INTO the venue as a nested quadlet).
const memberTreeNodeJSON = `{
	"target": "vm",
	"from": "fedora-base",
	"member": [
		{"name": "docs", "position": "deploy-level",
		 "node": {"target": "pod", "image": "localhost/charly-docs:latest"}},
		{"name": "sidecar", "position": "in-substrate",
		 "node": {"target": "pod", "image": "localhost/charly-sidecar:latest"}}
	]
}`

// TestVmPostApply_MemberTreeNodeDecodesClassifies proves the plugin's config-consumption
// surface against a CONVERTED member-tree node: the CRITICAL RUNTIME CHECK of the Cutover C
// re-pin. The plugin consumes the MERGED node the host ships (it never re-reads charly.yml —
// the unified-file load goes through the executor, so the schema-CalVer stamp gate is
// charly-core-owned), so THIS decode+classify is the plugin's whole authoring-tree contract:
// the former dual Children/Members maps must arrive as the ONE ordered Member list with the
// position vocabulary, and the walk must classify it exactly as the old map indexes did.
func TestVmPostApply_MemberTreeNodeDecodesClassifies(t *testing.T) {
	var node spec.FleetNode
	if err := json.Unmarshal([]byte(memberTreeNodeJSON), &node); err != nil {
		t.Fatalf("converted member-tree node must decode into spec.FleetNode: %v", err)
	}
	if !node.HasMembers() {
		t.Fatal("HasMembers() = false on a two-member node — the member-tree fold did not arrive")
	}
	if node.MemberByName("sidecar") == nil {
		t.Error("MemberByName(\"sidecar\") = nil — the name lookup twin of the former map index is broken")
	}
	insub := node.InSubstrateMembers()
	if len(insub) != 1 || insub[0].Name != "sidecar" || insub[0].Node == nil || insub[0].Node.Image != "localhost/charly-sidecar:latest" {
		t.Errorf("InSubstrateMembers() = %+v, want exactly [sidecar] with its own node", insub)
	}
	dep := node.DeployLevelMembers()
	if len(dep) != 1 || dep[0].Name != "docs" || !dep[0].Alongside() {
		t.Errorf("DeployLevelMembers() = %+v, want exactly [docs] marked alongside", dep)
	}
}

// TestInGuestPodMembers_ConvertedShape proves the vmPostApply member walk on the converted
// shape: the in-substrate pod is deployed in-guest; the deploy-level pod (same target, same
// image-bearing shape) must NEVER appear — the former Children/Members split is now carried
// by Position alone.
func TestInGuestPodMembers_ConvertedShape(t *testing.T) {
	var node spec.FleetNode
	if err := json.Unmarshal([]byte(memberTreeNodeJSON), &node); err != nil {
		t.Fatalf("decode: %v", err)
	}
	guests := inGuestPodMembers(&node)
	if len(guests) != 1 || guests[0].Name != "sidecar" {
		t.Fatalf("inGuestPodMembers() = %+v, want exactly [sidecar] — deploy-level docs must stay host-side", guests)
	}
	if guests[0].Node.Image != "localhost/charly-sidecar:latest" {
		t.Errorf("in-guest member image = %q, want localhost/charly-sidecar:latest", guests[0].Node.Image)
	}
}

// TestInGuestPodMembers_SkipsNonPodAndImageless covers the filter edges the old map walk
// carried as inline guards: image-less members (agent-provisioned) and non-pod substrates
// (android / kubernetes / vm) are not in-guest pods.
func TestInGuestPodMembers_SkipsNonPodAndImageless(t *testing.T) {
	nodeJSON := `{"member": [
		{"name": "agent", "position": "in-substrate", "node": {"target": "pod"}},
		{"name": "kube", "position": "in-substrate", "node": {"target": "kubernetes", "image": "x"}},
		{"name": "nested-vm", "position": "in-substrate", "node": {"target": "vm", "from": "base"}},
		{"name": "real", "position": "in-substrate", "node": {"target": "container", "image": "localhost/real:latest"}}
	]}`
	var node spec.FleetNode
	if err := json.Unmarshal([]byte(nodeJSON), &node); err != nil {
		t.Fatalf("decode: %v", err)
	}
	guests := inGuestPodMembers(&node)
	if len(guests) != 1 || guests[0].Name != "real" {
		t.Fatalf("inGuestPodMembers() = %+v, want exactly [real]", guests)
	}
}

// TestVmPostApply_MemberTreeLegacyShapeIsHardCutover pins the cutover: a node still carrying
// the REMOVED dual-map surface decodes to a member-less tree (the plugin binary predating this
// tag is the only thing that could ever produce one — the host-side loader rejects it at the
// schema gate first), and vmPostApply treats it as the noop it must be.
func TestVmPostApply_MemberTreeLegacyShapeIsHardCutover(t *testing.T) {
	var node spec.FleetNode
	if err := json.Unmarshal([]byte(`{"children": {"nested": {"image": "x"}}}`), &node); err != nil {
		t.Fatalf("legacy JSON must still decode (into a member-less tree): %v", err)
	}
	if node.HasMembers() {
		t.Fatal("a removed-shape node must not classify as member-bearing — no dual-map resurrection")
	}
}
