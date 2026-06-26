#!/usr/bin/env python3
"""Generate the README screenshots (SVG) headlessly — no real device needed.

Run from the repo root:  .venv/bin/python scripts/screenshot.py
Writes docs/menu.svg and docs/apply.svg.
"""

from __future__ import annotations

import asyncio
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent))

from simpleciscotui.app import ApplyAclScreen, MenuScreen, SimpleCiscoTUI
from simpleciscotui.cisco import (
    CiscoCredentials,
    CiscoSession,
    Interface,
    InterfaceAcls,
)

DOCS = pathlib.Path(__file__).resolve().parent.parent / "docs"

_INTERFACES = [
    Interface("Vlan1", "172.16.0.1", "up", "up"),
    Interface("Vlan10", "10.10.10.1", "up", "up"),
    Interface("Vlan20", "10.10.20.1", "up", "up"),
    Interface("Vlan30", "10.10.30.1", "up", "up"),
    Interface("Vlan99", "10.10.99.1", "up", "up"),
    Interface("GigabitEthernet0/1", "unassigned", "up", "up"),
]


class DemoSession(CiscoSession):
    """A fake session so screenshots never touch a real switch."""

    def connect(self) -> None:
        self._connected = True

    def disconnect(self) -> None:
        self._connected = False

    def list_interfaces(self):
        return _INTERFACES

    def interface_acls(self, interface: str):
        return InterfaceAcls(inbound="ZONE-DEV-IN")


async def main() -> None:
    DOCS.mkdir(exist_ok=True)
    app = SimpleCiscoTUI()
    async with app.run_test(size=(92, 22)) as pilot:
        app.session = DemoSession(CiscoCredentials("172.16.0.1", "admin"))

        # --- main menu, with a populated status log ---
        await app.push_screen(MenuScreen())
        await pilot.pause()
        log = app.screen.query_one("#log")
        log.info(f"Loaded {len(_INTERFACES)} interface(s).")
        log.info("Applying ZONE-DEV-IN in on Vlan20…")
        log.ok("Applied ZONE-DEV-IN in on Vlan20.")
        app.screen.query_one("#apply").focus()
        await pilot.pause()
        (DOCS / "menu.svg").write_text(app.export_screenshot(title="SimpleCiscoTUI"))
        print("wrote docs/menu.svg")

        # --- apply-ACL screen, interface picker loaded ---
        await app.push_screen(ApplyAclScreen())
        for _ in range(10):
            await pilot.pause()
            if len(app.screen.query_one("#interface")._options) > 1:
                break
        app.screen.query_one("#interface").value = "Vlan20"
        app.screen.query_one("#acl").value = "ZONE-APP-IN"
        app.screen.query_one("#acl").focus()
        await pilot.pause()
        (DOCS / "apply.svg").write_text(app.export_screenshot(title="SimpleCiscoTUI — Apply ACL"))
        print("wrote docs/apply.svg")


if __name__ == "__main__":
    asyncio.run(main())
