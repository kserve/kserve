"""Resolve public GitHub revisions and headings without inventing missing targets."""

import unittest
import base64
import json

from link_fixtures import API
from linkcheck.github import GitHubVerifier, inspect_fragment


class GitHubTests(unittest.TestCase):
    def test_notebook_cells_do_not_share_unclosed_fences(self):
        content = json.dumps(
            {
                "cells": [
                    {"cell_type": "markdown", "source": ["```python\n# example"]},
                    {"cell_type": "markdown", "source": ["# Actual heading"]},
                    {"cell_type": "code", "source": ["# Not a heading"]},
                ]
            }
        )

        cells = []

        def fragments(markdown, suffix, fragment):
            cells.append(markdown)
            self.assertNotIn("Not a heading", markdown)
            return "ok" if markdown == "# Actual heading" else "missing"

        self.assertEqual(
            inspect_fragment(content, ".ipynb", "actual-heading", fragments)["status"],
            "ok",
        )
        self.assertEqual(cells, ["```python\n# example", "# Actual heading"])

    def test_renamed_branch_urls_follow_github_redirects(self):
        api = API(refs=[], rename="main", content=(200, {"type": "file"}))
        result = GitHubVerifier(api, None).check(
            "https://github.com/o/r/blob/master/a.md"
        )
        self.assertEqual(result["status"], "ok")
        self.assertIn("/repos/o/r/contents/a.md?ref=main", api.calls)

    def test_short_raw_urls_follow_branch_renames(self):
        api = API(refs=[], rename="main", content=(200, {"type": "file"}))
        result = GitHubVerifier(api, None).check(
            "https://raw.githubusercontent.com/o/r/master/a.md"
        )
        self.assertEqual(result["status"], "ok")
        self.assertTrue(any("/branches/" in path for path in api.calls))

    def test_explicit_branch_namespace_and_github_raw_do_not_follow_renames(self):
        for url in (
            "https://github.com/o/r/blob/refs/heads/master/a.md",
            "https://github.com/o/r/raw/master/a.md",
            "https://raw.githubusercontent.com/o/r/refs/heads/master/a.md",
        ):
            api = API(refs=[], rename="main", content=(200, {"type": "file"}))
            self.assertEqual(GitHubVerifier(api, None).check(url)["status"], "missing")
            self.assertFalse(any("/branches/" in path for path in api.calls))

    def test_missing_revision_is_confirmed_in_an_accessible_repository(self):
        result = GitHubVerifier(API(refs=[]), None).check(
            "https://github.com/o/r/blob/absent/a.md"
        )
        self.assertEqual(result["status"], "missing")

    def test_head_resolves_to_the_default_branch(self):
        api = API(content=(200, {"type": "file"}))
        result = GitHubVerifier(api, None).check(
            "https://github.com/o/r/blob/HEAD/a.md"
        )
        self.assertEqual(result["status"], "ok")
        self.assertIn("/repos/o/r/contents/a.md?ref=main", api.calls)

    def test_encoded_slash_in_branch_name_is_supported(self):
        api = API(refs=["release/v1"], content=(200, {"type": "file"}))
        result = GitHubVerifier(api, None).check(
            "https://github.com/o/r/blob/release%2Fv1/a.md"
        )
        self.assertEqual(result["status"], "ok")

    def test_other_organizations_missing_files_are_confirmed(self):
        for repo in ("kserve/website", "kubeflow/pipelines", "llm-d/llm-d"):
            api = API()
            result = GitHubVerifier(api, lambda *args: "ok").check(
                f"https://github.com/{repo}/blob/main/missing.md"
            )
            self.assertEqual(result["status"], "missing")
            self.assertEqual(sum("/contents/" in p for p in api.calls), 2)

    def test_hidden_repository_404_is_inconclusive(self):
        result = GitHubVerifier(API(repository=404), None).check(
            "https://github.com/other/private/blob/main/doc.md"
        )
        self.assertEqual(result["status"], "restricted")

    def test_rate_limits_outages_and_recovery_do_not_confirm_missing(self):
        for second, expected in (
            (403, "restricted"),
            (429, "restricted"),
            (503, "unreachable"),
            (200, "ok"),
        ):
            api = API(content=[(404, {}), (second, {"type": "file"})])
            result = GitHubVerifier(api, None).check(
                "https://github.com/o/r/blob/main/a.md"
            )
            self.assertEqual(result["status"], expected)

    def test_longest_matching_branch_name_is_used(self):
        api = API(refs=["release", "release/v1"], content=(200, {"type": "file"}))
        result = GitHubVerifier(api, None).check(
            "https://github.com/o/r/blob/release/v1/docs/a.md"
        )
        self.assertEqual(result["status"], "ok")
        self.assertIn("/repos/o/r/contents/docs/a.md?ref=release%2Fv1", api.calls)

    def test_fragments_are_checked_against_the_requested_document(self):
        encoded = base64.b64encode(b"# Existing heading\n").decode()
        api = API(
            content=(
                200,
                {
                    "type": "file",
                    "encoding": "base64",
                    "content": encoded,
                    "name": "doc.md",
                },
            )
        )
        calls = []

        def fragments(content, suffix, fragment):
            calls.append((content, suffix, fragment))
            return "missing"

        result = GitHubVerifier(api, fragments).check(
            "https://raw.githubusercontent.com/o/r/main/doc.md#missing-heading"
        )
        self.assertEqual(result["status"], "missing")
        self.assertEqual(calls[0], ("# Existing heading\n", ".md", "missing-heading"))

    def test_source_line_anchors_are_not_treated_as_markdown_headings(self):
        encoded = base64.b64encode(b"one\ntwo\n").decode()
        api = API(
            content=(
                200,
                {
                    "type": "file",
                    "encoding": "base64",
                    "content": encoded,
                    "name": "a.py",
                },
            )
        )
        verifier = GitHubVerifier(api, None)
        self.assertEqual(
            verifier.check("https://github.com/o/r/blob/main/a.py#L2")["status"], "ok"
        )
        self.assertEqual(
            verifier.check("https://github.com/o/r/blob/main/a.py#L9")["status"],
            "missing",
        )


class ExplicitRefTests(unittest.TestCase):
    def test_explicit_raw_branch_and_tag_namespaces_are_preserved(self):
        for namespace in ("heads", "tags"):
            api = API(content=(200, {"type": "file"}))
            result = GitHubVerifier(api, None).check(
                f"https://raw.githubusercontent.com/o/r/refs/{namespace}/main/README.md"
            )
            self.assertEqual(result["status"], "ok")
            self.assertIn(
                f"/repos/o/r/contents/README.md?ref=refs%2F{namespace}%2Fmain",
                api.calls,
            )
            other = "tags" if namespace == "heads" else "heads"
            self.assertFalse(
                any(f"/matching-refs/{other}/" in path for path in api.calls)
            )
