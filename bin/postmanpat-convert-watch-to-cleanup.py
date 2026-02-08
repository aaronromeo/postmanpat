#!/usr/bin/env python3
import argparse
import re
import sys
from typing import Any, Dict, List

try:
    import yaml  # type: ignore
except Exception as exc:  # pragma: no cover
    print("error: PyYAML is required for this script (pip install pyyaml)", file=sys.stderr)
    raise


def yaml_quote(value: str) -> str:
    if "\\" in value:
        escaped = value.replace("'", "''")
        return f"'{escaped}'"
    escaped = value.replace("\\", "\\\\").replace('"', "\\\"")
    return f"\"{escaped}\""


def yaml_lines(value: Any, indent: int = 0) -> List[str]:
    prefix = " " * indent
    lines: List[str] = []
    if isinstance(value, dict):
        for key, item in value.items():
            if item is None:
                continue
            if isinstance(item, (dict, list)):
                lines.append(f"{prefix}{key}:")
                lines.extend(yaml_lines(item, indent + 2))
            else:
                lines.append(f"{prefix}{key}: {yaml_quote(str(item))}")
        return lines
    if isinstance(value, list):
        for item in value:
            if isinstance(item, dict):
                if not item:
                    lines.append(f"{prefix}- {{}}")
                    continue
                first_key = next(iter(item))
                first_val = item[first_key]
                rest = {k: v for k, v in item.items() if k != first_key}
                if isinstance(first_val, (dict, list)):
                    lines.append(f"{prefix}- {first_key}:")
                    lines.extend(yaml_lines(first_val, indent + 2))
                else:
                    lines.append(f"{prefix}- {first_key}: {yaml_quote(str(first_val))}")
                if rest:
                    lines.extend(yaml_lines(rest, indent + 2))
            else:
                lines.append(f"{prefix}- {yaml_quote(str(item))}")
        return lines
    lines.append(f"{prefix}{yaml_quote(str(value))}")
    return lines


def write_yaml(path: str, data: Dict[str, Any]) -> None:
    lines = yaml_lines(data)
    with open(path, "w", encoding="utf-8") as handle:
        handle.write("\n".join(lines))
        handle.write("\n")


def regex_to_substring(pattern: str) -> str:
    # Strip common regex escaping used for literal matches.
    # This is intentionally simple and assumes the regex was built from a literal.
    return re.sub(r"\\(.)", r"\1", pattern)


def warn_if_regexy(original: str) -> None:
    # Warn if the pattern still looks like a regex (wildcards/anchors/character classes).
    if re.search(r"[\^\$\|\[\]\(\)\?\*\+]", original):
        print(
            f"warning: pattern looks like a regex; conversion is best-effort: {original}",
            file=sys.stderr,
        )


def convert_rule(rule: Dict[str, Any], folders: List[str]) -> Dict[str, Any]:
    name = rule.get("name", "")
    client = rule.get("client") or {}
    if not client:
        raise ValueError(f"rule {name!r} has no client matchers")
    if "server" in rule and rule["server"]:
        raise ValueError(f"rule {name!r} already has server matchers")

    server: Dict[str, Any] = {
        "folders": folders,
    }

    def map_list(src_key: str, dest_key: str) -> None:
        values = client.get(src_key) or []
        if not values:
            return
        mapped: List[str] = []
        for value in values:
            if not isinstance(value, str):
                continue
            warn_if_regexy(value)
            mapped.append(regex_to_substring(value))
        if mapped:
            server[dest_key] = mapped

    map_list("list_id_regex", "list_id_substring")
    map_list("sender_regex", "sender_substring")
    map_list("replyto_regex", "replyto_substring")
    map_list("recipients_regex", "recipients")

    converted = {
        "name": name,
        "server": server,
        "actions": rule.get("actions", []),
    }
    return converted


def main() -> int:
    parser = argparse.ArgumentParser(description="Convert watch rules to cleanup rules.")
    parser.add_argument("--watch", required=True, help="Path to watch YAML config")
    parser.add_argument("--out", required=True, help="Path to write cleanup YAML")
    parser.add_argument(
        "--folders",
        default="INBOX",
        help="Comma-separated IMAP source folders for cleanup rules (default: INBOX)",
    )
    args = parser.parse_args()

    with open(args.watch, "r", encoding="utf-8") as handle:
        data = yaml.safe_load(handle)

    if not isinstance(data, dict) or "rules" not in data:
        print("error: watch config must include rules", file=sys.stderr)
        return 1

    folders = [item.strip() for item in args.folders.split(",") if item.strip()]
    if not folders:
        print("error: at least one folder is required", file=sys.stderr)
        return 1

    rules = data.get("rules") or []
    converted_rules: List[Dict[str, Any]] = []
    for rule in rules:
        if not isinstance(rule, dict):
            continue
        converted_rules.append(convert_rule(rule, folders))

    write_yaml(args.out, {"rules": converted_rules})
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
