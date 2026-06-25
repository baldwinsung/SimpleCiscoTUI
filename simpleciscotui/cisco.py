"""Cisco IOS device access for SimpleCiscoTUI.

A thin wrapper around Netmiko. The pure parsing helpers
(:func:`parse_interface_brief`, :func:`parse_interface_acls`) are kept free of
any network I/O so they can be unit-tested against captured ``show`` output
without a live device. Everything that touches the wire lives on
:class:`CiscoSession`.
"""

from __future__ import annotations

import os
import re
from dataclasses import dataclass, field
from typing import Optional

# Netmiko is only needed at runtime; importing it lazily keeps the parsing
# helpers (and their tests) usable without the dependency installed.
try:  # pragma: no cover - exercised indirectly
    from netmiko import ConnectHandler
except Exception:  # pragma: no cover
    ConnectHandler = None  # type: ignore[assignment]


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
    """Connection parameters for a single device.

    Leave ``password`` blank to authenticate the same way ``ssh <host>`` does —
    via the SSH agent and the keys in ``~/.ssh`` (plus ``key_file`` if set).
    """

    host: str
    username: str
    password: str = ""
    secret: str = ""  # enable secret; falls back to password if blank
    port: int = 22
    device_type: str = "cisco_ios"
    key_file: Optional[str] = None  # explicit private key path (optional)

    @property
    def uses_password(self) -> bool:
        return bool(self.password)

    def netmiko_kwargs(self) -> dict:
        kwargs: dict = {
            "device_type": self.device_type,
            "host": self.host,
            "username": self.username,
            "port": self.port,
            "secret": self.secret or self.password,
            "fast_cli": False,
        }
        if self.password:
            kwargs["password"] = self.password
        else:
            # No password: fall back to SSH key / agent auth, exactly like
            # running `ssh <host>` would.
            kwargs["password"] = ""
            kwargs["use_keys"] = True
            kwargs["allow_agent"] = True
        if self.key_file:
            kwargs["key_file"] = os.path.expanduser(self.key_file)
            kwargs["use_keys"] = True
        return kwargs


# --------------------------------------------------------------------------- #
# Pure parsing helpers (no network I/O)
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
            continue
        name, ip = parts[0], parts[1]
        # Columns: Interface IP-Address OK? Method Status Protocol
        status, protocol = parts[-2], parts[-1]
        if not re.match(r"^[A-Za-z]", name):
            continue
        interfaces.append(Interface(name=name, ip=ip, status=status, protocol=protocol))
    return interfaces


def parse_interface_acls(text: str) -> InterfaceAcls:
    """Parse ``ip access-group <name> <in|out>`` lines from interface config."""
    acls = InterfaceAcls()
    for match in re.finditer(
        r"ip access-group\s+(\S+)\s+(in|out)", text, re.IGNORECASE
    ):
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


# --------------------------------------------------------------------------- #
# Live session
# --------------------------------------------------------------------------- #


@dataclass
class CiscoSession:
    """A connected Netmiko session plus the high-level operations the TUI needs."""

    credentials: CiscoCredentials
    _conn: object = field(default=None, repr=False)

    @property
    def connected(self) -> bool:
        return self._conn is not None

    def connect(self) -> None:
        if ConnectHandler is None:  # pragma: no cover
            raise RuntimeError(
                "netmiko is not installed; run `pip install -r requirements.txt`"
            )
        self._conn = ConnectHandler(**self.credentials.netmiko_kwargs())
        # Enter privileged EXEC if the login didn't already land there.
        if not self._conn.check_enable_mode():
            self._conn.enable()

    def disconnect(self) -> None:
        if self._conn is not None:
            try:  # pragma: no cover - best effort
                self._conn.disconnect()
            finally:
                self._conn = None

    # -- read ------------------------------------------------------------- #

    def list_interfaces(self) -> list[Interface]:
        out = self._send("show ip interface brief")
        return parse_interface_brief(out)

    def interface_acls(self, interface: str) -> InterfaceAcls:
        out = self._send(f"show running-config interface {interface}")
        return parse_interface_acls(out)

    # -- write ------------------------------------------------------------ #

    def apply_acl(self, interface: str, acl: str, direction: str) -> str:
        cmds = build_apply_commands(interface, acl, direction)
        return self._config(cmds)

    def remove_acl(self, interface: str, acl: str, direction: str) -> str:
        cmds = build_remove_commands(interface, acl, direction)
        return self._config(cmds)

    def save_config(self) -> str:
        """``copy running-config startup-config`` (Netmiko's ``save_config``)."""
        self._require()
        return self._conn.save_config()  # type: ignore[union-attr]

    # -- internals -------------------------------------------------------- #

    def _require(self) -> None:
        if self._conn is None:
            raise RuntimeError("not connected")

    def _send(self, command: str) -> str:
        self._require()
        return self._conn.send_command(command)  # type: ignore[union-attr]

    def _config(self, commands: list[str]) -> str:
        self._require()
        return self._conn.send_config_set(commands)  # type: ignore[union-attr]
