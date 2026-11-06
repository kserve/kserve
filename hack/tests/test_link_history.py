"""Outage timing and recovery across missing, stale and repeated audits."""

import copy
import unittest
from datetime import date, timedelta

from link_fixtures import audit, report, observation, item, issue, plan as issue_plan
from linkcheck.history import advance, identity, restore_reported, by_target
from linkcheck.notifications import reconcile_from_issue

DAY = date(2026, 1, 1)


def observe(previous, day, status="missing", *, kind="external", stamp=None):
    day = DAY + timedelta(days=day - 1) if isinstance(day, int) else day
    return advance(previous, audit(status, stamp or f"{day}T12:00:00+00:00", kind), day)


def history(events, *, kind="external", previous=None):
    for day, status in events:
        previous = observe(previous, day, status, kind=kind)
    return previous


def record(state):
    assert len(state["records"]) == 1
    return next(iter(state["records"].values()))


class HistoryTests(unittest.TestCase):
    def test_references_to_one_target_are_grouped_with_all_sources(self):
        first = {**observation(kind="local"), "sources": [{"path": "README.md"}]}
        second = {**observation(kind="local"), "sources": [{"path": "guide.md"}]}
        state = advance(None, report(first, second), DAY)
        self.assertEqual(len(state["records"]), 1)
        self.assertEqual(len(state["records"][identity(first)]["sources"]), 2)

    def test_old_retired_records_are_pruned_after_recovery(self):
        item = observation(kind="local")
        state = advance(None, report(item), DAY)
        state = advance(state, report(), DAY + timedelta(days=1))
        state = advance(state, report(), DAY + timedelta(days=2))
        state = advance(state, report(), DAY + timedelta(days=32))
        self.assertEqual(state["records"], {})

    def test_external_failure_needs_a_week_and_multiple_checks(self):
        state = None
        item = observation()
        for offset in range(8):
            state = advance(state, report(item), DAY + timedelta(days=offset))
            self.assertEqual(
                state["records"][identity(item)]["actionable"], offset == 7
            )

    def test_missing_runs_and_repeated_same_day_runs_do_not_prove_a_week(self):
        item = observation()
        state = advance(None, report(item), DAY)
        for _ in range(8):
            state = advance(state, report(item), DAY)
        self.assertEqual(len(state["records"][identity(item)]["samples"]), 1)
        state = advance(state, report(item), DAY + timedelta(days=7))
        self.assertFalse(state["records"][identity(item)]["actionable"])

    def test_local_and_verified_cross_org_failures_are_immediate(self):
        for kind in ("local", "github"):
            item = observation(kind=kind)
            state = advance(None, report(item), DAY)
            self.assertTrue(state["records"][identity(item)]["actionable"])

    def test_access_restrictions_never_become_broken_links(self):
        state = None
        item = observation("restricted", "github")
        for offset in range(15):
            state = advance(state, report(item), DAY + timedelta(days=offset))
        self.assertFalse(state["records"][identity(item)]["actionable"])

    def test_persistent_outages_escalate_as_unreachable(self):
        item = observation("unreachable")
        state = None
        for offset in range(8):
            state = advance(state, report(item), DAY + timedelta(days=offset))
        record = state["records"][identity(item)]
        self.assertTrue(record["actionable"])
        self.assertEqual(record["status"], "unreachable")

    def test_one_success_resets_age_and_two_successful_days_resolve_alert(self):
        bad = observation(kind="github")
        good = observation("ok", "github")
        state = advance(None, report(bad), DAY)
        state = advance(state, report(good), DAY + timedelta(days=1))
        record = state["records"][identity(bad)]
        self.assertIsNone(record["first_failed"])
        self.assertTrue(record["actionable"])
        state = advance(state, report(good), DAY + timedelta(days=1))
        self.assertTrue(state["records"][identity(bad)]["actionable"])
        state = advance(state, report(good), DAY + timedelta(days=2))
        self.assertFalse(state["records"][identity(bad)]["actionable"])

    def test_removed_targets_resolve_without_a_network_request(self):
        item = observation(kind="local")
        state = advance(None, report(item), DAY)
        state = advance(state, report(), DAY + timedelta(days=1))
        state = advance(state, report(), DAY + timedelta(days=2))
        self.assertFalse(state["records"][identity(item)]["actionable"])

    def test_cache_loss_does_not_close_an_existing_alert(self):
        item = observation()
        previous = restore_reported({identity(item): {**item, "actionable": True}})
        state = advance(previous, report(item), DAY)
        self.assertTrue(state["records"][identity(item)]["actionable"])

    def test_incomplete_or_pr_reports_never_update_history(self):
        for invalid in ({**report(), "complete": False}, {**report(), "scope": "pr"}):
            with self.subTest(report=invalid), self.assertRaises(ValueError):
                advance(None, invalid, DAY)

    def test_unknown_results_break_the_failure_window_without_resolving_alerts(self):
        item = observation()
        state = advance(None, report(item), DAY)
        state = advance(state, report(observation("unknown")), DAY + timedelta(days=1))
        state = advance(state, report(item), DAY + timedelta(days=7))
        self.assertFalse(state["records"][identity(item)]["actionable"])


class HistoryTimingTests(unittest.TestCase):
    def test_inconclusive_days_preserve_short_gaps_but_reset_long_outages(self):
        state = observe(None, 1)
        for day in range(2, 15):
            state = observe(state, day, "unknown")
            self.assertEqual(
                record(state)["first_failed"], "2026-01-01" if day <= 3 else None
            )
        for day in range(15, 20):
            state = observe(state, day)
            self.assertEqual(record(state)["first_failed"], "2026-01-15")
            self.assertFalse(record(state)["actionable"])
        state = history(
            (day, "unknown" if day == 5 else "missing") for day in range(1, 9)
        )
        self.assertTrue(record(state)["actionable"])

    def test_older_same_day_report_cannot_replace_recovery(self):
        state = history([(1, "missing"), (2, "ok"), (3, "ok")], kind="local")
        with self.assertRaisesRegex(ValueError, "older"):
            observe(state, 3, kind="local", stamp="2026-01-03T10:00:00+00:00")

    def test_five_recent_observations_and_short_gaps_are_required(self):
        # Four recent failures at day 9 are too few despite eight days of downtime.
        for days, actionable in (
            ((1, 3, 5, 7, 8), True),
            ((1, 3, 5, 7, 9), False),
            ((1, 3, 5, 8), False),
        ):
            with self.subTest(days=days):
                state = history((day, "missing") for day in days)
                self.assertEqual(record(state)["actionable"], actionable)

    def test_source_order_and_line_selection_are_stable(self):
        original = {
            **item(),
            "sources": [
                {"path": "z.md", "line": 9},
                {"path": "a.md", "line": 8},
                {"path": "a.md", "line": 2},
            ],
        }
        first = by_target([original])
        self.assertEqual(
            first,
            by_target([{**original, "sources": list(reversed(original["sources"]))}]),
        )
        self.assertEqual(
            next(iter(first.values()))["sources"],
            [{"path": "a.md", "line": 2}, {"path": "z.md", "line": 9}],
        )

    def test_manual_closure_allows_only_a_new_outage_after_recovery(self):
        for closed_day in (1, 3):
            for gap in ([], [(4, "unknown")]):
                with self.subTest(closed_day=closed_day, gap=gap):
                    state = observe(None, 1, kind="github")
                    closed = {
                        **issue(issue_plan(None, state)["body"], "closed"),
                        "closed_at": f"2026-01-0{closed_day}T15:00:00+00:00",
                    }
                    self.assertEqual(issue_plan(closed, state)["action"], "none")
                    events = [(2, "ok"), (3, "ok"), *gap, (5 if gap else 4, "missing")]
                    state = history(events, kind="github", previous=state)
                    self.assertEqual(issue_plan(closed, state)["action"], "reopen")

    def test_stale_healthy_history_cannot_override_a_new_manual_closure(self):
        state = existing = None
        for day, status in ((1, "missing"), (2, "ok"), (3, "ok")):
            state = observe(
                reconcile_from_issue(state, existing), day, status, kind="github"
            )
            plan = issue_plan(existing, state)
            if plan["action"] != "none":
                existing = issue(
                    plan["body"], "closed" if plan["action"] == "close" else "open"
                )
        stale = copy.deepcopy(state)
        state = observe(reconcile_from_issue(state, existing), 4, kind="github")
        plan = issue_plan(existing, state)
        self.assertEqual(plan["action"], "reopen")
        closed = {
            **issue(plan["body"], "closed"),
            "closed_at": "2026-01-04T15:00:00+00:00",
        }
        state = observe(reconcile_from_issue(stale, closed), 5, kind="github")
        self.assertEqual(issue_plan(closed, state)["action"], "none")


class HistoryRecoveryTests(unittest.TestCase):
    def closed_incident(self):
        state = history((day, "missing") for day in range(1, 9))
        existing = issue(issue_plan(None, state)["body"])
        first_good = observe(state, 9, "ok")
        recovered = observe(first_good, 10, "ok")
        closed = issue(issue_plan(existing, recovered)["body"], "closed")
        return state, first_good, closed

    def test_closure_keeps_unreported_outage_from_the_same_audit(self):
        state = existing = None
        for offset in range(3):
            local = observation(
                "missing" if offset == 0 else "ok", "local", "repo:/missing.md"
            )
            state = advance(
                state, report(observation(), local), DAY + timedelta(days=offset)
            )
            plan = issue_plan(existing, state)
            if "body" in plan:
                existing = issue(
                    plan["body"], "closed" if plan["action"] == "close" else "open"
                )
        state = observe(reconcile_from_issue(state, existing), 4)
        external = next(r for r in state["records"].values() if r["kind"] == "external")
        self.assertEqual(external["first_failed"], DAY.isoformat())
        self.assertEqual(len(external["samples"]), 4)

    def test_stale_registry_cannot_reopen_after_confirmed_recovery(self):
        _, stale, closed = self.closed_incident()
        for status in ("unknown", "restricted"):
            state = observe(reconcile_from_issue(stale, closed), 11, status)
            self.assertEqual(issue_plan(closed, state)["action"], "none")

    def test_new_external_failure_after_closure_starts_a_fresh_week(self):
        stale, _, closed = self.closed_incident()
        state = observe(reconcile_from_issue(stale, closed), 11)
        self.assertFalse(record(state)["actionable"])
        self.assertEqual(record(state)["first_failed"], "2026-01-11")

    def test_closed_checkpoint_preserves_proven_newer_recurrence(self):
        _, stale, closed = self.closed_incident()
        state = history(
            ((day, "missing") for day in range(11, 19)),
            previous=reconcile_from_issue(stale, closed),
        )
        restored = reconcile_from_issue(state, closed)
        self.assertTrue(record(restored)["actionable"])
        self.assertEqual(record(restored)["first_failed"], "2026-01-11")

    def test_recovery_snapshot_does_not_count_a_midnight_crossing_twice(self):
        state = history((day, "missing") for day in range(1, 9))
        state = observe(state, 9, "ok", stamp="2026-01-08T23:59:00+00:00")
        existing = issue(issue_plan(None, state)["body"])
        repeated = observe(
            reconcile_from_issue(state, existing),
            9,
            "ok",
            stamp="2026-01-09T08:00:00+00:00",
        )
        self.assertTrue(record(repeated)["actionable"])

    def test_same_day_new_publication_is_not_mistaken_for_earlier_recovery(self):
        state = observe(None, 1, kind="github")
        for day in (2, 3):
            state = observe(
                state, day, "ok", kind="github", stamp=f"2026-01-0{day}T01:00:00+00:00"
            )
        published = observe(state, 3, kind="github", stamp="2026-01-03T08:00:00+00:00")
        existing = issue(issue_plan(None, published)["body"])
        restored = observe(
            reconcile_from_issue(state, existing),
            3,
            "ok",
            kind="github",
            stamp="2026-01-03T09:00:00+00:00",
        )
        self.assertTrue(record(restored)["actionable"])

    def test_expired_samples_do_not_preserve_failure_age_across_a_month_gap(self):
        state = observe(None, 1)
        for day in range(10, 18):
            state = observe(state, date(2026, 2, day))
            self.assertEqual(record(state)["first_failed"], "2026-02-10")
            self.assertEqual(record(state)["actionable"], day == 17)

    def test_published_alert_is_reconciled_with_older_saved_history(self):
        saved = history((day, "missing") for day in range(1, 8))
        existing = issue(issue_plan(None, observe(saved, 8))["body"])
        once = observe(reconcile_from_issue(saved, existing), 9, "ok")
        self.assertTrue(record(once)["actionable"])
        self.assertNotEqual(issue_plan(existing, once)["action"], "close")
        twice = observe(reconcile_from_issue(once, existing), 10, "ok")
        self.assertFalse(record(twice)["actionable"])

    def test_newer_publication_invalidates_recovery_from_an_older_incident(self):
        state = history((day, "missing") for day in range(1, 9))
        saved = history([(9, "ok"), (10, "ok")], previous=state)
        state = history(((day, "missing") for day in range(11, 19)), previous=saved)
        existing = issue(issue_plan(None, state)["body"])
        once = observe(reconcile_from_issue(saved, existing), 19, "ok")
        self.assertTrue(record(once)["actionable"])

    def test_reconciliation_preserves_recovery_evidence_after_publication(self):
        state = history((day, "missing") for day in range(1, 9))
        existing = issue(issue_plan(None, state)["body"])
        recovered = history([(9, "ok"), (10, "ok")], previous=state)
        self.assertFalse(
            record(reconcile_from_issue(recovered, existing))["actionable"]
        )


class RegistryValidationTests(unittest.TestCase):
    """Registries come from earlier artifacts; one bad record rejects all of it."""

    def test_malformed_registry_is_rejected_whole(self):
        saved = observe(None, 1)
        key, valid = next(iter(saved["records"].items()))
        record_cases = {
            "url": ["javascript:alert(1)", "history:" + "z" * 64, 5],
            "kind": ["other"],
            "status": ["broken", ["missing"]],
            "detail": [5, "x" * 2001, "first\rsecond"],
            "sources": [[{"path": "../outside.md"}], "README.md"],
            "actionable": ["false", 1, None],
            "first_failed": [20260101, "20260101", "2026-1-1", "yesterday"],
            "last_seen": ["2026-01-01T00:00:00", None],
            "last_failure": ["2026-02-30"],
            "last_referenced": [None],
            "last_success": [20260101],
            "observed_day": ["today"],
            "samples": [
                "2026-01-01",
                ["2026-01-01"],
                [{"date": "2026-02-30", "status": "missing"}],
                [{"date": "2026-01-01", "status": "gone"}],
                [{"date": "2026-01-01", "status": "missing", "checked_at": 5}],
            ],
            "checked_at": [5, "later"],
            "observed_at": [None],
            "recovered_at": ["soon"],
            "outage_started_at": [[]],
            "publication_id": ["abc", 5],
        }
        state_cases = {
            "records": [[], None],
            "last_run": ["2026/01/01", 5],
            "checked_at": [5],
            "issue_number": ["42", True, 0, -1],
            "closed_checkpoint": [5, "x"],
        }
        cases = [
            (field, value, {**saved, "records": {key: {**valid, field: value}}})
            for field, values in record_cases.items()
            for value in values
        ]
        cases += [
            (field, value, {**saved, field: value})
            for field, values in state_cases.items()
            for value in values
        ]
        cases.append(("identity", "not-a-digest", {**saved, "records": {"x": valid}}))
        for field, value, previous in cases:
            with (
                self.subTest(field=field, value=repr(value)[:40]),
                self.assertRaisesRegex(ValueError, "link history"),
            ):
                observe(previous, 2)

    def test_restored_registry_shapes_remain_valid(self):
        saved = history((day, "missing") for day in range(1, 9))
        existing = issue(issue_plan(None, saved)["body"])
        restored = reconcile_from_issue(None, existing)
        self.assertTrue(record(restored)["url"].startswith("https://"))
        restored["records"][identity(observation())]["url"] = "history:" + identity(
            observation()
        )
        published = {**saved, "issue_number": 42, "closed_checkpoint": "0" * 64}
        for previous in (restored, published):
            with self.subTest(keys=sorted(previous)):
                self.assertTrue(record(observe(previous, 9))["actionable"])

    def test_reconciliation_rejects_a_malformed_registry_before_merging(self):
        # The published snapshot would overwrite this detail; the malformed
        # registry must still be rejected rather than partly reused.
        saved = observe(None, 1)
        existing = issue(
            issue_plan(None, history((day, "missing") for day in range(1, 9)))["body"]
        )
        record(saved)["detail"] = {"html": "<img>"}
        with self.assertRaisesRegex(ValueError, "link history"):
            reconcile_from_issue(saved, existing)
