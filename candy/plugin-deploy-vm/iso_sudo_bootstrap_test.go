package deployvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/sdk/kit"
	"github.com/opencharly/spec/spec"
)

// recorder is a RemoteRunner that records every command and stdin it was handed, and lets a
// test decide which commands fail. That makes the credential path assertable: the test can
// prove the password went to STDIN and never into a command string.
type recorder struct {
	cmds   []string
	stdins []string
	fail   func(cmd string, callN int) error
	n      int
}

func (r *recorder) run(_ context.Context, _ kit.SSHArgs, cmd, stdin string) error {
	r.cmds = append(r.cmds, cmd)
	r.stdins = append(r.stdins, stdin)
	r.n++
	if r.fail != nil {
		return r.fail(cmd, r.n)
	}
	return nil
}

func isoVM(user, pw string) *spec.ResolvedVm {
	vm := &spec.ResolvedVm{}
	vm.Source.Kind = "iso"
	vm.Source.Installer = &spec.VmInstaller{Username: user, Password: pw}
	return vm
}

// A guest that ALREADY has passwordless sudo is left completely alone: one probe, no write,
// and — the part that matters — the password is never sent anywhere. This is the whole
// function on every re-deploy.
func TestEnsureIsoGuestSudo_AlreadyPasswordlessIsANoOp(t *testing.T) {
	r := &recorder{}
	note, err := EnsureIsoGuestSudo(context.Background(), kit.SSHArgs{}, isoVM("user", "hunter2"), r.run)
	if err != nil {
		t.Fatalf("EnsureIsoGuestSudo: %v", err)
	}
	if note != "" {
		t.Errorf("nothing was changed, so nothing should be reported; got %q", note)
	}
	if len(r.cmds) != 1 || r.cmds[0] != "sudo -n true" {
		t.Fatalf("want exactly one probe; got %v", r.cmds)
	}
	for i, in := range r.stdins {
		if strings.Contains(in, "hunter2") {
			t.Errorf("the password was sent on call %d despite sudo already working", i)
		}
	}
}

// The credential contract, asserted rather than assumed: when the grant IS needed, the
// password goes on STDIN and never into a command string (argv is world-readable in `ps`).
func TestEnsureIsoGuestSudo_PasswordGoesToStdinNeverArgv(t *testing.T) {
	r := &recorder{fail: func(cmd string, n int) error {
		if n == 1 { // the first probe fails: sudo needs a password
			return fmt.Errorf("a password is required")
		}
		return nil
	}}
	note, err := EnsureIsoGuestSudo(context.Background(), kit.SSHArgs{}, isoVM("user", "hunter2"), r.run)
	if err != nil {
		t.Fatalf("EnsureIsoGuestSudo: %v", err)
	}
	if !strings.Contains(note, "passwordless sudo") {
		t.Errorf("a change was made, so it must be reported; got %q", note)
	}
	for i, c := range r.cmds {
		if strings.Contains(c, "hunter2") {
			t.Fatalf("the password appeared in the COMMAND on call %d (%q) — visible in ps to every user on the host", i, c)
		}
	}
	joined := strings.Join(r.stdins, "")
	if !strings.Contains(joined, "hunter2") {
		t.Fatal("the password never reached stdin, so the grant cannot have authenticated")
	}
	// and the drop-in is validated before it is installed
	if !strings.Contains(joined, "visudo -c") {
		t.Error("the sudoers drop-in is installed without visudo validation — an unparseable file locks the account out of sudo entirely")
	}
}

// The write is VERIFIED. A drop-in that did not take effect must be an error here, not a
// success that surfaces later as the same opaque "a password is required" inside a package
// step.
func TestEnsureIsoGuestSudo_UnverifiedGrantIsAnError(t *testing.T) {
	r := &recorder{fail: func(cmd string, n int) error {
		if cmd == "sudo -n true" { // never works, before or after
			return fmt.Errorf("a password is required")
		}
		return nil
	}}
	_, err := EnsureIsoGuestSudo(context.Background(), kit.SSHArgs{}, isoVM("user", "hunter2"), r.run)
	if err == nil {
		t.Fatal("a grant that did not take effect must be an error")
	}
	if !strings.Contains(err.Error(), "still cannot sudo") {
		t.Fatalf("the error must say the verification failed; got: %v", err)
	}
}

// No password and no passwordless sudo is a clear error naming both ways out — not a
// silent skip that fails later in a package step.
func TestEnsureIsoGuestSudo_NoPasswordIsADirectedError(t *testing.T) {
	r := &recorder{fail: func(string, int) error { return fmt.Errorf("a password is required") }}
	_, err := EnsureIsoGuestSudo(context.Background(), kit.SSHArgs{}, isoVM("user", ""), r.run)
	if err == nil {
		t.Fatal("no password and no passwordless sudo must be an error")
	}
	for _, want := range []string{"source.installer.password", "NOPASSWD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error must name %q as a way out; got: %v", want, err)
		}
	}
}

// Every OTHER source kind is untouched — it never probes, never writes, never sends a
// credential. Those guests get their sudoers drop-in when charly builds their rootfs.
func TestEnsureIsoGuestSudo_OtherSourceKindsAreUntouched(t *testing.T) {
	for _, kind := range []string{"cloud_image", "bootc", "bootstrap", ""} {
		r := &recorder{}
		vm := isoVM("user", "hunter2")
		vm.Source.Kind = kind
		note, err := EnsureIsoGuestSudo(context.Background(), kit.SSHArgs{}, vm, r.run)
		if err != nil || note != "" {
			t.Errorf("kind %q: want a silent no-op; got note=%q err=%v", kind, note, err)
		}
		if len(r.cmds) != 0 {
			t.Errorf("kind %q: it touched the guest (%v) — those guests get sudo at build time", kind, r.cmds)
		}
	}
	// and a nil vm must not panic
	if _, err := EnsureIsoGuestSudo(context.Background(), kit.SSHArgs{}, nil, (&recorder{}).run); err != nil {
		t.Errorf("a nil vm must be a no-op; got %v", err)
	}
}

// The readiness wait for an iso VM must not consult OR write a known_hosts.
//
// The guest's host identity changes exactly once, mid-wait: the live installer environment
// brings up its own sshd with fresh host keys, the probe pins them with accept-new, the
// guest then reboots into the installed system with entirely different keys, and every
// later probe fails "Host key ... has changed" — forever. The poll burns its whole cap
// against a condition that cannot become true.
//
// Measured end to end on a real guest: the pin appears during the wait, and afterwards the
// managed alias returns "Host key verification failed" while the same connection with the
// pin removed reports hostname=omarchy, root=btrfs, sshd=enabled.
func TestIsoReadinessSSHArgs_DoesNotPinDuringTheIdentityChange(t *testing.T) {
	base := kit.SSHArgs{Host: "charly-x", Args: []string{"-o", "LogLevel=ERROR"}}
	got := IsoReadinessSSHArgs(base, isoVM("user", "pw"))

	joined := strings.Join(got.Args, " ")
	for _, want := range []string{"UserKnownHostsFile=/dev/null", "StrictHostKeyChecking=no"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the readiness wait must not pin: missing %q in %v", want, got.Args)
		}
	}
	// the caller's own args survive
	if !strings.Contains(joined, "LogLevel=ERROR") {
		t.Errorf("the caller's ssh args were dropped: %v", got.Args)
	}
	// and the ORIGINAL must not be mutated — it is reused for every later connection, which
	// SHOULD pin the installed system's key.
	if strings.Contains(strings.Join(base.Args, " "), "StrictHostKeyChecking=no") {
		t.Error("the caller's SSHArgs was mutated — later connections would stop pinning too")
	}
}

// Every other source kind keeps full host-key checking throughout: one system, one identity,
// nothing to tolerate.
func TestIsoReadinessSSHArgs_OtherSourceKindsKeepPinning(t *testing.T) {
	base := kit.SSHArgs{Host: "charly-x"}
	for _, kind := range []string{"cloud_image", "bootc", "bootstrap", ""} {
		vm := isoVM("user", "pw")
		vm.Source.Kind = kind
		got := IsoReadinessSSHArgs(base, vm)
		if strings.Contains(strings.Join(got.Args, " "), "StrictHostKeyChecking=no") {
			t.Errorf("kind %q: host-key checking was relaxed for a VM with one stable identity", kind)
		}
	}
	if got := IsoReadinessSSHArgs(base, nil); len(got.Args) != 0 {
		t.Errorf("a nil vm must pass through untouched; got %v", got.Args)
	}
}

// A pin left by a PREVIOUS run's readiness wait is cleared, so a re-run is not poisoned by
// the last one. Only for iso, only the per-domain file charly writes itself.
func TestClearStaleHostKeyPin_ClearsOnlyTheIsoPerDomainPin(t *testing.T) {
	dir := t.TempDir()
	kh := filepath.Join(dir, "known_hosts")
	write := func() {
		if err := os.WriteFile(kh, []byte("[127.0.0.1]:40563 ssh-ed25519 INSTALLERKEY\n"), 0o600); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	write()
	cleared, err := ClearStaleHostKeyPin(isoVM("user", "pw"), kh)
	if err != nil || !cleared {
		t.Fatalf("an iso pin must be cleared; got cleared=%v err=%v", cleared, err)
	}
	if _, err := os.Stat(kh); !os.IsNotExist(err) {
		t.Fatal("the stale pin survived")
	}

	for _, kind := range []string{"cloud_image", "bootstrap"} {
		write()
		vm := isoVM("user", "pw")
		vm.Source.Kind = kind
		cleared, err := ClearStaleHostKeyPin(vm, kh)
		if err != nil || cleared {
			t.Errorf("kind %q: a legitimate pin must survive; got cleared=%v err=%v", kind, cleared, err)
		}
	}

	// no path, no file, no error
	if cleared, err := ClearStaleHostKeyPin(isoVM("user", "pw"), ""); err != nil || cleared {
		t.Errorf("an empty path must be a no-op; got cleared=%v err=%v", cleared, err)
	}
}
