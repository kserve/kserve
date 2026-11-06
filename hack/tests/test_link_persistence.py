"""Only completed default-branch audits may supply the registry fallback."""

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1]))
from linkcheck.persistence import previous_run


class API:
    def __init__(self, runs, expired=()):
        self.runs = runs
        self.expired = expired
        self.calls = []

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
            return 200, {
                "artifacts": [
                    {
                        "id": key,
                        "name": "link-triage-report",
                        "expired": key in self.expired,
                    }
                ]
            }
        raise AssertionError(path)


def run(**changes):
    return {
        "event": "schedule",
        "status": "completed",
        "conclusion": "success",
        "head_branch": "master",
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


if __name__ == "__main__":
    unittest.main()
