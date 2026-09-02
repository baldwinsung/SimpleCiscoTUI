# SimpleCiscoTUI

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
![Go](https://img.shields.io/badge/go-1.24%2B-00ADD8.svg)
[![Built with tview](https://img.shields.io/badge/built%20with-tview-5a4fcf.svg)](https://github.com/rivo/tview)

A tiny terminal UI for the three Cisco IOS chores you reach for most when
shuffling interface ACLs:

1. **Apply** an ACL to an interface
2. **Remove** an ACL from an interface
3. **Copy running-config → startup-config** (save)

> Built with [Claude Code](https://claude.com/claude-code) (Opus).

It drives your **system `ssh` client** (under a PTY, via [creack/pty]) rather
than a Go SSH library — so it inherits everything in your `~/.ssh/config`
(keys, host aliases, and the legacy crypto old IOS needs) and works wherever
plain `ssh <host>` already works. It renders with [tview], and every device
call runs in its own goroutine so the UI never freezes mid-operation.

[creack/pty]: https://github.com/creack/pty
[tview]: https://github.com/rivo/tview

## What it does

| Action | What runs on the device |
|--------|--------------------------|
| Apply ACL | `interface <intf>` → `ip access-group <acl> <in\|out>` |
| Remove ACL | `interface <intf>` → `no ip access-group <acl> <in\|out>` |
| Save | `write memory` (copy running-config → startup-config) |

The **Apply** and **Remove** screens pull the live interface list from
`show ip interface brief`, and **Remove** reads the interface's current ACL
bindings (`show running-config interface <intf>`) so you pick from what's
actually attached instead of guessing names.

Move around with the **arrow keys** or **Tab**; every action prints the exact
IOS commands and the device's reply in the side log.

## Install & run

Requires Go 1.24+ and the `ssh` client on your `PATH`.

```sh
git clone https://github.com/baldwinsung/SimpleCiscoTUI.git
cd SimpleCiscoTUI
scripts/run.sh
```

`scripts/run.sh` is just `go run .`. Or build a binary:

```sh
go build -o simpleciscotui .
./simpleciscotui
```

## Config file (recommended)

Save your devices once in a TOML file and skip the form. The smallest possible
config is just a host:

```toml
# config.toml
[[devices]]
host = "172.16.0.1"
```

With no password set, the app authenticates **exactly like `ssh 172.16.0.1`** —
your SSH agent and the keys in `~/.ssh` — and the username defaults to your
local login. If there's a single device, **the app connects to it on launch**;
with several, you get a picker on the connect screen.

The app looks for, in order:

1. `$SIMPLECISCOTUI_CONFIG`
2. `./config.toml` (next to where you run it)
3. `~/.config/simpleciscotui/config.toml`

See [`config.example.toml`](config.example.toml) for every option (`name`,
`username`, `password`, `secret`, `port`, `key_file`, `legacy_ssh`, and a
`[defaults]` table applied to all devices). `config.toml` is git-ignored so your
device list never lands in the repo.

`key_file` and `legacy_ssh` map straight onto `ssh` flags (`-i`, and the `-o`
crypto options old IOS needs), so a device works even **without** a matching
`~/.ssh/config` block:

```toml
[[devices]]
host = "172.16.0.1"
key_file = "~/path/to/id_rsa"   # ssh -i  (needed if the key isn't ~/.ssh/id_rsa)
legacy_ssh = true               # add ssh-rsa / dh-group14-sha1 / aes-cbc for old IOS
```

## Connecting without a config file

You can also just fill in the connect form, or pre-seed it from the environment
(handy with a local, git-ignored `.env` that `scripts/run.sh` auto-sources):

```sh
# .env  — never commit this
export CISCO_HOST=172.16.0.1
export CISCO_USERNAME=admin
export CISCO_PASSWORD=...     # leave unset to use SSH key / agent auth
export CISCO_SECRET=...       # enable secret, sent after login if set
export CISCO_PORT=22
```

Credentials are only ever held in memory for the session — the app writes
nothing to disk.

> **Password auth:** because the app drives the real `ssh` client with
> `BatchMode` on (so the TUI never blocks on a hidden prompt), a configured
> password is only used if [`sshpass`](https://linux.die.net/man/1/sshpass) is
> installed. **Key / agent auth is the supported path.** An `enable` secret, if
> set, is sent after login.

### Legacy switches (Catalyst 2960G and friends)

Older IOS boxes need dated SSH crypto (`diffie-hellman-group14-sha1`, `ssh-rsa`
host/pubkey algorithms, `aes-cbc`/`3des-cbc`). Since the app uses your system
`ssh`, anything already working in `~/.ssh/config` just works. If you'd rather
keep it in the app, set `legacy_ssh = true` on the device and it passes the
needed `ssh -o` flags for you.

## Develop

```sh
scripts/test.sh                              # go test ./... — parser/config unit tests
go vet ./...
```

The pure parsing/command helpers in `internal/cisco/cisco.go`
(`ParseInterfaceBrief`, `ParseInterfaceAcls`, `BuildApplyCommands`, …) carry
no network I/O and are fully covered by `internal/cisco/cisco_test.go`, so the
command-generation logic is testable without a live device.

## Project layout

```
main.go                        Entry point
internal/
  cisco/
    cisco.go     Pure parsing/command helpers + Credentials
    session.go   System-ssh/PTY session (connect, run commands)
    expect.go    pexpect-style buffered regex matching over the PTY
  config/
    config.go    TOML device config loader (pure parsing)
  tui/
    app.go       tview Application, screen stack, global key handling
    connect.go   Connect screen
    menu.go      Main menu
    apply.go     Apply ACL screen
    remove.go    Remove ACL screen
    save.go      Save (copy run → startup) screen
    statuslog.go Shared color-coded output pane
    layout.go    Centered fixed-size box helper
config.example.toml   Documented sample config (copy to config.toml)
scripts/
  run.sh           go run .
  test.sh          go test ./...
```

## Safety notes

- **Removing an inbound ACL on an SVI drops the firewall for that VLAN.** The
  app does exactly what you tell it and shows the resulting device output — it
  does not second-guess your rules.
- Changes are live immediately but **not persisted** until you run **Save**
  (`write memory`). Conversely, a bad change you *haven't* saved can be rolled
  back with a device reload.

## License

MIT — see [LICENSE](LICENSE). Designed and built by
**[Claude Code](https://claude.com/claude-code) (Opus)**.
