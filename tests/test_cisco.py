"""Unit tests for the pure parsing/command helpers in :mod:`simpleciscotui.cisco`.

These run without Netmiko or a live device.
"""

import pytest

from simpleciscotui.cisco import (
    build_apply_commands,
    build_remove_commands,
    normalize_direction,
    parse_interface_acls,
    parse_interface_brief,
)

BRIEF = """\
Interface              IP-Address      OK? Method Status                Protocol
Vlan1                  172.16.0.1      YES NVRAM  up                    up
Vlan10                 10.10.10.1      YES manual up                    up
Vlan20                 10.10.20.1      YES manual administratively down down
GigabitEthernet0/1     unassigned      YES unset  up                    up
GigabitEthernet0/2     unassigned      YES unset  down                  down
"""


def test_parse_interface_brief_counts_and_fields():
    rows = parse_interface_brief(BRIEF)
    assert len(rows) == 5
    vlan1 = rows[0]
    assert vlan1.name == "Vlan1"
    assert vlan1.ip == "172.16.0.1"
    assert vlan1.status == "up"
    assert vlan1.protocol == "up"
    assert vlan1.is_up is True


def test_parse_interface_brief_handles_admin_down():
    rows = parse_interface_brief(BRIEF)
    vlan20 = next(r for r in rows if r.name == "Vlan20")
    # "administratively down" collapses to status="down", protocol="down"
    assert vlan20.status == "down"
    assert vlan20.protocol == "down"
    assert vlan20.is_up is False


def test_parse_interface_brief_ignores_blank_and_header():
    assert parse_interface_brief("") == []
    header_only = "Interface              IP-Address      OK? Method Status   Protocol\n"
    assert parse_interface_brief(header_only) == []


INTERFACE_CFG = """\
Building configuration...

Current configuration : 142 bytes
!
interface Vlan10
 ip address 10.10.10.1 255.255.255.0
 ip access-group ZONE-APP-IN in
 ip access-group ZONE-APP-OUT out
end
"""


def test_parse_interface_acls_both_directions():
    acls = parse_interface_acls(INTERFACE_CFG)
    assert acls.inbound == "ZONE-APP-IN"
    assert acls.outbound == "ZONE-APP-OUT"
    assert acls.get("in") == "ZONE-APP-IN"
    assert acls.get("out") == "ZONE-APP-OUT"
    assert acls.bindings() == [("in", "ZONE-APP-IN"), ("out", "ZONE-APP-OUT")]


def test_parse_interface_acls_none():
    acls = parse_interface_acls("interface Vlan99\n ip address 10.10.99.1 255.255.255.0\nend")
    assert acls.inbound is None
    assert acls.outbound is None
    assert acls.bindings() == []


def test_build_apply_commands():
    assert build_apply_commands("Vlan10", "ZONE-APP-IN", "in") == [
        "interface Vlan10",
        "ip access-group ZONE-APP-IN in",
    ]


def test_build_remove_commands():
    assert build_remove_commands("Vlan10", "ZONE-APP-IN", "out") == [
        "interface Vlan10",
        "no ip access-group ZONE-APP-IN out",
    ]


def test_normalize_direction_accepts_case_and_whitespace():
    assert normalize_direction(" IN ") == "in"
    assert normalize_direction("Out") == "out"


def test_normalize_direction_rejects_garbage():
    with pytest.raises(ValueError):
        normalize_direction("sideways")
