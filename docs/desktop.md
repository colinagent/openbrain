# OpenBrain Desktop

OpenBrain Desktop is the Electron app in `desktop/`. It connects to a local
OpenBrain server and runtime, runs agent threads in your workspace, and stores
user settings under `~/.openbrain`.

This page covers desktop behavior that users configure in the app or on disk.
Runtime and agent design live in other docs:

- `docs/runtime.md`
- `docs/skills.md`
- `docs/tools.md`

## Settings location

Desktop settings are JSONC files under:

```text
~/.openbrain/configs/settings/
```

Common files:

| File | Purpose |
| --- | --- |
| `system.jsonc` | Logging, diagnostics, power management |
| `user.jsonc` | OpenBrain provider (cloud / local) |
| `ui.jsonc` | Theme, locale, layout, editor UI |
| `editor.jsonc` | File associations, excludes, markdown preview |
| `terminal.jsonc` | Terminal profiles |
| `keybindings.jsonc` | Keyboard shortcuts |

You can edit these files directly. The app reloads changes automatically when
a watched settings file is saved.

To open the settings folder from the app: use the logo menu → **Settings**
(opens the settings directory as a workspace).

The default workspace is not a Desktop setting. Desktop reads the connected
Runtime's `defaultWorkspace` from `config/system/get`; explicit folders and
restored tabs remain per-tab state.

## Prevent idle sleep

OpenBrain can ask the operating system not to idle-sleep the machine in two
optional ways. Both use **prevent app suspension** only: your display can still
dim and lock normally.

### Default

Idle sleep prevention is **off by default**.

### Enable in the app

1. Open the logo menu (top-left).
2. Choose **Desktop**.
3. Under **Prevent idle sleep**, choose one of:
   - **Off** — no change to system power behavior
   - **While agent is running** — only during active agent turns
   - **While OpenBrain is running** — for the whole app session

The setting is saved immediately to `system.jsonc`.

### Enable by editing config

Edit `~/.openbrain/configs/settings/system.jsonc`:

```jsonc
{
  "version": 1,
  "power": {
    "idleSleepPolicy": "whileAgentRunning"
  }
}
```

Allowed values:

| Value | Behavior |
| --- | --- |
| `off` | Do not prevent idle sleep |
| `whileAgentRunning` | Prevent idle sleep only while a turn is in progress |
| `whileAppRunning` | Prevent idle sleep while the OpenBrain app process is running |

Legacy configs may still use `preventSleepWhileAgentRunning: true`; on load
that maps to `whileAgentRunning`.

### While agent is running

Sleep prevention turns **on** when any conversation in the current window has
an in-progress agent turn, for example:

- a chat thread streaming a model response or running tools;
- a command file execution started by the agent.

It turns **off** when that turn finishes, is stopped, or errors out.

If you use multiple OpenBrain windows, sleep is prevented while **any**
window has a running turn.

This matches the turn-scoped behavior used by tools such as Codex CLI.

### While OpenBrain is running

Sleep prevention stays **on** from app launch until you quit OpenBrain, even
when no agent turn is active.

Use this when you want the bundled local server and runtime to stay available
while the machine is locked—for example as a hub for future mobile or external
surfaces. Remote access still requires separate network setup; this setting
only keeps the system from idle-suspending the app.

### What it does not do

- It does not keep the screen on. Your display can still dim or lock.
- It does not replace laptop lid-close behavior or manual sleep.
- It does not by itself expose OpenBrain to the internet or your phone.

### Platform notes

Desktop uses the operating system's standard “prevent app suspension” API
(Electron `powerSaveBlocker`). Behavior is built in on macOS, Windows, and
Linux; no extra install step is required.

## Related product docs

- `desktop/README.md` — product identity, packaging, runtime bundle layout
- `docs/runtime.md` — how the bundled runtime runs agents and threads
