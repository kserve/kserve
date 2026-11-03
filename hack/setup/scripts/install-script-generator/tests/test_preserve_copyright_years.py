#!/usr/bin/env python3

"""Tests for the copyright-year restoration helper."""

import importlib.util
from pathlib import Path


HELPER_PATH = Path(__file__).parents[3] / "preserve-copyright-years.py"
SPEC = importlib.util.spec_from_file_location("preserve_copyright_years", HELPER_PATH)
assert SPEC is not None and SPEC.loader is not None
preserve_copyright_years = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(preserve_copyright_years)


def test_restore_copyright_years_updates_cached_files(tmp_path):
    source = tmp_path / "pkg" / "example.go"
    source.parent.mkdir()
    source.write_text(
        "// Copyright 2026 The KServe Authors.\npackage example\n",
        encoding="utf-8",
    )
    cache = tmp_path / "copyright-years.tsv"
    cache.write_text("pkg/example.go\t2019\n", encoding="utf-8")

    preserve_copyright_years.restore_copyright_years(cache, tmp_path)

    assert source.read_text(encoding="utf-8") == (
        "// Copyright 2019 The KServe Authors.\npackage example\n"
    )


def test_restore_copyright_years_ignores_missing_files(tmp_path):
    cache = tmp_path / "copyright-years.tsv"
    cache.write_text("pkg/missing.go\t2019\n", encoding="utf-8")

    preserve_copyright_years.restore_copyright_years(cache, tmp_path)


def test_restore_copyright_years_preserves_unrelated_content(tmp_path):
    source = tmp_path / "example.py"
    source.write_text(
        "# Copyright 2026 The KServe Authors.\n"
        "# Copyright notice in a string: Copyright 2026 The KServe Authors.\n",
        encoding="utf-8",
    )
    cache = tmp_path / "copyright-years.tsv"
    cache.write_text("example.py\t2024\n", encoding="utf-8")

    preserve_copyright_years.restore_copyright_years(cache, tmp_path)

    assert source.read_text(encoding="utf-8") == (
        "# Copyright 2024 The KServe Authors.\n"
        "# Copyright notice in a string: Copyright 2024 The KServe Authors.\n"
    )
