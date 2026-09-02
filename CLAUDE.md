# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

SimpleCiscoTUI is a small [tview] terminal UI that performs three Cisco IOS
operations:

1. Apply an ACL to an interface — `ip access-group <acl> <in|out>`
2. Remove an ACL from an interface — `no ip access-group <acl> <in|out>`
3. Copy running-config to startup-config — `write memory`

It is intentionally generic (no hard-coded hosts or ACL names) because it is
public.

The app was originally written in Python (Textual + pexpect) and ported to
Go; there is no Python left in the repo.

[tview]: https://github.com/rivo/tview

## Connection model (read this first)

The app does **not** use a Go SSH library. It drives the **system `ssh`
client** under a real PTY via [creack/pty]. This mirrors a deliberate switch
made in the Python original away from Netmiko/Paramiko-style SSH libraries,
which cannot authenticate to old IOS like the Catalyst 2960G: those
libraries fail to fall back to `ssh-rsa` pubkey auth and then choke on
Cisco's keyboard-interactive reply. The real `ssh` client handles all of
this and inherits `~/.ssh/config`. **Do not reintroduce a Go SSH library
(golang.org/x/crypto/ssh or similar) in place of the system client.**

[creack/pty]: https://github.com/creack/pty

## Architecture

Three packages, with a deliberate seam between pure logic, network I/O, and UI:

- **`internal/cisco`** (`cisco.go`, `session.go`, `expect.go`)
  - Pure, network-free helpers in `cisco.go` — unit-tested in `cisco_test.go`:
    `ParseInterfaceBrief`, `ParseInterfaceAcls`, `NormalizeDirection`,
    `BuildApplyCommands`, `BuildRemoveCommands`.
  - `Credentials` — `SSHOptions()` builds `ssh` flags (`BatchMode` unless a
    password is set; `-i` for `KeyFile`; legacy `-o` crypto when
    `LegacySSH`), and `Target()` is `user@host`.
  - `Session` (`session.go`) — spawns one persistent `ssh` child under a PTY
    (`github.com/creack/pty`), syncs on the IOS prompt (`PromptRe`), disables
    paging, and sends commands one at a time reading back to the prompt.
    `ApplyAcl`/`RemoveAcl` wrap the builder output in
    `configure terminal … end`; `SaveConfig` runs `write memory`. **All wire
    access lives here.**
  - `expect.go` — `outputBuffer` is the pexpect-equivalent: a background
    goroutine appends raw PTY output to a buffer, and `expect()` polls it for
    the earliest regex match across a candidate list (mirroring pexpect's
    `expect([patterns])` semantics), consuming the buffer up to the match.

- **`internal/config`** — optional TOML config loader (also pure /
  unit-tested via `ParseConfig`). `DeviceConfig.ToCredentials()` defaults the
  username to the local login (`$USER`/`$LOGNAME`) and leaves the password
  blank, so a host-only entry authenticates like `ssh <host>` (SSH agent +
  `~/.ssh` keys). Search order: `$SIMPLECISCOTUI_CONFIG` → `./config.toml` →
  `~/.config/simpleciscotui/config.toml`. `config.toml` is git-ignored;
  `config.example.toml` is the shipped sample.

- **`internal/tui`** — the tview app. Screen flow:
  `ConnectScreen → MenuScreen → {ApplyAclScreen, RemoveAclScreen, SaveScreen}`,
  modeled as a small stack (`App.push`/`App.pop`/`App.switchTop`) of
  `tview.Primitive`s, each its own file (`connect.go`, `menu.go`, `apply.go`,
  `remove.go`, `save.go`).
  - `ConnectScreen` loads the config in its constructor; a single saved
    device auto-connects immediately, multiple devices populate a `Select
    device` dropdown. Key auth is implicit: a blank password → `BatchMode=yes`
    key/agent auth in `Credentials.SSHOptions()`.
  - `ApplyAclScreen`/`RemoveAclScreen` share `loadInterfaces` (`operation.go`)
    for the interface picker + status log. Remove also loads live bindings
    per interface.
  - `StatusLog` (`statuslog.go`) wraps `tview.TextView` with `OK`/`Err`/
    `Info`/`Device` helpers, using tview's `[color]` tags in place of Rich
    markup.
  - `Shortcuts` (`app.go`) is a small interface (`Shortcut(event) bool`) that
    a screen implements to claim first crack at a key while it's on top of
    the stack — used for Escape-to-back and the Menu screen's a/r/s/d letters.

## Concurrency rule (important)

ssh/pty calls **block**, so every device interaction runs in its own
goroutine (`go s.xWorker(...)`), mirroring the Python original's
`@work(thread=True)` Textual workers. Worker goroutines must **not** touch
tview widgets directly — they marshal UI updates back with
`app.queueUpdate(func() { ... })` (`tapp.QueueUpdateDraw`), the equivalent of
`call_from_thread`. Keep new device operations on this pattern.

## tview layout gotchas

- `tview.Form` fields need `form.SetItemPadding(0)` or the default 1-row gap
  between items roughly doubles the form's real height, and fixed-size
  `Flex` allocations will clip the button row. Even with padding zeroed,
  Form's own minimum draw height for a form with N labeled items (dropdowns/
  inputs) is `N + 4`, not `N + 2` — verified empirically (see the screens'
  `AddItem(form, N+4, 0, true)` calls); a button-only form (no labeled items,
  e.g. `SaveScreen`) doesn't need the extra padding.
- An **open** `tview.DropDown` hands Application-level focus to its internal
  `*tview.List` while showing its option list, not to the `*tview.DropDown`
  itself. Any global `Up`/`Down` remapping (see `App.globalInputCapture`)
  must exclude both `*tview.DropDown` and `*tview.List` from being
  translated into `Tab`/`Backtab`, or arrow-key navigation inside an open
  dropdown (and the Menu screen's own `List`) breaks.

## Commands

```sh
scripts/run.sh         # go run . — launch the TUI
scripts/test.sh        # go test ./... — run the parser/config unit tests
go build -o simpleciscotui .   # build a binary
```

There is **no CI** — tests are run locally via `scripts/test.sh`.

## Conventions

- Credentials come from `config.toml`, the connect form, or `CISCO_*` env
  vars (`CISCO_HOST`, `CISCO_USERNAME`, `CISCO_PASSWORD`, `CISCO_SECRET`,
  `CISCO_PORT`). The app never writes credentials to disk; `.env` and
  `config.toml` are git-ignored.
- License is MIT; attribution line on user-facing docs is
  "Built with Claude Code (Opus)".
- When adding a screen, give it a `Shortcut(event *tcell.EventKey) bool`
  method if it needs Escape-to-back or letter accelerators, and update the
  footer hint text passed to `newFrame`.
