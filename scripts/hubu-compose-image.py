#!/usr/bin/env python3
"""Read or atomically update only the backend image in the HUBU compose file."""

from __future__ import annotations

import os
import re
import stat
import sys
import tempfile
from pathlib import Path


def backend_image_line(lines: list[str]) -> tuple[int, re.Match[str]]:
    services = next((i for i, line in enumerate(lines) if line.strip() == "services:"), None)
    if services is None:
        raise ValueError("compose services section is missing")
    start = next((i for i in range(services + 1, len(lines)) if re.match(r"^  backend:\s*$", lines[i])), None)
    if start is None:
        raise ValueError("compose backend service is missing")
    end = next(
        (i for i in range(start + 1, len(lines)) if re.match(r"^  [A-Za-z0-9_-]+:\s*$", lines[i])),
        len(lines),
    )
    matches = [(i, re.match(r"^(    image:\s*)([^\s#]+)(.*)$", lines[i].rstrip("\n"))) for i in range(start + 1, end)]
    matches = [(i, match) for i, match in matches if match]
    if len(matches) != 1:
        raise ValueError("compose backend must contain exactly one image entry")
    return matches[0]


def get_image(path: Path) -> str:
    lines = path.read_text(encoding="utf-8").splitlines()
    _, match = backend_image_line(lines)
    return match.group(2)


def set_image(path: Path, expected: str, replacement: str) -> None:
    lines = path.read_text(encoding="utf-8").splitlines(keepends=True)
    index, match = backend_image_line(lines)
    if match.group(2) != expected:
        raise ValueError("compose backend image changed; refusing unexpected update")
    ending = "\n" if lines[index].endswith("\n") else ""
    lines[index] = match.group(1) + replacement + match.group(3) + ending
    previous = path.stat()
    fd, temporary = tempfile.mkstemp(prefix=".compose.yml.", dir=path.parent, text=True)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            stream.writelines(lines)
        os.chmod(temporary, stat.S_IMODE(previous.st_mode))
        os.chown(temporary, previous.st_uid, previous.st_gid)
        os.replace(temporary, path)
    except Exception:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass
        raise


def main(argv: list[str]) -> int:
    if len(argv) not in (2, 4):
        raise SystemExit("usage: hubu-compose-image.py get <compose-file> | set <compose-file> <expected> <replacement>")
    mode, filename = argv[:2]
    path = Path(filename)
    if mode == "get" and len(argv) == 2:
        print(get_image(path))
        return 0
    if mode == "set" and len(argv) == 4:
        set_image(path, argv[2], argv[3])
        return 0
    raise SystemExit("invalid mode or arguments")


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except (OSError, ValueError) as error:
        print(f"HUBU compose image update refused: {error}", file=sys.stderr)
        raise SystemExit(1)
