# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

SimpleCiscoTUI is a small [Textual] terminal UI that performs three Cisco IOS
operations over SSH (via [Netmiko]'s `cisco_ios` driver):

1. Apply an ACL to an interface — `ip access-group <acl> <in|out>`
2. Remove an ACL from an interface — `no ip access-group <acl> <in|out>`
3. Copy running-config to startup-config — `copy run start`

It is intentionally generic (no hard-coded hosts or ACL names) because it is
slated to go public.

[Textual]: https://github.com/Textualize/textual
[Netmiko]: https://github.com/ktbyers/netmiko

## Architecture

Two modules, with a deliberate seam between pure logic and network I/O:

- **`simpleciscotui/cisco.py`**
  - Pure, network-free helpers — unit-tested in `tests/test_cisco.py`:
    `parse_interface_brief`, `parse_interface_acls`, `normalize_direction`,
    `build_apply_commands`, `build_remove_commands`.
  - `CiscoCredentials` — connection params; `netmiko_kwargs()` maps to Netmiko.
  - `CiscoSession` — owns the live Netmiko handle. `connect()` enters enable
    mode; `apply_acl` / `remove_acl` use `send_config_set`; `save_config` calls
    Netmiko's `save_config()`. **All wire access lives here.**
  - Netmiko is imported lazily so the parsing helpers (and their tests) work
    without the dependency installed.

- **`simpleciscotui/config.py`** — optional TOML config loader (also pure /
  unit-tested via `parse_config`). `DeviceConfig.to_credentials()` defaults the
  username to `getpass.getuser()` and leaves the password blank, so a host-only
  entry authenticates like `ssh <host>` (SSH agent + `~/.ssh` keys). Search
  order: `$SIMPLECISCOTUI_CONFIG` → `./config.toml` → `~/.config/simpleciscotui/
  config.toml`. `config.toml` is git-ignored; `config.example.toml` is the
  shipped sample.

- **`simpleciscotui/app.py`** — the Textual app. Screen flow:
  `ConnectScreen → MenuScreen → {ApplyAclScreen, RemoveAclScreen, SaveScreen}`.
  - `ConnectScreen` loads the config in `__init__`; a single saved device
    auto-connects from `on_mount`, multiple devices populate a `#device`
    `Select`. Key auth is implicit: a blank password → `use_keys`/`allow_agent`
    in `CiscoCredentials.netmiko_kwargs()`.
  - `ApplyAclScreen`/`RemoveAclScreen` share `_OperationScreen` (interface
    picker + status log). Remove also loads live bindings per interface.
  - `StatusLog` is a `RichLog` subclass with `ok`/`err`/`info`/`device` helpers.

## Concurrency rule (important)

Netmiko calls **block**, so every device interaction runs in a Textual thread
worker (`@work(thread=True, exclusive=True)`). Worker bodies must **not** touch
widgets directly — they marshal UI updates back with
`self.app.call_from_thread(...)`. Keep new device operations on this pattern.

## Naming gotcha

Do **not** add a method or attribute named `log` to a `Screen`/`Widget` —
Textual already exposes `self.log` (a logger) and uses `self.log.debug(...)`
internally during focus handling. The status-pane accessor is therefore named
`status_log()`, and the `RichLog` widget has `id="log"`. Shadowing `log` raises
`AttributeError: 'function' object has no attribute 'debug'` at compose time.

## Commands

```sh
scripts/run.sh        # create .venv if missing, install deps, launch the TUI
scripts/test.sh       # run the pytest suite in .venv
.venv/bin/python -m pytest tests -q        # tests directly
textual run --dev simpleciscotui.app:SimpleCiscoTUI   # dev mode + console
```

There is **no CI** — tests are run locally via `scripts/test.sh`.

## Conventions

- Credentials come from `config.toml`, the connect form, or `CISCO_*` env vars
  (`CISCO_HOST`, `CISCO_USERNAME`, `CISCO_PASSWORD`, `CISCO_SECRET`,
  `CISCO_PORT`). The app never writes credentials to disk; `.env` and
  `config.toml` are git-ignored.
- License is MIT; attribution line on user-facing docs is
  "Built with Claude Code (Opus)".
- When adding a screen, register its buttons with `@on(Button.Pressed, "#id")`
  and keep keyboard `BINDINGS` in sync so the footer stays accurate.
