"""CLI interface for the test-selector tool."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


def find_repo_root(start: Path | None = None) -> Path:
    """Walk up from start to find the repo root (contains go.mod)."""
    current = start or Path.cwd()
    while current != current.parent:
        if (current / "go.mod").exists() and (current / "pkg").is_dir():
            return current
        current = current.parent
    raise SystemExit("Could not find repo root (no go.mod found)")


def cmd_learn(args: argparse.Namespace) -> None:
    from .mapping.builder import build_mapping

    repo_root = find_repo_root(Path(args.repo) if args.repo else None)
    mapping = build_mapping(repo_root)

    output_path = repo_root / "tools" / "test_selector" / "mapping.json"
    output_path.write_text(json.dumps(mapping.to_dict(), indent=2) + "\n")
    print(f"Mapping written to {output_path}", file=sys.stderr)

    ep_count = len(mapping.entrypoints)
    test_count = len(mapping.test_files)
    pkg_count = len(mapping.go_file_to_package)
    print(
        f"Discovered: {ep_count} entrypoints, {pkg_count} Go file mappings, "
        f"{test_count} test files",
        file=sys.stderr,
    )


def cmd_query(args: argparse.Namespace) -> None:
    from .mapping.loader import load_mapping
    from .output.formatter import format_selection
    from .selector.engine import select_tests

    repo_root = find_repo_root(Path(args.repo) if args.repo else None)

    if args.changed_files:
        changed_files = args.changed_files
    else:
        changed_files = [line.strip() for line in sys.stdin if line.strip()]

    if not changed_files:
        print("{}", file=sys.stdout)
        return

    mapping = load_mapping(repo_root)
    selection = select_tests(mapping, changed_files, repo_root)
    print(format_selection(selection, args.format), file=sys.stdout)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="test-selector",
        description="CI-agnostic test selector using AST and dependency analysis",
    )
    parser.add_argument(
        "--repo",
        default=None,
        help="Path to repo root (auto-detected if omitted)",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("learn", help="Scan repo and build mapping.json")

    query_parser = sub.add_parser(
        "query",
        help="Select tests for changed files (reads from stdin or --changed-files)",
    )
    query_parser.add_argument(
        "--changed-files",
        nargs="*",
        default=None,
        help="Changed file paths (reads stdin if omitted)",
    )
    query_parser.add_argument(
        "--format",
        choices=["json", "yaml"],
        default="json",
        help="Output format (default: json)",
    )

    return parser


def main() -> None:
    parser = build_parser()
    args = parser.parse_args()

    if args.command == "learn":
        cmd_learn(args)
    elif args.command == "query":
        cmd_query(args)
