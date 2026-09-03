package deployvm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/opencharly/sdk/kit"
	"github.com/opencharly/spec/spec"
	"github.com/opencharly/spec/sshx"
)

// EnsureIsoGuestSSH establishes charly's reachability precondition on an
// ISO-installed guest: sshd enabled + running, the charly SSH key authorized, and the
// firewall port open — exactly what omarchy's own harness (omarchy-iso-test
// bootstrap_ssh) does, implemented in charly and run through the console BEFORE the
// SSH-based deploy steps (sshd is not up yet at that point).
//
// WHY: a stock Omarchy install ships openssh with the service disabled and the port
// closed (ufw default deny incoming), so an unattended install reaches the greeter with
// sshd inactive and unreachable — the manual's "when authorized_keys is present, the
// install ... enables sshd and opens the firewall" promise is not delivered by the
// 4.0.1 install path (measured). The existing authorized_keys approach stays INTACT:
// this is idempotent and does nothing when SSH already works (a distro whose installer
// DID honor authorized_keys).
//
// The SSH key is resolved through the SAME sdk path every other injection channel uses
// (spec/sshx.ResolveSSHPubKey — the resolver behind resolveSSHPubKeyForSpec), so the
// key typed into the console is byte-identical to the one cloud-init, the ISO seed and
// the libvirt SMBIOS channels author — one key, one resolver (R3).
func EnsureIsoGuestSSH(ctx context.Context, ssh kit.SSHArgs, vm *spec.ResolvedVm, runSSH RemoteRunner, console ConsoleRunner, keySource, vmStateDir string) (string, error) {
	if vm == nil || vm.Source.Kind != "iso" {
		return "", nil
	}
	user := isoDeploySSHUser(vm)
	if user == "" {
		return "", fmt.Errorf("iso guest ssh: the vm declares no ssh user, so there is no account to set up ssh for")
	}
	pw := isoInstallerPassword(vm)
	if pw == "" {
		return "", fmt.Errorf("iso guest ssh: %q needs a password for the console login and sudo, and no installer password is available", user)
	}

	// Idempotent probe FIRST: if SSH already works with the charly-managed key, the
	// installer's authorized_keys path (or a prior bootstrap) delivered — nothing to do.
	if err := runSSH(ctx, ssh, "true", ""); err == nil {
		return "", nil
	}

	// The SAME resolver every other injection channel uses — one key across all paths.
	pubKey, err := sshx.ResolveSSHPubKey(keySource, vmStateDir)
	if err != nil || pubKey == "" {
		return "", fmt.Errorf("iso guest ssh: resolving the SSH public key for the console bootstrap: %v", err)
	}

	// Console login: a text TTY (mirrors bootstrap_ssh's ctrl-alt-f3), settle any
	// half-typed prompt, then authenticate as the install user.
	console.SendKeys(ctx, "ctrl-alt-f3")
	console.WaitSeconds(6)
	console.SendKeys(ctx, "ret") // settle a half-typed prompt from a prior attempt
	console.WaitSeconds(2)
	console.TypeText(ctx, user)
	console.SendKeys(ctx, "ret")
	console.WaitSeconds(3)
	console.TypeText(ctx, pw)
	console.SendKeys(ctx, "ret")
	console.WaitSeconds(4)

	// The bootstrap — exactly the commands omarchy-iso-test bootstrap_ssh runs:
	// key into authorized_keys (grep-first so a re-run never duplicates), open the
	// firewall for the qemu user-net source, enable+start sshd.
	lines := []string{
		"mkdir -p ~/.ssh && chmod 700 ~/.ssh",
		"grep -qxF " + quote(pubKey) + " ~/.ssh/authorized_keys 2>/dev/null || echo " + quote(pubKey) + " >> ~/.ssh/authorized_keys",
		"chmod 600 ~/.ssh/authorized_keys",
		"echo " + quote(pw) + " | sudo -S ufw allow from 10.0.2.2 to any port 22 proto tcp || true",
		"echo " + quote(pw) + " | sudo -S systemctl enable --now sshd.service",
	}
	for _, line := range lines {
		console.TypeText(ctx, line)
		console.SendKeys(ctx, "ret")
		console.WaitSeconds(1)
	}
	console.SendKeys(ctx, "ctrl-alt-f1") // back to the graphical session
	return fmt.Sprintf("bootstrap-ssh: enabled sshd + firewall + authorized key for %s via the console", user), nil
}

// quote single-quotes a string for the guest shell (the key and password contain no
// single quotes; the harness types them the same way).
func quote(s string) string {
	return "'" + s + "'"
}

// ConsoleRunner types into the guest's console display — the harness's QMP send-key +
// screendump equivalent. Injected for testability; the production implementation is
// VirshConsoleRunner.
type ConsoleRunner interface {
	TypeText(ctx context.Context, text string) error
	SendKeys(ctx context.Context, chord string) error
	WaitSeconds(seconds int)
}

// VirshConsoleRunner types into the guest via libvirt send-key (virsh -c <uri>
// send-key <domain> ...) — the same primitive omarchy-iso-test uses via QMP.
type VirshConsoleRunner struct {
	LibvirtURI string
	Domain     string
}

func (r *VirshConsoleRunner) WaitSeconds(seconds int) {
	if seconds <= 0 {
		return
	}
	time.Sleep(time.Duration(seconds) * time.Second)
}

var virshKeyNames = map[rune]string{
	'a': "KEY_A", 'b': "KEY_B", 'c': "KEY_C", 'd': "KEY_D", 'e': "KEY_E",
	'f': "KEY_F", 'g': "KEY_G", 'h': "KEY_H", 'i': "KEY_I", 'j': "KEY_J",
	'k': "KEY_K", 'l': "KEY_L", 'm': "KEY_M", 'n': "KEY_N", 'o': "KEY_O",
	'p': "KEY_P", 'q': "KEY_Q", 'r': "KEY_R", 's': "KEY_S", 't': "KEY_T",
	'u': "KEY_U", 'v': "KEY_V", 'w': "KEY_W", 'x': "KEY_X", 'y': "KEY_Y",
	'z': "KEY_Z",
	'0': "KEY_0", '1': "KEY_1", '2': "KEY_2", '3': "KEY_3", '4': "KEY_4",
	'5': "KEY_5", '6': "KEY_6", '7': "KEY_7", '8': "KEY_8", '9': "KEY_9",
	' ': "KEY_SPACE", '-': "KEY_MINUS", '.': "KEY_DOT", '/': "KEY_SLASH",
	'=': "KEY_EQUAL", ';': "KEY_SEMICOLON",
}
var virshShiftNames = map[rune]string{
	'~': "KEY_GRAVE", '$': "KEY_4", '"': "KEY_APOSTROPHE", '>': "KEY_DOT",
	'|': "KEY_BACKSLASH", '+': "KEY_EQUAL", ':': "KEY_SEMICOLON", '!': "KEY_1",
	'<': "KEY_COMMA", '?': "KEY_SLASH", '@': "KEY_2", '#': "KEY_3",
	'%': "KEY_5", '&': "KEY_7", '*': "KEY_8", '(': "KEY_9", ')': "KEY_0",
	'_': "KEY_MINUS", '{': "KEY_LEFTBRACE", '}': "KEY_RIGHTBRACE",
}

// virshChord is the Linux input-event key-name mapping for the chords the console
// bootstrap sends. The modifiers are KEY_LEFTCTRL/KEY_LEFTALT (KEY_CTRL/KEY_ALT are
// not valid input-event names and virsh rejects them).
var virshChord = map[string]string{
	"ctrl":      "KEY_LEFTCTRL",
	"alt":       "KEY_LEFTALT",
	"meta":      "KEY_LEFTMETA",
	"ret":       "KEY_ENTER",
	"f1":        "KEY_F1",
	"f3":        "KEY_F3",
	"minus":     "KEY_MINUS",
	"space":     "KEY_SPACE",
	"backspace": "KEY_BACKSPACE",
}

func (r *VirshConsoleRunner) SendKeys(ctx context.Context, chord string) error {
	parts := strings.Split(chord, "-")
	args := []string{"-c", r.LibvirtURI, "send-key", r.Domain}
	for _, p := range parts {
		if k, ok := virshChord[p]; ok {
			args = append(args, k)
			continue
		}
		args = append(args, "KEY_"+strings.ToUpper(p))
	}
	return exec.CommandContext(ctx, "virsh", args...).Run()
}

func (r *VirshConsoleRunner) TypeText(ctx context.Context, text string) error {
	for _, ch := range text {
		if key, ok := virshKeyNames[ch]; ok {
			if err := r.sendOne(ctx, key); err != nil {
				return err
			}
			continue
		}
		if key, ok := virshShiftNames[ch]; ok {
			if err := r.sendTwo(ctx, "KEY_LEFTSHIFT", key); err != nil {
				return err
			}
			continue
		}
		if ch >= 'A' && ch <= 'Z' {
			if err := r.sendTwo(ctx, "KEY_LEFTSHIFT", "KEY_"+string(ch)); err != nil {
				return err
			}
			continue
		}
		return fmt.Errorf("console typer: no mapping for char %q", ch)
	}
	return nil
}

func (r *VirshConsoleRunner) sendOne(ctx context.Context, key string) error {
	return exec.CommandContext(ctx, "virsh", "-c", r.LibvirtURI, "send-key", r.Domain, key).Run()
}
func (r *VirshConsoleRunner) sendTwo(ctx context.Context, a, b string) error {
	return exec.CommandContext(ctx, "virsh", "-c", r.LibvirtURI, "send-key", r.Domain, a, b).Run()
}
