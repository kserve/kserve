"""Only completed default-branch audits may supply the registry fallback."""

import sys
import unittest
from pathlib import Path
from urllib.parse import parse_qs, unquote, urlsplit

sys.path.insert(0, str(Path(__file__).parents[1]))
from linkcheck.persistence import previous_run

OLDER, TIP = "a" * 40, "b" * 40
TAGGED, GONE, FAILING = "c" * 40, "d" * 40, "e" * 40


class API:
    def __init__(self, runs, expired=(), previews=(), tags=None):
        self.runs = runs
        self.expired = expired
        self.previews = previews
        self.tags = tags or {}
        self.calls = []

    def compare(self, path):
        base, head = unquote(urlsplit(path).path.split("/compare/", 1)[1]).split("...")
        if base == GONE:
            return 404, {}
        if base == FAILING:
            return 503, {}
        # Git resolves an ambiguous short name to a tag before a branch.
        name = head.removeprefix("refs/tags/")
        if (
            head.startswith("refs/tags/")
            or not head.startswith("refs/")
            and name in self.tags
        ):
            commits = [self.tags[name]]
        elif head in ("master", "refs/heads/master"):
            commits = [OLDER, TIP]
        else:
            return 404, {}
        if base == commits[-1]:
            return 200, {"status": "identical"}
        return 200, {"status": "ahead" if base in commits else "diverged"}

    def get(self, path):
        self.calls.append(path)
        if "/workflows/check-links.yml/runs?" in path:
            return 200, {
                "workflow_runs": [
                    {"id": key, **value} for key, value in self.runs.items()
                ]
            }
        if "/artifacts?" in path:
            key = int(path.split("/runs/")[1].split("/")[0])
            name = parse_qs(urlsplit(path).query)["name"][0]
            uploaded = name == "link-triage-report" or (
                name == "link-triage-preview" and key in self.previews
            )
            return 200, {
                "artifacts": [{"id": key, "name": name, "expired": key in self.expired}]
                if uploaded
                else []
            }
        if "/compare/" in path:
            return self.compare(path)
        raise AssertionError(path)


def run(**changes):
    return {
        "event": "schedule",
        "status": "completed",
        "conclusion": "success",
        "head_branch": "master",
        "head_sha": OLDER,
        "head_repository": {"full_name": "kserve/kserve"},
        "path": ".github/workflows/check-links.yml",
        **changes,
    }


class PersistenceTests(unittest.TestCase):
    def test_missing_new_workflow_has_no_history_but_access_failures_are_errors(self):
        class Missing:
            def get(self, path):
                return 404, {}

        self.assertIsNone(previous_run(Missing(), "kserve/kserve", "master", 99))

        class Forbidden:
            def get(self, path):
                return 403, {}

        with self.assertRaises(RuntimeError):
            previous_run(Forbidden(), "kserve/kserve", "master", 99)

    def test_untrusted_repository_artifact_lists_are_never_scanned(self):
        api = API({1: run()})
        self.assertEqual(previous_run(api, "kserve/kserve", "master", 99), 1)
        self.assertFalse(any("/actions/artifacts?" in path for path in api.calls))

    def test_only_trusted_completed_audits_are_eligible(self):
        api = API(
            {
                1: run(),
                2: run(event="pull_request"),
                3: run(head_branch="feature"),
                4: run(head_repository={"full_name": "fork/kserve"}),
                5: run(conclusion="failure"),
                6: run(path=".github/workflows/unrelated.yml"),
                7: run(status="in_progress"),
            }
        )
        self.assertEqual(previous_run(api, "kserve/kserve", "master", 99), 1)

    def test_current_run_is_never_used_as_its_own_history(self):
        self.assertIsNone(previous_run(API({1: run()}), "kserve/kserve", "master", 1))

    def test_expired_artifacts_are_skipped_and_newest_manual_audit_wins(self):
        for expired, expected in (((), 2), ((2,), 1), ((1, 2), None)):
            with self.subTest(expired=expired):
                api = API({1: run(), 2: run(event="workflow_dispatch")}, expired)
                self.assertEqual(
                    previous_run(api, "kserve/kserve", "master", 99), expected
                )

    def test_history_requires_the_audited_commit_on_the_default_branch(self):
        # Tag runs report the tag name as head_branch, so a tag called
        # "master" looks like a default-branch run until its commit is checked.
        for name, runs, tags, expected in (
            ("older commit on the branch", {1: run()}, {}, 1),
            ("branch tip", {1: run(head_sha=TIP)}, {}, 1),
            (
                "tag named like the branch",
                {1: run(), 2: run(head_sha=TAGGED)},
                {"master": TAGGED},
                1,
            ),
            ("commit no longer known", {1: run(), 2: run(head_sha=GONE)}, {}, 1),
        ):
            with self.subTest(name):
                api = API(runs, tags=tags)
                self.assertEqual(
                    previous_run(api, "kserve/kserve", "master", 99), expected
                )

    def test_unverifiable_commit_is_an_error_not_missing_history(self):
        with self.assertRaisesRegex(RuntimeError, "HTTP 503"):
            previous_run(API({1: run(head_sha=FAILING)}), "kserve/kserve", "master", 99)

    def test_dry_run_cannot_supply_history_with_a_planted_registry(self):
        # A dry run uploads its own registry as link-triage-preview, so a
        # link-triage-report in the same run came from another job.
        api = API({1: run(), 2: run(event="workflow_dispatch")}, previews=(2,))
        self.assertEqual(previous_run(api, "kserve/kserve", "master", 99), 1)


if __name__ == "__main__":
    unittest.main()
