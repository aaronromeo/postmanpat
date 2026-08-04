#!/usr/bin/env python3
"""Regression tests: generated YAML must emit booleans unquoted.

Bug: yaml_lines() used yaml_quote(str(item)) for all scalars, turning
Python True/False into the quoted strings "True"/"False", which the Go
config parser rejects ("cannot unmarshal !!str `False` into bool").

Run: python3 bin/test_yaml_scalar.py
"""
import importlib.util
import sys
import unittest
from pathlib import Path

import yaml

BIN = Path(__file__).resolve().parent


def load_module(name: str):
    spec = importlib.util.spec_from_file_location(name, BIN / f"{name}.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


SCRIPTS = [
    "postmanpat-convert-watch-to-cleanup",
    "postmanpat-generate-rules",
]


class TestYamlScalarBooleans(unittest.TestCase):
    def load(self):
        for name in SCRIPTS:
            yield name, load_module(name)

    def test_dict_value_bool_unquoted(self):
        for name, mod in self.load():
            with self.subTest(script=name):
                out = mod.yaml_lines({"list_unsubscribe": False})
                self.assertEqual(out, ["list_unsubscribe: false"])
                out = mod.yaml_lines({"list_unsubscribe": True})
                self.assertEqual(out, ["list_unsubscribe: true"])

    def test_list_of_dict_first_value_bool_unquoted(self):
        for name, mod in self.load():
            with self.subTest(script=name):
                out = mod.yaml_lines([{"expunge_after_delete": True, "type": "delete"}])
                self.assertEqual(out[0], "- expunge_after_delete: true")
                self.assertEqual(out[1], '  type: "delete"')

    def test_list_scalar_bool_unquoted(self):
        for name, mod in self.load():
            with self.subTest(script=name):
                out = mod.yaml_lines([False])
                self.assertEqual(out, ["- false"])

    def test_strings_stay_quoted(self):
        for name, mod in self.load():
            with self.subTest(script=name):
                out = mod.yaml_lines({"destination": "@News"})
                self.assertEqual(out, ['destination: "@News"'])

    def test_round_trip_parses_as_bool(self):
        fixture = {
            "rules": [
                {
                    "name": "eBay",
                    "client": {
                        "sender_regex": ["ebay\\.com"],
                        "list_unsubscribe": False,
                    },
                    "actions": [
                        {"type": "delete", "expunge_after_delete": True},
                    ],
                }
            ]
        }
        for name, mod in self.load():
            with self.subTest(script=name):
                text = "\n".join(mod.yaml_lines(fixture)) + "\n"
                self.assertNotIn('"True"', text)
                self.assertNotIn('"False"', text)
                parsed = yaml.safe_load(text)
                rule = parsed["rules"][0]
                self.assertIs(rule["client"]["list_unsubscribe"], False)
                self.assertIs(rule["actions"][0]["expunge_after_delete"], True)


if __name__ == "__main__":
    unittest.main(verbosity=2)
