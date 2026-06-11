#!/usr/bin/env python3
"""
Qubic Lite Guardian — a single-screen dashboard for a Network Guardian
Lite node operator.

It shows the things that actually decide your weekly reward:
  • Guardian score (uptime / sync / p2p / final), reward points, estimated reward,
    eligibility and flag status — pulled live from the public Guardians API.
  • Local sync state: node tick vs network reference, ticks behind, and whether
    you are inside the 50-tick buffer that earns a full sync score, plus catch-up ETA.
  • Local node health from the running container.
  • A live log tail.
  • One-key node actions (install / start / stop / restart / … ) driven by lite.sh.

It does NOT expose the low-level node console keys (drop connections, skip
security, force epoch, …): those are not part of running a Guardian node and
are dangerous to hit by accident.

Requirements: pip install 'textual>=0.80,<1'  (lite.sh sets up a venv for you)

Usage:
    python3 lite-guardian.py
    python3 lite-guardian.py --container qubic-lite --operator <ID>
"""
from __future__ import annotations

import argparse
import asyncio
import json
import logging
import os
import re
import shutil
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
   ____        _     _         _     _ _
  / __ \      | |   (_)       | |   (_) |
 | |  | |_   _| |__  _  ___   | |    _| |_ ___
 | |  | | | | | '_ \| |/ __|  | |   | | __/ _ \
 | |__| | |_| | |_) | | (__   | |___| | ||  __/
  \___\_\\__,_|_.__/|_|\___|  |_____|_|\__\___|
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
from rich.text import Text  # noqa: E402
from rich.highlighter import ReprHighlighter  # noqa: E402

logger = logging.getLogger(__name__)

# ─── Config ───────────────────────────────────────────────────────────────────

BUILD = "ui13-runfield"  # visible build marker (NODE ACTIONS title) to confirm the running version
DEFAULT_DATA_DIR = "/opt/qubic-lite"
DEFAULT_API_BASE = "https://guardians.qubic.org/api/v1"
NETWORK_RPC = "https://rpc.qubic.org/v1/tick-info"
SYNC_BUFFER = 50  # ticks behind <= this still scores a full sync score
# The Guardians CDN rejects the default Python-urllib UA with HTTP 403.
HTTP_HEADERS = {"Accept": "application/json", "User-Agent": "lite-guardian/1.0"}

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
    "healthy": QUBIC_MINT, "starting": QUBIC_CREAM, "saving_snapshot": QUBIC_CREAM,
    "unknown": QUBIC_DIM, "stuck": QUBIC_CORAL, "crashed": QUBIC_RED,
    "misaligned": QUBIC_CORAL, "version_incompatible": QUBIC_CORAL, "epoch_behind": QUBIC_CORAL,
}
HEALTH_ICONS = {
    "healthy": "●", "starting": "○", "saving_snapshot": "◔", "unknown": "○",
    "stuck": "●", "crashed": "✖", "misaligned": "▲", "version_incompatible": "⚠", "epoch_behind": "▼",
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


class NodeClient:
    """Talks to the local node: orchestrator-ctl (docker exec) + HTTP tick-info."""

    def __init__(self, container: str, http_port: int) -> None:
        self._container = container
        self._http_base = f"http://localhost:{http_port}"

    async def _exec(self, *args: str) -> dict[str, Any]:
        proc = await asyncio.create_subprocess_exec(
            "docker", "exec", self._container, "orchestrator-ctl", *args,
            stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE,
        )
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=30)
        out = stdout.decode("utf-8", "replace").strip()
        if not out:
            raise RuntimeError(stderr.decode("utf-8", "replace").strip() or "no output")
        return json.loads(out)

    async def _http(self, path: str) -> dict[str, Any]:
        def _fetch():
            req = urllib.request.Request(f"{self._http_base}{path}", headers=HTTP_HEADERS)
            with urllib.request.urlopen(req, timeout=5) as resp:
                return json.loads(resp.read().decode("utf-8"))
        return await asyncio.get_event_loop().run_in_executor(None, _fetch)

    async def status(self) -> dict[str, Any]:
        return await self._exec("status")

    async def tick_info(self) -> dict[str, Any]:
        return await self._http("/tick-info")


class GuardiansClient:
    """Reads the public Guardians API: the node's score/reward + epoch stats."""

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
        while self._running:
            try:
                self._process = await asyncio.create_subprocess_exec(
                    "docker", "logs", "-f", "--tail", str(self._tail), self._container,
                    stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.STDOUT,
                )
                while self._running:
                    line = await self._process.stdout.readline()
                    if not line:
                        break
                    yield line.decode("utf-8", "replace").rstrip()
                if self._running:
                    await asyncio.sleep(3)
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


# the cryptic qubic stdout prefix: "260529115731 000:000(485).54778313.215 "
RE_QPREFIX = re.compile(r"^\d{12}\s+\d{3}:\d{3}\(\d{3}\)\.[\d']+\.\d+\s*")
# pull votes + live tick.epoch out of that prefix (the freshest tick the node is on)
RE_QTICK = re.compile(r"^\d{12}\s+(\d{3}:\d{3}\(\d{3}\))\.([\d']+)\.(\d+)")
# the per-tick connection/peer line: "[+2'580 -0 *3'066 /17'885] 148|37 72/0/251 ..."
RE_CONN = re.compile(r"^\[[+-][\d']+\s+[+-][\d']+\s+\*([\d']+)\s+/([\d']+)\]\s+([\d']+)\|([\d']+)")
# pure per-tick performance spam — hidden unless raw-log mode is on
RE_NOISE = re.compile(r"Main loop duration|Ticker loop duration|^\d+\s*\|\s*Tick\s*=|^\[[+-]\d")
# snapshot bootstrap (runs before the node starts ticking): chunked download + interleaved extract.
# "Downloading chunked snapshot: 182 chunks, 90.54 GB (parallel=3)"
RE_SNAP_TOTAL = re.compile(r"Downloading chunked snapshot: (\d+) chunks, ([\d.]+) GB")
# "  chunk 7/182 done (3.50/90.54 GB, download 10.3 MB/s aggregate, extract 80.2 MB/s)"
RE_SNAP_DONE = re.compile(r"chunk (\d+)/(\d+) done \(([\d.]+)/([\d.]+) GB, download ([\d.]+) MB/s")
# "Downloading chunk 30/182: ep215-…" — the chunk just *started* (parallel=3 leads 'done' by ~3)
RE_SNAP_FETCH = re.compile(r"Downloading chunk (\d+)/\d+:")


def _num(s: str) -> int:
    return int(s.replace("'", ""))


class LogLine:
    __slots__ = ("ts", "level", "text", "noise", "conns", "peers", "tick", "epoch", "qtag")

    def __init__(self, ts: str, level: str, text: str) -> None:
        self.ts = ts
        self.level = level
        self.text = text
        self.noise = bool(RE_NOISE.search(text))
        self.conns: tuple[int, int] | None = None
        self.peers: tuple[int, int] | None = None
        self.tick: int | None = None
        self.epoch: int | None = None
        self.qtag: str | None = None  # compact "votes tick" tag from the qubic prefix
        m = RE_CONN.match(text)
        if m:
            self.conns = (_num(m.group(1)), _num(m.group(2)))   # active, total
            self.peers = (_num(m.group(3)), _num(m.group(4)))   # aligned, misaligned


def parse_log(line: str) -> LogLine | None:
    """Parse one docker log line (JSON or raw) into a classified LogLine."""
    line = line.strip()
    if not line:
        return None
    try:
        data = json.loads(line)
    except json.JSONDecodeError:
        return LogLine("", "", line)
    ts = data.get("timestamp", "")
    if ts and "T" in ts:
        ts = ts.split("T")[1][:8]
    msg = data.get("message", "")
    for tag in ("[qubic:stdout]", "[qubic:stderr]"):
        if tag in msg:
            msg = msg.split("]", 1)[1].lstrip() if "]" in msg else msg
            break
    mt = RE_QTICK.match(msg)               # capture live tick BEFORE stripping prefix
    msg = RE_QPREFIX.sub("", msg)          # drop the cryptic numeric prefix
    entry = LogLine(ts, data.get("level", ""), msg)
    if mt:
        entry.qtag = f"{mt.group(1)} {mt.group(2)}"   # e.g. "000:000(546) 57'830'644"
        entry.tick = _num(mt.group(2))
        entry.epoch = int(mt.group(3))
    return entry


_LOG_HIGHLIGHTER = ReprHighlighter()


def format_log_markup(entry: LogLine) -> Text:
    """Render a parsed log line: dim timestamp, green votes/tick tag, coloured text.
    Built as a Text object so RichLog's auto-highlighter cannot recolour the tag
    (it restyles every number cyan, which used to swallow the qtag colour)."""
    line = Text()
    if entry.ts:
        line.append(entry.ts + " ", style=QUBIC_DIM)
    if entry.qtag:
        line.append(entry.qtag + " ", style="#3fb950")  # green: votes + current tick
    body = Text(entry.text, style=LEVEL_COLORS.get(entry.level, ""))
    _LOG_HIGHLIGHTER.highlight(body)  # keep the usual number highlighting in the message
    line.append_text(body)
    return line


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
        ("nd-peers", "Peers"), ("nd-conns", "Connections"),
    ]

    def update_net(self, conns: tuple[int, int], peers: tuple[int, int]) -> None:
        active, total = conns
        aligned, misaligned = peers
        self.set_row("nd-peers", "Peers",
                     f"[{QUBIC_MINT}]{aligned}[/][{QUBIC_DIM}] aligned · [/][{QUBIC_CORAL}]{misaligned}[/][{QUBIC_DIM}] mis[/]")
        self.set_row("nd-conns", "Connections",
                     f"[{QUBIC_TEXT}]{active}[/][{QUBIC_DIM}] active / {total} known[/]")

    def set_health_snapshot(self, snap: dict) -> None:
        """Short health line while bootstrapping from a chunked snapshot — the
        full progress + ETA live in the (otherwise idle) SYNC panel, which has
        the width for it. The NODE panel is only ~half the row, so keep it tight."""
        c = HEALTH_COLORS["saving_snapshot"]
        i = HEALTH_ICONS["saving_snapshot"]
        done, total = snap["done"], snap["total"]
        chunk, chunks = snap["chunk"], snap["chunks"]
        txt = "applying snapshot…" if chunks and chunk >= chunks else f"snapshot {done:.0f}/{total:.0f} GB"
        self.set_row("nd-health", "Health", f"[{c}]{i} {txt}[/]")

    def update_local(self, container_state: str, status: dict | None, tick: dict | None,
                     snap: dict | None = None) -> None:
        self.set_line("nd-container", f"[{QUBIC_LABEL}]{'Container':<{self.LABEL_W}}[/] {container_state}")
        extra = (tick or {}).get("extraInfo", {}) if tick else {}
        alias = extra.get("alias") or "-"
        operator = extra.get("operator") or "-"
        op_disp = f"{operator[:8]}…{operator[-4:]}" if len(operator) > 14 else operator
        self.set_row("nd-alias", "Alias", f"[bold {QUBIC_CREAM}]{alias}[/]")
        self.set_row("nd-operator", "Operator", f"[{QUBIC_TEAL}]{op_disp}[/]")
        ver = extra.get("version") or (status or {}).get("version", {}).get("local") or "-"
        self.set_row("nd-version", "Version", f"[{QUBIC_TEXT}]{ver}[/]")
        up = extra.get("uptime") or (status or {}).get("uptime_seconds")
        self.set_row("nd-uptime", "Uptime", f"[{QUBIC_TEXT}]{_fmt_uptime(up)}[/]")
        if status:
            health = status.get("node", {}).get("health", "unknown")
            c = HEALTH_COLORS.get(health, QUBIC_DIM)
            i = HEALTH_ICONS.get(health, "?")
            self.set_row("nd-health", "Health", f"[{c}]{i} {health}[/]")
        elif snap:
            # API not up yet because the node is still pulling its snapshot
            self.set_health_snapshot(snap)
        elif not tick:
            self.set_row("nd-health", "Health", f"[{QUBIC_DIM}]unreachable[/]")


class SyncPanel(_RowPanel):
    TITLE = "SYNC"
    LABEL_W = 14
    ROWS = [
        ("sy-state", "State"), ("sy-epoch", "Epoch"), ("sy-node", "Node Tick"),
        ("sy-ref", "Net Tick"), ("sy-behind", "Behind"), ("sy-eta", "ETA"), ("sy-bar", ""),
    ]

    def update_sync(self, node_tick: int | None, ref_tick: int | None,
                    epoch: int | None, eta_text: str | None,
                    initial_tick: int | None = None) -> None:
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
            self.set_row("sy-state", "State", f"[{QUBIC_MINT}]● SYNCED[/]  [{QUBIC_DIM}](≤{SYNC_BUFFER} buffer → sync score 100)[/]")
            self.set_row("sy-behind", "Behind", f"[{QUBIC_MINT}]{behind}[/] ticks")
            self.set_row("sy-eta", "ETA", f"[{QUBIC_MINT}]in sync[/]")
            self.set_line("sy-bar", f"[{QUBIC_LABEL}]{'Sync':<{self.LABEL_W}}[/] {_bar(100, 30, QUBIC_MINT)} 100%")
        else:
            self.set_row("sy-state", "State", f"[{QUBIC_CREAM}]● SYNCING[/]")
            self.set_row("sy-behind", "Behind", f"[{QUBIC_CORAL}]{_fmt_tick(behind)}[/] ticks")
            self.set_row("sy-eta", "ETA", f"[{QUBIC_CYAN}]{eta_text or 'warming up…'}[/]")
            # progress within the current epoch (sync replays from initialTick),
            # not node_tick/ref_tick — absolute ticks always read ~99.x%
            if initial_tick and ref_tick > initial_tick:
                pct = 100.0 * max(0, node_tick - initial_tick) / (ref_tick - initial_tick)
            else:
                pct = 100.0 * node_tick / ref_tick if ref_tick else 0
            pct = min(pct, 99.99)
            self.set_line("sy-bar", f"[{QUBIC_LABEL}]{'Sync':<{self.LABEL_W}}[/] {_bar(pct, 30, QUBIC_CYAN)} {pct:.2f}%")

    def update_snapshot(self, snap: dict) -> None:
        """Repurpose the (idle) SYNC rows to show snapshot-download progress + a
        running ETA while the node isn't ticking yet. Labels revert on the next
        update_sync() call, so this cleanly vanishes once the snapshot is done."""
        done, total = snap["done"], snap["total"]
        chunk, chunks, rate, eta = snap["chunk"], snap["chunks"], snap["rate"], snap.get("eta")
        applying = bool(chunks) and chunk >= chunks
        pct = 100.0 * done / total if total else 0.0
        self.set_row("sy-state", "State",
                     f"[{QUBIC_CREAM}]◔ {'applying snapshot…' if applying else 'restoring from snapshot'}[/]")
        self.set_row("sy-epoch", "Epoch", f"[{QUBIC_DIM}]-[/]")
        fetching = snap.get("fetching", 0)
        lead = f"[{QUBIC_DIM}]  ·{fetching} loading[/]" if fetching > chunk else ""
        self.set_row("sy-node", "Chunks", f"[bold {QUBIC_CYAN}]{chunk}[/][{QUBIC_DIM}] / {chunks}[/]{lead}")
        self.set_row("sy-ref", "Downloaded", f"[{QUBIC_TEXT}]{done:.1f}[/][{QUBIC_DIM}] / {total:.1f} GB[/]")
        self.set_row("sy-behind", "Speed", f"[{QUBIC_TEXT}]{rate:.0f}[/][{QUBIC_DIM}] MB/s[/]")
        if applying:
            self.set_row("sy-eta", "ETA", f"[{QUBIC_CREAM}]almost done…[/]")
        elif eta:
            self.set_row("sy-eta", "ETA", f"[bold {QUBIC_CYAN}]{_fmt_eta(eta)}[/] [{QUBIC_DIM}]left[/]")
        else:
            self.set_row("sy-eta", "ETA", f"[{QUBIC_DIM}]calculating…[/]")
        self.set_line("sy-bar", f"[{QUBIC_LABEL}]{'Progress':<{self.LABEL_W}}[/] {_bar(pct, 30, QUBIC_CREAM)} {pct:.0f}%")


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
                yield Static("", id="gd-region", classes="row")
                yield Static("", id="gd-points", classes="row")
                yield Static("", id="gd-reward", classes="row")

    def _set(self, row_id: str, label: str, value: str, w: int = 9) -> None:
        self.query_one(f"#{row_id}", Static).update(f"[{QUBIC_LABEL}]{label:<{w}}[/] {value}")

    def _line(self, row_id: str, markup: str) -> None:
        self.query_one(f"#{row_id}", Static).update(markup)

    def update_score(self, node: dict | None, stats: dict | None) -> None:
        if node is None:
            self._line("gd-eligible", f"[{QUBIC_DIM}]node not yet tracked by Guardians[/]")
            for rid in ("gd-epoch", "gd-checks", "gd-region", "gd-final", "gd-uptime",
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

            region = ls.get("region") or node.get("country") or "-"
            mult = ls.get("regionMultiplier")
            if mult is None:
                self._set("gd-region", "Region", f"[{QUBIC_TEXT}]{region}[/]", 7)
            else:
                mcol = QUBIC_MINT if mult >= 1.0 else QUBIC_CREAM if mult >= 0.7 else QUBIC_CORAL
                self._set("gd-region", "Region", f"[{QUBIC_TEXT}]{region}[/] [{mcol}]×{mult:g}[/][{QUBIC_DIM}] weight[/]", 7)
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


# ─── Node Actions Panel (lite.sh lifecycle — NOT node console keys) ───────────────

# (action_id, label, description). Mirrors the lite.sh menu a Guardian operator uses.
# status + logs are shown live natively; nothing here touches the node console.
ACTION_BUTTONS = [
    ("install", "⬇ Install", "Install the Lite node (asks for seed + alias)"),
    ("start", "▶ Start", "Start the stopped node container"),
    ("stop", "⏹ Stop", "Graceful stop: ESC + 30s wait, then stop the container"),
    ("restart", "↻ Restart", "Restart the container (compose down + up)"),
    ("logs", "📜 Logs", "Open the full-screen live log viewer (500-line history, scroll, R=events-only)"),
    ("reconfigure", "✎ Reconfigure", "Change operator seed/alias and restart fresh"),
    ("snapshot", "◔ Snapshot", "Save a tick-storage snapshot now (lite.sh snapshot)"),
    ("watch", "📡 Snapshot DL", "Watch live snapshot download progress"),
    ("deploy", "↑ Deploy", "Deploy a specific Docker image tag (e.g. E207.2)"),
    ("update", "↺ Update", "Self-update lite.sh to the latest version"),
    ("reset", "⚠ Reset", "Wipe ALL node data, restart fresh (keeps seed/alias)"),
    ("uninstall", "✖ Uninstall", "Remove containers AND data directory (irreversible)"),
]
ACTION_DANGER = {"stop", "reset", "uninstall"}
# actions that bring the node down→up: drop the stale tick so SYNC re-initialises
ACTION_RESETS_SYNC = {"restart", "start", "reset", "reconfigure", "deploy", "install"}
# grouped like the classic `lite.sh -old` menu (INSTALL · MANAGE · OTHER); each
# group sits under a full-width divider header for a clear visual split. A group is
# (name, rows); each row is up to 4 action ids, padded to 4 cols so widths align.
ACTION_GROUPS = [
    ("INSTALL", [["install", "uninstall"]]),
    ("MANAGE",  [["start", "stop", "restart", "logs"],
                 ["reconfigure", "snapshot", "watch", "reset"]]),
    ("OTHER",   [["deploy", "update"]]),
]

# Number every action in display order (top→bottom, left→right) so each button's
# badge matches the "Run #" field — mirrors the numbered classic `lite.sh -old` menu.
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
        self.border_title = f" NODE ACTIONS · lite.sh · build {BUILD} (hover · H) "
        meta = {a: (label, desc) for a, label, desc in ACTION_BUTTONS}
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
                        yield Static("Run [1-12]:", classes="run-label")
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

    def write_entry(self, entry: LogLine) -> None:
        self.query_one("#log-output", RichLog).write(format_log_markup(entry))

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
            yield Static("Qubic Lite Guardian — Help", classes="help-title")
            with VerticalScroll():
                yield Static(self._build())
            yield Static("[ Esc / H / Q to close ]", classes="help-hint")

    def _build(self) -> str:
        lines = [
            f"[bold {QUBIC_CYAN}]What this shows[/]",
            f"  [{QUBIC_TEXT}]GUARDIAN[/]  your live score & reward from guardians.qubic.org.",
            f"            [{QUBIC_DIM}]Reward needs ≥1500 checks, uptime ≥70%, sync ≥50%.[/]",
            f"  [{QUBIC_TEXT}]SYNC[/]      node tick vs network. ≤{SYNC_BUFFER} ticks behind = full sync score.",
            f"  [{QUBIC_TEXT}]NODE[/]      local container health / version / uptime.",
            "",
            f"[bold {QUBIC_CYAN}]Keys[/]",
        ]
        for key, desc in [
            ("G", "Refresh Guardian score now"), ("R", "Log filter: full (default) ↔ events-only"),
            ("C", "Clear the log"), ("L", "Copy log: frozen full-screen, drag to select (no Shift)"),
            ("H", "This help"), ("Q", "Quit"),
        ]:
            lines.append(f"  [bold {QUBIC_TEAL}]{key:<4}[/] [{QUBIC_TEXT}]{desc}[/]")
        lines += ["", f"[bold {QUBIC_CYAN}]NODE ACTIONS[/] [{QUBIC_DIM}](lite.sh)[/]",
                  f"  [{QUBIC_DIM}]Click a button, or just press its number (1-12) — it jumps[/]\n"
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
                rl.write(format_log_markup(entry))
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
            rl.write(format_log_markup(entry))
        rl.scroll_end(animate=False)
        rl.focus()

    def action_dismiss(self) -> None:
        self._set_mouse(enabled=True)
        self.dismiss(None)

    async def on_unmount(self) -> None:
        self._set_mouse(enabled=True)


# ─── App ─────────────────────────────────────────────────────────────────────────


class GuardianApp(App):
    TITLE = "Qubic Lite Guardian"

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

    def __init__(self, container_name: str, http_port: int, lite_script: str | None,
                 api_base: str, operator: str | None) -> None:
        super().__init__()
        self._container_name = container_name
        self._http_port = http_port
        self._lite_script = lite_script
        self._operator = operator
        self._node = NodeClient(container_name, http_port)
        self._guardians = GuardiansClient(api_base)
        self._streamer = DockerLogStreamer(container_name)
        self._connected = False
        self._action_running = False
        self._events_only = False  # default: show every line, like the classic logs
        # sync state — node tick comes from the live log (freshest), ref from RPC
        self._node_tick: int | None = None
        self._initial_tick: int | None = None
        self._ref_tick: int | None = None
        self._epoch: int | None = None
        self._eta_text: str | None = None
        # sync ETA history: (timestamp, behind)
        self._behind_hist: list[tuple[float, int]] = []
        # snapshot bootstrap progress (None unless a chunked download is running)
        self._snap: dict | None = None

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

    async def _container_state(self) -> str:
        try:
            proc = await asyncio.create_subprocess_exec(
                "docker", "ps", "-a", "--filter", f"name=^{self._container_name}$",
                "--format", "{{.State}}",
                stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.DEVNULL)
            out, _ = await asyncio.wait_for(proc.communicate(), timeout=8)
            state = out.decode().strip()
            if state == "running":
                return f"[{QUBIC_MINT}]● running[/]"
            if state:
                return f"[{QUBIC_CREAM}]● {state}[/]"
            return f"[{QUBIC_DIM}]○ not installed[/]"
        except Exception:
            return f"[{QUBIC_DIM}]?[/]"

    @work(exclusive=True, group="node")
    async def poll_node(self) -> None:
        while True:
            try:
                node_panel = self.query_one(NodePanel)
                sync_panel = self.query_one(SyncPanel)
            except Exception:
                await asyncio.sleep(1)
                continue

            cstate = await self._container_state()
            status = tick = None
            try:
                status = await self._node.status()
            except Exception:
                pass
            try:
                tick = await self._node.tick_info()
            except Exception:
                pass

            self._connected = bool(status or tick)
            self._set_conn()
            node_panel.update_local(cstate, status, tick, self._snap)

            # capture operator for the guardian lookup
            if not self._operator and tick:
                op = tick.get("extraInfo", {}).get("operator")
                if op:
                    self._operator = op

            # polled tick (HTTP/orchestrator) lags; the live log is usually fresher,
            # so only adopt the polled tick when it is actually ahead of the log.
            polled_tick = polled_epoch = None
            if status and status.get("node", {}).get("tick"):
                t = status["node"]["tick"]
                polled_tick, polled_epoch = t.get("tick"), t.get("epoch")
            elif tick:
                polled_tick, polled_epoch = tick.get("tick"), tick.get("epoch")
            if polled_tick and (self._node_tick is None or polled_tick > self._node_tick):
                self._node_tick = polled_tick
            if polled_epoch:
                self._epoch = polled_epoch
            if tick and tick.get("initialTick"):
                self._initial_tick = tick["initialTick"]

            self._ref_tick = await fetch_network_reference()
            self._eta_text = self._eta(self._node_tick, self._ref_tick)
            self._render_sync()
            await asyncio.sleep(3)

    def _reset_sync_state(self) -> None:
        """A lifecycle action is restarting the node — drop the stale tick so the
        SYNC panel re-initialises from the fresh container instead of freezing on
        the pre-restart values (node tick is monotonic, so without this a restart
        would never show). The next poll / log line rebuilds it."""
        self._node_tick = None
        self._initial_tick = None
        self._epoch = None
        self._eta_text = None
        self._behind_hist.clear()
        self._snap = None
        self._render_sync()

    def _render_sync(self) -> None:
        try:
            sp = self.query_one(SyncPanel)
        except Exception:
            return
        # while a snapshot is being pulled the node isn't ticking — show the
        # download progress + ETA here instead of an empty SYNC panel
        if self._snap and not self._node_tick:
            sp.update_snapshot(self._snap)
        else:
            sp.update_sync(self._node_tick, self._ref_tick, self._epoch, self._eta_text,
                           self._initial_tick)

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
        # Least-squares slope of "behind" over the whole ~3 min window, not just
        # oldest-vs-now: the network reference tick jitters sample to sample, so a
        # single endpoint diff would flip to "not catching up" even while the node
        # was steadily closing the gap. The trend over many samples is stable.
        n = len(self._behind_hist)
        t_mean = sum(t for t, _ in self._behind_hist) / n
        b_mean = sum(b for _, b in self._behind_hist) / n
        var = sum((t - t_mean) ** 2 for t, _ in self._behind_hist)
        if var <= 0:
            return "warming up…"
        slope = sum((t - t_mean) * (b - b_mean) for t, b in self._behind_hist) / var
        closing = -slope  # ticks/sec the gap is shrinking; >0 means catching up
        if closing > 0:
            eta = int(behind / closing)
            return f"{_fmt_eta(eta)}  ({closing * 60:.0f} t/min)"
        return "∞ not catching up"

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
                # the log is the freshest tick source — keep the SYNC panel live
                if entry.tick:
                    if self._node_tick is None or entry.tick > self._node_tick:
                        self._node_tick = entry.tick
                        if entry.epoch:
                            self._epoch = entry.epoch
                        self._render_sync()
                    self._snap = None  # node is ticking → snapshot bootstrap done
                else:
                    # before the node ticks it may be pulling a chunked snapshot
                    self._scan_snapshot(entry.text, node_panel)
                # pull live peer/connection counts out of the per-tick line
                if entry.conns and entry.peers:
                    node_panel.update_net(entry.conns, entry.peers)
                # show every line by default; only hide per-tick spam if asked
                if self._events_only and entry.noise:
                    continue
                log_panel.write_entry(entry)
        except asyncio.CancelledError:
            pass
        except Exception as e:
            log_panel.write_note(f"[{QUBIC_CORAL}]log stream error: ", str(e))

    def _scan_snapshot(self, text: str, node_panel: NodePanel) -> None:
        """Pull chunked-snapshot progress out of a log line and surface it as the
        node Health (the HTTP API isn't up yet, so it'd otherwise read 'unreachable')."""
        m = RE_SNAP_DONE.search(text)
        if m:
            chunk, chunks, done, total, rate = (int(m.group(1)), int(m.group(2)),
                                                float(m.group(3)), float(m.group(4)), float(m.group(5)))
            prev = self._snap or {}
            # the reported aggregate rate is jumpy (9–35 MB/s) — smooth it for a stable ETA
            rates = (prev.get("rates", []) + [rate])[-8:]
            avg = sum(rates) / len(rates)
            eta = int((total - done) * 1024 / avg) if avg > 0 and chunk < chunks else None
            self._snap = {"chunk": chunk, "chunks": chunks, "done": done, "total": total,
                          "rate": rate, "rates": rates, "eta": eta,
                          "fetching": max(chunk, prev.get("fetching", 0))}
            node_panel.set_health_snapshot(self._snap)
            self._render_sync()
            return
        m = RE_SNAP_FETCH.search(text)
        if m and self._snap is not None:
            # a chunk just *started* downloading — parallel=3, so this leads 'done' by ~3
            f = int(m.group(1))
            if f > self._snap.get("fetching", 0):
                self._snap["fetching"] = f
                self._render_sync()
            return
        m = RE_SNAP_TOTAL.search(text)
        if m and self._snap is None:
            chunks, total = m.groups()
            self._snap = {"chunk": 0, "chunks": int(chunks), "done": 0.0,
                          "total": float(total), "rate": 0.0, "rates": [], "eta": None, "fetching": 0}
            node_panel.set_health_snapshot(self._snap)
            self._render_sync()

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
                      "events only: hiding per-tick lines (snapshots, votes, epoch, HTTP, warnings)"
                      if self._events_only else "full log: showing every line (like classic logs)")

    def action_help(self) -> None:
        self.push_screen(HelpScreen())

    # ─── lite.sh actions ────────────────────────────────────────────────────────
    def _lite_path(self) -> str | None:
        if self._lite_script and os.path.isfile(self._lite_script):
            return self._lite_script
        data_copy = os.path.join(DEFAULT_DATA_DIR, "lite.sh")
        return data_copy if os.path.isfile(data_copy) else None

    def _confirm_then(self, title: str, msg: str, cb) -> None:
        def _result(ok: bool) -> None:
            if ok:
                cb()
        self.push_screen(ConfirmDialog(title, msg), _result)

    async def on_actions_panel_action(self, message: ActionsPanel.Action) -> None:
        a = message.action
        if a == "stop":
            self._confirm_then("Stop Node?", "Graceful stop: ESC, ~30s wait, then container down.",
                               lambda: self._run_lite(["stop"], label="graceful stop ~60s"))
        elif a == "start":
            self._run_lite(["start"])
        elif a == "restart":
            self._run_lite(["restart"], label="container restart")
        elif a == "logs":
            self.push_screen(LogScreen(self._container_name, self._events_only))
        elif a == "snapshot":
            self._run_lite(["snapshot"], label="save snapshot")
        elif a == "watch":
            self._run_lite(["watch"], label="snapshot download progress")
        elif a == "info":
            self._run_lite(["info"])
        elif a == "update":
            self._run_lite(["update"], label="self-update lite.sh")
        elif a == "install":
            self._prompt_install()
        elif a == "reconfigure":
            self._prompt_reconfigure()
        elif a == "deploy":
            self._prompt_deploy()
        elif a == "reset":
            self._confirm_then("Reset Node?", "Wipe ALL node data and restart fresh. Seed/alias kept.",
                               lambda: self._run_lite(["reset"], stdin_data="y\n", label="reset (wipe data)"))
        elif a == "uninstall":
            self._confirm_then("Uninstall Node?", "Remove containers AND the data directory. Cannot be undone.",
                               lambda: self._run_lite(["uninstall"], stdin_data="y\n", label="uninstall"))

    def _prompt_install(self) -> None:
        def _done(res: dict | None) -> None:
            if not res:
                return
            seed, alias = res.get("seed", "").strip(), res.get("alias", "").strip()
            if not seed or not alias:
                self.query_one(LogPanel).write_note(f"[{QUBIC_CORAL}]» ", "install: seed and alias required")
                return
            self._run_lite(["install", "--seed", seed, "--alias", alias], label="install")
        self.push_screen(InputDialog("Install Lite Node", [
            ("seed", "Operator Seed", "55-char operator seed", True),
            ("alias", "Operator Alias", "my-node", False)]), _done)

    def _prompt_reconfigure(self) -> None:
        def _done(res: dict | None) -> None:
            if res is None:
                return
            seed, alias = res.get("seed", "").strip(), res.get("alias", "").strip()
            self._run_lite(["reconfigure"], stdin_data=f"{seed}\n{alias}\n",
                           label="reconfigure (blank = keep current)")
        self.push_screen(InputDialog("Reconfigure (blank = keep current)", [
            ("seed", "New Seed", "blank = keep current", True),
            ("alias", "New Alias", "blank = keep current", False)]), _done)

    def _prompt_deploy(self) -> None:
        def _done(res: dict | None) -> None:
            if res is None:
                return
            tag = res.get("tag", "").strip() or "latest"
            self._run_lite(["deploy", "--tag", tag], label=f"deploy {tag}")
        self.push_screen(InputDialog("Deploy Docker Tag", [("tag", "Image Tag", "latest / E207.2", False, "latest")]), _done)

    @work(group="lite_action")
    async def _run_lite(self, args: list[str], *, stdin_data: str | None = None,
                        label: str | None = None) -> None:
        log = self.query_one(LogPanel)
        if self._action_running:
            log.write_note(f"[{QUBIC_CREAM}]» ", "an action is already running, please wait…")
            return
        script = self._lite_path()
        if not script:
            log.write_note(f"[{QUBIC_CORAL}]» lite.sh not found: ",
                           str(self._lite_script or os.path.join(DEFAULT_DATA_DIR, "lite.sh")))
            return
        self._action_running = True
        if args and args[0] in ACTION_RESETS_SYNC:
            self._reset_sync_state()
        log.write_note(f"[{QUBIC_TEAL}]» lite.sh {markup_escape(' '.join(args))}  ",
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
    parser = argparse.ArgumentParser(prog="lite-guardian",
                                     description="Network Guardian dashboard for a Qubic Lite node")
    parser.add_argument("--container", default="qubic-lite", help="Docker container name")
    parser.add_argument("--http-port", type=int, default=41841, help="Lite HTTP API port")
    parser.add_argument("--operator", default=None, help="Operator ID (auto-detected if omitted)")
    parser.add_argument("--api-base", default=DEFAULT_API_BASE, help="Guardians API base URL")
    default_lite = os.path.join(os.path.dirname(os.path.abspath(__file__)), "lite.sh")
    parser.add_argument("--lite-script", default=default_lite, help="Path to lite.sh for node actions")
    args = parser.parse_args()
    GuardianApp(container_name=args.container, http_port=args.http_port,
                lite_script=args.lite_script, api_base=args.api_base,
                operator=args.operator).run()


if __name__ == "__main__":
    main()
