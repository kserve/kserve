"""Exercise the publisher CLI, its provenance guards and read-only preview."""

from contextlib import redirect_stdout
from datetime import datetime, timedelta, timezone
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from link_fixtures import ROOT, audit, issue, load_script, plan, write_files
from linkcheck.history import advance


class TriageTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.now = datetime.now(timezone.utc)
        self.report = {
            **audit(kind="local", stamp=self.now.isoformat()),
            "run_id": "123",
        }
        self.args = [
            "--report",
            str(self.root / "input/observations.json"),
            "--state",
            str(self.root / "state.json"),
        ]

    def run_cli(self, *args):
        write_files(self.root, {"input/observations.json": json.dumps(self.report)})
        return subprocess.run(
            [sys.executable, str(ROOT / "hack/triage-doc-links.py"), *self.args, *args],
            capture_output=True,
            text=True,
            env={**os.environ, "GITHUB_TOKEN": "intentionally-invalid-token"},
        )

    def test_preview_uses_only_the_explicit_state_and_never_writes_to_github(self):
        write_files(
            self.root, {"input/state.json": '{"version":999,"issue_number":12345}'}
        )
        run = "https://github.com/kserve/kserve/actions/runs/123"
        artifact = run + "/artifacts/456"
        result = self.run_cli("--run-url", run, "--audit-url", artifact)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Dry run: create", result.stdout)
        self.assertIn(
            f"[Download full audit (ZIP)]({artifact})",
            (self.root / "issue-preview.md").read_text(),
        )
        self.assertNotIn(
            "issue_number", json.loads((self.root / "state.json").read_text())
        )

    def test_pr_audit_cannot_advance_history_or_publish(self):
        self.report["scope"] = "pr"
        for args in ((), ("--publish",)):
            with self.subTest(args=args):
                self.assertNotEqual(self.run_cli(*args).returncode, 0)
                self.assertFalse((self.root / "state.json").exists())
                self.assertFalse((self.root / "issue-plan.json").exists())

    def test_publisher_reconciles_an_existing_stale_registry(self):
        triage = load_script("triage-doc-links")
        saved = None
        for offset in range(7):
            saved = advance(
                saved, audit(), self.now.date() - timedelta(days=8 - offset)
            )
        published = advance(saved, audit(), self.now.date() - timedelta(days=1))
        existing = issue(plan(None, published)["body"])
        self.report["observations"] = audit("ok")["observations"]
        write_files(
            self.root,
            {
                "input/observations.json": json.dumps(self.report),
                "state.json": json.dumps(saved),
            },
        )
        environment = {
            "GITHUB_REPOSITORY": "kserve/kserve",
            "GITHUB_EVENT_NAME": "schedule",
            "GITHUB_REF": "refs/heads/master",
            "DEFAULT_BRANCH": "master",
            "GITHUB_RUN_ID": "123",
        }
        with (
            patch.dict(os.environ, environment),
            patch.object(sys, "argv", ["triage-doc-links.py", "--publish", *self.args]),
            patch.object(triage, "find_issue", return_value=existing),
            patch.object(triage, "publish_plan", return_value=existing) as publish,
            redirect_stdout(io.StringIO()),
        ):
            triage.main()
        self.assertNotEqual(publish.call_args.args[-1]["action"], "close")
        state = json.loads((self.root / "state.json").read_text())
        self.assertTrue(next(iter(state["records"].values()))["actionable"])

    def test_workflow_keeps_untrusted_files_out_of_code_and_history(self):
        workflow = (ROOT / ".github/workflows/check-links.yml").read_text()
        for required in (
            "cache-mode: none",
            "path: bin/link-check/input",
            "--report bin/link-check/input/observations.json",
        ):
            self.assertIn(required, workflow)
        self.assertNotIn("uses: actions/cache", workflow)
        self.assertNotIn("needs.check-links.outputs.sha", workflow)
