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
    from .selector.matcher import expression_matches, extract_positive_markers

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

    if args.match_jobs is not None:
        selected = set(selection.e2e_tests.markers)
        jobs = json.loads(args.match_jobs)
        for job_name, expr in jobs.items():
            matched = expression_matches(expr, selected)
            print(f"{job_name}={str(matched).lower()}")
        return

    if args.match is not None:
        selected = set(selection.e2e_tests.markers)
        expr_markers = extract_positive_markers(args.match)
        matched = expression_matches(args.match, selected)
        result = {
            **selection.to_dict(),
            "match": matched,
            "match_details": {
                "expression": args.match,
                "expression_markers": sorted(expr_markers),
                "matched_markers": sorted(expr_markers & selected),
            },
        }
        print(json.dumps(result, indent=2), file=sys.stdout)
        if matched:
            print(
                f"MATCH: expression has markers in selected set "
                f"({', '.join(sorted(expr_markers & selected))})",
                file=sys.stderr,
            )
        else:
            print(
                f"NO MATCH: none of {sorted(expr_markers)} in selected markers",
                file=sys.stderr,
            )
        sys.exit(0 if matched else 1)

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
    query_parser.add_argument(
        "--match",
        default=None,
        help="Marker expression to evaluate against selected markers "
        "(exit 0 if match, 1 if no match)",
    )
    query_parser.add_argument(
        "--match-jobs",
        default=None,
        help="JSON object mapping job names to marker expressions. "
        "Outputs job=true/false lines (for $GITHUB_OUTPUT).",
    )

    return parser


def main() -> None:
    parser = build_parser()
    args = parser.parse_args()

    if args.command == "learn":
        cmd_learn(args)
    elif args.command == "query":
        cmd_query(args)
