#!/usr/bin/env python3
"""Select iTerm2 tab by tmux pane ID for iTerm2 + tmux.

Usage:
    iterm2-select-tab.py --pane %42 --tmux-path /opt/homebrew/bin/tmux
    iterm2-select-tab.py --list

Requires:
    - iterm2 Python module (pip install iterm2)
    - iTerm2 with 'Enable Python API' enabled in Settings > General > Magic
"""
import argparse
import os
import subprocess
import sys

EXIT_PYTHON_API_DISABLED = 11
EXIT_MODULE_MISSING = 12
EXIT_OTHER = 13

try:
    import iterm2
except ImportError:
    print("iterm2 module not installed. Run: pip install iterm2", file=sys.stderr)
    sys.exit(EXIT_MODULE_MISSING)


def parse_args():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pane", help="tmux pane ID, for example %%42")
    parser.add_argument(
        "--termid",
        help="exact iTerm2 session termid (matches $ITERM_SESSION_ID)",
    )
    parser.add_argument(
        "--cwd",
        default="",
        help="fallback working directory for unique-session matching",
    )
    parser.add_argument(
        "--session",
        default="",
        help="tmux or zmx session name; when no tab shows it, an open si/tld window shows it",
    )
    parser.add_argument(
        "--tmux-path",
        default="tmux",
        help="absolute path to tmux binary",
    )
    parser.add_argument(
        "--socket",
        default="",
        help="optional tmux socket path from the TMUX environment",
    )
    parser.add_argument(
        "--list",
        action="store_true",
        help="list iTerm2 tabs with tmuxWindowPane and tty variables",
    )
    parser.add_argument(
        "--healthcheck",
        action="store_true",
        help="check that the iTerm2 Python API is reachable",
    )
    args = parser.parse_args()
    if not args.list and not args.healthcheck and not args.pane and not args.termid and not args.cwd and not args.session:
        parser.error("--pane, --termid, --cwd or --session is required unless --list is used")
    return args


def normalized_pane(target_pane):
    target = str(target_pane).strip()
    if not target:
        raise ValueError("tmux pane target is empty")
    if target.startswith("%"):
        return target
    return f"%{target}"


def normalized_termid(termid):
    target = str(termid or "").strip()
    if not target:
        return ""

    # iTerm2 exports ITERM_SESSION_ID as wXtYpZ:UUID, while the Python API
    # exposes session variable "termid" as wXtYpZ.UUID. Normalize both forms
    # so exact tab/pane targeting keeps working across notification clicks.
    prefix, separator, suffix = target.partition(":")
    if separator and prefix.startswith("w") and suffix:
        return f"{prefix}.{suffix}"
    return target


def run_tmux(tmux_path, socket_path, tmux_args):
    cmd = [tmux_path]
    if socket_path:
        cmd.extend(["-S", socket_path])
    cmd.extend(tmux_args)

    result = subprocess.run(
        cmd,
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        stderr = (result.stderr or "").strip()
        raise RuntimeError(stderr or f"tmux command failed: {' '.join(tmux_args)}")
    return (result.stdout or "").strip()


def get_session_client_ttys(tmux_path, socket_path, pane_target):
    session_name = run_tmux(
        tmux_path,
        socket_path,
        ["display-message", "-t", pane_target, "-p", "#{session_name}"],
    )
    if not session_name:
        raise RuntimeError(f"tmux session lookup returned empty for {pane_target}")

    client_lines = run_tmux(
        tmux_path,
        socket_path,
        ["list-clients", "-F", "#{client_session}\t#{client_tty}"],
    )

    client_ttys = []
    for line in client_lines.splitlines():
        parts = line.split("\t", 1)
        if len(parts) != 2:
            continue
        client_session, client_tty = parts
        if client_session == session_name and client_tty:
            client_ttys.append(client_tty)

    if not client_ttys:
        raise RuntimeError(f"no tmux client tty found for session {session_name}")
    return client_ttys


def focus_tmux_client(tmux_path, socket_path, pane_target, client_tty):
    run_tmux(
        tmux_path,
        socket_path,
        ["switch-client", "-c", client_tty, "-t", pane_target],
    )
    run_tmux(
        tmux_path,
        socket_path,
        ["select-window", "-t", pane_target],
    )
    run_tmux(
        tmux_path,
        socket_path,
        ["select-pane", "-t", pane_target],
    )


async def select_tab(connection, target_pane, tmux_path, socket_path):
    """Find and select the iTerm2 tab for the target pane.

    Strategy:
    1. tmuxWindowPane match for tmux -CC mode
    2. tty match for plain tmux, then targeted switch-client/select-window/select-pane
    """
    pane_target = normalized_pane(target_pane)
    pane_number = pane_target[1:] if pane_target.startswith("%") else pane_target
    app = await iterm2.async_get_app(connection)
    tabs = []
    for window in app.windows:
        for tab in window.tabs:
            tab_ttys = set()
            for session in tab.sessions:
                wp = await session.async_get_variable("tmuxWindowPane")
                if wp is not None and str(wp) == pane_number:
                    await app.async_activate(
                        raise_all_windows=False,
                        ignoring_other_apps=True,
                    )
                    await tab.async_activate(order_window_front=True)
                    return True
                tty = await session.async_get_variable("tty")
                if tty:
                    tab_ttys.add(str(tty))
            tabs.append((window, tab, tab_ttys))

    client_ttys = get_session_client_ttys(tmux_path, socket_path, pane_target)
    for window, tab, tab_ttys in tabs:
        for client_tty in client_ttys:
            if client_tty in tab_ttys:
                await app.async_activate(
                    raise_all_windows=False,
                    ignoring_other_apps=True,
                )
                await tab.async_activate(order_window_front=True)
                focus_tmux_client(tmux_path, socket_path, pane_target, client_tty)
                return True
    return False


TLD_OPEN_TIMEOUT = 3


def tld_open(session_name):
    """Ask an open si/tld window (~/.claude/scripts/tld.py) to show the session on its right.

    Contract of `tld.py open <session>`: rc 0 and one stdout line with the tty of the terminal
    that shows the window; rc 1 when no window is open; rc 2 when the session is unknown.
    Returns the tty or ''.
    """
    script = os.environ.get("JADLIS_TLD_SCRIPT") or os.path.expanduser("~/.claude/scripts/tld.py")
    if not session_name or not os.access(script, os.X_OK):
        return ""
    env = dict(os.environ, TMUX="")
    if not env.get("TMPDIR"):
        # The window keeps its state files in the per-user temp dir; a click handler may lack it.
        try:
            env["TMPDIR"] = subprocess.run(
                ["/usr/bin/getconf", "DARWIN_USER_TEMP_DIR"],
                capture_output=True, text=True, timeout=2,
            ).stdout.strip()
        except (OSError, subprocess.SubprocessError):
            pass
    try:
        result = subprocess.run(
            [script, "open", session_name],
            capture_output=True, text=True, env=env, timeout=TLD_OPEN_TIMEOUT,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        print(f"tld open {session_name}: {exc}", file=sys.stderr)
        return ""
    tty = (result.stdout or "").strip().splitlines()[-1:] or [""]
    tty = tty[0].strip()
    if result.returncode != 0 or not tty.startswith("/dev/"):
        return ""
    return tty


async def select_by_tty(connection, tty):
    """Activate the iTerm2 session whose tty is tty (the si window's right half)."""
    app = await iterm2.async_get_app(connection)
    for window in app.windows:
        for tab in window.tabs:
            for session in tab.sessions:
                if str(await session.async_get_variable("tty") or "") == tty:
                    await app.async_activate(
                        raise_all_windows=False,
                        ignoring_other_apps=True,
                    )
                    await session.async_activate(
                        select_tab=True,
                        order_window_front=True,
                    )
                    return True
    return False


async def select_via_tld(connection, session_name):
    tty = tld_open(session_name)
    return bool(tty) and await select_by_tty(connection, tty)


def tmux_session_name(tmux_path, socket_path, pane_target):
    try:
        return run_tmux(
            tmux_path,
            socket_path,
            ["display-message", "-t", normalized_pane(pane_target), "-p", "#{session_name}"],
        )
    except (RuntimeError, ValueError, OSError):
        return ""


async def select_session(connection, target_termid, target_cwd):
    """Find and activate an iTerm2 session directly.

    Strategy:
    1. Exact termid match (ITERM_SESSION_ID) for precise tab/pane targeting
    2. Unique cwd match as a safe fallback
    """
    app = await iterm2.async_get_app(connection)
    target_termid = normalized_termid(target_termid)
    cwd_matches = []

    for window in app.windows:
        for tab in window.tabs:
            for session in tab.sessions:
                termid = normalized_termid(await session.async_get_variable("termid"))
                if target_termid and termid == target_termid:
                    await app.async_activate(
                        raise_all_windows=False,
                        ignoring_other_apps=True,
                    )
                    await session.async_activate(
                        select_tab=True,
                        order_window_front=True,
                    )
                    return True

                path = await session.async_get_variable("path")
                if target_cwd and path == target_cwd:
                    cwd_matches.append(session)

    if not target_cwd:
        return False

    if len(cwd_matches) == 1:
        await app.async_activate(
            raise_all_windows=False,
            ignoring_other_apps=True,
        )
        await cwd_matches[0].async_activate(
            select_tab=True,
            order_window_front=True,
        )
        return True

    if len(cwd_matches) > 1:
        raise RuntimeError(f"multiple iTerm2 sessions match cwd {target_cwd}")

    return False


async def list_tabs(connection):
    """List all iTerm2 tabs with their tmuxWindowPane and tty mappings."""
    app = await iterm2.async_get_app(connection)
    for window in app.windows:
        print(f"Window: {window.window_id}")
        for i, tab in enumerate(window.tabs):
            for session in tab.sessions:
                wp = await session.async_get_variable("tmuxWindowPane")
                tty = await session.async_get_variable("tty")
                termid = await session.async_get_variable("termid")
                path = await session.async_get_variable("path")
                print(f"  Tab {i}: tmuxWindowPane={wp} tty={tty} termid={termid} path={path}")


async def main(connection):
    args = parse_args()

    if args.healthcheck:
        await iterm2.async_get_app(connection)
        return

    if args.list:
        await list_tabs(connection)
        return

    if args.pane:
        try:
            if await select_tab(connection, args.pane, args.tmux_path, args.socket):
                return
        except RuntimeError as exc:
            # Typical: "no tmux client tty found" — the session has no tab of its own.
            print(str(exc), file=sys.stderr)
        name = args.session or tmux_session_name(args.tmux_path, args.socket, args.pane)
        if await select_via_tld(connection, name):
            return
        print(f"No tab found for tmux pane {args.pane}", file=sys.stderr)
        sys.exit(1)

    # Order: the exact iTerm2 session → an open si/tld window (zmx sessions run without
    # ITERM_SESSION_ID) → a unique cwd match.
    if args.termid and await select_session(connection, args.termid, ""):
        return
    if args.session and await select_via_tld(connection, args.session):
        return
    if args.cwd and await select_session(connection, "", args.cwd):
        return
    detail = args.termid or args.session or args.cwd
    print(f"No iTerm2 session found for {detail}", file=sys.stderr)
    sys.exit(1)


if __name__ == "__main__":
    try:
        iterm2.run_until_complete(main)
    except Exception as e:
        print(f"iTerm2 API error: {e}", file=sys.stderr)
        print(
            "Ensure 'Enable Python API' is checked in "
            "iTerm2 > Settings > General > Magic",
            file=sys.stderr,
        )
        message = str(e)
        if "not enabled" in message.lower() or "problem connecting to iterm2" in message.lower():
            sys.exit(EXIT_PYTHON_API_DISABLED)
        sys.exit(EXIT_OTHER)
