package deployvm

import (
	"context"
	"fmt"
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
