"""Unit tests for config parsing (no file I/O, no device)."""

import pytest

from simpleciscotui.config import ConfigError, parse_config


def test_minimal_host_only():
    devices = parse_config('[[devices]]\nhost = "192.168.1.2"\n')
    assert len(devices) == 1
    d = devices[0]
    assert d.host == "192.168.1.2"
    assert d.name == "192.168.1.2"  # name defaults to host
    assert d.port == 22
    assert d.label == "192.168.1.2"


def test_defaults_are_merged_and_overridable():
    text = """
    [defaults]
    username = "admin"
    port = 2222

    [[devices]]
    host = "192.168.1.2"
    name = "SW1"

    [[devices]]
    host = "192.168.1.3"
    port = 22
    """
    devices = parse_config(text)
    assert devices[0].username == "admin" and devices[0].port == 2222
    assert devices[0].name == "SW1"
    assert devices[0].label == "SW1  (192.168.1.2)"
    # per-device value overrides the default
    assert devices[1].port == 22


def test_missing_host_raises():
    with pytest.raises(ConfigError, match="missing a `host`"):
        parse_config('[[devices]]\nname = "SW1"\n')


def test_unknown_key_raises():
    with pytest.raises(ConfigError, match="unknown key"):
        parse_config('[[devices]]\nhost = "x"\nuser = "oops"\n')


def test_no_password_yields_key_auth_credentials():
    devices = parse_config('[[devices]]\nhost = "192.168.1.2"\nusername = "admin"\n')
    creds = devices[0].to_credentials()
    assert creds.uses_password is False
    kwargs = creds.netmiko_kwargs()
    assert kwargs["use_keys"] is True
    assert kwargs["allow_agent"] is True
    assert kwargs["password"] == ""


def test_password_yields_password_auth():
    devices = parse_config('[[devices]]\nhost = "x"\nusername = "a"\npassword = "p"\n')
    kwargs = devices[0].to_credentials().netmiko_kwargs()
    assert kwargs["password"] == "p"
    assert "use_keys" not in kwargs


def test_username_defaults_to_local_user(monkeypatch):
    monkeypatch.setattr("getpass.getuser", lambda: "someuser")
    creds = parse_config('[[devices]]\nhost = "x"\n')[0].to_credentials()
    assert creds.username == "someuser"
