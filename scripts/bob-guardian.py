#!/usr/bin/env python3
"""
Qubic Bob Guardian — a single-screen dashboard for a Network Guardian
Bob node operator.

It shows the things that actually decide your weekly reward:
  • Guardian score (uptime / sync / p2p / final), reward points, estimated reward,
    eligibility and flag status — pulled live from the public Guardians API.
  • Local sync state: node tick vs network reference, ticks behind, the Bob
    fetch/log/index/verify pipeline lag, and catch-up ETA.
  • Local node health from the running container (KeyDB memory, peers, uptime).
  • A live log tail.
  • One-key node actions (install / start / stop / restart / cleanup / … ) driven by bob.sh.

It does NOT expose low-level node internals: those are not part of running a
Guardian node and are dangerous to hit by accident.

Requirements: pip install 'textual>=0.80,<1'  (bob.sh sets up a venv for you)

Usage:
    python3 bob-guardian.py
    python3 bob-guardian.py --container qubic-bob --operator <ID>
"""
from __future__ import annotations

import argparse
import asyncio
import json
import logging
import os
import re
import shutil
import socket
import subprocess
import sys
import urllib.request
from collections.abc import AsyncIterator
from typing import Any

# Force 24-bit colour: ssh sessions rarely forward COLORTERM (sshd AcceptEnv omits it),
# so without this Textual/Rich downsample the brand palette to a dull 256-colour
# approximation. setdefault keeps any real value the terminal already provides.
os.environ.setdefault("COLORTERM", "truecolor")

# ─── Dependency Check ─────────────────────────────────────────────────────────

REQUIRED_PYTHON = (3, 10)
REQUIRED_PACKAGES = {"textual": "textual>=0.80,<1", "rich": "rich"}

QUBIC_BANNER_PLAIN = r"""
   ____        _     _         ____        _
  / __ \      | |   (_)       |  _ \      | |
 | |  | |_   _| |__  _  ___   | |_) | ___ | |__
 | |  | | | | | '_ \| |/ __|  |  _ < / _ \| '_ \
 | |__| | |_| | |_) | | (__   | |_) | (_) | |_) |
  \___\_\\__,_|_.__/|_|\___|  |____/ \___/|_.__/
            G U A R D I A N
"""


def _print_banner() -> None:
    print()
    print("\033[38;2;35;255;255m" + QUBIC_BANNER_PLAIN + "\033[0m")
    print()


def _install_packages(packages: list[str]) -> bool:
    cmd = [sys.executable, "-m", "pip", "install", *packages]
    print(f"\033[38;2;35;255;255m  Installing: {', '.join(packages)} ...\033[0m")
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode == 0:
        print("\033[38;2;210;253;209m  OK\033[0m")
        return True
    print("\033[91m  pip install failed:\033[0m")
    for line in result.stderr.strip().splitlines()[-5:]:
        print(f"\033[91m    {line}\033[0m")
    return False


def check_dependencies() -> None:
    if sys.version_info < REQUIRED_PYTHON:
        _print_banner()
        print(f"\033[91m  Python {REQUIRED_PYTHON[0]}.{REQUIRED_PYTHON[1]}+ required\033[0m")
        sys.exit(1)

    missing = []
    for mod_name, pip_name in REQUIRED_PACKAGES.items():
        try:
            __import__(mod_name)
        except ImportError:
            missing.append(pip_name)
    if missing:
        _print_banner()
        if not _install_packages(missing):
            print(f"\n\033[93m  Install manually: pip install {' '.join(missing)}\033[0m\n")
            sys.exit(1)
        print()

    if not shutil.which("docker"):
        _print_banner()
        print("\033[91m  Docker CLI not found in PATH\033[0m\n")
        sys.exit(1)


check_dependencies()

from textual import work  # noqa: E402
from textual.app import App, ComposeResult  # noqa: E402
from textual.binding import Binding  # noqa: E402
from textual.containers import Horizontal, Vertical, VerticalScroll  # noqa: E402
from textual.design import ColorSystem  # noqa: E402
from textual.message import Message  # noqa: E402
from textual.screen import ModalScreen  # noqa: E402
from textual.widgets import Button, Footer, Header, Input, RichLog, Static  # noqa: E402
from rich.markup import escape as markup_escape  # noqa: E402

logger = logging.getLogger(__name__)

# ─── Config ───────────────────────────────────────────────────────────────────

BUILD = "bob-ui1"  # visible build marker (NODE ACTIONS title) to confirm the running version
DEFAULT_DATA_DIR = "/opt/qubic-bob"
DEFAULT_API_BASE = "https://guardians.qubic.org/api/v1"
NETWORK_RPC = "https://rpc.qubic.org/v1/tick-info"
SYNC_BUFFER = 50  # ticks behind <= this counts as in-sync for the live display
# The Guardians CDN rejects the default Python-urllib UA with HTTP 403.
HTTP_HEADERS = {"Accept": "application/json", "User-Agent": "bob-guardian/1.0"}

# ─── Qubic Brand Colors ────────────────────────────────────────────────────────

QUBIC_CYAN = "#23ffff"
QUBIC_CREAM = "#ffdea1"
QUBIC_DARK = "#232429"
QUBIC_DARKER = "#1a1b1f"
QUBIC_SURFACE = "#2a2b31"
QUBIC_CORAL = "#fc997a"
QUBIC_MINT = "#d2fdd1"
QUBIC_TEXT = "#d2d6db"
QUBIC_LABEL = "#b4bcc8"   # left-column labels: bright enough to read on the dark bg
QUBIC_DIM = "#7d8590"     # units / annotations / bar track: visible but subordinate
QUBIC_BORDER = "#3a3b42"
QUBIC_TEAL = "#32d9d9"
QUBIC_RED = "#ff4444"

QUBIC_THEME = ColorSystem(
    primary=QUBIC_CYAN, secondary=QUBIC_CREAM, warning=QUBIC_CORAL, error=QUBIC_RED,
    success=QUBIC_MINT, accent=QUBIC_TEAL, dark=True, luminosity_spread=0.15,
    text_alpha=0.95, background=QUBIC_DARK, surface=QUBIC_SURFACE, panel=QUBIC_DARKER,
)

LEVEL_COLORS = {
    "INFO": QUBIC_CYAN, "WARNING": QUBIC_CREAM, "ERROR": QUBIC_CORAL,
    "CRITICAL": f"{QUBIC_RED} bold", "DEBUG": QUBIC_DIM,
}
HEALTH_COLORS = {
    "healthy": QUBIC_MINT, "syncing": QUBIC_CREAM, "starting": QUBIC_CREAM,
    "stopped": QUBIC_DIM, "not_installed": QUBIC_DIM, "unreachable": QUBIC_CORAL,
    "unknown": QUBIC_DIM,
}
HEALTH_ICONS = {
    "healthy": "●", "syncing": "●", "starting": "○", "stopped": "⏹",
    "not_installed": "○", "unreachable": "✖", "unknown": "?",
}

# ─── Formatting helpers ─────────────────────────────────────────────────────────


def _fmt_tick(tick: int | None) -> str:
    if not tick:
        return "-"
    s = str(int(tick))
    parts = []
    while len(s) > 3:
        parts.append(s[-3:])
        s = s[:-3]
    parts.append(s)
    return "'".join(reversed(parts))


def _fmt_uptime(seconds: float | int | None) -> str:
    if not seconds:
        return "-"
    s = int(seconds)
    if s < 60:
        return f"{s}s"
    if s < 3600:
        return f"{s // 60}m {s % 60}s"
    h, m = s // 3600, (s % 3600) // 60
    if h < 24:
        return f"{h}h {m}m"
    d, h = h // 24, h % 24
    return f"{d}d {h}h {m}m"


def _fmt_eta(seconds: int) -> str:
    if seconds < 60:
        return "< 1 min"
    if seconds < 3600:
        return f"~{seconds // 60} min"
    if seconds < 86400:
        return f"~{seconds // 3600}h {(seconds % 3600) // 60}m"
    return f"~{seconds // 86400}d {(seconds % 86400) // 3600}h"


def _bar(pct: float, width: int, color: str) -> str:
    pct = max(0.0, min(100.0, pct))
    filled = int(round(pct * width / 100))
    return f"[{color}]{'█' * filled}[/][{QUBIC_DIM}]{'░' * (width - filled)}[/]"


def _score_color(score: float) -> str:
    if score >= 90:
        return QUBIC_MINT
    if score >= 70:
        return QUBIC_CREAM
    return QUBIC_CORAL


# ─── Clients ────────────────────────────────────────────────────────────────────


def _primary_host_ip() -> str | None:
    """The host's primary outbound IPv4 — no packets are sent, the kernel just
    resolves which source address the default route would use."""
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        try:
            s.connect(("8.8.8.8", 80))
            return s.getsockname()[0]
        finally:
            s.close()
    except Exception:
        return None


class BobNodeClient:
    """Talks to the local Bob node over its HTTP API (single /status endpoint).

    The published port is reached over a list of candidate hosts: on a normal box
    127.0.0.1 works; on hardened nodes where the firewall blocks host→bridge
    forwarding (and route_localnet is off) the loopback DNAT stalls and only the
    host's primary IP — which hits docker-proxy on 0.0.0.0 — answers. We try them
    in order and cache the first that responds."""

    def __init__(self, container: str, api_port: int) -> None:
        self._container = container
        self._port = api_port
        self._base: str | None = None  # first reachable base, cached after success
        hosts = ["127.0.0.1"]
        ip = _primary_host_ip()
        if ip and ip not in hosts:
            hosts.append(ip)
        hosts.append("localhost")
        self._candidates = [f"http://{h}:{api_port}" for h in hosts]

    async def _http(self, path: str) -> dict[str, Any]:
        def _fetch(base: str):
            req = urllib.request.Request(f"{base}{path}", headers=HTTP_HEADERS)
            with urllib.request.urlopen(req, timeout=3) as resp:
                return json.loads(resp.read().decode("utf-8"))
        loop = asyncio.get_event_loop()
        # cached base first, then the remaining candidates
        bases = ([self._base] if self._base else []) + [b for b in self._candidates if b != self._base]
        last_err: Exception | None = None
        for base in bases:
            try:
                data = await loop.run_in_executor(None, _fetch, base)
                self._base = base
                return data
            except Exception as e:  # timeout / refused / etc → try the next host
                last_err = e
        raise last_err or RuntimeError("no reachable endpoint")

    async def status(self) -> dict[str, Any]:
        return await self._http("/status")


class GuardiansClient:
    """Reads the public Guardians API: the node's score/reward + epoch stats.
    Bob nodes appear in /nodes with type == 'bob', same shape as Lite nodes."""

    def __init__(self, api_base: str) -> None:
        self._base = api_base.rstrip("/")

    def _get(self, path: str) -> Any:
        req = urllib.request.Request(f"{self._base}{path}", headers=HTTP_HEADERS)
        with urllib.request.urlopen(req, timeout=10) as resp:
            return json.loads(resp.read().decode("utf-8"))

    async def node_by_operator(self, operator: str) -> dict[str, Any] | None:
        # The single-node endpoint 404s; the list carries the full live record.
        def _fetch():
            nodes = self._get("/nodes")
            for n in nodes:
                if n.get("operator") == operator:
                    return n
            return None
        return await asyncio.get_event_loop().run_in_executor(None, _fetch)

    async def stats(self) -> dict[str, Any]:
        return await asyncio.get_event_loop().run_in_executor(None, lambda: self._get("/stats"))


async def fetch_network_reference() -> int | None:
    """Network current tick from the public RPC (the sync reference)."""
    def _fetch():
        try:
            req = urllib.request.Request(NETWORK_RPC, headers=HTTP_HEADERS)
            with urllib.request.urlopen(req, timeout=5) as resp:
                data = json.loads(resp.read().decode("utf-8"))
            return data.get("tick") or data.get("tickInfo", {}).get("tick")
        except Exception:
            return None
    return await asyncio.get_event_loop().run_in_executor(None, _fetch)


# ─── Log streaming ──────────────────────────────────────────────────────────────


class DockerLogStreamer:
    def __init__(self, container: str, tail: int = 200) -> None:
        self._container = container
        self._tail = tail
        self._process: asyncio.subprocess.Process | None = None
        self._running = False

    async def stream(self) -> AsyncIterator[str]:
        self._running = True
        backoff = 3
        while self._running:
            try:
                self._process = await asyncio.create_subprocess_exec(
                    "docker", "logs", "-f", "--tail", str(self._tail), self._container,
                    stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.STDOUT,
                )
                got_line = False
                while self._running:
                    line = await self._process.stdout.readline()
                    if not line:
                        break
                    text = line.decode("utf-8", "replace").rstrip()
                    # Don't surface docker CLI errors as log content. After an
                    # uninstall the container is gone and `docker logs` keeps
                    # printing "No such container" — that would spam the panel.
                    if RE_DOCKER_ERR.search(text):
                        continue
                    got_line = True
                    yield text
                if self._running:
                    # No real lines = container absent (removed / not yet
                    # created); back off so we stop hammering docker every 3s.
                    backoff = 3 if got_line else min(backoff * 2, 30)
                    await asyncio.sleep(backoff)
                    self._tail = 10
            except asyncio.CancelledError:
                self._running = False
                raise
            except Exception as e:
                logger.warning(f"log stream: {e}")
                if self._running:
                    await asyncio.sleep(5)

    def stop(self) -> None:
        self._running = False
        if self._process and self._process.returncode is None:
            try:
                self._process.terminate()
            except ProcessLookupError:
                pass


# Bob log line: "[2026-06-05 10:39:53.217] [info] [:] <message>"
RE_BOB_LINE = re.compile(r"^\[(?:\d{4}-\d\d-\d\d )?(\d\d:\d\d:\d\d)(?:\.\d+)?\]\s+\[(\w+)\]\s+\[[^\]]*\]\s*(.*)$")
# "Current state: FetchingTick: 56377898 (2.6) | FetchingLog: 56377895 (2.4) | Indexing: 56377891 (2.0) | Verifying: 56377892 (2.0) | GC: …"
RE_BOB_STATE = re.compile(r"FetchingTick:\s*(\d+).*?FetchingLog:\s*(\d+).*?Indexing:\s*(\d+).*?Verifying:\s*(\d+)")
# "Local Tick: 56377633 | Network tick: 56377634 | Network epoch: 216"
RE_BOB_NET = re.compile(r"Local Tick:\s*(\d+)\s*\|\s*Network tick:\s*(\d+)\s*\|\s*Network epoch:\s*(\d+)")
# "Peer 5.9.98.143:21842 => Last Activity: 8 seconds ago"
RE_BOB_PEER = re.compile(r"Peer\s+([\d.]+:\d+)\s*=>\s*Last Activity:\s*(\d+)\s*seconds")
# "KeyDB memory: 782.77 MB / 8.00 GB (9.6%)"
RE_BOB_MEM = re.compile(r"KeyDB memory:\s*([\d.]+\s*\w+)\s*/\s*([\d.]+\s*\w+)\s*\(([\d.]+)%\)")
# per-tick / periodic spam — hidden unless raw-log mode is on. The "events-only"
# view keeps the meaningful ones (Network tick, Replaced peer, Saving/Saved
# checkpoints, check-in, TickStream, warnings/errors) and drops the high-rate churn.
RE_BOB_NOISE = re.compile(r"Current state:|KeyDB memory:|Last Activity:|\[PEER INFO\]|\[-+\]|WS connection (opened|closed)")
# docker CLI / daemon errors (merged from stderr) — never real bob log content.
# Seen after uninstall: "Error response from daemon: No such container: qubic-bob".
RE_DOCKER_ERR = re.compile(r"Error response from daemon:|Error: No such container|Cannot connect to the Docker daemon")

_LEVEL_MAP = {
    "info": "INFO", "warn": "WARNING", "warning": "WARNING", "error": "ERROR",
    "err": "ERROR", "critical": "CRITICAL", "fatal": "CRITICAL", "debug": "DEBUG",
}

# peer-block boundary markers (the "-----[PEER INFO]-----" header / footer)
PEER_START = "[PEER INFO]"
PEER_END_RE = re.compile(r"\[-+\]")  # "-----[---------]-----" footer (no PEER INFO text)


class LogLine:
    __slots__ = ("ts", "level", "text", "noise", "fetch_tick", "pipeline",
                 "net_tick", "net_epoch", "peer", "peer_start", "peer_end", "mem")

    def __init__(self, ts: str, level: str, text: str) -> None:
        self.ts = ts
        self.level = level
        self.text = text
        self.noise = bool(RE_BOB_NOISE.search(text))
        self.fetch_tick: int | None = None
        self.pipeline: tuple[int, int, int] | None = None   # (log, index, verify)
        self.net_tick: int | None = None
        self.net_epoch: int | None = None
        self.peer: tuple[str, int] | None = None            # (ip:port, seconds_ago)
        self.peer_start = False
        self.peer_end = False
        self.mem: tuple[str, str, float] | None = None      # (used, total, pct)

        m = RE_BOB_STATE.search(text)
        if m:
            self.fetch_tick = int(m.group(1))
            self.pipeline = (int(m.group(2)), int(m.group(3)), int(m.group(4)))
            return
        m = RE_BOB_NET.search(text)
        if m:
            self.net_tick = int(m.group(2))
            self.net_epoch = int(m.group(3))
            return
        m = RE_BOB_PEER.search(text)
        if m:
            self.peer = (m.group(1), int(m.group(2)))
            return
        m = RE_BOB_MEM.search(text)
        if m:
            self.mem = (m.group(1), m.group(2), float(m.group(3)))
            return
        if PEER_START in text:
            self.peer_start = True
        elif PEER_END_RE.search(text):
            self.peer_end = True


def parse_log(line: str) -> LogLine | None:
    """Parse one Bob docker log line into a classified LogLine."""
    line = line.strip()
    if not line:
        return None
    if RE_DOCKER_ERR.search(line):
        return None
    m = RE_BOB_LINE.match(line)
    if not m:
        return LogLine("", "", line)
    ts, level_raw, msg = m.group(1), m.group(2), m.group(3)
    return LogLine(ts, _LEVEL_MAP.get(level_raw.lower(), ""), msg)


# ─── Panels ──────────────────────────────────────────────────────────────────────

_PANEL_CSS = f"""
    height: auto;
    padding: 0 2;
    background: {QUBIC_DARKER};
    border: round {QUBIC_BORDER};
    border-title-color: {QUBIC_CYAN};
    border-title-style: bold;
    border-title-align: left;
"""


class _RowPanel(Static):
    """Base panel: a titled box of label/value rows (title sits in the border)."""
    TITLE = ""
    ROWS: list[tuple[str, str]] = []  # (row_id, label)
    LABEL_W = 14

    DEFAULT_CSS = f"""
    _RowPanel {{ {_PANEL_CSS} }}
    _RowPanel .row {{ height: 1; }}
    """

    def compose(self) -> ComposeResult:
        self.border_title = f" {self.TITLE} "
        for row_id, _ in self.ROWS:
            yield Static("", id=row_id, classes="row")

    def set_row(self, row_id: str, label: str, value: str) -> None:
        self.query_one(f"#{row_id}", Static).update(
            f"[{QUBIC_LABEL}]{label:<{self.LABEL_W}}[/] {value}"
        )

    def set_line(self, row_id: str, markup: str) -> None:
        self.query_one(f"#{row_id}", Static).update(markup)


class NodePanel(_RowPanel):
    TITLE = "NODE"
    ROWS = [
        ("nd-alias", "Alias"), ("nd-operator", "Operator"), ("nd-health", "Health"),
        ("nd-version", "Version"), ("nd-uptime", "Uptime"), ("nd-container", "Container"),
        ("nd-peers", "Peers"), ("nd-memory", "KeyDB Mem"),
    ]

    def update_net(self, active: int, total: int) -> None:
        self.set_row("nd-peers", "Peers",
                     f"[{QUBIC_MINT}]{active}[/][{QUBIC_DIM}] active / {total} known[/]")

    def update_mem(self, used: str, total: str, pct: float) -> None:
        col = QUBIC_CORAL if pct >= 85 else QUBIC_TEXT
        self.set_row("nd-memory", "KeyDB Mem",
                     f"[{col}]{used}[/][{QUBIC_DIM}] / {total} ({pct:.0f}%)[/]")

    def update_local(self, container_state: str, status: dict | None, health: str) -> None:
        self.set_line("nd-container", f"[{QUBIC_LABEL}]{'Container':<{self.LABEL_W}}[/] {container_state}")
        extra = (status or {}).get("extraInfo", {}) if status else {}
        alias = extra.get("alias") or "-"
        operator = extra.get("operator") or "-"
        op_disp = f"{operator[:8]}…{operator[-4:]}" if len(operator) > 14 else operator
        self.set_row("nd-alias", "Alias", f"[bold {QUBIC_CREAM}]{alias}[/]")
        self.set_row("nd-operator", "Operator", f"[{QUBIC_TEAL}]{op_disp}[/]")
        ver = extra.get("version") or (status or {}).get("bobVersion") or "-"
        self.set_row("nd-version", "Version", f"[{QUBIC_TEXT}]{ver}[/]")
        up = extra.get("uptime")
        self.set_row("nd-uptime", "Uptime", f"[{QUBIC_TEXT}]{_fmt_uptime(up)}[/]")
        c = HEALTH_COLORS.get(health, QUBIC_DIM)
        i = HEALTH_ICONS.get(health, "?")
        self.set_row("nd-health", "Health", f"[{c}]{i} {health.replace('_', ' ')}[/]")


class SyncPanel(_RowPanel):
    TITLE = "SYNC"
    LABEL_W = 14
    ROWS = [
        ("sy-state", "State"), ("sy-epoch", "Epoch"), ("sy-node", "Node Tick"),
        ("sy-ref", "Net Tick"), ("sy-behind", "Behind"), ("sy-pipeline", "Pipeline"),
        ("sy-eta", "ETA"), ("sy-bar", ""),
    ]

    def update_pipeline(self, node_tick: int | None, pipeline: tuple[int, int, int] | None) -> None:
        if not node_tick or not pipeline:
            self.set_row("sy-pipeline", "Pipeline", f"[{QUBIC_DIM}]-[/]")
            return
        log_t, idx_t, vfy_t = pipeline
        def lag(x: int) -> str:
            d = node_tick - x
            col = QUBIC_DIM if d <= 5 else QUBIC_CORAL
            return f"[{col}]-{d}[/]" if d > 0 else f"[{QUBIC_MINT}]0[/]"
        self.set_row("sy-pipeline", "Pipeline",
                     f"[{QUBIC_DIM}]Log[/] {lag(log_t)} [{QUBIC_DIM}]· Idx[/] {lag(idx_t)} [{QUBIC_DIM}]· Vfy[/] {lag(vfy_t)}")

    def update_sync(self, node_tick: int | None, ref_tick: int | None,
                    epoch: int | None, eta_text: str | None) -> None:
        self.set_row("sy-epoch", "Epoch", f"[{QUBIC_TEXT}]{epoch or '-'}[/]")
        self.set_row("sy-node", "Node Tick", f"[bold {QUBIC_CYAN}]{_fmt_tick(node_tick)}[/]")
        self.set_row("sy-ref", "Net Tick", f"[{QUBIC_TEXT}]{_fmt_tick(ref_tick)}[/]")

        if not node_tick:
            self.set_row("sy-state", "State", f"[{QUBIC_CREAM}]● booting / restoring[/]")
            self.set_row("sy-behind", "Behind", "-")
            self.set_row("sy-eta", "ETA", "-")
            self.set_line("sy-bar", "")
            return

        if not ref_tick:
            self.set_row("sy-state", "State", f"[{QUBIC_DIM}]● network unreachable[/]")
            self.set_row("sy-behind", "Behind", "-")
            self.set_row("sy-eta", "ETA", "-")
            self.set_line("sy-bar", "")
            return

        behind = max(0, ref_tick - node_tick)
        if behind <= SYNC_BUFFER:
            self.set_row("sy-state", "State", f"[{QUBIC_MINT}]● SYNCED[/]  [{QUBIC_DIM}](≤{SYNC_BUFFER} buffer)[/]")
            self.set_row("sy-behind", "Behind", f"[{QUBIC_MINT}]{behind}[/] ticks")
            self.set_row("sy-eta", "ETA", f"[{QUBIC_MINT}]in sync[/]")
            self.set_line("sy-bar", f"[{QUBIC_LABEL}]{'Sync':<{self.LABEL_W}}[/] {_bar(100, 30, QUBIC_MINT)} 100%")
        else:
            self.set_row("sy-state", "State", f"[{QUBIC_CREAM}]● SYNCING[/]")
            self.set_row("sy-behind", "Behind", f"[{QUBIC_CORAL}]{_fmt_tick(behind)}[/] ticks")
            self.set_row("sy-eta", "ETA", f"[{QUBIC_CYAN}]{eta_text or 'warming up…'}[/]")
            pct = 100.0 * node_tick / ref_tick if ref_tick else 0
            self.set_line("sy-bar", f"[{QUBIC_LABEL}]{'Sync':<{self.LABEL_W}}[/] {_bar(pct, 30, QUBIC_CYAN)} {pct:.2f}%")


class GuardianPanel(Static):
    """Live Guardian score from guardians.qubic.org, in three columns:
    STATUS (eligibility / epoch / checks) · SCORES · REWARD."""
    TITLE = "GUARDIAN  (guardians.qubic.org)"

    DEFAULT_CSS = f"""
    GuardianPanel {{ {_PANEL_CSS} }}
    GuardianPanel #gd-cols {{ layout: horizontal; height: auto; }}
    GuardianPanel .gd-col {{ width: 1fr; height: auto; }}
    GuardianPanel .gd-col.mid {{ margin: 0 3; }}
    GuardianPanel .gd-head {{ color: {QUBIC_CYAN}; text-style: bold; height: 1; }}
    GuardianPanel .row {{ height: 1; }}
    GuardianPanel .grow {{ height: auto; }}
    """

    def compose(self) -> ComposeResult:
        self.border_title = f" {self.TITLE} "
        with Horizontal(id="gd-cols"):
            with Vertical(classes="gd-col"):
                yield Static(f"[{QUBIC_CYAN}]STATUS[/]", classes="gd-head")
                yield Static("", id="gd-eligible", classes="grow")
                yield Static("", id="gd-epoch", classes="grow")
                yield Static("", id="gd-checks", classes="row")
            with Vertical(classes="gd-col mid"):
                yield Static(f"[{QUBIC_CYAN}]SCORES[/]", classes="gd-head")
                yield Static("", id="gd-final", classes="row")
                yield Static("", id="gd-uptime", classes="row")
                yield Static("", id="gd-sync", classes="row")
                yield Static("", id="gd-p2p", classes="row")
            with Vertical(classes="gd-col"):
                yield Static(f"[{QUBIC_CYAN}]REWARD[/]", classes="gd-head")
                yield Static("", id="gd-points", classes="row")
                yield Static("", id="gd-reward", classes="row")

    def _set(self, row_id: str, label: str, value: str, w: int = 9) -> None:
        self.query_one(f"#{row_id}", Static).update(f"[{QUBIC_LABEL}]{label:<{w}}[/] {value}")

    def _line(self, row_id: str, markup: str) -> None:
        self.query_one(f"#{row_id}", Static).update(markup)

    def update_score(self, node: dict | None, stats: dict | None) -> None:
        if node is None:
            self._line("gd-eligible", f"[{QUBIC_DIM}]node not yet tracked by Guardians[/]")
            for rid in ("gd-epoch", "gd-checks", "gd-final", "gd-uptime",
                        "gd-sync", "gd-p2p", "gd-points", "gd-reward"):
                self._line(rid, "")
        else:
            flagged = node.get("flagged")
            eligible = node.get("eligibleForReward")
            if flagged:
                self._set("gd-eligible", "Eligible", f"[{QUBIC_RED}]✖ FLAGGED[/] [{QUBIC_DIM}]{node.get('flaggedReason') or ''}[/]")
            elif eligible:
                self._set("gd-eligible", "Eligible", f"[{QUBIC_MINT}]✔ yes[/]")
            else:
                reason = node.get("ineligibleReason") or "thresholds not met yet"
                self._set("gd-eligible", "Eligible", f"[{QUBIC_CREAM}]… not yet[/] [{QUBIC_DIM}]{reason}[/]")

            ls = node.get("liveScore") or {}
            for rid, label, key in [
                ("gd-final", "Final", "finalScore"),
                ("gd-uptime", "Uptime", "uptimeScore"),
                ("gd-sync", "Sync", "syncScore"),
                ("gd-p2p", "P2P", "p2pScore"),
            ]:
                v = ls.get(key)
                if v is None:
                    self._set(rid, label, "-", 7)
                else:
                    self._set(rid, label, f"[{_score_color(v)}]{v:.1f}[/][{QUBIC_DIM}]/100[/]", 7)

            tc, sc = node.get("totalChecks", 0), node.get("successfulChecks", 0)
            tcol = QUBIC_MINT if tc >= 1500 else QUBIC_CREAM
            self._set("gd-checks", "Checks", f"[{QUBIC_TEXT}]{sc}[/]/[{tcol}]{tc}[/] [{QUBIC_DIM}](≥1500)[/]")
            self._set("gd-points", "Points", f"[{QUBIC_TEXT}]{_fmt_tick(int(ls.get('rewardPoints', 0)))}[/]", 7)
            est = ls.get("estimatedReward")
            self._set("gd-reward", "Est.", f"[bold {QUBIC_CREAM}]{_fmt_tick(est)}[/] [{QUBIC_DIM}]QU[/]" if est else "-", 7)

        if stats:
            prog = stats.get("epochProgress", {})
            ep = stats.get("reference", {}).get("epoch", "-")
            pct = prog.get("progress_percent")
            rem = prog.get("time_remaining_seconds")
            phase = stats.get("epochPhase", {}).get("phase", "")
            txt = f"[{QUBIC_TEXT}]{ep}[/]"
            if pct is not None:
                txt += f" [{QUBIC_DIM}]{pct:.0f}%"
                if rem:
                    txt += f" · {_fmt_eta(int(rem))} left"
                txt += "[/]"
            if phase and phase != "active":
                txt += f" [{QUBIC_CORAL}]{phase}[/]"
            self._set("gd-epoch", "Epoch", txt)


# ─── Node Actions Panel (bob.sh lifecycle) ────────────────────────────────────────

# (action_id, label, description). Mirrors the bob.sh menu a Guardian operator uses.
# status + logs are shown live natively; nothing here touches node internals.
ACTION_BUTTONS = [
    ("install", "⬇ Install", "Install the Bob node (asks for seed + alias)"),
    ("start", "▶ Start", "Start the stopped node container"),
    ("stop", "⏹ Stop", "Stop the node container (docker stop)"),
    ("restart", "↻ Restart", "Restart the container (compose down + up)"),
    ("logs", "📜 Logs", "Open the full-screen live log viewer (scroll, R=events-only)"),
    ("reconfigure", "✎ Reconfigure", "Change node seed/alias and restart fresh"),
    ("update", "↺ Update", "Self-update bob.sh + dashboard to the latest version"),
    ("reset", "⚠ Reset", "Wipe ALL node data, restart fresh (keeps seed/alias)"),
    ("uninstall", "✖ Uninstall", "Remove containers AND data directory (irreversible)"),
]
ACTION_DANGER = {"stop", "reset", "uninstall"}
# actions that bring the node down→up: drop the stale tick so SYNC re-initialises
ACTION_RESETS_SYNC = {"restart", "start", "reset", "reconfigure", "install"}
# grouped like the classic `bob.sh` menu (INSTALL · MANAGE · OTHER); each group sits
# under a full-width divider header. A group is (name, rows); each row is up to 4
# action ids, padded to 4 cols so widths align.
ACTION_GROUPS = [
    ("INSTALL", [["install", "uninstall"]]),
    ("MANAGE",  [["start", "stop", "restart", "logs"],
                 ["reconfigure", "reset"]]),
    ("OTHER",   [["update"]]),
]

# Number every action in display order (top→bottom, left→right) so each button's
# badge matches the "Run #" field.
ACTION_ORDER = [aid for _cat, _rows in ACTION_GROUPS for _row in _rows for aid in _row]
ACTION_NUM = {aid: i + 1 for i, aid in enumerate(ACTION_ORDER)}
NUM_ACTION = {i + 1: aid for i, aid in enumerate(ACTION_ORDER)}


class ActionsPanel(Static):
    DEFAULT_CSS = f"""
    ActionsPanel {{ {_PANEL_CSS} padding: 0 1; }}
    ActionsPanel .button-row {{ height: 1; layout: horizontal; width: 100%; margin-bottom: 1; }}
    ActionsPanel .group-head {{
        width: 100%; height: 1; margin-bottom: 1; border-top: solid {QUBIC_BORDER};
        border-title-color: {QUBIC_MINT}; border-title-style: bold; border-title-align: left;
    }}
    ActionsPanel .spacer {{ width: 1fr; height: 1; margin: 0 1 0 0; }}
    ActionsPanel .run-label {{
        width: 1fr; height: 1; margin: 0 1 0 0; color: {QUBIC_DIM};
        text-align: right; content-align-vertical: middle;
    }}
    ActionsPanel Input {{
        width: 1fr; height: 1; min-height: 1; margin: 0; padding: 0 1; border: none;
        background: {QUBIC_DARKER}; color: {QUBIC_TEXT};
    }}
    ActionsPanel Input:focus {{ border: none; background: {QUBIC_SURFACE}; color: #ffffff; }}
    /* never let the default red `-invalid` / `-valid` border (tall $error) creep in —
       it adds a row to the height:1 field and leaves a red ring after blur */
    ActionsPanel Input.-invalid, ActionsPanel Input.-invalid:focus,
    ActionsPanel Input.-valid, ActionsPanel Input.-valid:focus {{ border: none; }}
    ActionsPanel Button {{
        margin: 0 1 0 0; width: 1fr; height: 1; min-width: 0; border: none;
        background: {QUBIC_SURFACE}; color: {QUBIC_TEXT}; text-style: none;
    }}
    ActionsPanel Button:hover, ActionsPanel Button:focus {{
        background: #0e6b6b; color: #ffffff; text-style: bold; border: none;
    }}
    ActionsPanel .danger {{ background: #3d1a1a; color: {QUBIC_CORAL}; }}
    ActionsPanel .danger:hover, ActionsPanel .danger:focus {{
        background: #7a1f1f; color: #ffffff; text-style: bold; border: none;
    }}
    """

    class Action(Message):
        def __init__(self, action: str) -> None:
            super().__init__()
            self.action = action

    def compose(self) -> ComposeResult:
        self.border_title = f" NODE ACTIONS · bob.sh · build {BUILD} (hover · H) "
        meta = {a: (label, desc) for a, label, desc in ACTION_BUTTONS}
        n_actions = len(ACTION_ORDER)
        for cat, rows in ACTION_GROUPS:
            head = Static("", classes="group-head")
            head.border_title = f" {cat} "
            yield head
            for row_idx, ids in enumerate(rows):
                with Horizontal(classes="button-row"):
                    for action_id in ids:
                        label, desc = meta[action_id]
                        btn = Button(f"{ACTION_NUM[action_id]} {label}", id=f"act-{action_id}",
                                     classes="danger" if action_id in ACTION_DANGER else "")
                        btn.tooltip = desc
                        yield btn
                    # spare columns: on INSTALL's first row hold the run-by-number
                    # field; everywhere else just pad so the grid stays aligned.
                    if cat == "INSTALL" and row_idx == 0:
                        yield Static(f"Run [1-{n_actions}]:", classes="run-label")
                        yield Input(placeholder="#", id="act-num", max_length=2, restrict=r"[0-9]*",
                                    tooltip="Type an action number, Enter to run · Esc to leave the field")
                    else:
                        for _ in range(4 - len(ids)):
                            yield Static("", classes="spacer")

    def on_button_pressed(self, event: Button.Pressed) -> None:
        if event.button.id and event.button.id.startswith("act-"):
            self.post_message(self.Action(event.button.id[4:]))

    def on_input_submitted(self, event: Input.Submitted) -> None:
        if event.input.id != "act-num":
            return
        raw = event.value.strip()
        event.input.value = ""
        # release focus so the single-key shortcuts (L, C, R, G, H, Q) work again —
        # a focused Input swallows every letter key, which looked like "L is broken".
        self.app.set_focus(None)
        action_id = NUM_ACTION.get(int(raw)) if raw.isdigit() else None
        if action_id:
            self.post_message(self.Action(action_id))

    def on_key(self, event) -> None:
        # Esc inside the run field = clear it and hand focus back to the shortcuts.
        if event.key == "escape" and getattr(self.app.focused, "id", None) == "act-num":
            self.query_one("#act-num", Input).value = ""
            self.app.set_focus(None)
            event.stop()


class LogPanel(Static):
    DEFAULT_CSS = f"""
    LogPanel {{
        height: 1fr; min-height: 10; padding: 0 1;
        background: {QUBIC_DARKER}; border: round {QUBIC_BORDER};
        border-title-color: {QUBIC_CYAN}; border-title-style: bold; border-title-align: left;
        border-subtitle-color: {QUBIC_DIM}; border-subtitle-align: right;
    }}
    LogPanel RichLog {{
        height: 1fr; scrollbar-size: 1 1; background: {QUBIC_DARKER};
        scrollbar-color: {QUBIC_TEAL};
    }}
    """

    def __init__(self, max_lines: int = 800, **kwargs) -> None:
        super().__init__(**kwargs)
        self._max_lines = max_lines

    def compose(self) -> ComposeResult:
        self.border_title = " LOG · full "
        self.border_subtitle = " full · R events-only · C clear · L hide "
        yield RichLog(highlight=True, markup=True, max_lines=self._max_lines, wrap=True, id="log-output")

    def set_mode(self, events_only: bool) -> None:
        self.border_title = " LOG · events " if events_only else " LOG · full "

    def write_log(self, ts: str, level: str, message: str) -> None:
        log = self.query_one("#log-output", RichLog)
        safe = markup_escape(message)
        color = LEVEL_COLORS.get(level, "")
        body = f"[{color}]{safe}[/]" if color else safe
        log.write(f"[{QUBIC_DIM}]{ts}[/] {body}" if ts else body)

    def write_note(self, prefix_markup: str, text: str) -> None:
        self.query_one("#log-output", RichLog).write(f"{prefix_markup}{markup_escape(text)}")

    def clear(self) -> None:
        self.query_one("#log-output", RichLog).clear()


# ─── Dialogs ─────────────────────────────────────────────────────────────────────


class ConfirmDialog(ModalScreen[bool]):
    DEFAULT_CSS = f"""
    ConfirmDialog {{ align: center middle; }}
    ConfirmDialog #dialog {{ width: 64; height: auto; padding: 1 2; border: thick {QUBIC_CORAL}; background: {QUBIC_SURFACE}; }}
    ConfirmDialog .title {{ text-style: bold; color: {QUBIC_CORAL}; text-align: center; width: 100%; margin-bottom: 1; }}
    ConfirmDialog .message {{ text-align: center; width: 100%; color: {QUBIC_TEXT}; margin-bottom: 1; }}
    ConfirmDialog .buttons {{ height: 3; align: center middle; layout: horizontal; }}
    ConfirmDialog .buttons Button {{ margin: 0 2; }}
    ConfirmDialog #confirm {{ background: {QUBIC_CORAL}; color: {QUBIC_DARK}; border: tall {QUBIC_CORAL}; }}
    ConfirmDialog #cancel {{ background: {QUBIC_SURFACE}; color: {QUBIC_TEXT}; border: tall {QUBIC_BORDER}; }}
    """

    def __init__(self, title: str, message: str) -> None:
        super().__init__()
        self._title = title
        self._message = message

    def compose(self) -> ComposeResult:
        with Vertical(id="dialog"):
            yield Static(self._title, classes="title")
            yield Static(self._message, classes="message")
            with Horizontal(classes="buttons"):
                yield Button("Confirm", id="confirm")
                yield Button("Cancel", id="cancel")

    def on_button_pressed(self, event: Button.Pressed) -> None:
        self.dismiss(event.button.id == "confirm")


class InputDialog(ModalScreen[dict | None]):
    DEFAULT_CSS = f"""
    InputDialog {{ align: center middle; }}
    InputDialog #dialog {{ width: 66; height: auto; padding: 1 2; border: thick {QUBIC_CYAN}; background: {QUBIC_SURFACE}; }}
    InputDialog .title {{ text-style: bold; color: {QUBIC_CYAN}; text-align: center; width: 100%; margin-bottom: 1; }}
    InputDialog .field-label {{ color: {QUBIC_DIM}; margin-top: 1; }}
    InputDialog Input {{ background: {QUBIC_DARKER}; border: tall {QUBIC_BORDER}; }}
    InputDialog Input:focus {{ border: tall {QUBIC_CYAN}; }}
    InputDialog .buttons {{ height: 3; align: center middle; layout: horizontal; margin-top: 1; }}
    InputDialog .buttons Button {{ margin: 0 2; }}
    InputDialog #ok {{ background: {QUBIC_TEAL}; color: {QUBIC_DARK}; border: tall {QUBIC_CYAN}; }}
    InputDialog #cancel {{ background: {QUBIC_SURFACE}; color: {QUBIC_TEXT}; border: tall {QUBIC_BORDER}; }}
    """

    # fields: (key, label, placeholder, password[, default_value])
    def __init__(self, title: str, fields: list[tuple]) -> None:
        super().__init__()
        self._title = title
        self._fields = fields

    def compose(self) -> ComposeResult:
        with Vertical(id="dialog"):
            yield Static(self._title, classes="title")
            for field in self._fields:
                key, label, placeholder, password = field[:4]
                value = field[4] if len(field) > 4 else ""
                yield Static(label, classes="field-label")
                yield Input(value=value, placeholder=placeholder, password=password, id=f"in-{key}")
            with Horizontal(classes="buttons"):
                yield Button("OK", id="ok")
                yield Button("Cancel", id="cancel")

    def on_mount(self) -> None:
        if self._fields:
            self.query_one(f"#in-{self._fields[0][0]}", Input).focus()

    def _collect(self) -> dict[str, str]:
        return {f[0]: self.query_one(f"#in-{f[0]}", Input).value for f in self._fields}

    def on_button_pressed(self, event: Button.Pressed) -> None:
        self.dismiss(self._collect() if event.button.id == "ok" else None)

    def on_input_submitted(self, event: Input.Submitted) -> None:
        self.dismiss(self._collect())


class HelpScreen(ModalScreen[None]):
    BINDINGS = [Binding("escape", "dismiss", "Close"), Binding("h", "dismiss", "Close"),
                Binding("q", "dismiss", "Close")]
    DEFAULT_CSS = f"""
    HelpScreen {{ align: center middle; }}
    HelpScreen #help-box {{ width: 88; height: 90%; padding: 1 2; border: thick {QUBIC_CYAN}; background: {QUBIC_SURFACE}; }}
    HelpScreen .help-title {{ text-style: bold; color: {QUBIC_CYAN}; text-align: center; width: 100%; margin-bottom: 1; }}
    HelpScreen .help-hint {{ color: {QUBIC_DIM}; text-align: center; width: 100%; margin-top: 1; }}
    """

    def compose(self) -> ComposeResult:
        with Vertical(id="help-box"):
            yield Static("Qubic Bob Guardian — Help", classes="help-title")
            with VerticalScroll():
                yield Static(self._build())
            yield Static("[ Esc / H / Q to close ]", classes="help-hint")

    def _build(self) -> str:
        lines = [
            f"[bold {QUBIC_CYAN}]What this shows[/]",
            f"  [{QUBIC_TEXT}]GUARDIAN[/]  your live score & reward from guardians.qubic.org.",
            f"            [{QUBIC_DIM}]Bob nodes are tracked under type 'bob'; eligibility is reported live.[/]",
            f"  [{QUBIC_TEXT}]SYNC[/]      node tick vs network; ≤{SYNC_BUFFER} ticks behind = in sync.",
            f"            [{QUBIC_DIM}]Pipeline = lag of FetchingLog / Indexing / Verifying behind FetchingTick.[/]",
            f"  [{QUBIC_TEXT}]NODE[/]      local container health / version / uptime / peers / KeyDB memory.",
            "",
            f"[bold {QUBIC_CYAN}]Keys[/]",
        ]
        for key, desc in [
            ("G", "Refresh Guardian score now"), ("R", "Log filter: full (default) ↔ events-only"),
            ("C", "Clear the log"), ("L", "Copy log: frozen full-screen, drag to select (no Shift)"),
            ("H", "This help"), ("Q", "Quit"),
        ]:
            lines.append(f"  [bold {QUBIC_TEAL}]{key:<4}[/] [{QUBIC_TEXT}]{desc}[/]")
        lines += ["", f"[bold {QUBIC_CYAN}]NODE ACTIONS[/] [{QUBIC_DIM}](bob.sh)[/]",
                  f"  [{QUBIC_DIM}]Click a button, or just press its number — it jumps[/]\n"
                  f"  [{QUBIC_DIM}]into the[/] [bold {QUBIC_TEAL}]Run[/] [{QUBIC_DIM}]field; press Enter to run, Esc to leave it.[/]"]
        for action_id, label, desc in sorted(ACTION_BUTTONS, key=lambda b: ACTION_NUM.get(b[0], 99)):
            if action_id not in ACTION_NUM:
                continue
            danger = f"  [{QUBIC_RED}](destructive)[/]" if action_id in ACTION_DANGER else ""
            clean = label.split(" ", 1)[-1]
            lines.append(f"  [bold {QUBIC_TEAL}]{ACTION_NUM[action_id]:>2}[/] "
                         f"[bold {QUBIC_CREAM}]{clean:<13}[/] [{QUBIC_TEXT}]{desc}[/]{danger}")
        return "\n".join(lines)

    def action_dismiss(self) -> None:
        self.dismiss(None)


class LogScreen(ModalScreen[None]):
    """Full-screen live log viewer — more history, scrollable, raw/filtered."""

    BINDINGS = [
        Binding("escape", "dismiss", "Close"), Binding("q", "dismiss", "Close"),
        Binding("l", "dismiss", "Close"), Binding("r", "toggle_filter", "Filter"),
        Binding("c", "clear", "Clear"),
    ]
    DEFAULT_CSS = f"""
    LogScreen {{ align: center middle; background: $background 60%; }}
    LogScreen #logbox {{
        width: 96%; height: 92%; padding: 0 1; background: {QUBIC_DARKER};
        border: round {QUBIC_CYAN}; border-title-color: {QUBIC_CYAN};
        border-title-style: bold; border-title-align: left;
        border-subtitle-color: {QUBIC_DIM}; border-subtitle-align: right;
    }}
    LogScreen RichLog {{ height: 1fr; scrollbar-size: 1 1; background: {QUBIC_DARKER}; scrollbar-color: {QUBIC_TEAL}; }}
    """

    def __init__(self, container: str, events_only: bool) -> None:
        super().__init__()
        self._container = container
        self._events_only = events_only
        self._streamer = DockerLogStreamer(container, tail=500)

    def compose(self) -> ComposeResult:
        with Vertical(id="logbox"):
            yield RichLog(highlight=True, markup=True, max_lines=4000, wrap=True, id="full-log")

    def on_mount(self) -> None:
        self._retitle()
        self.query_one("#logbox", Vertical).border_subtitle = " R full/events · C clear · Esc close "
        self._pump()

    def _retitle(self) -> None:
        mode = "events" if self._events_only else "full"
        self.query_one("#logbox", Vertical).border_title = f" LIVE LOG · {mode} "

    @work(exclusive=True, group="fulllog")
    async def _pump(self) -> None:
        rl = self.query_one("#full-log", RichLog)
        n = 0
        try:
            async for line in self._streamer.stream():
                n += 1
                if n % 50 == 0:
                    await asyncio.sleep(0)  # yield: a log burst must not starve input/render
                entry = parse_log(line)
                if entry is None or not entry.text:
                    continue
                if self._events_only and entry.noise:
                    continue
                safe = markup_escape(entry.text)
                color = LEVEL_COLORS.get(entry.level, "")
                body = f"[{color}]{safe}[/]" if color else safe
                rl.write(f"[{QUBIC_DIM}]{entry.ts}[/] {body}" if entry.ts else body)
        except asyncio.CancelledError:
            pass

    def action_toggle_filter(self) -> None:
        self._events_only = not self._events_only
        self._retitle()

    def action_clear(self) -> None:
        self.query_one("#full-log", RichLog).clear()

    def action_dismiss(self) -> None:
        self._streamer.stop()
        self.dismiss(None)

    async def on_unmount(self) -> None:
        self._streamer.stop()


class LogCopyScreen(ModalScreen[None]):
    """Frozen, selectable full-log snapshot for copying. While it is open the
    terminal's mouse tracking is turned OFF, so a plain mouse drag does native text
    selection — no Shift needed. The snapshot is frozen (not live) on purpose: live
    redraws would wipe the terminal selection mid-drag."""

    BINDINGS = [
        Binding("escape", "dismiss", "Close"), Binding("q", "dismiss", "Close"),
        Binding("l", "dismiss", "Close"),
    ]
    DEFAULT_CSS = f"""
    LogCopyScreen {{ align: center middle; background: $background 60%; }}
    LogCopyScreen #copybox {{
        width: 96%; height: 92%; padding: 0 1; background: {QUBIC_DARKER};
        border: round {QUBIC_MINT}; border-title-color: {QUBIC_MINT};
        border-title-style: bold; border-title-align: left;
        border-subtitle-color: {QUBIC_DIM}; border-subtitle-align: right;
    }}
    LogCopyScreen RichLog {{ height: 1fr; scrollbar-size: 1 1; background: {QUBIC_DARKER}; scrollbar-color: {QUBIC_TEAL}; }}
    """

    def __init__(self, container: str, events_only: bool) -> None:
        super().__init__()
        self._container = container
        self._events_only = events_only

    def compose(self) -> ComposeResult:
        with Vertical(id="copybox"):
            yield RichLog(highlight=False, markup=True, max_lines=4000, wrap=True,
                          auto_scroll=False, id="copy-log")

    def on_mount(self) -> None:
        box = self.query_one("#copybox", Vertical)
        box.border_title = " COPY LOG · frozen snapshot "
        box.border_subtitle = " drag to select (no Shift) · ↑↓/PgUp/PgDn scroll · Esc close "
        self._set_mouse(enabled=False)
        self._load()

    def _set_mouse(self, *, enabled: bool) -> None:
        drv = getattr(self.app, "_driver", None)
        try:
            if enabled and hasattr(drv, "_enable_mouse_support"):
                drv._enable_mouse_support()
            elif not enabled and hasattr(drv, "_disable_mouse_support"):
                drv._disable_mouse_support()
        except Exception:
            pass

    @work(exclusive=True, group="copylog")
    async def _load(self) -> None:
        rl = self.query_one("#copy-log", RichLog)
        try:
            proc = await asyncio.create_subprocess_exec(
                "docker", "logs", "--tail", "600", self._container,
                stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.STDOUT)
            out, _ = await proc.communicate()
        except Exception as e:
            rl.write(f"[{QUBIC_CORAL}]could not read logs: {markup_escape(str(e))}[/]")
            return
        for raw in out.decode("utf-8", "replace").splitlines():
            entry = parse_log(raw)
            if entry is None or not entry.text:
                continue
            if self._events_only and entry.noise:
                continue
            safe = markup_escape(entry.text)
            color = LEVEL_COLORS.get(entry.level, "")
            body = f"[{color}]{safe}[/]" if color else safe
            rl.write(f"[{QUBIC_DIM}]{entry.ts}[/] {body}" if entry.ts else body)
        rl.scroll_end(animate=False)
        rl.focus()

    def action_dismiss(self) -> None:
        self._set_mouse(enabled=True)
        self.dismiss(None)

    async def on_unmount(self) -> None:
        self._set_mouse(enabled=True)


# ─── App ─────────────────────────────────────────────────────────────────────────


class BobGuardianApp(App):
    TITLE = "Qubic Bob Guardian"

    CSS = f"""
    Screen {{ layout: vertical; background: {QUBIC_DARK}; }}
    *:focus {{ background-tint: transparent !important; }}
    #conn {{ dock: top; height: 1; text-align: right; padding: 0 2; background: {QUBIC_DARKER}; color: {QUBIC_DIM}; }}
    #top-panels {{ height: auto; layout: horizontal; margin: 0 1; }}
    #top-panels NodePanel {{ width: 1fr; margin: 0 1 0 0; }}
    #top-panels SyncPanel {{ width: 1fr; }}
    GuardianPanel {{ margin: 1 1 0 1; }}
    ActionsPanel {{ margin: 1 1 0 1; }}
    LogPanel {{ height: 1fr; min-height: 10; margin: 1 1 0 1; }}
    Header {{ background: {QUBIC_DARKER}; color: {QUBIC_CYAN}; dock: top; }}
    Footer {{ background: {QUBIC_DARKER}; color: {QUBIC_DIM}; }}
    Footer > .footer--key {{ background: {QUBIC_SURFACE}; color: {QUBIC_CYAN}; }}
    Footer > .footer--description {{ color: {QUBIC_TEXT}; }}
    """

    BINDINGS = [
        Binding("q", "quit", "Quit", key_display="Q"),
        Binding("g", "refresh_guardian", "Guardian", key_display="G"),
        Binding("r", "toggle_filter", "Filter", key_display="R"),
        Binding("c", "clear_logs", "Clear", key_display="C"),
        Binding("l", "copy_log", "Copy log", key_display="L"),
        Binding("h", "help", "Help", key_display="H"),
    ]

    def __init__(self, container_name: str, api_port: int, bob_script: str | None,
                 api_base: str, operator: str | None) -> None:
        super().__init__()
        self._container_name = container_name
        self._api_port = api_port
        self._bob_script = bob_script
        self._operator = operator
        self._node = BobNodeClient(container_name, api_port)
        self._guardians = GuardiansClient(api_base)
        self._streamer = DockerLogStreamer(container_name)
        self._connected = False
        self._action_running = False
        self._events_only = False  # default: show every line, like the classic logs
        # sync state — node tick comes from the live log (freshest), ref from RPC
        self._node_tick: int | None = None
        self._ref_tick: int | None = None
        self._epoch: int | None = None
        self._eta_text: str | None = None
        self._pipeline: tuple[int, int, int] | None = None
        # sync ETA history: (timestamp, behind)
        self._behind_hist: list[tuple[float, int]] = []
        # live node-health snapshot pushed to the NODE panel
        self._net: tuple[int, int] | None = None       # (active peers, known peers)
        self._mem: tuple[str, str, float] | None = None  # (used, total, pct)
        # peer-block accumulator: ip:port -> seconds_ago for the in-progress block
        self._peer_buf: dict[str, int] = {}
        self._in_peer_block = False

    def get_css_variables(self) -> dict[str, str]:
        variables = QUBIC_THEME.generate()
        # Textual's default focus style is `bold reverse`; `reverse` swaps fg/bg,
        # which paints a focused/hovered button's label in its own background
        # colour → text vanishes. Kill it globally: every button sets its own
        # explicit focus colours (white on teal/red), so reverse buys us nothing
        # but the bug.
        variables["button-focus-text-style"] = "none"
        return variables

    def compose(self) -> ComposeResult:
        yield Header()
        yield Static("", id="conn")
        with Horizontal(id="top-panels"):
            yield NodePanel()
            yield SyncPanel()
        yield GuardianPanel()
        yield ActionsPanel()
        yield LogPanel()
        yield Footer()

    def on_mount(self) -> None:
        self._set_conn()
        self.poll_node()
        self.poll_guardian()
        self.stream_logs()

    def _set_conn(self) -> None:
        dot = f"[{QUBIC_MINT}]● connected[/]" if self._connected else f"[{QUBIC_CORAL}]● disconnected[/]"
        self.query_one("#conn", Static).update(
            f"{dot}  [{QUBIC_DIM}]container:[/] [{QUBIC_TEXT}]{self._container_name}[/]"
        )

    async def _container_state(self) -> tuple[str, str]:
        """Returns (markup, raw_state). raw_state in: running / exited / created / '' """
        try:
            proc = await asyncio.create_subprocess_exec(
                "docker", "ps", "-a", "--filter", f"name=^{self._container_name}$",
                "--format", "{{.State}}",
                stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.DEVNULL)
            out, _ = await asyncio.wait_for(proc.communicate(), timeout=8)
            state = out.decode().strip()
            if state == "running":
                return f"[{QUBIC_MINT}]● running[/]", state
            if state:
                return f"[{QUBIC_CREAM}]● {state}[/]", state
            return f"[{QUBIC_DIM}]○ not installed[/]", ""
        except Exception:
            return f"[{QUBIC_DIM}]?[/]", "?"

    def _health(self, raw_state: str, status: dict | None, behind: int | None) -> str:
        if raw_state == "":
            return "not_installed"
        if raw_state not in ("running", "?"):
            return "stopped"
        if status is None:
            return "starting"  # container up, API not ready yet
        if behind is None:
            return "unknown"
        return "healthy" if behind <= SYNC_BUFFER else "syncing"

    @work(exclusive=True, group="node")
    async def poll_node(self) -> None:
        while True:
            try:
                node_panel = self.query_one(NodePanel)
                self.query_one(SyncPanel)
            except Exception:
                await asyncio.sleep(1)
                continue

            cstate_markup, raw_state = await self._container_state()
            status = None
            try:
                status = await self._node.status()
            except Exception:
                pass

            self._connected = bool(status)
            self._set_conn()

            # capture operator for the guardian lookup; re-trigger the guardian
            # poll the moment we learn it, so the score shows up immediately
            # instead of after the 30s poll interval.
            if not self._operator and status:
                op = status.get("extraInfo", {}).get("operator")
                if op:
                    self._operator = op
                    self.poll_guardian()

            # node tick / epoch / pipeline straight from /status (the live log keeps
            # node_tick fresher between polls; only adopt the polled tick if ahead)
            if status:
                polled_tick = status.get("currentFetchingTick")
                if polled_tick and (self._node_tick is None or polled_tick > self._node_tick):
                    self._node_tick = polled_tick
                ep = status.get("currentProcessingEpoch")
                if ep:
                    self._epoch = ep
                pl = (status.get("currentFetchingLogTick"), status.get("currentIndexingTick"),
                      status.get("currentVerifyLoggingTick"))
                if all(pl):
                    self._pipeline = pl  # type: ignore[assignment]

            self._ref_tick = await fetch_network_reference()
            behind = (self._ref_tick - self._node_tick) if (self._ref_tick and self._node_tick) else None
            health = self._health(raw_state, status, behind)
            node_panel.update_local(cstate_markup, status, health)
            if self._net:
                node_panel.update_net(*self._net)
            if self._mem:
                node_panel.update_mem(*self._mem)

            self._eta_text = self._eta(self._node_tick, self._ref_tick)
            self._render_sync()
            await asyncio.sleep(3)

    def _reset_sync_state(self) -> None:
        """A lifecycle action is restarting the node — drop the stale tick so the
        SYNC panel re-initialises from the fresh container instead of freezing on
        the pre-restart values (node tick is monotonic, so without this a restart
        would never show). The next poll / log line rebuilds it."""
        self._node_tick = None
        self._epoch = None
        self._eta_text = None
        self._pipeline = None
        self._behind_hist.clear()
        self._render_sync()

    def _render_sync(self) -> None:
        try:
            sp = self.query_one(SyncPanel)
        except Exception:
            return
        sp.update_sync(self._node_tick, self._ref_tick, self._epoch, self._eta_text)
        sp.update_pipeline(self._node_tick, self._pipeline)

    def _eta(self, node_tick: int | None, ref: int | None) -> str | None:
        if not node_tick or not ref:
            self._behind_hist.clear()
            return None
        behind = ref - node_tick
        if behind <= SYNC_BUFFER:
            self._behind_hist.clear()
            return None
        now = asyncio.get_event_loop().time()
        self._behind_hist.append((now, behind))
        self._behind_hist = self._behind_hist[-60:]
        if len(self._behind_hist) < 4:
            return "warming up…"
        t0, b0 = self._behind_hist[0]
        elapsed = now - t0
        closed = b0 - behind
        if elapsed > 0 and closed > 0:
            eta = int(behind * elapsed / closed)
            rate = closed * 60 / elapsed
            return f"{_fmt_eta(eta)}  ({rate:.0f} t/min)"
        if closed <= 0:
            return "∞ not catching up"
        return "warming up…"

    @work(exclusive=True, group="guardian")
    async def poll_guardian(self) -> None:
        while True:
            try:
                panel = self.query_one(GuardianPanel)
            except Exception:
                await asyncio.sleep(2)
                continue
            node = stats = None
            try:
                stats = await self._guardians.stats()
            except Exception:
                pass
            if self._operator:
                try:
                    node = await self._guardians.node_by_operator(self._operator)
                except Exception:
                    pass
            panel.update_score(node, stats)
            await asyncio.sleep(30)

    async def action_refresh_guardian(self) -> None:
        self.poll_guardian()
        self.query_one(LogPanel).write_note(f"[{QUBIC_CYAN}]» ", "refreshing Guardian score…")

    @work(exclusive=True, group="logs")
    async def stream_logs(self) -> None:
        try:
            log_panel = self.query_one(LogPanel)
            node_panel = self.query_one(NodePanel)
        except Exception:
            return
        n = 0
        try:
            async for line in self._streamer.stream():
                n += 1
                if n % 50 == 0:
                    await asyncio.sleep(0)  # yield: a log burst must not starve input/render
                entry = parse_log(line)
                if entry is None or not entry.text:
                    continue
                # the "Current state" line is the freshest tick source — keep SYNC live
                if entry.fetch_tick:
                    if self._node_tick is None or entry.fetch_tick > self._node_tick:
                        self._node_tick = entry.fetch_tick
                    if entry.pipeline:
                        self._pipeline = entry.pipeline
                    self._render_sync()
                if entry.net_epoch:
                    self._epoch = entry.net_epoch
                # peer block: reset on header, accumulate, commit on footer
                if entry.peer_start:
                    self._peer_buf = {}
                    self._in_peer_block = True
                elif entry.peer is not None:
                    if self._in_peer_block:
                        self._peer_buf[entry.peer[0]] = entry.peer[1]
                elif entry.peer_end and self._in_peer_block:
                    self._in_peer_block = False
                    total = len(self._peer_buf)
                    active = sum(1 for s in self._peer_buf.values() if s <= 60)
                    self._net = (active, total)
                    node_panel.update_net(active, total)
                if entry.mem is not None:
                    used, total_s, pct = entry.mem
                    self._mem = (used, total_s, pct)
                    node_panel.update_mem(used, total_s, pct)
                # show every line by default; only hide per-tick spam if asked
                if self._events_only and entry.noise:
                    continue
                log_panel.write_log(entry.ts, entry.level, entry.text)
        except asyncio.CancelledError:
            pass
        except Exception as e:
            log_panel.write_note(f"[{QUBIC_CORAL}]log stream error: ", str(e))

    # ─── log / help actions ───────────────────────────────────────────────────
    def action_clear_logs(self) -> None:
        self.query_one(LogPanel).clear()

    def action_copy_log(self) -> None:
        # Full-screen frozen log you can select WITHOUT holding Shift (mouse tracking
        # is disabled while it is open). The live inline log stays put underneath.
        self.push_screen(LogCopyScreen(self._container_name, self._events_only))

    def on_key(self, event) -> None:
        # Pressing a digit on the main screen jumps straight into the Run field, so you
        # can type a number + Enter without clicking the field first. Skipped while a
        # dialog/modal is open (digits there belong to that screen).
        if (len(self.screen_stack) == 1 and event.character and event.character.isdigit()
                and getattr(self.focused, "id", None) != "act-num"):
            try:
                inp = self.query_one("#act-num", Input)
            except Exception:
                return
            inp.focus()
            inp.value = (inp.value + event.character)[-2:]
            event.stop()

    def action_toggle_filter(self) -> None:
        self._events_only = not self._events_only
        lp = self.query_one(LogPanel)
        lp.set_mode(self._events_only)
        lp.write_note(f"[{QUBIC_CYAN}]» ",
                      "events only: hiding per-tick state, KeyDB memory and peer-info lines"
                      if self._events_only else "full log: showing every line (like classic logs)")

    def action_help(self) -> None:
        self.push_screen(HelpScreen())

    # ─── bob.sh actions ───────────────────────────────────────────────────────
    def _bob_path(self) -> str | None:
        if self._bob_script and os.path.isfile(self._bob_script):
            return self._bob_script
        data_copy = os.path.join(DEFAULT_DATA_DIR, "bob.sh")
        return data_copy if os.path.isfile(data_copy) else None

    def _confirm_then(self, title: str, msg: str, cb) -> None:
        def _result(ok: bool) -> None:
            if ok:
                cb()
        self.push_screen(ConfirmDialog(title, msg), _result)

    async def on_actions_panel_action(self, message: ActionsPanel.Action) -> None:
        a = message.action
        if a == "stop":
            self._confirm_then("Stop Node?", "Stop the node container (docker stop).",
                               lambda: self._run_bob(["stop"], label="stop container"))
        elif a == "start":
            self._run_bob(["start"])
        elif a == "restart":
            self._run_bob(["restart"], label="container restart")
        elif a == "logs":
            self.push_screen(LogScreen(self._container_name, self._events_only))
        elif a == "update":
            self._run_bob(["update"], label="self-update bob.sh")
        elif a == "install":
            self._prompt_install()
        elif a == "reconfigure":
            self._prompt_reconfigure()
        elif a == "reset":
            self._confirm_then("Reset Node?", "Wipe ALL node data and restart fresh. Seed/alias kept.",
                               lambda: self._run_bob(["reset"], stdin_data="y\n", label="reset (wipe data)"))
        elif a == "uninstall":
            self._confirm_then("Uninstall Node?", "Remove containers AND the data directory. Cannot be undone.",
                               lambda: self._run_bob(["uninstall"], stdin_data="y\n", label="uninstall"))

    def _prompt_install(self) -> None:
        def _done(res: dict | None) -> None:
            if not res:
                return
            seed, alias = res.get("seed", "").strip(), res.get("alias", "").strip()
            if not seed or not alias:
                self.query_one(LogPanel).write_note(f"[{QUBIC_CORAL}]» ", "install: seed and alias required")
                return
            self._run_bob(["install", "--seed", seed, "--alias", alias], label="install")
        self.push_screen(InputDialog("Install Bob Node", [
            ("seed", "Node Seed", "55-char node seed", True),
            ("alias", "Node Alias", "my-node", False)]), _done)

    def _prompt_reconfigure(self) -> None:
        def _done(res: dict | None) -> None:
            if res is None:
                return
            seed, alias = res.get("seed", "").strip(), res.get("alias", "").strip()
            self._run_bob(["reconfigure"], stdin_data=f"{seed}\n{alias}\n",
                          label="reconfigure (blank = keep current)")
        self.push_screen(InputDialog("Reconfigure (blank = keep current)", [
            ("seed", "New Seed", "blank = keep current", True),
            ("alias", "New Alias", "blank = keep current", False)]), _done)

    @work(group="bob_action")
    async def _run_bob(self, args: list[str], *, stdin_data: str | None = None,
                       label: str | None = None) -> None:
        log = self.query_one(LogPanel)
        if self._action_running:
            log.write_note(f"[{QUBIC_CREAM}]» ", "an action is already running, please wait…")
            return
        script = self._bob_path()
        if not script:
            log.write_note(f"[{QUBIC_CORAL}]» bob.sh not found: ",
                           str(self._bob_script or os.path.join(DEFAULT_DATA_DIR, "bob.sh")))
            return
        self._action_running = True
        if args and args[0] in ACTION_RESETS_SYNC:
            self._reset_sync_state()
        log.write_note(f"[{QUBIC_TEAL}]» bob.sh {markup_escape(' '.join(args))}  ",
                       f"{label}" if label else "")
        try:
            proc = await asyncio.create_subprocess_exec(
                "bash", script, *args,
                stdin=(asyncio.subprocess.PIPE if stdin_data is not None else asyncio.subprocess.DEVNULL),
                stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.STDOUT,
                env={**os.environ, "TERM": "dumb"})
            if stdin_data is not None and proc.stdin is not None:
                proc.stdin.write(stdin_data.encode())
                await proc.stdin.drain()
                proc.stdin.close()
            assert proc.stdout is not None
            while True:
                raw = await proc.stdout.readline()
                if not raw:
                    break
                text = raw.decode("utf-8", "replace").rstrip()
                if text:
                    log.write_note(f"[{QUBIC_DIM}]  ", text)
            await proc.wait()
            if proc.returncode == 0:
                log.write_note(f"[{QUBIC_MINT}]» ", "done")
            else:
                log.write_note(f"[{QUBIC_CORAL}]» ", f"exited with code {proc.returncode}")
        except Exception as e:
            log.write_note(f"[{QUBIC_CORAL}]» action error: ", str(e))
        finally:
            self._action_running = False

    async def on_unmount(self) -> None:
        self._streamer.stop()


# ─── Entry point ─────────────────────────────────────────────────────────────────


def main() -> None:
    parser = argparse.ArgumentParser(prog="bob-guardian",
                                     description="Network Guardian dashboard for a Qubic Bob node")
    parser.add_argument("--container", default="qubic-bob", help="Docker container name")
    parser.add_argument("--api-port", type=int, default=40420, help="Bob HTTP API port")
    parser.add_argument("--operator", default=None, help="Operator ID (auto-detected if omitted)")
    parser.add_argument("--api-base", default=DEFAULT_API_BASE, help="Guardians API base URL")
    default_bob = os.path.join(os.path.dirname(os.path.abspath(__file__)), "bob.sh")
    parser.add_argument("--bob-script", default=default_bob, help="Path to bob.sh for node actions")
    args = parser.parse_args()
    BobGuardianApp(container_name=args.container, api_port=args.api_port,
                   bob_script=args.bob_script, api_base=args.api_base,
                   operator=args.operator).run()


if __name__ == "__main__":
    main()
