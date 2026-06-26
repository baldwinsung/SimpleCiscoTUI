"""Optional TOML config file for SimpleCiscoTUI.

Save devices so you don't retype connection details. The smallest valid file is
just a host:

    [[devices]]
    host = "192.168.1.2"

With no password set, the app authenticates the same way ``ssh 192.168.1.2``
does — your SSH agent and the keys in ``~/.ssh`` — and the username defaults to
your local login. One saved device → the app connects to it on launch.

Search order (first match wins):
    1. $SIMPLECISCOTUI_CONFIG
    2. ./config.toml            (current directory)
    3. ~/.config/simpleciscotui/config.toml

The parsing here is pure (no I/O beyond reading the file) so it is unit-tested.
"""

from __future__ import annotations

import getpass
import os
import tomllib
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

from .cisco import CiscoCredentials

CONFIG_ENV = "SIMPLECISCOTUI_CONFIG"
DEFAULT_PATHS = (
    Path("config.toml"),
    Path.home() / ".config" / "simpleciscotui" / "config.toml",
)

_ALLOWED_KEYS = {
    "host",
    "name",
    "username",
    "password",
    "secret",
    "port",
    "key_file",
    "legacy_ssh",
}


class ConfigError(Exception):
    """Raised for a malformed or unreadable config file."""


@dataclass
class DeviceConfig:
    host: str
    name: str = ""
    username: str = ""
    password: str = ""
    secret: str = ""
    port: int = 22
    key_file: Optional[str] = None
    legacy_ssh: bool = False

    def __post_init__(self) -> None:
        if not self.name:
            self.name = self.host

    @property
    def label(self) -> str:
        return f"{self.name}  ({self.host})" if self.name != self.host else self.host

    def to_credentials(self) -> CiscoCredentials:
        return CiscoCredentials(
            host=self.host,
            username=self.username or getpass.getuser(),
            password=self.password,
            secret=self.secret,
            port=self.port,
            key_file=self.key_file,
            legacy_ssh=self.legacy_ssh,
        )


@dataclass
class Config:
    devices: list[DeviceConfig]
    path: Optional[Path] = None


def parse_config(text: str) -> list[DeviceConfig]:
    """Parse TOML text into a list of :class:`DeviceConfig` (pure)."""
    data = tomllib.loads(text)
    defaults = data.get("defaults", {})
    if not isinstance(defaults, dict):
        raise ConfigError("`[defaults]` must be a table")
    raw_devices = data.get("devices", [])
    if not isinstance(raw_devices, list):
        raise ConfigError("`devices` must be a list of [[devices]] tables")

    devices: list[DeviceConfig] = []
    for i, raw in enumerate(raw_devices, start=1):
        if not isinstance(raw, dict):
            raise ConfigError(f"device #{i} must be a [[devices]] table")
        merged = {**defaults, **raw}
        if not merged.get("host"):
            raise ConfigError(f"device #{i} is missing a `host`")
        unknown = set(merged) - _ALLOWED_KEYS
        if unknown:
            keys = ", ".join(sorted(unknown))
            raise ConfigError(f"device #{i} ({merged['host']}) has unknown key(s): {keys}")
        devices.append(DeviceConfig(**merged))
    return devices


def find_config_path() -> Optional[Path]:
    """Return the first config file that exists, or ``None``."""
    env = os.environ.get(CONFIG_ENV)
    if env:
        candidate = Path(env).expanduser()
        return candidate if candidate.is_file() else None
    for candidate in DEFAULT_PATHS:
        if candidate.is_file():
            return candidate
    return None


def load_config(path: Optional[Path] = None) -> Config:
    """Load the config file (or return an empty config if there is none)."""
    path = path or find_config_path()
    if path is None:
        return Config(devices=[], path=None)
    try:
        text = Path(path).read_text()
    except OSError as exc:
        raise ConfigError(f"could not read {path}: {exc}") from exc
    try:
        devices = parse_config(text)
    except tomllib.TOMLDecodeError as exc:
        raise ConfigError(f"invalid TOML in {path}: {exc}") from exc
    return Config(devices=devices, path=Path(path))
