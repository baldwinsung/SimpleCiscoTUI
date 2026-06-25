"""SimpleCiscoTUI — a small Textual app to manage interface ACLs on a Cisco device.

Three operations:
  1. Apply an ACL to an interface
  2. Remove an ACL from an interface
  3. Copy running-config to startup-config (save)

All network calls run in Textual thread workers so the UI never blocks.
"""

from __future__ import annotations

import os

from textual import on, work
from textual.app import App, ComposeResult
from textual.containers import Center, Horizontal, Middle, Vertical
from textual.screen import ModalScreen, Screen
from textual.widgets import (
    Button,
    Footer,
    Header,
    Input,
    Label,
    RichLog,
    Select,
    Static,
)

from .cisco import CiscoCredentials, CiscoSession


class StatusLog(RichLog):
    """Shared output pane. Color-coded helpers for ok/err/info lines."""

    def ok(self, msg: str) -> None:
        self.write(f"[green]✓[/] {msg}")

    def err(self, msg: str) -> None:
        self.write(f"[red]✗[/] {msg}")

    def info(self, msg: str) -> None:
        self.write(f"[dim]·[/] {msg}")

    def device(self, msg: str) -> None:
        text = msg.strip()
        if text:
            self.write(f"[cyan]{text}[/]")


class ConnectScreen(Screen):
    """Collect credentials and open a session."""

    TITLE = "Connect"

    def compose(self) -> ComposeResult:
        yield Header()
        with Middle():
            with Center():
                with Vertical(id="connect-box"):
                    yield Label("Connect to a Cisco device", id="connect-title")
                    yield Input(
                        placeholder="Host / IP",
                        id="host",
                        value=os.environ.get("CISCO_HOST", ""),
                    )
                    yield Input(
                        placeholder="Username",
                        id="username",
                        value=os.environ.get("CISCO_USERNAME", ""),
                    )
                    yield Input(
                        placeholder="Password",
                        password=True,
                        id="password",
                        value=os.environ.get("CISCO_PASSWORD", ""),
                    )
                    yield Input(
                        placeholder="Enable secret (optional)",
                        password=True,
                        id="secret",
                        value=os.environ.get("CISCO_SECRET", ""),
                    )
                    yield Input(
                        placeholder="Port",
                        id="port",
                        value=os.environ.get("CISCO_PORT", "22"),
                    )
                    yield Button("Connect", variant="primary", id="connect")
                    yield Label("", id="connect-error")
        yield Footer()

    @on(Input.Submitted)
    @on(Button.Pressed, "#connect")
    def do_connect(self) -> None:
        host = self.query_one("#host", Input).value.strip()
        username = self.query_one("#username", Input).value.strip()
        password = self.query_one("#password", Input).value
        secret = self.query_one("#secret", Input).value
        port_raw = self.query_one("#port", Input).value.strip() or "22"
        error = self.query_one("#connect-error", Label)

        if not host or not username:
            error.update("[red]Host and username are required.[/]")
            return
        try:
            port = int(port_raw)
        except ValueError:
            error.update("[red]Port must be a number.[/]")
            return

        error.update("[yellow]Connecting…[/]")
        self.query_one("#connect", Button).disabled = True
        creds = CiscoCredentials(
            host=host,
            username=username,
            password=password,
            secret=secret,
            port=port,
        )
        self._connect_worker(creds)

    @work(thread=True, exclusive=True)
    def _connect_worker(self, creds: CiscoCredentials) -> None:
        session = CiscoSession(creds)
        try:
            session.connect()
        except Exception as exc:  # noqa: BLE001 - surface any connect failure
            self.app.call_from_thread(self._connect_failed, str(exc))
            return
        self.app.call_from_thread(self._connect_ok, session)

    def _connect_ok(self, session: CiscoSession) -> None:
        self.app.session = session  # type: ignore[attr-defined]
        self.app.push_screen(MenuScreen())

    def _connect_failed(self, message: str) -> None:
        self.query_one("#connect", Button).disabled = False
        self.query_one("#connect-error", Label).update(f"[red]{message}[/]")


class MenuScreen(Screen):
    """Main menu: pick one of the three operations."""

    TITLE = "SimpleCiscoTUI"
    BINDINGS = [
        ("a", "apply", "Apply ACL"),
        ("r", "remove", "Remove ACL"),
        ("s", "save", "Save config"),
        ("d", "disconnect", "Disconnect"),
    ]

    def compose(self) -> ComposeResult:
        yield Header()
        with Horizontal(id="menu-layout"):
            with Vertical(id="menu-actions"):
                yield Static(self._target(), id="target")
                yield Button("Apply ACL to interface", id="apply", variant="success")
                yield Button("Remove ACL from interface", id="remove", variant="warning")
                yield Button("Copy run → startup (save)", id="save", variant="primary")
                yield Button("Disconnect", id="disconnect", variant="error")
            yield StatusLog(id="log", wrap=True, markup=True)
        yield Footer()

    def on_mount(self) -> None:
        self.query_one("#log", StatusLog).ok(f"Connected to {self._host()}")

    def _host(self) -> str:
        return self.app.session.credentials.host  # type: ignore[attr-defined]

    def _target(self) -> str:
        return f"[b]Device:[/] {self._host()}"

    # actions / buttons --------------------------------------------------- #

    def action_apply(self) -> None:
        self.app.push_screen(ApplyAclScreen())

    def action_remove(self) -> None:
        self.app.push_screen(RemoveAclScreen())

    def action_save(self) -> None:
        self.app.push_screen(SaveScreen())

    def action_disconnect(self) -> None:
        self._disconnect_worker()

    @on(Button.Pressed, "#apply")
    def _btn_apply(self) -> None:
        self.action_apply()

    @on(Button.Pressed, "#remove")
    def _btn_remove(self) -> None:
        self.action_remove()

    @on(Button.Pressed, "#save")
    def _btn_save(self) -> None:
        self.action_save()

    @on(Button.Pressed, "#disconnect")
    def _btn_disconnect(self) -> None:
        self.action_disconnect()

    @work(thread=True, exclusive=True)
    def _disconnect_worker(self) -> None:
        session: CiscoSession = self.app.session  # type: ignore[attr-defined]
        session.disconnect()
        self.app.call_from_thread(self.app.switch_screen, ConnectScreen())


class _OperationScreen(Screen):
    """Shared layout for the apply/remove screens: an interface picker + log."""

    DIRECTIONS = [("inbound (in)", "in"), ("outbound (out)", "out")]

    def session(self) -> CiscoSession:
        return self.app.session  # type: ignore[attr-defined]

    def status_log(self) -> StatusLog:
        return self.query_one("#log", StatusLog)

    def on_mount(self) -> None:
        self._load_interfaces()

    @work(thread=True, exclusive=True)
    def _load_interfaces(self) -> None:
        try:
            interfaces = self.session().list_interfaces()
        except Exception as exc:  # noqa: BLE001
            self.app.call_from_thread(self.status_log().err, f"Failed to list interfaces: {exc}")
            return
        options = [(f"{i.name}  ({i.ip}, {i.status}/{i.protocol})", i.name) for i in interfaces]
        self.app.call_from_thread(self._populate_interfaces, options)

    def _populate_interfaces(self, options: list[tuple[str, str]]) -> None:
        select = self.query_one("#interface", Select)
        select.set_options(options)
        self.status_log().info(f"Loaded {len(options)} interface(s).")


class ApplyAclScreen(_OperationScreen):
    TITLE = "Apply ACL"
    BINDINGS = [("escape", "app.pop_screen", "Back")]

    def compose(self) -> ComposeResult:
        yield Header()
        with Horizontal(id="op-layout"):
            with Vertical(id="op-form"):
                yield Label("Apply ACL to interface", classes="op-title")
                yield Label("Interface")
                yield Select([], id="interface", prompt="Select interface")
                yield Label("ACL name")
                yield Input(placeholder="e.g. ZONE-APP-IN", id="acl")
                yield Label("Direction")
                yield Select(self.DIRECTIONS, id="direction", value="in", allow_blank=False)
                with Horizontal(classes="op-buttons"):
                    yield Button("Apply", id="apply", variant="success")
                    yield Button("Back", id="back", variant="default")
            yield StatusLog(id="log", wrap=True, markup=True)
        yield Footer()

    @on(Button.Pressed, "#back")
    def _back(self) -> None:
        self.app.pop_screen()

    @on(Button.Pressed, "#apply")
    def _apply(self) -> None:
        interface = self.query_one("#interface", Select).value
        acl = self.query_one("#acl", Input).value.strip()
        direction = self.query_one("#direction", Select).value
        if interface == Select.BLANK or not acl:
            self.status_log().err("Pick an interface and enter an ACL name.")
            return
        self.query_one("#apply", Button).disabled = True
        self.status_log().info(f"Applying {acl} {direction} on {interface}…")
        self._apply_worker(str(interface), acl, str(direction))

    @work(thread=True, exclusive=True)
    def _apply_worker(self, interface: str, acl: str, direction: str) -> None:
        try:
            out = self.session().apply_acl(interface, acl, direction)
        except Exception as exc:  # noqa: BLE001
            self.app.call_from_thread(self._done, False, f"Apply failed: {exc}", out=None)
            return
        self.app.call_from_thread(
            self._done, True, f"Applied {acl} {direction} on {interface}.", out=out
        )

    def _done(self, ok: bool, message: str, out: str | None) -> None:
        self.query_one("#apply", Button).disabled = False
        log = self.status_log()
        if out:
            log.device(out)
        (log.ok if ok else log.err)(message)


class RemoveAclScreen(_OperationScreen):
    TITLE = "Remove ACL"
    BINDINGS = [("escape", "app.pop_screen", "Back")]

    def compose(self) -> ComposeResult:
        yield Header()
        with Horizontal(id="op-layout"):
            with Vertical(id="op-form"):
                yield Label("Remove ACL from interface", classes="op-title")
                yield Label("Interface")
                yield Select([], id="interface", prompt="Select interface")
                yield Label("Current bindings")
                yield Select([], id="binding", prompt="(pick an interface first)")
                with Horizontal(classes="op-buttons"):
                    yield Button("Remove", id="remove", variant="warning")
                    yield Button("Back", id="back", variant="default")
            yield StatusLog(id="log", wrap=True, markup=True)
        yield Footer()

    @on(Select.Changed, "#interface")
    def _interface_changed(self, event: Select.Changed) -> None:
        if event.value == Select.BLANK:
            return
        self._load_bindings(str(event.value))

    @work(thread=True, exclusive=True)
    def _load_bindings(self, interface: str) -> None:
        try:
            acls = self.session().interface_acls(interface)
        except Exception as exc:  # noqa: BLE001
            self.app.call_from_thread(self.status_log().err, f"Failed to read bindings: {exc}")
            return
        # value encodes "<direction>|<acl>" so the remove worker has both.
        options = [
            (f"{acl}  ({direction})", f"{direction}|{acl}")
            for direction, acl in acls.bindings()
        ]
        self.app.call_from_thread(self._populate_bindings, interface, options)

    def _populate_bindings(self, interface: str, options: list[tuple[str, str]]) -> None:
        select = self.query_one("#binding", Select)
        select.set_options(options)
        if options:
            self.status_log().info(f"{interface}: {len(options)} ACL binding(s).")
        else:
            self.status_log().info(f"{interface}: no ACLs bound.")

    @on(Button.Pressed, "#back")
    def _back(self) -> None:
        self.app.pop_screen()

    @on(Button.Pressed, "#remove")
    def _remove(self) -> None:
        interface = self.query_one("#interface", Select).value
        binding = self.query_one("#binding", Select).value
        if interface == Select.BLANK or binding == Select.BLANK:
            self.status_log().err("Pick an interface and a binding to remove.")
            return
        direction, acl = str(binding).split("|", 1)
        self.query_one("#remove", Button).disabled = True
        self.status_log().info(f"Removing {acl} {direction} from {interface}…")
        self._remove_worker(str(interface), acl, direction)

    @work(thread=True, exclusive=True)
    def _remove_worker(self, interface: str, acl: str, direction: str) -> None:
        try:
            out = self.session().remove_acl(interface, acl, direction)
        except Exception as exc:  # noqa: BLE001
            self.app.call_from_thread(self._done, False, f"Remove failed: {exc}", None, interface)
            return
        self.app.call_from_thread(
            self._done, True, f"Removed {acl} {direction} from {interface}.", out, interface
        )

    def _done(self, ok: bool, message: str, out: str | None, interface: str) -> None:
        self.query_one("#remove", Button).disabled = False
        log = self.status_log()
        if out:
            log.device(out)
        (log.ok if ok else log.err)(message)
        if ok:
            self._load_bindings(interface)  # refresh the list


class SaveScreen(ModalScreen):
    """Confirm + run copy running-config startup-config."""

    TITLE = "Save config"
    BINDINGS = [("escape", "app.pop_screen", "Back")]

    def compose(self) -> ComposeResult:
        with Center():
            with Middle():
                with Vertical(id="save-box"):
                    yield Label("Copy running-config → startup-config", id="save-title")
                    yield Static(
                        "This writes the current running configuration to NVRAM "
                        "so it survives a reload. Continue?",
                        id="save-body",
                    )
                    yield StatusLog(id="log", wrap=True, markup=True)
                    with Horizontal(id="save-buttons"):
                        yield Button("Save", id="save", variant="primary")
                        yield Button("Cancel", id="cancel", variant="default")

    def session(self) -> CiscoSession:
        return self.app.session  # type: ignore[attr-defined]

    @on(Button.Pressed, "#cancel")
    def _cancel(self) -> None:
        self.app.pop_screen()

    @on(Button.Pressed, "#save")
    def _save(self) -> None:
        self.query_one("#save", Button).disabled = True
        self.query_one("#log", StatusLog).info("Saving…")
        self._save_worker()

    @work(thread=True, exclusive=True)
    def _save_worker(self) -> None:
        try:
            out = self.session().save_config()
        except Exception as exc:  # noqa: BLE001
            self.app.call_from_thread(self._done, False, f"Save failed: {exc}", None)
            return
        self.app.call_from_thread(self._done, True, "Saved to startup-config.", out)

    def _done(self, ok: bool, message: str, out: str | None) -> None:
        log = self.query_one("#log", StatusLog)
        if out:
            log.device(out)
        (log.ok if ok else log.err)(message)
        self.query_one("#save", Button).disabled = False


class SimpleCiscoTUI(App):
    """The application."""

    CSS_PATH = "app.tcss"
    TITLE = "SimpleCiscoTUI"
    BINDINGS = [("ctrl+q", "quit", "Quit")]

    #: set after a successful connection
    session: CiscoSession | None = None

    def on_mount(self) -> None:
        self.push_screen(ConnectScreen())

    def on_unmount(self) -> None:
        if self.session is not None:
            self.session.disconnect()


def main() -> None:
    SimpleCiscoTUI().run()


if __name__ == "__main__":
    main()
