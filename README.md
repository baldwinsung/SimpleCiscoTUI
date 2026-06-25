# SimpleCiscoTUI

A tiny terminal UI for the three Cisco IOS chores you reach for most when
shuffling interface ACLs:

1. **Apply** an ACL to an interface
2. **Remove** an ACL from an interface
3. **Copy running-config → startup-config** (save)

> Built with [Claude Code](https://claude.com/claude-code) (Opus).

```
┌ SimpleCiscoTUI ───────────────────────────────────────────────┐
│ Device: 192.168.1.2          ✓ Connected to 192.168.1.2        │
│                              · Loaded 6 interface(s).          │
│  Apply ACL to interface      ✓ Applied ZONE-APP-IN in on Vlan10│
│  Remove ACL from interface                                     │
│  Copy run → startup (save)                                     │
│  Disconnect                                                    │
└────────────────────────────────────────────────────────────────┘
```

It speaks plain SSH to the device through [Netmiko]'s `cisco_ios` driver
(enable mode, config mode, `copy run start`), and renders with [Textual]. Every
network call runs on a worker thread, so the UI never freezes mid-operation.

[Netmiko]: https://github.com/ktbyers/netmiko
[Textual]: https://github.com/Textualize/textual

## What it does

| Action | What runs on the device |
|--------|--------------------------|
| Apply ACL | `interface <intf>` → `ip access-group <acl> <in\|out>` |
| Remove ACL | `interface <intf>` → `no ip access-group <acl> <in\|out>` |
| Save | `copy running-config startup-config` (Netmiko `save_config`) |

The **Apply** and **Remove** screens pull the live interface list from
`show ip interface brief`, and **Remove** reads the interface's current ACL
bindings (`show running-config interface <intf>`) so you pick from what's
actually attached instead of guessing names.

## Install & run

Requires Python 3.10+.

```sh
git clone https://github.com/baldwinsung/SimpleCiscoTUI.git
cd SimpleCiscoTUI
scripts/run.sh
```

`scripts/run.sh` creates a local `.venv`, installs `textual` + `netmiko` on
first run, and launches the app. Or do it by hand:

```sh
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
python -m simpleciscotui          # or: pip install -e . && simpleciscotui
```

## Config file (recommended)

Save your devices once in a TOML file and skip the form. The smallest possible
config is just a host:

```toml
# config.toml
[[devices]]
host = "192.168.1.2"
```

With no password set, the app authenticates **exactly like `ssh 192.168.1.2`** —
your SSH agent and the keys in `~/.ssh` — and the username defaults to your
local login. If there's a single device, **the app connects to it on launch**;
with several, you get a picker on the connect screen.

The app looks for, in order:

1. `$SIMPLECISCOTUI_CONFIG`
2. `./config.toml` (next to where you run it)
3. `~/.config/simpleciscotui/config.toml`

See [`config.example.toml`](config.example.toml) for every option (`name`,
`username`, `password`, `secret`, `port`, `device_type`, `key_file`, and a
`[defaults]` table applied to all devices). `config.toml` is git-ignored so your
device list never lands in the repo.

## Connecting without a config file

You can also just fill in the connect form, or pre-seed it from the environment
(handy with a local, git-ignored `.env` that `scripts/run.sh` auto-sources):

```sh
# .env  — never commit this
export CISCO_HOST=192.168.1.2
export CISCO_USERNAME=admin
export CISCO_PASSWORD=...     # leave unset to use SSH key / agent auth
export CISCO_SECRET=...       # enable secret; defaults to the login password
export CISCO_PORT=22
```

Credentials are only ever held in memory for the session — the app writes
nothing to disk.

### Legacy switches (Catalyst 2960G and friends)

Older IOS boxes negotiate dated SSH crypto (`diffie-hellman-group14-sha1`,
`ssh-rsa` host keys). Modern Paramiko still offers these, so Netmiko generally
connects fine. If a device is *too* old and the handshake fails, add the legacy
algorithms to your `~/.ssh/config` and Netmiko will inherit them.

## Develop

```sh
scripts/test.sh        # run the parser unit tests
textual run --dev simpleciscotui.app:SimpleCiscoTUI   # with the Textual console
```

The pure parsing/command helpers in `simpleciscotui/cisco.py`
(`parse_interface_brief`, `parse_interface_acls`, `build_apply_commands`, …)
carry no network I/O and are fully covered by `tests/test_cisco.py`, so the
command-generation logic is testable without a live device.

## Project layout

```
simpleciscotui/
  cisco.py     Netmiko wrapper + pure parsing/command helpers
  config.py    TOML device config loader (pure parsing)
  app.py       Textual app: Connect → Menu → Apply / Remove / Save screens
  app.tcss     Styles
config.example.toml   Documented sample config (copy to config.toml)
tests/
  test_cisco.py    Parser + command-builder unit tests
  test_config.py   Config parsing + credential tests
scripts/
  run.sh       Create venv (if needed) and launch
  test.sh      Run the test suite
```

## Safety notes

- **Removing an inbound ACL on an SVI drops the firewall for that VLAN.** The
  app does exactly what you tell it and shows the resulting device output — it
  does not second-guess your rules.
- Changes are live immediately but **not persisted** until you run **Save**
  (`copy run start`). Conversely, a bad change you *haven't* saved can be
  rolled back with a device reload.

## License

MIT — see [LICENSE](LICENSE). Designed and built by
**[Claude Code](https://claude.com/claude-code) (Opus)**.
