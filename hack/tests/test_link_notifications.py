"""Publishing preserves maintainer decisions and validates scanner-controlled content."""

import unittest
from unittest.mock import Mock
from datetime import date

from link_fixtures import timed_audit, item, issue, plan as issue_plan
from linkcheck.history import advance, identity
from linkcheck.validation import validate_report
from linkcheck.notifications import (
    MARKER,
    finding_row,
    find_issue,
    link_label,
    reported_keys,
    seed_from_issue,
    publish_plan,
    reconcile_from_issue,
    reported_records,
    safe_text,
)


def state():
    return advance(
        None,
        {"version": 1, "scope": "audit", "complete": True, "observations": [item()]},
        date(2026, 10, 1),
    )


class NotificationTests(unittest.TestCase):
    def test_issue_links_the_uploaded_artifact_and_explains_expiry(self):
        run = "https://github.com/kserve/kserve/actions/runs/123"
        artifact = run + "/artifacts/456"
        created = issue_plan(None, state(), audit_url=artifact, run_url=run)
        body = created["body"]
        self.assertIn(f"[Download full audit (ZIP)]({artifact})", body)
        self.assertIn(f"[Workflow run]({run})", body)
        self.assertIn("GitHub sign-in", body)
        self.assertIn("30 days", body)
        self.assertIn(
            "[latest audits](https://github.com/kserve/kserve/actions/workflows/check-links.yml)",
            body,
        )
        unchanged = issue_plan(
            issue(body),
            state(),
            audit_url=run + "4/artifacts/789",
            sha="def",
            run_url=run + "4",
        )
        self.assertEqual(unchanged["action"], "none")

    def test_local_preview_does_not_invent_a_run_or_link_local_files(self):
        body = issue_plan(None, state())["body"]
        self.assertIn("populated when this check runs in GitHub Actions", body)
        self.assertNotIn("/tmp/", body)
        self.assertNotIn("Download full audit", body)

    def test_artifact_link_can_be_previewed_without_a_run_link(self):
        artifact = "https://github.com/kserve/kserve/actions/runs/123/artifacts/456"
        body = issue_plan(None, state(), audit_url=artifact)["body"]
        self.assertIn(f"[Download full audit (ZIP)]({artifact})", body)
        self.assertNotIn("populated when this check runs", body)

    def test_issue_groups_findings_and_names_source_locations(self):
        current = state()
        local = {
            **item(),
            "url": "repo:/missing.md",
            "kind": "local",
            "sources": [{"path": "docs/guide.md", "line": 12}],
        }
        current["records"][identity(local)] = {**local, "actionable": True}
        plan = issue_plan(None, current, run_url="https://run/1")
        body = plan["body"]
        self.assertIn("**2 targets · 2 references · 2 source files**", body)
        self.assertIn("Local files and headings (1)", body)
        self.assertIn("Verified GitHub targets (1)", body)
        self.assertIn(
            "[docs/guide.md:12](https://github.com/kserve/kserve/blob/abc/docs/guide.md#L12)",
            body,
        )
        self.assertIn("fix the generator", body)
        self.assertIn("two completed audits on different UTC days", body)
        self.assertEqual(body.count("<details>"), body.count("</details>"))

    def test_partial_recovery_does_not_reopen_a_manually_closed_issue(self):
        current = state()
        another = {
            **item(),
            "url": "https://github.com/other/repo/blob/main/another.md",
        }
        current["records"][identity(another)] = {**another, "actionable": True}
        created = issue_plan(None, current, run_url="https://run/1")
        current["records"][identity(another)]["actionable"] = False
        result = issue_plan(
            issue(created["body"], "closed"), current, run_url="https://run/2"
        )
        self.assertEqual(result["action"], "none")

    def test_issue_api_writes_match_the_reviewed_plan_and_noop_writes_nothing(self):
        class API:
            def __init__(self):
                self.writes = []

            def write(self, method, path, payload):
                self.writes.append((method, path, payload))
                return {"number": 42, **payload}

        api = API()
        created = issue_plan(None, state(), run_url="https://run/1")
        result = publish_plan(api, "kserve/kserve", None, created)
        self.assertEqual(result["number"], 42)
        self.assertEqual(api.writes[0][:2], ("POST", "/repos/kserve/kserve/issues"))
        self.assertIn("<!-- kserve-link-check:v1 -->", api.writes[0][2]["body"])
        existing = issue(created["body"])
        publish_plan(api, "kserve/kserve", existing, {"action": "none"})
        self.assertEqual(len(api.writes), 1)
        publish_plan(
            api, "kserve/kserve", existing, {"action": "close", "body": "resolved"}
        )
        self.assertEqual(
            api.writes[-1],
            (
                "PATCH",
                "/repos/kserve/kserve/issues/42",
                {"body": "resolved", "state": "closed"},
            ),
        )

    def test_new_targets_can_reopen_a_manually_closed_aggregate_issue(self):
        created = issue_plan(None, state(), run_url="https://run/1")
        current = state()
        another = {
            **item(),
            "url": "https://github.com/other/repo/blob/main/another.md",
        }
        current["records"][identity(another)] = {**another, "actionable": True}
        reopened = issue_plan(
            issue(created["body"], "closed"), current, run_url="https://run/2"
        )
        self.assertEqual(reopened["action"], "reopen")

    def test_cache_loss_preserves_removed_target_details_until_recovery_is_confirmed(
        self,
    ):
        created = issue_plan(None, state(), run_url="https://run/1")
        report = {"version": 1, "complete": True, "scope": "audit", "observations": []}
        restored = seed_from_issue(issue(created["body"]), report)
        current = advance(restored, report, date(2026, 10, 2))
        record = current["records"][identity(item())]
        self.assertEqual(record["url"], item()["url"])
        self.assertTrue(record["actionable"])
        current = advance(current, report, date(2026, 10, 3))
        self.assertFalse(current["records"][identity(item())]["actionable"])

    def test_create_then_unchanged_run_does_not_update(self):
        current = state()
        plan = issue_plan(None, current, run_url="https://run/1")
        self.assertEqual(plan["action"], "create")
        later = issue_plan(
            issue(plan["body"]), current, sha="def", run_url="https://run/2"
        )
        self.assertEqual(later["action"], "none")

    def test_maintainer_text_is_preserved(self):
        created = issue_plan(None, state(), run_url="https://run/1")
        existing = issue(
            "Maintainer investigation notes.\n\n"
            + created["body"]
            + "\n\nManual checklist."
        )
        current = state()
        current["records"][identity(item())]["detail"] = "Updated failure"
        update = issue_plan(existing, current, run_url="https://run/2")
        self.assertEqual(update["action"], "update")
        self.assertIn("Maintainer investigation notes.", update["body"])
        self.assertIn("Manual checklist.", update["body"])

    def test_manually_closed_issue_is_not_reopened_for_same_targets(self):
        created = issue_plan(None, state(), run_url="https://run/1")
        result = issue_plan(
            issue(created["body"], "closed"), state(), run_url="https://run/2"
        )
        self.assertEqual(result["action"], "none")

    def test_recovered_issue_closes_and_can_reopen_after_recurrence(self):
        current = state()
        created = issue_plan(None, current, run_url="https://run/1")
        for value in current["records"].values():
            value["actionable"] = False
        closed = issue_plan(issue(created["body"]), current, run_url="https://run/2")
        self.assertEqual(closed["action"], "close")
        reopened = issue_plan(
            issue(closed["body"], "closed"), state(), run_url="https://run/3"
        )
        self.assertEqual(reopened["action"], "reopen")

    def test_cache_miss_recovers_reported_targets_before_advancing(self):
        created = issue_plan(None, state(), run_url="https://run/1")
        self.assertEqual(reported_keys(created["body"]), [identity(item())])
        report = {
            "version": 1,
            "complete": True,
            "scope": "audit",
            "observations": [item("ok")],
        }
        restored = seed_from_issue(issue(created["body"]), report)
        current = advance(restored, report, date(2026, 10, 2))
        self.assertTrue(current["records"][identity(item())]["actionable"])

    def test_empty_audit_does_not_create_an_issue(self):
        self.assertEqual(
            issue_plan(None, {"records": {}}, run_url="https://run/1")["action"],
            "none",
        )


class PublisherValidationTests(unittest.TestCase):
    def test_tracked_issue_identity_is_checked_independently(self):
        valid = {
            "number": 42,
            "repository_url": "https://api.github.com/repos/kserve/kserve",
            "user": {"login": "github-actions[bot]"},
            "body": MARKER,
        }
        api = Mock()
        api.get.return_value = (200, valid)
        self.assertEqual(find_issue(api, "kserve/kserve", 42), valid)
        for change in (
            {"number": 77},
            {"repository_url": "https://api.github.com/repos/kserve/website"},
            {"body": "Marker removed"},
            {"user": {"login": "someone"}},
        ):
            with self.subTest(change=change), self.assertRaises(ValueError):
                api.get.return_value = (200, {**valid, **change})
                find_issue(api, "kserve/kserve", 42)
        api.get.return_value = (503, valid)
        with self.assertRaises(ValueError):
            find_issue(api, "kserve/kserve", 42)

    def test_untrusted_report_rejects_malformed_source_locations(self):
        validate_report(timed_audit())
        for source in (
            {"path": "../outside.md"},
            {"path": "/absolute.md"},
            {"path": "README.md", "line": True},
            {"path": "README.md", "line": 0},
            {"path": "README.md", "line": "1) injected"},
        ):
            report = timed_audit()
            report["observations"][0]["sources"] = [source]
            with self.subTest(source=source), self.assertRaises(ValueError):
                validate_report(report)

    def test_untrusted_report_rejects_oversized_or_multiline_text(self):
        at_limit = timed_audit()
        at_limit["observations"][0]["detail"] = "x" * 2000
        validate_report(at_limit)
        for field, value in (
            ("detail", "x" * 2001),
            ("detail", "first\rsecond"),
            ("detail", "tab\tseparated"),
            ("detail", "delete\x7f"),
            ("url", "https://site.invalid/\x7f"),
            ("sources", [{"path": "docs/guide\x7f.md"}]),
        ):
            report = timed_audit()
            report["observations"][0][field] = value
            with (
                self.subTest(field=field, value=repr(value)[:40]),
                self.assertRaises(ValueError),
            ):
                validate_report(report)

    def test_a_detail_at_the_limit_does_not_hide_the_rest_of_its_group(self):
        current = state()
        # Sorts before item(); each quote expands to a six-character entity.
        first = {
            **item(),
            "url": "https://github.com/other/repo/blob/main/a.md",
            "detail": "'" * 2000,
        }
        current["records"][identity(first)] = {**first, "actionable": True}
        body = issue_plan(None, current)["body"]
        self.assertIn("Verified GitHub targets (2)", body)
        self.assertIn(f"[{item()['url']}]", body)
        self.assertNotIn("Additional targets", body)

    def test_scanner_text_cannot_form_links_images_rows_or_broken_entities(self):
        for value, expected in (
            (
                "See [guide.md:12](https://evil.example/x)",
                "See \\[guide.md:12\\](https://evil.example/x)",
            ),
            (
                "![t](https://evil.example/p.png)",
                "!\\[t\\](https://evil.example/p.png)",
            ),
            ("one\rtwo\nthree\x7ffour", "one two three four"),
            (
                "it's `x` | #12 @me",
                "it&#x27;s &#96;x&#96; &#124; #\u200b12 @\u200bme",
            ),
            ("back\\", "back\\\\"),
            # Escaping other punctuation would put backslashes into the URLs
            # that GitHub autolinks in lychee's network error messages.
            (
                "url (https://host.invalid/simple_string/)",
                "url (https://host.invalid/simple_string/)",
            ),
        ):
            with self.subTest(value=value):
                self.assertEqual(safe_text(value), expected)

    def test_link_labels_escape_markdown_and_keep_their_closing_bracket(self):
        for value, expected in (
            ("docs/path\\", "docs/path\\\\"),
            ("docs/__init__.md", "docs/\\_\\_init\\_\\_.md"),
            ("a*b~c(d)!e[f]", "a\\*b\\~c\\(d\\)\\!e\\[f\\]"),
            ("it's#x", "it&#x27;s#\u200bx"),
        ):
            with self.subTest(value=value):
                self.assertEqual(link_label(value), expected)

    def test_targets_above_the_repository_root_are_not_linked(self):
        local = {**item(), "kind": "local"}
        sources = (
            "[README.md:7](https://github.com/kserve/kserve/blob/abc/README.md#L7)"
        )
        for url, target in (
            ("repo:/../../../../evil/repo", "../../../../evil/repo"),
            ("repo:/../outside.md#a", "../outside.md#\u200ba"),
            (
                "repo:/docs/./guide.md#..",
                "[docs/./guide.md#\u200b..](https://github.com/kserve/kserve/blob/abc/docs/./guide.md#..)",
            ),
        ):
            with self.subTest(url=url):
                self.assertEqual(
                    finding_row({**local, "url": url}, "kserve/kserve", "abc"),
                    f"| {target} | missing: missing | {sources} |",
                )

    def test_lost_registry_does_not_replace_an_unmarked_bot_issue(self):
        class API:
            def get(self, path):
                return 200, [
                    {
                        "number": 42,
                        "title": "Documentation link failures",
                        "user": {"login": "github-actions[bot]", "type": "Bot"},
                        "body": "Maintainer removed automation",
                    }
                ]

        with self.assertRaisesRegex(ValueError, "marker"):
            find_issue(API(), "kserve/kserve")

    def test_source_line_cannot_inject_markdown(self):
        value = {**item(), "sources": [{"path": "README.md", "line": "1) @someone"}]}
        with self.assertRaises(ValueError):
            finding_row(value, "kserve/kserve", "abc")

    def test_bot_text_cannot_create_mentions(self):
        self.assertNotIn("@someone", safe_text("ask @someone or @org/team about #12"))
        self.assertNotIn("#12", safe_text("ask @someone or @org/team about #12"))

    def test_large_issues_keep_recoverable_identities_within_github_limit(self):
        report = timed_audit(kind="github")
        report["observations"] = [
            {
                **report["observations"][0],
                "url": f"https://github.com/o/r/blob/main/{number}/"
                + "long-path-" * 80
                + ".md",
            }
            for number in range(950)
        ]
        current = advance(None, report, date(2026, 1, 1))
        published = issue_plan(None, current)
        self.assertLess(len(published["body"]), 65000)
        self.assertEqual(len(reported_records(published["body"])), 950)
        active = issue(published["body"])
        current = reconcile_from_issue(None, active)
        for day in (2, 3):
            current = advance(
                current,
                {
                    **timed_audit(stamp=f"2026-01-0{day}T12:00:00+00:00"),
                    "observations": [],
                },
                date(2026, 1, day),
            )
        closing = issue_plan(active, current)
        self.assertEqual(closing["action"], "close")
        self.assertLess(len(closing["body"]), 65000)
