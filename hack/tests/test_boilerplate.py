#!/usr/bin/env python3

"""Tests for the boilerplate checker."""

import importlib.util
from pathlib import Path
from subprocess import CompletedProcess, run


HELPER_PATH = Path(__file__).parents[1] / "boilerplate.py"
SPEC = importlib.util.spec_from_file_location("boilerplate", HELPER_PATH)
assert SPEC is not None and SPEC.loader is not None
boilerplate = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(boilerplate)


def test_collect_source_files_uses_git_visible_files(monkeypatch, tmp_path):
    visible_files = (
        b"pkg/controller.go\0"
        b"python/client.py\0"
        b"python/client_pb2.py\0"
        b"test/e2e/fixture.py\0"
    )
    for relative_path in (
        "pkg/controller.go",
        "python/client.py",
        "python/client_pb2.py",
        "test/e2e/fixture.py",
    ):
        path = tmp_path / relative_path
        path.parent.mkdir(parents=True, exist_ok=True)
        path.touch()

    def fake_run(command, **kwargs):
        assert command[:5] == [
            "git",
            "ls-files",
            "-z",
            "--cached",
            "--others",
        ]
        assert kwargs["cwd"] == tmp_path
        return CompletedProcess(command, 0, stdout=visible_files)

    monkeypatch.setattr(boilerplate.subprocess, "run", fake_run)

    assert boilerplate.collect_source_files(tmp_path) == [
        tmp_path / "pkg/controller.go",
        tmp_path / "python/client.py",
        tmp_path / "test/e2e/fixture.py",
    ]


def test_collect_source_files_includes_files_at_source_root(tmp_path):
    root_file = tmp_path / "test" / "e2e" / "conftest.py"
    nested_file = tmp_path / "test" / "e2e" / "fixtures" / "fixture.py"
    nested_file.parent.mkdir(parents=True)
    root_file.write_text("# root-level source\n")
    nested_file.write_text("# nested source\n")

    run(["git", "init", "--quiet"], cwd=tmp_path, check=True)
    run(["git", "add", "--", "."], cwd=tmp_path, check=True)

    assert boilerplate.collect_source_files(tmp_path) == [root_file, nested_file]


def test_add_header_uses_supplied_year(tmp_path, monkeypatch):
    source = tmp_path / "example.go"
    source.write_bytes(b"package example\n")
    template = tmp_path / "boilerplate.go.txt"
    template.write_bytes(b"// Copyright YEAR The KServe Authors.\n\n")

    monkeypatch.setattr(boilerplate, "file_year", lambda path, root: "2019")

    boilerplate.add_header(source, template, tmp_path)

    assert source.read_bytes() == (
        b"// Copyright 2019 The KServe Authors.\n\npackage example\n"
    )


def test_add_missing_headers_uses_cached_year_without_git_lookup(tmp_path, monkeypatch):
    source = tmp_path / "python" / "example.py"
    source.parent.mkdir()
    source.write_bytes(b"print('hello')\n")
    template = tmp_path / "hack" / "boilerplate.python.txt"
    template.parent.mkdir()
    template.write_bytes(b"# Copyright YEAR The KServe Authors.\n\n")
    cache = tmp_path / "copyright_years_cache"
    cache.write_text("python/example.py\t2019\n")

    def fail_file_year(path, repo_root):
        raise AssertionError("cached files must not use git history lookup")

    monkeypatch.setattr(boilerplate, "file_year", fail_file_year)

    boilerplate.add_missing_headers(tmp_path, [source], cache)

    assert source.read_bytes() == (
        b"# Copyright 2019 The KServe Authors.\n\nprint('hello')\n"
    )
