#!/usr/bin/env python3
"""Rendered PTY smoke test for the curl installer's interactive five-step flow."""

import fcntl
import hashlib
import os
import platform
import pty
import re
import select
import shutil
import subprocess
import tempfile
import termios
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def release_target() -> tuple[str, str, str]:
    systems = {"Linux": "linux", "Darwin": "darwin"}
    machines = {"x86_64": "x64", "AMD64": "x64", "aarch64": "arm64", "arm64": "arm64"}
    system = systems.get(platform.system())
    machine = machines.get(platform.machine())
    if not system or not machine:
        raise RuntimeError(f"unsupported PTY fixture host: {platform.system()}/{platform.machine()}")
    return system, machine, ""


def main() -> None:
    system, machine, suffix = release_target()
    fixture = Path(tempfile.mkdtemp(prefix="ani-tui-installer-pty-"))
    try:
        release = fixture / "release"
        release.mkdir()
        asset = f"ani-tui-{system}-{machine}{suffix}"
        binary = b"#!/bin/sh\nprintf '%s\\n' 'ani-tui v9.9.9-pty (commit fixture)'\n"
        (release / asset).write_bytes(binary)
        (release / asset).chmod(0o755)
        digest = hashlib.sha256(binary).hexdigest()
        (release / "checksums.txt").write_text(f"{digest}  {asset}\n", encoding="utf-8")

        env = os.environ.copy()
        env.update(
            HOME=str(fixture / "home"),
            SHELL="/bin/bash",
            ANI_TUI_DOWNLOAD_BASE=release.as_uri(),
        )

        def run_installer() -> tuple[int, str]:
            master, slave = pty.openpty()

            def make_controlling_terminal() -> None:
                os.setsid()
                fcntl.ioctl(slave, termios.TIOCSCTTY, 0)

            process = subprocess.Popen(
                ["sh", str(ROOT / "install.sh"), "--version", "v9.9.9"],
                stdin=slave,
                stdout=slave,
                stderr=slave,
                env=env,
                close_fds=True,
                preexec_fn=make_controlling_terminal,
            )
            os.close(slave)

            output = bytearray()
            answered: set[bytes] = set()
            prompts = {
                b"Choose [1]:": b"1\n",
                b"Replace the existing": b"\n",
                b"to your shell PATH?": b"\n",
            }
            deadline = time.monotonic() + 20
            while process.poll() is None and time.monotonic() < deadline:
                ready, _, _ = select.select([master], [], [], 0.2)
                if not ready:
                    continue
                try:
                    output.extend(os.read(master, 65536))
                except OSError:
                    break
                for marker, response in prompts.items():
                    if marker in output and marker not in answered:
                        os.write(master, response)
                        answered.add(marker)
            if process.poll() is None:
                process.kill()
                process.wait()
                raise RuntimeError("interactive installer timed out")
            while True:
                ready, _, _ = select.select([master], [], [], 0.05)
                if not ready:
                    break
                try:
                    output.extend(os.read(master, 65536))
                except OSError:
                    break
            os.close(master)

            rendered = re.sub(
                r"\x1b\[[0-9;?]*[ -/]*[@-~]",
                "",
                output.decode("utf-8", "replace"),
            ).replace("\r", "")
            return process.returncode, rendered

        returncode, rendered = run_installer()
        markers = (
            "[1/5] Detecting your system",
            "Installation scope:",
            "[2/5] Downloading",
            "[3/5] Verifying SHA-256 checksum",
            "Verified",
            "[4/5] Installing the global ani-tui command",
            "Updated   " + str(fixture / "home" / ".bashrc"),
            "[5/5] Ready",
            "Launch with:  ani-tui",
        )
        missing = [marker for marker in markers if marker not in rendered]
        if returncode != 0 or missing:
            raise RuntimeError(
                f"installer status={returncode}, missing={missing}\n{rendered}"
            )

        installed = fixture / "home" / ".local" / "bin" / "ani-tui"
        if installed.stat().st_mode & 0o777 != 0o755:
            raise RuntimeError(f"installed mode is {installed.stat().st_mode & 0o777:o}, want 755")
        profile_path = fixture / "home" / ".bashrc"
        profile = profile_path.read_text(encoding="utf-8")
        path_line = f"export PATH='{installed.parent}':\"$PATH\""
        if profile.count(path_line) != 1:
            raise RuntimeError(f"profile does not contain one safe PATH update: {profile!r}")

        second_returncode, second_rendered = run_installer()
        if second_returncode != 0:
            raise RuntimeError(f"repeat installer status={second_returncode}\n{second_rendered}")
        profile = profile_path.read_text(encoding="utf-8")
        if profile.count(path_line) != 1:
            raise RuntimeError(f"repeat install duplicated the PATH update: {profile!r}")
        if "to your shell PATH?" in second_rendered:
            raise RuntimeError(
                "repeat install offered an already configured PATH update\n"
                f"profile={profile!r}\n{second_rendered}"
            )
        print(
            "interactive installer PTY passed all five rendered steps, permissions, "
            "and idempotent PATH update"
        )
    finally:
        shutil.rmtree(fixture, ignore_errors=True)


if __name__ == "__main__":
    main()
