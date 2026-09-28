#!/usr/bin/env python3
"""Verify an explicit legacy migration baseline, never infer the deployed app SHA."""
import hashlib
import pathlib
import re
import subprocess
import sys


def verify(base, ledger):
    if not re.fullmatch(r"[0-9a-f]{40}", base):
        raise ValueError("baseline must be a complete Git SHA")
    subprocess.run(["git", "cat-file", "-e", base + "^{commit}"], check=True)
    paths = subprocess.check_output(
        ["git", "ls-tree", "-r", "--name-only", base, "backend/migrations"], text=True
    ).splitlines()
    expected = {}
    for path in paths:
        if path.endswith(".sql"):
            content = subprocess.check_output(["git", "show", base + ":" + path])
            expected[pathlib.PurePosixPath(path).name] = hashlib.sha256(
                content.decode("utf-8").strip().encode("utf-8")
            ).hexdigest()
    actual = {}
    for line in ledger.splitlines():
        fields = line.split("\t")
        if len(fields) != 2 or not re.fullmatch(r"[0-9a-f]{64}", fields[1]):
            raise ValueError("invalid migration ledger row")
        name, checksum = fields
        if name in actual:
            raise ValueError("duplicate migration: " + name)
        actual[name] = checksum
    if not expected or actual != expected:
        missing = sorted(expected.keys() - actual.keys())
        extra = sorted(actual.keys() - expected.keys())
        changed = sorted(k for k in actual.keys() & expected.keys() if actual[k] != expected[k])
        raise ValueError(f"ledger mismatch: missing={missing}, extra={extra}, changed={changed}")
    print(f"Verified migration baseline {base}: {len(actual)} exact checksums; legacy app SHA remains unknown")


if __name__ == "__main__":
    verify(sys.argv[1], pathlib.Path(sys.argv[2]).read_text())
