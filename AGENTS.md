# AGENTS.md — plugin-deploy-vm

Standalone out-of-tree DEPLOY plugin repo serving the `vm` deploy substrate
(`deploy:vm`). The plugin is a Go module at `candy/plugin-deploy-vm/` (module
path `github.com/opencharly/plugin-deploy-vm/candy/plugin-deploy-vm`); the root
`charly.yml` only declares `discover: candy` so the repo is a project and its
candy is scanned.

Canonical files:

- `candy/plugin-deploy-vm/charly.yml` — the `plugin-deploy-vm:` candy entity
  (`plugin:` block, `plan:` check).
- `candy/plugin-deploy-vm/plugin.go` — the deploy provider (`NewProvider()` /
  `NewMeta()` / the `Invoke` lifecycle + plan walk).
- `candy/plugin-deploy-vm/lifecycle.go` — the VM venue lifecycle
  (prepare-venue/post-apply/start/stop/status/logs/shell/rebuild/teardown).
- `candy/plugin-deploy-vm/iso_ssh_bootstrap.go` /
  `iso_sudo_bootstrap.go` — the ISO bootstrap helpers.
- `candy/plugin-deploy-vm/schema/vm.cue` — the self-contained `#DeployVMPlugin`.
- `charly.yml` — the root project manifest (`discover: candy`).
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.
- `README.md` — user overview only; never agent guidance.

## Load these skills first (R0)

- `/charly-vm:vm` — `charly vm` commands, `kind: vm` entities, the cloud_image vs
  bootc source types, libvirt/QEMU backends, and VM lifecycle. Load before
  changing the lifecycle. This candy carries no `skill:` entity of its own; the
  gap is tracked in
  [opencharly/opencharly#291](https://github.com/opencharly/opencharly/issues/291).
- `/charly-internals:vm-deploy-target` — the external VM deploy substrate
  (plugin-deploy-vm, VM boot/readiness, the reverse-channel SSH executor,
  `VmDeployState`). Load before changing the deploy target seams.
- `/charly-internals:plugin` — the plugin authoring reference: the `plugin:`
  block, the `deploy` provider class, the per-plugin CUE-schema contract.
- `/charly-internals:git-workflow` — before any git/PR action.

## Build / validate / test

- `go build ./...` in `candy/plugin-deploy-vm/` — compile the plugin module.
- `go test ./...` in `candy/plugin-deploy-vm/` — the plugin's Go tests (the
  lifecycle hop/member-tree/teardown + schema-serve seams).
- `charly box validate` at the repo root — the structural check (the candy +
  `plugin:` block, CUE schema).
- The merge gate is the **org-wide** `charly/pr-validator` (required check
  `validate / validate`, defined in `opencharly/.github`); this repo has **no**
  per-repo candy gate.
- The R10 consumers are the VM beds (`check-local-vm`, `check-charly-vm`), run to
  a fresh `charly update`.

## Modify this repo

- Edit the `plugin-deploy-vm:` candy entity, the Go source, and
  `schema/vm.cue` **together** — the schema is the served declaration surface.
- This plugin owns BOTH the VM lifecycle and the plan WALK: keep the walk in the
  shared `kit.WalkPlans` (one implementation shared with the other substrate
  plugins, R3) and route host-engine steps over `RunHostStep`, never a local
  copy.

## Landing

- PR-only. Every change lands through a pull request; the org-required
  `charly/pr-validator` validates the diff and body and arms native auto-merge on
  PASS. Direct pushes to `main` are blocked.
- History lives in `CHANGELOG/` (written by `tag-on-merge` at merge time); the PR
  body IS the changelog.
- The authoritative rulebook is the umbrella `AGENTS.md` in
  `opencharly/opencharly` and `charly/AGENTS.md` in the charly repo. Do not
  restate its rules here.
