package deployvm

import (
	"context"
	osexec "os/exec"
	"strings"
	"testing"

	"github.com/opencharly/sdk/kit"
	"github.com/opencharly/spec/spec"
)

// recordingExec captures the scripts the walk runs on the venue, so the emitted host
// commands can be asserted without a live host.
type recordingExec struct {
	sysScripts []string
}

func (e *recordingExec) Venue(context.Context) (string, error) { return "fake://venue", nil }
func (e *recordingExec) RunSystem(_ context.Context, s string, _ []byte) error {
	e.sysScripts = append(e.sysScripts, s)
	return nil
}
func (e *recordingExec) RunUser(_ context.Context, s string, _ []byte) error {
	e.sysScripts = append(e.sysScripts, s)
	return nil
}
func (e *recordingExec) PutFile(_ context.Context, _ string, _ []byte, _ uint32, _ bool) error {
	return nil
}
func (e *recordingExec) GetFile(_ context.Context, _ string, _ bool) ([]byte, error) { return nil, nil }
func (e *recordingExec) RunCapture(_ context.Context, _ string) (string, string, int, error) {
	return "/home/u", "", 0, nil
}
func (e *recordingExec) RunHostStep(_ context.Context, _ spec.InstallStepView, _ []byte) ([]spec.ReverseOp, error) {
	return nil, nil
}

// TestWalkPlans_HostDownloadGuardsAndEmptyTo pins the fix for the host-renderer
// defect filed as sdk#302 and merged as opencharly/sdk#303, from THIS plugin's
// entry point: plugin-deploy-vm hands its executor straight to kit.WalkPlans
// (plugin.go: `kit.WalkPlans(ctx, exec, plans, kit.WalkOpts{})`), so the
// machine-venue emitted script for the exact plan that broke the Phase 5
// check-kind-host-vm bed — a `download:` with `extract: sh`, `unless_exists`, and
// no `to:` (layer-kubernetes' helm step) — must (a) NOT emit `install -d ”`,
// (b) honour unless_exists, and (c) parse under `sh -n`. Against the pre-fix sdk
// pin (v0.2026269.1159) this test fails on (a): the guest renderer emitted
// `install -d -m0755 ”` and the deploy died with
// `install: cannot create directory ”`.
func TestWalkPlans_HostDownloadGuardsAndEmptyTo(t *testing.T) {
	rec := &recordingExec{}
	plans := []spec.InstallPlanView{{
		Steps: []spec.InstallStepView{{
			Kind:      "Op",
			Scope:     spec.ScopeSystem,
			CandyName: "kubernetes",
			Op: &spec.Op{
				Download:     "https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3",
				Extract:      "sh",
				UnlessExists: "/usr/bin/helm",
				RunAs:        "root",
			},
		}},
	}}
	if _, err := kit.WalkPlans(context.Background(), rec, plans, kit.WalkOpts{}); err != nil {
		t.Fatalf("WalkPlans: %v", err)
	}
	if len(rec.sysScripts) == 0 {
		t.Fatal("no script emitted for the download step")
	}
	script := strings.Join(rec.sysScripts, "\n")

	// (a) the exact defect: `install -d -m0755 ''` must be gone.
	if strings.Contains(script, "-m0755 ''") || strings.Contains(script, `-m0755 ""`) {
		t.Fatalf("empty-to download emitted install -d with an empty destination:\n%s", script)
	}
	// (b) the host renderer must honour unless_exists.
	if !strings.Contains(script, "if [ -e '/usr/bin/helm' ]; then") {
		t.Fatalf("unless_exists was not honoured by the host renderer:\n%s", script)
	}
	// (c) the emitted script must be valid sh — a wrong gate terminator fails here.
	c := osexec.Command("sh", "-n")
	c.Stdin = strings.NewReader(script)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("emitted script is not valid sh (%v):\n%s\n---\n%s", err, out, script)
	}
}
