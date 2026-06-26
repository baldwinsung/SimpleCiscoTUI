"""Cisco IOS device access for SimpleCiscoTUI.

Connections are driven through the **system ``ssh`` binary**, not a Python SSH
library. That means the app inherits everything in your ``~/.ssh/config`` (keys,
host aliases, and the legacy crypto old IOS needs) and works wherever plain
``ssh <host>`` already works — which sidesteps the keyboard-interactive and
legacy-algorithm problems Paramiko hits on older switches.

The pure parsing/command helpers (:func:`parse_interface_brief`,
:func:`parse_interface_acls`, :func:`build_apply_commands`, …) carry no I/O and
are unit-tested against captured ``show`` output. Everything that touches the
wire lives on :class:`CiscoSession`.
"""

from __future__ import annotations

import os
import re
import shutil
from dataclasses import dataclass, field
from typing import Optional

try:  # pragma: no cover - import guard
    import pexpect
except Exception:  # pragma: no cover
    pexpect = None  # type: ignore[assignment]

DIRECTIONS = ("in", "out")


@dataclass
class Interface:
    """One row of ``show ip interface brief``."""

    name: str
    ip: str
    status: str
    protocol: str

    @property
    def is_up(self) -> bool:
        return self.status.lower() == "up" and self.protocol.lower() == "up"


@dataclass
class InterfaceAcls:
    """ACLs bound to an interface, keyed by direction."""

    inbound: Optional[str] = None
    outbound: Optional[str] = None

    def get(self, direction: str) -> Optional[str]:
        return self.inbound if direction == "in" else self.outbound

    def bindings(self) -> list[tuple[str, str]]:
        """Return ``(direction, acl_name)`` pairs that are actually set."""
        out: list[tuple[str, str]] = []
        if self.inbound:
            out.append(("in", self.inbound))
        if self.outbound:
            out.append(("out", self.outbound))
        return out


@dataclass
class CiscoCredentials:
    """How to reach one device via the system ``ssh`` client.

    With no ``password`` the app uses key / agent auth and never blocks on a
    prompt (``BatchMode=yes``) — exactly like ``ssh <host>``. ``key_file`` and
    ``legacy_ssh`` map onto ``ssh -i`` / ``ssh -o`` flags, so a device works
    even without a matching ``~/.ssh/config`` block.
    """

    host: str
    username: str = ""
    password: str = ""  # only usable if `sshpass` is installed; key auth preferred
    secret: str = ""  # enable secret; sent after login when set
    port: int = 22
    key_file: Optional[str] = None  # explicit private key path
    legacy_ssh: bool = False  # add the crypto old IOS (e.g. 2960G) requires

    @property
    def uses_password(self) -> bool:
        return bool(self.password)

    def target(self) -> str:
        return f"{self.username}@{self.host}" if self.username else self.host

    def ssh_options(self, conn_timeout: int = 25) -> list[str]:
        """The ``ssh`` flags (without the target host) for this device."""
        opts = ["-o", f"ConnectTimeout={conn_timeout}"]
        if not self.password:
            # Key / agent auth only — never hang waiting for a password prompt
            # the TUI can't display.
            opts += ["-o", "BatchMode=yes"]
        if self.port and self.port != 22:
            opts += ["-p", str(self.port)]
        if self.key_file:
            opts += ["-o", "IdentitiesOnly=yes", "-i", os.path.expanduser(self.key_file)]
        if self.legacy_ssh:
            opts += [
                "-o",
                "KexAlgorithms=+diffie-hellman-group14-sha1,diffie-hellman-group-exchange-sha1",
                "-o", "HostKeyAlgorithms=+ssh-rsa",
                "-o", "PubkeyAcceptedAlgorithms=+ssh-rsa",
                "-o", "Ciphers=+aes128-cbc,aes192-cbc,aes256-cbc,3des-cbc",
            ]
        return opts


# --------------------------------------------------------------------------- #
# Pure parsing / command helpers (no I/O)
# --------------------------------------------------------------------------- #

_BRIEF_HEADER = re.compile(r"^\s*Interface\s+IP-Address", re.IGNORECASE)


def parse_interface_brief(text: str) -> list[Interface]:
    """Parse ``show ip interface brief`` into :class:`Interface` rows."""
    interfaces: list[Interface] = []
    for line in text.splitlines():
        if not line.strip() or _BRIEF_HEADER.match(line):
            continue
        parts = line.split()
        if len(parts) < 6:
            # Skips blank lines, echoed commands, and the device prompt.
            continue
        name, ip = parts[0], parts[1]
        status, protocol = parts[-2], parts[-1]
        if not re.match(r"^[A-Za-z]", name):
            continue
        interfaces.append(Interface(name=name, ip=ip, status=status, protocol=protocol))
    return interfaces


def parse_interface_acls(text: str) -> InterfaceAcls:
    """Parse ``ip access-group <name> <in|out>`` lines from interface config."""
    acls = InterfaceAcls()
    for match in re.finditer(r"ip access-group\s+(\S+)\s+(in|out)", text, re.IGNORECASE):
        name, direction = match.group(1), match.group(2).lower()
        if direction == "in":
            acls.inbound = name
        else:
            acls.outbound = name
    return acls


def normalize_direction(direction: str) -> str:
    direction = direction.strip().lower()
    if direction not in DIRECTIONS:
        raise ValueError(f"direction must be one of {DIRECTIONS}, got {direction!r}")
    return direction


def build_apply_commands(interface: str, acl: str, direction: str) -> list[str]:
    """Config-mode commands to bind ``acl`` to ``interface``."""
    direction = normalize_direction(direction)
    return [f"interface {interface}", f"ip access-group {acl} {direction}"]


def build_remove_commands(interface: str, acl: str, direction: str) -> list[str]:
    """Config-mode commands to unbind ``acl`` from ``interface``."""
    direction = normalize_direction(direction)
    return [f"interface {interface}", f"no ip access-group {acl} {direction}"]


#: Matches an IOS exec/config prompt at the end of the buffer, e.g.
#: ``c2960r1#``, ``c2960r1>``, ``c2960r1(config)#``, ``c2960r1(config-if)#``.
PROMPT = r"[A-Za-z0-9._\-]+(?:\([^)]+\))?[>#] ?$"


def _clean_output(text: str) -> str:
    """Normalise pexpect output: strip CRs and the leading echoed command."""
    return text.replace("\r", "")


# --------------------------------------------------------------------------- #
# Live session (drives the system ssh client through a PTY via pexpect)
# --------------------------------------------------------------------------- #


@dataclass
class CiscoSession:
    """An interactive IOS session over the system ``ssh`` client.

    ``ssh`` is spawned once under a pseudo-terminal (via ``pexpect``) and kept
    open; commands are sent one at a time and read back up to the device prompt.
    This is what lets old IOS — which dribbles its interactive parser one line
    at a time — work reliably, unlike piping a whole script at once.
    """

    credentials: CiscoCredentials
    conn_timeout: int = 25
    command_timeout: int = 60
    _child: object = field(default=None, repr=False)
    _connected: bool = field(default=False, repr=False)

    @property
    def connected(self) -> bool:
        return self._connected

    def _ssh_argv(self) -> list[str]:
        creds = self.credentials
        argv = ["ssh", *creds.ssh_options(self.conn_timeout), creds.target()]
        if creds.password and shutil.which("sshpass"):
            argv = ["sshpass", "-p", creds.password, *argv]
        return argv

    # -- lifecycle -------------------------------------------------------- #

    def connect(self) -> None:
        if pexpect is None:  # pragma: no cover
            raise RuntimeError("pexpect is not installed; run `pip install -r requirements.txt`")
        if shutil.which("ssh") is None:  # pragma: no cover
            raise RuntimeError("the `ssh` client was not found on PATH")
        key_file = self.credentials.key_file
        if key_file and not os.path.isfile(os.path.expanduser(key_file)):
            raise FileNotFoundError(f"SSH key not found: {key_file}")

        argv = self._ssh_argv()
        child = pexpect.spawn(argv[0], argv[1:], timeout=self.conn_timeout, encoding="utf-8")
        try:
            child.expect(PROMPT)
        except Exception as exc:  # pexpect EOF/TIMEOUT → auth or connect failure
            detail = _clean_output((child.before or "")).strip().splitlines()
            child.close(force=True)
            msg = detail[-1] if detail else "ssh connection failed"
            raise RuntimeError(msg) from exc
        self._child = child

        # Privileged mode + disable paging.
        if self.credentials.secret and (child.after or "").rstrip().endswith(">"):
            child.sendline("enable")
            child.expect([r"[Pp]assword:", PROMPT])
            if "assword" in (child.after or ""):
                child.sendline(self.credentials.secret)
                child.expect(PROMPT)
        child.sendline("terminal length 0")
        child.expect(PROMPT)
        self._connected = True

    def disconnect(self) -> None:
        child = self._child
        if child is not None:
            try:  # best effort graceful logout
                child.sendline("exit")
                child.expect(pexpect.EOF, timeout=5)
            except Exception:  # noqa: BLE001
                pass
            finally:
                child.close(force=True)
                self._child = None
        self._connected = False

    # -- read ------------------------------------------------------------- #

    def list_interfaces(self) -> list[Interface]:
        return parse_interface_brief(self._run("show ip interface brief"))

    def interface_acls(self, interface: str) -> InterfaceAcls:
        return parse_interface_acls(self._run(f"show running-config interface {interface}"))

    # -- write ------------------------------------------------------------ #

    def apply_acl(self, interface: str, acl: str, direction: str) -> str:
        cmds = ["configure terminal", *build_apply_commands(interface, acl, direction), "end"]
        return self._run_many(cmds)

    def remove_acl(self, interface: str, acl: str, direction: str) -> str:
        cmds = ["configure terminal", *build_remove_commands(interface, acl, direction), "end"]
        return self._run_many(cmds)

    def save_config(self) -> str:
        """``write memory`` — copy running-config to startup-config (no prompt)."""
        return self._run("write memory")

    # -- internals -------------------------------------------------------- #

    def _run(self, command: str) -> str:
        """Send one command, return the device output (echo line stripped)."""
        if self._child is None:
            raise RuntimeError("not connected")
        child = self._child
        child.sendline(command)
        try:
            child.expect(PROMPT, timeout=self.command_timeout)
        except Exception as exc:  # noqa: BLE001
            raise RuntimeError(
                f"timed out waiting for prompt after {command!r}"
            ) from exc
        out = _clean_output(child.before or "")
        # Drop the first line (the echoed command itself).
        return out.split("\n", 1)[1] if "\n" in out else ""

    def _run_many(self, commands: list[str]) -> str:
        return "\n".join(self._run(cmd) for cmd in commands).strip()
