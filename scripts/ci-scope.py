#!/usr/bin/env python3
"""Select checks from changed paths; unknown runtime inputs fail closed to all."""
import os
import subprocess


def classify(paths):
    scopes = set()
    for path in paths:
        if path.startswith(("frontend/", "docs/legal/")):
            scopes.update(("frontend", "build"))
        elif path.startswith("backend/"):
            scopes.update(("backend", "build"))
        elif path.startswith("deploy/") or path == ".dockerignore":
            scopes.update(("build", "operations"))
        elif path.startswith(("scripts/", ".github/")):
            scopes.add("operations")
        elif path.startswith(("docs/", "design-evidence/")) or path.lower().endswith((".md", ".txt")) or path in ("LICENSE", ".gitignore"):
            continue
        else:
            scopes.update(("frontend", "backend", "build", "operations"))
    return scopes


def main():
    base, head = os.environ.get("BASE_SHA", ""), os.environ["HEAD_SHA"]
    if not base or set(base) == {"0"}:
        scopes = {"frontend", "backend", "build", "operations"}
    else:
        if os.environ.get("IS_PR") == "true":
            base = subprocess.check_output(["git", "merge-base", base, head], text=True).strip()
        # --no-renames includes both sides of renames, so moving code out of a
        # runtime directory still runs its tests. -z preserves unusual filenames.
        paths = subprocess.check_output(["git", "diff", "--no-renames", "--name-only", "-z", base, head]).decode().split("\0")
        scopes = classify(filter(None, paths))
    with open(os.environ["GITHUB_OUTPUT"], "a") as output:
        for scope in ("frontend", "backend", "build", "operations"):
            print(f"{scope}={str(scope in scopes).lower()}", file=output)
    print("Checks required:", ", ".join(sorted(scopes)) or "documentation only")


if __name__ == "__main__":
    main()
