package deployvm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/opencharly/sdk/kit"
	"github.com/opencharly/spec/spec"
)

// EnsureIsoGuestSudo establishes charly's deploy precondition — the SSH user can sudo
// without a password — on a guest installed from a distro's own installer ISO.
//
// WHY THIS IS NEEDED AT ALL, and why only here:
//
// Every other VM source kind gives charly a rootfs it controls, and the passwordless
// sudoers drop-in is written while building it (see the distro entities'
// bootloader.install_template, which does exactly this for a bootstrap VM). An installer
// ISO does not: the distro's own installer creates the account, and archinstall's
// enable_sudo writes
//
//	<user> ALL=(ALL) ALL
//
// with NO NOPASSWD option in the answer file, and no hook to change it. Omarchy's
// orchestrator adds nothing either — its sudoers.d ships only command-scoped rules
// (asdcontrol, dns, tzupdate), so there is no group to join declaratively. Verified
// against the shipping 4.0.1 ISO's own squashfs, not assumed.
//
// The visible failure without this is a deploy that dies in its first package step:
//
//	sudo: a terminal is required to read the password; either use ssh's -t option
//	      or configure an askpass helper
//	sudo: a password is required
//
// So charly finishes provisioning the guest it just installed, using the credential IT
// authored for that install. This is the same act as the bootstrap path's sudoers write,
// moved to the only moment an ISO install allows it.
//
// HANDLING OF THE CREDENTIAL:
//
//   - the password reaches sudo on STDIN, never in argv — it is not visible in `ps` to
//     other users on the host, and never reaches a log line
//   - it runs ONCE and is idempotent: a guest that already has passwordless sudo is
//     probed and left alone, so a re-deploy neither re-writes nor re-sends anything
//   - the result is VERIFIED before returning, so a silent failure cannot look like success
//   - no password, no fallback, no guessing: it returns an error naming what is missing
//
// It deliberately does NOT weaken anything for other source kinds, and it does not touch
// the guest's own sudoers policy beyond one drop-in named for the deploy user.
func EnsureIsoGuestSudo(ctx context.Context, ssh kit.SSHArgs, vm *spec.ResolvedVm, run RemoteRunner) (string, error) {
	if vm == nil || vm.Source.Kind != "iso" {
		return "", nil
	}
	user := isoDeploySSHUser(vm)
	if user == "" {
		return "", fmt.Errorf("iso guest sudo: the vm declares no ssh user, so there is no account to grant sudo to")
	}

	// Idempotent probe FIRST. On every re-deploy this is the whole function, and it also
	// covers a distro whose installer already grants passwordless sudo — charly should not
	// assume it is the only thing that can have done so.
	if err := run(ctx, ssh, "sudo -n true", ""); err == nil {
		return "", nil
	}

	pw := isoInstallerPassword(vm)
	if pw == "" {
		return "", fmt.Errorf("iso guest sudo: %q cannot sudo without a password, and no installer password is "+
			"available to establish it. charly deploys need passwordless sudo on the guest. Either set "+
			"source.installer.password on the vm entity (it is what the install used), or grant "+
			"%q NOPASSWD in the image", user, user)
	}

	// One command: write the drop-in, lock its mode down, and validate it. visudo -c is the
	// guard against ever leaving an unparseable file in sudoers.d — that would lock the
	// account out of sudo entirely, which is much worse than the state we started in.
	script := fmt.Sprintf(
		`set -eu
tmp=$(mktemp)
printf '%%s ALL=(ALL) NOPASSWD: ALL\n' %q > "$tmp"
chmod 0440 "$tmp"
visudo -c -f "$tmp" >/dev/null
install -m 0440 -o root -g root "$tmp" /etc/sudoers.d/99-charly-%s
rm -f "$tmp"`, user, user)

	if err := run(ctx, ssh, "sudo -S -p '' sh -s", pw+"\n"+script); err != nil {
		return "", fmt.Errorf("iso guest sudo: granting %q passwordless sudo failed (the installer password may be wrong): %w", user, err)
	}

	// VERIFY. A sudoers file that did not take effect must not be reported as success — the
	// next thing that happens is a package install that would fail with the same opaque
	// "a password is required".
	if err := run(ctx, ssh, "sudo -n true", ""); err != nil {
		return "", fmt.Errorf("iso guest sudo: the drop-in was written but %q still cannot sudo without a password: %w", user, err)
	}
	return fmt.Sprintf("granted %s passwordless sudo (/etc/sudoers.d/99-charly-%s)", user, user), nil
}

// RemoteRunner runs one command on the guest, optionally feeding it stdin. Injected so the
// bootstrap is testable without a live guest, and so the ONE place that handles the
// password is a single, reviewable function.
type RemoteRunner func(ctx context.Context, ssh kit.SSHArgs, command, stdin string) error

// SSHRemoteRunner is the production RemoteRunner: plain ssh over the managed alias.
func SSHRemoteRunner(ctx context.Context, ssh kit.SSHArgs, command, stdin string) error {
	args := append(ssh.BaseArgs(), command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	// stderr is captured, NOT forwarded: a sudo failure echoes its prompt, and the caller's
	// error text is written to say what happened without reproducing anything it read.
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// isoDeploySSHUser is the account the deploy connects as. The installer's username is the
// account it created, and ssh.user is what charly connects with; they are the same account
// on every shape charly authors, and ssh.user wins if they somehow differ because that is
// the one the deploy actually uses.
func isoDeploySSHUser(vm *spec.ResolvedVm) string {
	if vm.SSH != nil && vm.SSH.User != "" {
		return vm.SSH.User
	}
	if vm.Source.Installer != nil {
		return vm.Source.Installer.Username
	}
	return ""
}

// isoInstallerPassword returns the PLAINTEXT install password, which exists only when the
// entity authored one. A password_hash cannot be used here: sudo needs the password, not a
// hash, exactly as LUKS does.
func isoInstallerPassword(vm *spec.ResolvedVm) string {
	if vm.Source.Installer == nil {
		return ""
	}
	return vm.Source.Installer.Password
}

// IsoReadinessSSHArgs returns the ssh args to use for an iso VM's READINESS WAIT.
//
// An installer-driven VM changes its SSH host identity exactly once, mid-wait, and the
// ordinary args cannot survive that:
//
//  1. the live installer environment brings up its own sshd with freshly generated host
//     keys (Omarchy's ISO has cloud-init do this, visible on its console)
//  2. the readiness probe connects, and StrictHostKeyChecking=accept-new PINS that key
//  3. the installer finishes and the guest reboots into the INSTALLED system, whose host
//     keys are entirely different
//  4. every probe after that fails with "Host key for [...] has changed", forever — the
//     poll burns its whole cap against a condition that CANNOT become true
//
// Measured end to end: the pin appears during the wait, and afterwards the managed alias
// returns "Host key verification failed" while a connection with the pin removed reports
// hostname=omarchy, root=btrfs, sshd=enabled.
//
// So the readiness wait — and ONLY the readiness wait — runs without consulting or writing
// a known_hosts. Pinning during that window is not merely unhelpful, it is meaningless:
// the identity being pinned belongs to a system that is about to cease existing. Once the
// wait returns, every later connection uses the ordinary args and pins the INSTALLED
// system's key, which is the one worth pinning.
//
// Scoped to source.kind: iso. Every other kind boots ONE system with ONE identity, keeps
// full host-key checking throughout, and is not routed through here at all.
func IsoReadinessSSHArgs(ssh kit.SSHArgs, vm *spec.ResolvedVm) kit.SSHArgs {
	if vm == nil || vm.Source.Kind != "iso" {
		return ssh
	}
	relaxed := ssh
	relaxed.Args = append(append([]string{}, ssh.Args...),
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
	)
	return relaxed
}

// ClearStaleHostKeyPin removes a per-domain known_hosts left over from a PREVIOUS run whose
// readiness wait pinned an installer environment. IsoReadinessSSHArgs stops new ones being
// written; this clears one already on disk, so a re-run is not poisoned by the last one.
//
// No-op for every source kind but iso, and for a domain with no pin. It touches only the
// per-domain file charly writes itself.
func ClearStaleHostKeyPin(vm *spec.ResolvedVm, knownHostsPath string) (bool, error) {
	if vm == nil || vm.Source.Kind != "iso" || knownHostsPath == "" {
		return false, nil
	}
	kh := knownHostsPath
	if _, err := os.Stat(kh); err != nil {
		return false, nil
	}
	if err := os.Remove(kh); err != nil {
		return false, fmt.Errorf("clearing a stale host-key pin at %s: %w", kh, err)
	}
	return true, nil
}
