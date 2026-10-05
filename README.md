# plugin-deploy-vm

The `vm` deploy substrate for OpenCharly — `target: vm`, a deployment applied
INSIDE a running VM over SSH, served out-of-process (`deploy:vm`).

The plugin is a standalone Go module the charly loader host-builds and serves
over go-plugin gRPC. It is the vm-substrate sibling of `plugin-deploy-local`, and
it owns BOTH the VM venue lifecycle (boot/destroy/console/ssh + nested
pod-in-guest) and the plan walk.

## What it provides

| Capability | Surface |
|---|---|
| `deploy:vm` | the `vm:` deploy substrate (a deployment applied inside a running VM over SSH) |

## How it works

The generic plugin-side deploy target reaches it with the deployment's
InstallPlan views + a venue descriptor, and the host's executor served on the
broker — for vm the GUEST `SSHExecutor` this plugin's own venue lifecycle built
after booting the domain, waiting for sshd / cloud-init, and ensuring charly is
in the guest.

The plugin dials back through the SDK executor and hands the plans to the shared
`kit.WalkPlans`:

- **Plugin-renderable steps** (`Op` write/cmd/download, `File`, `ShellHook` + the
  `env.d` managed-block finalizer, `ShellSnippet`, `ServicePackaged`,
  `ServiceCustom`, `RepoChange`) it EXECUTES itself inside the guest via the F2
  reverse legs (`RunSystem` / `RunUser` / `PutFile`), echoing the host-computed
  reverse ops.
- **Host-engine steps** (`Builder` / `LocalPkgInstall` / `SystemPackages` /
  act-`Op` / `ExternalPlugin`) AND a `reboot: true` `RebootStep` it drives over
  the `RunHostStep` reverse leg (builders run on the host's podman and are scp'd
  into the guest; a reboot reboots the guest + waits for the `boot_id` change).

It returns the combined teardown ops the host records in the install ledger and
replays at `charly deploy del` (record-and-replay).

## How to use it

Compose the plugin candy in a project's `candy:` list:

```yaml
- '@github.com/opencharly/plugin-deploy-vm/candy/plugin-deploy-vm:<tag>'
```

Then author a `vm:` deploy:

```yaml
my-deploy:
    vm:
        from: my-vm-template
```

## Layout

- `candy/plugin-deploy-vm/` — the plugin module: `plugin.go` (the deploy provider
  + `NewProvider()` / `NewMeta()` / `Invoke`), `lifecycle.go` (the VM venue
  lifecycle), `iso_ssh_bootstrap.go` / `iso_sudo_bootstrap.go` (the ISO bootstrap
  helpers), `schema/vm.cue` (the self-contained `#DeployVMPlugin`), the Go tests,
  `cmd/serve/main.go`.
- `charly.yml` — the root project manifest (`discover: candy`).
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.

## Related

- Owning skill: `/charly-vm:vm` — `charly vm` commands, `kind: vm` entities,
  cloud_image vs bootc source types, and VM lifecycle.
- `/charly-core:deploy` — `charly deploy add`/`del` and deploy configuration.
- `/charly-internals:plugin` — the plugin/provider model, including the `deploy`
  provider class.
- [`opencharly/charly`](https://github.com/opencharly/charly) — the charly CLI.
