package deployvm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/sdk/loaderkit"
	"github.com/opencharly/spec/spec"
)

// errStopAfterConfigStack is a sentinel returned from the RunBootstrapPhase seam — the
// FIRST seam loaderkit.LoadUnified calls after it reads the layered config stack — so
// this test observes the MERGED root document without driving the registry-coupled
// walk/materialize/validate half (which needs a live host). It is not an error path
// under test; it is how the test stops at the seam it cares about.
var errStopAfterConfigStack = errors.New("stop after config stack")

// TestResolvePathStripsSystemConfigVersionStamp is the plugin-level regression guard
// for plugin-deploy-vm#40. The released charly PACKAGE writes the system config layer
// /etc/charly/charly.yml carrying a top-level `version:` stamp; spec #183 removed that
// key from the closed #NodeDoc, so the plugin's resolve path (vmPrepareVenue →
// loaderkit.ResolveVmEntityViaExecutor → loaderkit.LoadUnified) fails on EVERY project
// with `conflicting values "…" and {...} (mismatched types string and struct)` unless
// the pinned sdk strips the retired key. The strip entered the sdk at v0.2026276.1822;
// this plugin's OLD pin v0.2026273.207 had none (grep = 0).
//
// The test compiles and runs against THIS plugin's pinned sdk (go.mod), so it fails
// again if the pin is ever moved back below v0.2026276.1822. It drives the exact entry
// point the resolve path uses (LoadUnified → readConfigStack) and asserts the system
// layer's `version:` stamp never reaches the merged root — while the project's OWN
// authored content survives the merge.
func TestResolvePathStripsSystemConfigVersionStamp(t *testing.T) {
	systemDir := t.TempDir()
	projectDir := t.TempDir()

	// The system layer, exactly as the charly package writes it: a retired top-level
	// `version:` stamp plus a (here minimal) directive the current contract still
	// accepts, so the layer is not empty once the stamp is dropped.
	systemPath := filepath.Join(systemDir, "charly.yml")
	if err := os.WriteFile(systemPath, []byte("version: 2026.261.1747\n"), 0o644); err != nil {
		t.Fatalf("write system layer: %v", err)
	}
	t.Setenv(loaderkit.SystemConfigEnv, systemPath)

	// A project that declares the very kind:vm entity the failing resolve looked up.
	if err := os.WriteFile(filepath.Join(projectDir, spec.UnifiedFileName), []byte(
		"alpha:\n    candy:\n        description: project-alpha\n"), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	var mergedRoot []byte
	_, ok, err := loaderkit.LoadUnified(projectDir, loaderkit.LoadSeams{
		RunBootstrapPhase: func(data []byte) ([]byte, error) {
			mergedRoot = append([]byte(nil), data...)
			return nil, errStopAfterConfigStack
		},
	})
	if !ok {
		t.Fatalf("LoadUnified: ok=false, want a project (system layer + in-dir charly.yml)")
	}
	if !errors.Is(err, errStopAfterConfigStack) {
		t.Fatalf("LoadUnified: reached the stack read and a later step returned %v, want the sentinel", err)
	}
	if mergedRoot == nil {
		t.Fatal("RunBootstrapPhase was never called — the stack read did not produce a merged document")
	}

	// The retired system-layer stamp must NOT be a top-level key in the merged root.
	if strings.Contains(string(mergedRoot), "2026.261.1747") {
		t.Fatalf("the retired system-layer `version:` stamp reached the merged root — on a host carrying it the closed #NodeDoc rejects every project:\n%s", mergedRoot)
	}
	// The project's own authored content must survive.
	if !strings.Contains(string(mergedRoot), "project-alpha") {
		t.Fatalf("the project's own content was lost from the merged root:\n%s", mergedRoot)
	}
}
