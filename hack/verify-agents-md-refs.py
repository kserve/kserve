#!/usr/bin/env python3

# Copyright 2026 The KServe Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Checks that the code the agent guides name still exists.
#
# Guides (AGENTS.md and GATES.md) point at code in backticks: a path, a
# `file:symbol`, or a bare identifier in an "In the tree" line or table column.
# A rename that misses the guide sends the next reader to code that is gone, so
# every reference must resolve against the working tree:
#
# - a path resolves relative to the guide's directory or the repository root,
#   or names the tail of a file in the tree; a glob matches at least one file
# - the symbol after `file:` occurs as a word in that file
# - a bare identifier in an "In the tree" line or column occurs as a word in a
#   non-Markdown file

import fnmatch
import os
import re
import subprocess
import sys

GUIDE_NAMES = ("AGENTS.md", "GATES.md")

# Paths the guides name outside this tree: in another repository, or in a
# local or downstream file the Makefile includes when present.
EXTERNAL = {
    "apix/",  # llm-d-inference-scheduler
    "pkg/kvcache/indexer.go",  # llm-d-kv-cache
    "*_odh.go",  # midstream distro files
    "Makefile.overrides.mk",
}

EXTENSIONS = "go|py|sh|mk|md|yaml|yml|json|env|txt|tpl|toml|cfg"
FILE_REF = re.compile(
    r"(?P<path>[\w.*/-]*[\w*-]\.(?:%s))(?::(?P<sym>[A-Za-z_][\w.]*))?" % EXTENSIONS
)
DIR_REF = re.compile(r"(?P<path>[\w.*-]+(?:/[\w.*-]+)*/)")
# camelCase, PascalCase with an inner capital, or SNAKE_CASE: plain words are
# too often prose to check.
IDENT = re.compile(r"[A-Za-z_][a-z0-9_]*(?:[A-Z_][A-Za-z0-9_]*)+")
BACKTICK = re.compile(r"`([^`\s]+)`")
TREE_LINE = re.compile(r"^\s*(?:[-*]\s+)?In the tree:")


def git(*args):
    return subprocess.run(
        ["git", *args], check=True, capture_output=True, text=True
    ).stdout


def tree_files():
    out = git("ls-files", "--cached", "--others", "--exclude-standard")
    return [p for p in out.splitlines() if os.path.exists(p)]


def guide_files(files):
    return sorted(p for p in files if os.path.basename(p) in GUIDE_NAMES)


def tree_columns(header):
    cells = [c.strip() for c in header.strip().strip("|").split("|")]
    return {i for i, c in enumerate(cells) if c.lower() == "in the tree"}


def references(guide):
    """Yields (line number, token, in a tree line or column)."""
    columns = set()
    with open(guide, encoding="utf-8") as f:
        lines = f.read().splitlines()
    for n, line in enumerate(lines, 1):
        if line.lstrip().startswith("|"):
            if n < len(lines) and re.match(r"^\s*\|[\s:|-]+\|\s*$", lines[n]):
                columns = tree_columns(line)
            cells = line.strip().strip("|").split("|")
            for i, cell in enumerate(cells):
                for tok in BACKTICK.findall(cell):
                    yield n, tok, i in columns
            continue
        columns = set()
        in_tree = bool(TREE_LINE.match(line))
        for tok in BACKTICK.findall(line):
            yield n, tok, in_tree


def resolve(path, guide_dir, files):
    """Returns the files a path names, an empty list when it names none."""
    path = path.replace("X.Y.Z", "*")
    if path.startswith("_"):
        path = "*" + path
    if "*" in path:
        if path.endswith("/"):
            path += "*"
        patterns = [os.path.join(guide_dir, path), path, "*/" + path]
        return [p for p in files if any(fnmatch.fnmatch(p, g) for g in patterns)]
    if path.endswith("/"):
        hit = [d for d in (os.path.join(guide_dir, path), path) if os.path.isdir(d)]
        return hit or [p for p in files if ("/" + p).find("/" + path) >= 0][:1]
    for cand in (os.path.join(guide_dir, path), path):
        if os.path.isfile(cand):
            return [cand]
    return [p for p in files if p == path or p.endswith("/" + path)]


def has_words(path, words):
    with open(path, encoding="utf-8", errors="replace") as f:
        text = f.read()
    return all(re.search(r"\b%s\b" % re.escape(w), text) for w in words)


def code_words(idents):
    """Returns the identifiers that occur as a word outside Markdown."""
    if not idents:
        return set()
    args = ["grep", "--untracked", "-ohwF"]
    for i in sorted(idents):
        args += ["-e", i]
    args += ["--", ".", ":!*.md"]
    r = subprocess.run(["git", *args], capture_output=True, text=True)
    return set(r.stdout.split())


def main():
    os.chdir(git("rev-parse", "--show-toplevel").strip())
    files = tree_files()
    errors = []
    idents = {}
    for guide in guide_files(files):
        guide_dir = os.path.dirname(guide)
        for n, tok, in_tree in references(guide):
            if tok in EXTERNAL or any(c in tok for c in "<>${}()") or "..." in tok:
                continue
            where = "%s:%d: `%s`" % (guide, n, tok)
            m = FILE_REF.fullmatch(tok) or DIR_REF.fullmatch(tok)
            if m:
                hits = resolve(m.group("path"), guide_dir, files)
                sym = m.groupdict().get("sym")
                if not hits:
                    errors.append("%s: no such path" % where)
                elif sym and not any(
                    os.path.isfile(h) and has_words(h, sym.split(".")) for h in hits
                ):
                    errors.append("%s: %s not found in %s" % (where, sym, hits[0]))
            elif in_tree and IDENT.fullmatch(tok):
                idents.setdefault(tok, []).append(where)
    found = code_words(set(idents))
    for ident, wheres in sorted(idents.items()):
        if ident not in found:
            errors.extend("%s: not found outside Markdown" % w for w in wheres)
    if errors:
        print("\n".join(sorted(errors)))
        print(
            "\nAn agent guide names code that is not in the tree. "
            "Point the guide line at the new name, or drop the reference."
        )
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
