"""Check policy through HTTP outcomes and the real scanner's CLI."""

from datetime import date, timedelta
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from link_fixtures import API, LYCHEE, checkout, links, notebook, run_check, track
from linkcheck.validation import DETAIL_LIMIT


def http_report(url, code=None, *, timeout=False):
    entry = {"url": url, "status": {"text": "failed", "code": code}}
    bucket = "timeout_map" if timeout else "success_map" if code == 200 else "error_map"
    return {bucket: {"README.md": [entry]}}


class LinkPolicyTests(unittest.TestCase):
    def test_exception_expiry_and_invalid_entries(self):
        entry = {
            "url": "https://test.invalid/a",
            "reason": "temporary",
            "expires": "2026-10-01",
        }
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "exceptions.json"

            def check(entries, today=date(2026, 10, 1)):
                path.write_text(json.dumps({"version": 1, "exceptions": entries}))
                return links.exceptions(path, today)

            self.assertEqual(check([entry]), ({entry["url"]: "temporary"}, []))
            self.assertEqual(check([entry], date(2026, 10, 2)), ({}, [entry["url"]]))
            for entries in (
                [{**entry, "reason": None}],
                [{**entry, "expires": "invalid"}],
                [{**entry, "expires": "9999-12-31"}],
                [entry, entry],
                [{key: value for key, value in entry.items() if key != "reason"}],
            ):
                with self.subTest(entries=entries), self.assertRaises(ValueError):
                    check(entries)

    def test_general_fragments_share_history_but_github_headings_are_distinct(self):
        self.assertEqual(
            links.canonical_url("https://test.invalid/a#heading"),
            "https://test.invalid/a",
        )
        url = "https://github.com/other/repo/blob/main/doc.md#heading"
        self.assertEqual(links.canonical_url(url), url)

    def test_http_outcomes_require_explicit_evidence(self):
        url = "https://test.invalid/a"
        cases = [(code, code, "restricted") for code in (401, 403, 429)]
        cases += [(code, code, "unreachable") for code in (500, 503, None)]
        cases += [
            (404, 404, "missing"),
            (410, 410, "missing"),
            (404, 503, "unreachable"),
            (404, 200, "ok"),
            (503, 200, "ok"),
            (200, "absent", "ok"),
            (404, "absent", "unknown"),
            ("absent", "absent", "unknown"),
        ]
        for first, second, expected in cases:
            with self.subTest(first=first, second=second):
                result = links.external_observations(
                    {url},
                    {} if first == "absent" else http_report(url, first),
                    {} if second == "absent" else http_report(url, second),
                )
                self.assertEqual(result[0]["status"], expected)
        timed_out = http_report(url, timeout=True)
        self.assertEqual(
            links.external_observations({url}, timed_out, timed_out)[0]["status"],
            "unreachable",
        )
        truncated = http_report(url.rstrip("a"), 200)
        self.assertEqual(
            links.external_observations({url}, truncated, {})[0]["status"], "unknown"
        )

    def test_local_comparison_normalizes_checkout_paths(self):
        findings = []
        for root in (Path("/tmp/base"), Path("/tmp/head")):
            report = http_report((root / "missing.md").as_uri() + "#section")
            findings.append(links.local_failures(report, root))
        self.assertEqual(findings[0], findings[1])


@unittest.skipUnless(LYCHEE.is_file(), "Install Lychee with make lychee")
class ScannerTests(unittest.TestCase):
    def test_fetched_headings_and_local_repository_aliases(self):
        readme = "# New heading\n[home](https://github.com/kserve/kserve/#new-heading)\n[old alias](https://github.com/kubeflow/kfserving/#new-heading)\n[new file](https://github.com/kubeflow/kfserving/blob/master/added.md#new-heading)\n"
        with checkout({"README.md": readme, "added.md": "# New heading\n"}) as root:
            checker = links.Checker(str(LYCHEE), root)
            for heading, expected in (
                ("existing-heading", "ok"),
                ("missing-heading", "missing"),
            ):
                self.assertEqual(
                    checker.fragment("# Existing heading\n", ".md", heading), expected
                )
            self.assertEqual(
                links.local_failures(checker.local(root, name="home"), root), {}
            )

    def test_inventory_preserves_sources_and_omits_verbatim_urls(self):
        text = "[link](https://www.python.org/doc#heading)\n`https://www.python.org/command`\n"
        with checkout({name: text for name in ("README.md", "guide.md")}) as root:
            inventory = links.Checker(str(LYCHEE), root).inventory(root)
            self.assertEqual(set(inventory), {"https://www.python.org/doc"})
            self.assertEqual(
                {s["path"] for s in inventory["https://www.python.org/doc"]},
                {"README.md", "guide.md"},
            )

    def test_directory_links_resolve_readme_anchors(self):
        with checkout(
            {
                "guide/README.md": "# Install Prometheus\n",
                "README.md": "[guide](guide/#install-prometheus)\n",
            }
        ) as root:
            report = links.Checker(str(LYCHEE), root).local(root, name="directory")
            self.assertEqual(links.local_failures(report, root), {})

    def test_pr_gate_compares_baseline_and_only_requests_new_destinations(self):
        debt = "[existing debt](missing.md)\n[old](https://github.com/o/r/blob/main/old.md)\n"
        for addition, expected in (
            ("", 0),
            ("[new GitHub](https://github.com/o/r/blob/main/new.md)\n", 1),
            ("[new local](new-missing.md)\n", 1),
        ):
            with (
                self.subTest(addition=addition),
                checkout({"README.md": debt}) as base,
                checkout({"README.md": debt + addition}) as head,
            ):
                api = API(refs=["main"], content=(404, {}))
                code, report, _ = run_check(head, base, api)
                self.assertEqual(code, expected)
                self.assertEqual(report["scope"], "pr")
                self.assertFalse(any("/contents/old.md" in path for path in api.calls))
                if "new GitHub" in addition:
                    self.assertTrue(
                        any("/contents/new.md" in path for path in api.calls)
                    )
                else:
                    self.assertEqual(api.calls, [])

    def test_pr_gate_finds_deleted_targets_in_unchanged_documents(self):
        text = "[debt](missing.md)\n[keep](target.md#section)\n[absolute](https://github.com/kserve/kserve/blob/master/target.md)\n"
        files = {"README.md": text, "target.md": "# Section\n"}
        with checkout(files) as base, checkout(files) as head:
            self.assertEqual(run_check(head, base, API())[0], 0)
            (head / "target.md").unlink()
            track(head)
            code, report, _ = run_check(head, base, API())
            self.assertEqual(code, 1)
            self.assertEqual(
                {i["url"] for i in report["observations"]},
                {"repo:/missing.md", "repo:/target.md", "repo:/target.md#section"},
            )

    def test_notebook_cells_are_independent_and_code_is_not_executed(self):
        prefixes = (
            "<!--\n```\n-->",
            "<pre>\n```\n</pre>",
            "<!-- unclosed",
            "<script>\n```",
            "```python\n<!--\n```",
            "```literal `backtick` example```",
            "```python\n[ignored](ignored.md)\n```\u00a0",
        )
        code = {
            "cell_type": "code",
            "source": ["raise RuntimeError('must never execute')"],
        }
        for prefix in prefixes:
            with (
                self.subTest(prefix=prefix),
                checkout(
                    {
                        "example.ipynb": notebook(
                            prefix,
                            "[one](first-missing.md)",
                            "[two](second-missing.md)",
                            code,
                        )
                    }
                ) as root,
            ):
                report = links.Checker(str(LYCHEE), root).local(root, name="notebook")
                self.assertEqual(
                    set(links.local_failures(report, root)),
                    {
                        ("example.ipynb", "first-missing.md"),
                        ("example.ipynb", "second-missing.md"),
                    },
                )

    def test_notebook_self_anchor_resolves_across_cells(self):
        with checkout(
            {"example.ipynb": notebook("# Heading\n", "[Return](#heading)\n")}
        ) as root:
            report = links.Checker(str(LYCHEE), root).local(root, name="notebook")
            self.assertEqual(links.local_failures(report, root), {})

    def test_unknown_mdx_and_unsupported_formats_are_distinct_from_missing_files(self):
        text = "[head](https://github.com/kserve/kserve/blob/HEAD/added.md#new)\n[dynamic](dynamic.mdx#runtime-heading)\n[missing](missing.mdx#runtime-heading)\n[relative directory](guide/#runtime-heading)\n[GitHub directory](https://github.com/kserve/kserve/tree/HEAD/guide#runtime-heading)\n[unsupported relative](unsupported/#runtime)\n[unsupported GitHub](https://github.com/kserve/kserve/tree/HEAD/unsupported#runtime)\n"
        dynamic = "export const Heading = () => <h2 id={['runtime', 'heading'].join('-')}>Heading</h2>\n\n<Heading />\n"
        with checkout(
            {
                "README.md": text,
                "added.md": "# New\n",
                "dynamic.mdx": dynamic,
                "guide/README.mdx": dynamic,
                "unsupported/code.py": "pass\n",
            }
        ) as root:
            code, report, _ = run_check(root, offline=True, advisory=True)
            self.assertEqual(code, 0)
            self.assertEqual(
                {i["url"]: i["status"] for i in report["observations"]},
                {
                    "repo:/dynamic.mdx#runtime-heading": "unknown",
                    "repo:/missing.mdx#runtime-heading": "missing",
                    "repo:/guide#runtime-heading": "unknown",
                    "repo:/unsupported#runtime": "unknown",
                },
            )

    def test_head_and_explicit_branch_urls_use_unmerged_checkout(self):
        urls = (
            "https://github.com/kserve/kserve/blob/HEAD/added.md#new",
            "https://github.com/kserve/kserve/tree/HEAD/added.md#new",
            "https://raw.githubusercontent.com/kserve/kserve/refs/heads/master/added.md#new",
        )
        with checkout(
            {
                "README.md": "\n".join(f"[target]({url})" for url in urls),
                "added.md": "# New\n",
            }
        ) as root:
            checker = links.Checker(str(LYCHEE), root)
            self.assertEqual(
                links.local_failures(checker.local(root, name="before"), root), {}
            )
            (root / "added.md").unlink()
            track(root)
            self.assertEqual(
                set(links.local_failures(checker.local(root, name="after"), root)),
                {("README.md", "added.md#new")},
            )

    def test_github_ui_and_relative_source_line_anchors(self):
        text = "# Project\n[readme](https://github.com/kserve/kserve#readme)\n[line](https://github.com/kserve/kserve/blob/master/README.md?plain=1#L2)\n[code](https://github.com/kserve/kserve/blob/HEAD/code.py#L2)\n[bad line](https://github.com/kserve/kserve/blob/master/code.py#L999999)\n[bad local](code.py#L999) [good local](code.py#L2)\n"
        with checkout({"README.md": text, "code.py": "one\ntwo\nthree\n"}) as root:
            report = links.Checker(str(LYCHEE), root).local(root, name="anchors")
            self.assertEqual(
                set(links.local_failures(report, root)),
                {("README.md", "code.py#L999999"), ("README.md", "code.py#L999")},
            )

    def test_release_checkout_does_not_impersonate_default_branch(self):
        with (
            checkout() as root,
            patch.dict(
                os.environ,
                {"DEFAULT_BRANCH": "master", "CHECKOUT_BRANCH": "release-1.0"},
            ),
        ):
            checker = links.Checker(str(LYCHEE), root)
            for ref in ("master", "HEAD"):
                self.assertIsNone(
                    checker.checkout(
                        f"https://github.com/kserve/kserve/blob/{ref}/master-only.md",
                        root,
                    )
                )
            self.assertEqual(
                checker.checkout(
                    "https://github.com/kserve/kserve/blob/release-1.0/new.md", root
                )[0],
                root / "new.md",
            )

    def test_inaccessible_github_issue_link_is_inconclusive(self):
        url = "https://github.com/o/private/issues/1"
        with (
            checkout({"README.md": "# Base\n"}) as base,
            checkout({"README.md": f"[private issue]({url})\n"}) as head,
        ):
            code, report, _ = run_check(
                head,
                base,
                API(repository=404),
                http=lambda name, inputs: http_report(url, 404),
            )
            self.assertEqual(code, 0)
            self.assertEqual(report["observations"][0]["status"], "restricted")

    def test_only_404_and_410_are_rechecked(self):
        urls = ["https://www.python.org/down", "https://www.python.org/missing"]

        def http(name, inputs):
            return {
                "error_map": {
                    "stdin": [
                        {
                            "url": url,
                            "status": {
                                "code": 503 if url.endswith("down") else 404,
                                "text": "failed",
                            },
                        }
                        for url in inputs
                    ]
                }
            }

        with (
            checkout({"README.md": "# Base\n"}) as base,
            checkout(
                {"README.md": "\n".join(f"[link]({url})" for url in urls)}
            ) as head,
        ):
            code, report, calls = run_check(head, base, API(), http=http)
            self.assertEqual(code, 1)
            self.assertEqual(calls[-1], ("external-recheck", [urls[1]]))
            self.assertEqual(
                {i["url"]: i["status"] for i in report["observations"]},
                {urls[0]: "unreachable", urls[1]: "missing"},
            )

    def test_long_or_multiline_details_fit_the_report(self):
        # The report rejects details over DETAIL_LIMIT characters or with
        # control characters. Lychee errors embed the URL they failed on, and
        # exception reasons replace the detail.
        long_url = "https://www.python.org/" + "a" * DETAIL_LIMIT
        multiline_url = "https://www.python.org/reset"
        excepted_url = "https://www.python.org/excepted"
        errors = {
            long_url: f"Network error: error sending request for url ({long_url})",
            multiline_url: "Network error: connection reset\r\ncaused by: peer closed",
        }

        def http(name, inputs):
            return {
                "error_map": {
                    "stdin": [
                        {"url": url, "status": {"code": 503, "text": errors[url]}}
                        for url in inputs
                    ]
                }
            }

        expires = (date.today() + timedelta(days=30)).isoformat()
        reasons = {
            "repo:/missing.md": "r" * (DETAIL_LIMIT + 1),
            excepted_url: "tracked upstream\nsee the release notes",
        }
        files = {
            "README.md": "".join(
                f"[link]({url})\n" for url in (*errors, excepted_url, "missing.md")
            ),
            ".github/link-check-exceptions.json": json.dumps(
                {
                    "version": 1,
                    "exceptions": [
                        {"url": url, "reason": reason, "expires": expires}
                        for url, reason in reasons.items()
                    ],
                }
            ),
        }
        with checkout(files) as root:
            code, report, _ = run_check(root, api=API(), http=http, advisory=True)
        self.assertEqual(code, 0)
        self.assertEqual(
            {item["url"]: item["detail"] for item in report["observations"]},
            {
                long_url: errors[long_url][: DETAIL_LIMIT - 1] + "…",
                multiline_url: "Network error: connection reset  caused by: peer closed",
                excepted_url: "tracked upstream see the release notes",
                "repo:/missing.md": "r" * (DETAIL_LIMIT - 1) + "…",
            },
        )

    def test_stdin_markdown_preserves_punctuation_in_urls(self):
        with checkout() as root:
            urls = {f"https://www.python.org/end{suffix}" for suffix in (".", "!", "~")}
            self.assertEqual(
                links.Checker(str(LYCHEE), root).run(
                    root, ["--dump"], inputs=sorted(urls)
                ),
                urls,
            )

    def test_lychee_process_does_not_receive_the_api_token(self):
        result = subprocess.CompletedProcess(
            [], 0, '{"error_map":{},"timeout_map":{}}', ""
        )
        with (
            checkout() as root,
            patch.dict(
                os.environ, {"GITHUB_TOKEN": "sensitive", "GH_TOKEN": "sensitive"}
            ),
            patch.object(links.subprocess, "run", return_value=result) as run,
        ):
            links.Checker(str(LYCHEE), root).run(
                root,
                ["--format", "json"],
                inputs=["https://www.python.org/"],
                name="external",
            )
        self.assertNotIn("GITHUB_TOKEN", run.call_args.kwargs["env"])
        self.assertNotIn("GH_TOKEN", run.call_args.kwargs["env"])
