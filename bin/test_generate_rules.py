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


if __name__ == "__main__":
    unittest.main(verbosity=2)
