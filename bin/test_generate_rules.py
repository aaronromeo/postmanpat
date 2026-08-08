#!/usr/bin/env python3
"""Tests for suppression annotation handling in the rule generator."""
import contextlib
import importlib.util
import io
import sys
import unittest
from pathlib import Path
from unittest import mock

BIN = Path(__file__).resolve().parent


def load_module(name: str):
    spec = importlib.util.spec_from_file_location(name, BIN / f"{name}.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


mod = load_module("postmanpat-generate-rules")


class TestProcessClusterSuppression(unittest.TestCase):
    def _cluster(self, suppressed=None):
        return {
            "cluster_id": "list_lens:abc",
            "count": 5,
            "latest_date": "2026-08-01T00:00:00Z",
            "keys": {"ListID": "some.list"},
            "signals": {"has_list_id": True, "has_list_unsubscribe": True, "precedence_categories": {}},
            "examples": {"subject_raw": ["Hello"], "recipients": [], "reply_to_domains": [],
                         "sender_domains": [], "returnpath_domains": [], "list_unsubscribe_targets": []},
            "suppressed": suppressed or [],
        }

    def test_watch_suppressed_skips_watch_prompt(self):
        asked = []
        def fake_prompt(message, default=False, allow_ignore=False):
            asked.append(message)
            return "n"

        watch_rules = []
        cleanup_rules = []
        buf = io.StringIO()
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            with contextlib.redirect_stdout(buf):
                proceed, _ = mod.process_cluster(
                    "list_lens", self._cluster(suppressed=["watch"]),
                    watch_rules, cleanup_rules, "INBOX",
                )
        self.assertTrue(proceed)
        self.assertNotIn("Generate watch rule?", asked)
        self.assertIn("Generate cleanup rule?", asked)
        self.assertEqual(watch_rules, [])
        output = buf.getvalue()
        self.assertIn("Watch rule suppressed by ignore list.", output)

    def test_cleanup_suppressed_skips_cleanup_prompt(self):
        asked = []
        def fake_prompt(message, default=False, allow_ignore=False):
            asked.append(message)
            return "n"

        watch_rules = []
        cleanup_rules = []
        buf = io.StringIO()
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            with contextlib.redirect_stdout(buf):
                proceed, _ = mod.process_cluster(
                    "list_lens", self._cluster(suppressed=["cleanup"]),
                    watch_rules, cleanup_rules, "INBOX",
                )
        self.assertTrue(proceed)
        self.assertIn("Generate watch rule?", asked)
        self.assertNotIn("Generate cleanup rule?", asked)
        self.assertEqual(cleanup_rules, [])
        output = buf.getvalue()
        self.assertIn("Cleanup rule suppressed by ignore list.", output)

    def test_both_suppressed_skips_both_prompts(self):
        asked = []
        def fake_prompt(message, default=False, allow_ignore=False):
            asked.append(message)
            return "n"

        watch_rules = []
        cleanup_rules = []
        buf = io.StringIO()
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            with contextlib.redirect_stdout(buf):
                proceed, _ = mod.process_cluster(
                    "list_lens", self._cluster(suppressed=["watch", "cleanup"]),
                    watch_rules, cleanup_rules, "INBOX",
                )
        self.assertTrue(proceed)
        self.assertNotIn("Generate watch rule?", asked)
        self.assertNotIn("Generate cleanup rule?", asked)
        output = buf.getvalue()
        self.assertIn("Watch rule suppressed by ignore list.", output)
        self.assertIn("Cleanup rule suppressed by ignore list.", output)

    def test_no_suppression_asks_both(self):
        asked = []
        def fake_prompt(message, default=False, allow_ignore=False):
            asked.append(message)
            return "n"

        watch_rules = []
        cleanup_rules = []
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            proceed, _ = mod.process_cluster(
                "list_lens", self._cluster(suppressed=[]),
                watch_rules, cleanup_rules, "INBOX",
            )
        self.assertTrue(proceed)
        self.assertIn("Generate watch rule?", asked)
        self.assertIn("Generate cleanup rule?", asked)

    def test_no_suppressed_field_asks_both(self):
        asked = []
        def fake_prompt(message, default=False, allow_ignore=False):
            asked.append(message)
            return "n"

        cluster = self._cluster()
        del cluster["suppressed"]
        watch_rules = []
        cleanup_rules = []
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            proceed, _ = mod.process_cluster(
                "list_lens", cluster,
                watch_rules, cleanup_rules, "INBOX",
            )
        self.assertTrue(proceed)
        self.assertIn("Generate watch rule?", asked)
        self.assertIn("Generate cleanup rule?", asked)


class TestIgnorePureFunctions(unittest.TestCase):
    def test_extract_list_lens(self):
        cluster = {"keys": {"ListID": "github.notifications"}}
        result = mod.extract_ignore_identity("list_lens", cluster)
        self.assertEqual(result, {"list_ids": ["github.notifications"]})

    def test_extract_sender_unsub_single_domain(self):
        cluster = {"keys": {"SenderDomains": ["github.com"]}}
        result = mod.extract_ignore_identity("sender_unsub_lens", cluster)
        self.assertEqual(result, {"sender_domains": ["github.com"]})

    def test_extract_sender_unsub_multi_domain(self):
        cluster = {"keys": {"SenderDomains": ["github.com", "actions.githubusercontent.com"]}}
        result = mod.extract_ignore_identity("sender_unsub_lens", cluster)
        self.assertEqual(set(result["sender_domains"]), {"github.com", "actions.githubusercontent.com"})
        self.assertEqual(len(result["sender_domains"]), 2)

    def test_extract_recipient_tag(self):
        cluster = {"keys": {"recipient_tag": "newsletter,weekly"}}
        result = mod.extract_ignore_identity("recipient_tag_lens", cluster)
        self.assertEqual(result, {"recipient_tags": ["newsletter,weekly"]})

    def test_extract_unknown_lens(self):
        cluster = {"keys": {"ListID": "x"}}
        result = mod.extract_ignore_identity("template_lens", cluster)
        self.assertEqual(result, {})

    def test_extract_missing_key(self):
        cluster = {"keys": {}}
        result = mod.extract_ignore_identity("list_lens", cluster)
        self.assertEqual(result, {})

    def test_merge_entries(self):
        acc = {"list_ids": ["a"]}
        mod.merge_ignore_entries(acc, {"list_ids": ["b"], "sender_domains": ["x.com"]})
        self.assertEqual(acc["list_ids"], ["a", "b"])
        self.assertEqual(acc["sender_domains"], ["x.com"])

    def test_merge_into_empty(self):
        acc = {}
        mod.merge_ignore_entries(acc, {"list_ids": ["a"]})
        self.assertEqual(acc, {"list_ids": ["a"]})

    def test_dedup_entries(self):
        entries = {"list_ids": ["b", "a", "b"], "sender_domains": []}
        result = mod.dedup_ignore_entries(entries)
        self.assertEqual(result, {"list_ids": ["a", "b"]})

    def test_dedup_drops_empty(self):
        entries = {"list_ids": ["a"], "sender_domains": []}
        result = mod.dedup_ignore_entries(entries)
        self.assertEqual(result, {"list_ids": ["a"]})
        self.assertNotIn("sender_domains", result)

    def test_build_ignore_fragment_empty(self):
        result = mod.build_ignore_fragment({}, {})
        self.assertEqual(result, {})

    def test_build_ignore_fragment_watch_only(self):
        result = mod.build_ignore_fragment({"list_ids": ["a"]}, {})
        self.assertEqual(result, {"ignore": {"watch": {"list_ids": ["a"]}}})

    def test_build_ignore_fragment_both(self):
        result = mod.build_ignore_fragment(
            {"list_ids": ["a"]},
            {"sender_domains": ["x.com"]},
        )
        self.assertIn("watch", result["ignore"])
        self.assertIn("cleanup", result["ignore"])


class TestProcessClusterAuthoring(unittest.TestCase):
    def _cluster(self, suppressed=None):
        return {
            "cluster_id": "list_lens:abc",
            "count": 5,
            "latest_date": "2026-08-01T00:00:00Z",
            "keys": {"ListID": "some.list"},
            "signals": {"has_list_id": True, "has_list_unsubscribe": True, "precedence_categories": {}},
            "examples": {"subject_raw": ["Hello"], "recipients": [], "reply_to_domains": [],
                         "sender_domains": [], "returnpath_domains": [], "list_unsubscribe_targets": []},
            "suppressed": suppressed or [],
        }

    def test_watch_i_records_to_watch_accumulator(self):
        ignore_watch = {}
        ignore_cleanup = {}
        responses = iter(["i", "n", "n"])
        def fake_prompt(message, default=False, allow_ignore=False):
            return next(responses)

        watch_rules = []
        cleanup_rules = []
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            with mock.patch.object(mod, "_prompt_yes_no_simple", return_value=False):
                proceed, _ = mod.process_cluster(
                    "list_lens", self._cluster(),
                    watch_rules, cleanup_rules, "INBOX",
                    ignore_watch=ignore_watch,
                    ignore_cleanup=ignore_cleanup,
                )
        self.assertTrue(proceed)
        self.assertEqual(ignore_watch, {"list_ids": ["some.list"]})
        self.assertEqual(ignore_cleanup, {})

    def test_watch_i_followup_y_records_to_both_skips_cleanup_prompt(self):
        ignore_watch = {}
        ignore_cleanup = {}
        responses = iter(["i"])
        def fake_prompt(message, default=False, allow_ignore=False):
            return next(responses)

        watch_rules = []
        cleanup_rules = []
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            with mock.patch.object(mod, "_prompt_yes_no_simple", return_value=True):
                proceed, _ = mod.process_cluster(
                    "list_lens", self._cluster(),
                    watch_rules, cleanup_rules, "INBOX",
                    ignore_watch=ignore_watch,
                    ignore_cleanup=ignore_cleanup,
                )
        self.assertTrue(proceed)
        self.assertEqual(ignore_watch, {"list_ids": ["some.list"]})
        self.assertEqual(ignore_cleanup, {"list_ids": ["some.list"]})
        self.assertEqual(watch_rules, [])
        self.assertEqual(cleanup_rules, [])

    def test_cleanup_i_records_to_cleanup_only(self):
        ignore_watch = {}
        ignore_cleanup = {}
        responses = iter(["n", "i"])
        def fake_prompt(message, default=False, allow_ignore=False):
            return next(responses)

        watch_rules = []
        cleanup_rules = []
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            proceed, _ = mod.process_cluster(
                "list_lens", self._cluster(),
                watch_rules, cleanup_rules, "INBOX",
                ignore_watch=ignore_watch,
                ignore_cleanup=ignore_cleanup,
            )
        self.assertTrue(proceed)
        self.assertEqual(ignore_watch, {})
        self.assertEqual(ignore_cleanup, {"list_ids": ["some.list"]})


if __name__ == "__main__":
    unittest.main(verbosity=2)
