#!/usr/bin/env python3
import argparse
import json
import os
import re
import sys
from typing import Any, Dict, List, Optional, Tuple


def load_analyze(path: str) -> Dict[str, Any]:
    with open(path, "r", encoding="utf-8") as handle:
        return json.load(handle)


def escape_regex(value: str) -> str:
    return re.escape(value)


def split_list(value: str) -> List[str]:
    return [item.strip() for item in value.split(",") if item.strip()]


def prompt(message: str, default: Optional[str] = None, required: bool = False) -> str:
    while True:
        suffix = ""
        if default is not None and default != "":
            suffix = f" [{default}]"
        response = input(f"{message}{suffix}: ").strip()
        if response == "" and default is not None:
            return default
        if response == "" and not required:
            return ""
        if response != "":
            return response
        print("Value required.")


def prompt_yes_no(message: str, default: bool = False) -> str:
    default_hint = "y" if default else "n"
    response = input(f"{message} [y/n/q] (default {default_hint}): ").strip().lower()
    if response == "":
        return "y" if default else "n"
    if response in ("q", "quit"):
        return "q"
    if response not in ("y", "n"):
        return "n"
    return response


def yaml_quote(value: str) -> str:
    escaped = value.replace("\\", "\\\\").replace("\"", "\\\"")
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


def load_checkpoint(path: str) -> Optional[Dict[str, str]]:
    if not path or not os.path.exists(path):
        return None
    with open(path, "r", encoding="utf-8") as handle:
        return json.load(handle)


def save_checkpoint(path: str, lens: str, cluster_id: str) -> None:
    if not path:
        return
    with open(path, "w", encoding="utf-8") as handle:
        json.dump({"lens": lens, "cluster_id": cluster_id}, handle, indent=2)
        handle.write("\n")


def cluster_summary(cluster: Dict[str, Any]) -> str:
    keys = cluster.get("keys", {})
    count = cluster.get("count")
    return f"cluster_id={cluster.get('cluster_id')} count={count} keys={keys}"


def format_examples(cluster: Dict[str, Any], limit: int = 3) -> List[str]:
    examples = cluster.get("examples", {}) or {}
    lines: List[str] = []
    for label in ("subject_raw", "recipients", "reply_to_domains", "list_unsubscribe_targets"):
        values = examples.get(label, []) or []
        if not values:
            continue
        preview = values[:limit]
        lines.append(f"{label}: {', '.join(str(v) for v in preview)}")
    latest_date = cluster.get("latest_date")
    if latest_date:
        lines.append(f"latest_date: {latest_date}")
    return lines


def prompt_rule_action() -> Dict[str, Any]:
    action = prompt("Action (delete/move)", default="delete", required=True).lower()
    if action not in ("delete", "move"):
        print("Invalid action; using delete.")
        action = "delete"
    if action == "move":
        destination = prompt("Move destination", required=True)
        return {"type": "move", "destination": destination}
    return {"type": "delete"}


def prompt_folders(default_folders: Optional[str]) -> List[str]:
    while True:
        raw = prompt("IMAP Source folders (comma-separated)", default=default_folders, required=True)
        folders = split_list(raw)
        if folders:
            return folders
        print("At least one folder is required.")


def prompt_optional_list(message: str, defaults: List[str], allow_note: Optional[str] = None) -> Optional[List[str]]:
    if allow_note:
        print(allow_note)
    response = prompt_yes_no(f"Add {message}?", default=False)
    if response != "y":
        return None
    default_value = ", ".join(defaults) if defaults else ""
    raw = prompt(f"{message} (comma-separated)", default=default_value)
    values = split_list(raw)
    return values if values else []


def rule_name_prompt(lens: str, cluster: Dict[str, Any]) -> str:
    print(f"\nRule for {lens}:")
    print(cluster_summary(cluster))
    return prompt("Rule name", required=True)


def build_watch_rule_list(cluster: Dict[str, Any], name: str) -> Optional[Dict[str, Any]]:
    key = str(cluster.get("keys", {}).get("ListID", "")).strip()
    default_regex = escape_regex(key) if key else ""
    regex = prompt("list_id_regex", default=default_regex, required=True)
    rule = {
        "name": name,
        "client": {
            "list_id_regex": [regex],
        },
        "actions": [prompt_rule_action()],
    }
    return rule


def build_cleanup_rule_list(cluster: Dict[str, Any], name: str, default_folders: Optional[str]) -> Tuple[Optional[Dict[str, Any]], str]:
    key = str(cluster.get("keys", {}).get("ListID", "")).strip()
    substring = prompt("list_id_substring", default=key, required=True)
    folders = prompt_folders(default_folders)
    rule = {
        "name": name,
        "server": {
            "folders": folders,
            "list_id_substring": [substring],
        },
        "actions": [prompt_rule_action()],
    }
    return rule, ", ".join(folders)


def build_watch_rule_sender(cluster: Dict[str, Any], name: str) -> Optional[Dict[str, Any]]:
    sender_domains = cluster.get("keys", {}).get("SenderDomains", []) or []
    sender_defaults = [escape_regex(str(domain)) for domain in sender_domains]
    sender_prompt_default = ", ".join(sender_defaults)
    raw_sender = prompt("sender_regex (comma-separated)", default=sender_prompt_default, required=True)
    sender_values = split_list(raw_sender)
    rule = {
        "name": name,
        "client": {
            "sender_regex": sender_values,
        },
        "actions": [prompt_rule_action()],
    }
    reply_defaults = cluster.get("examples", {}).get("reply_to_domains", []) or []
    reply_defaults = [escape_regex(str(value)) for value in reply_defaults]
    if reply_defaults:
        reply_values = prompt_optional_list("replyto_regex", reply_defaults)
        if reply_values is not None:
            rule["client"]["replyto_regex"] = reply_values
    recipients_defaults = cluster.get("examples", {}).get("recipients", []) or []
    recipients_defaults = [escape_regex(str(value)) for value in recipients_defaults]
    recipients_values = prompt_optional_list(
        "recipients_regex",
        recipients_defaults,
        allow_note="Note: client.recipients_regex is not implemented yet.",
    )
    if recipients_values is not None:
        rule["client"]["recipients_regex"] = recipients_values
    return rule


def build_cleanup_rule_sender(cluster: Dict[str, Any], name: str, default_folders: Optional[str]) -> Tuple[Optional[Dict[str, Any]], str]:
    sender_domains = cluster.get("keys", {}).get("SenderDomains", []) or []
    sender_defaults = ", ".join(str(domain) for domain in sender_domains)
    raw_sender = prompt("sender_substring (comma-separated)", default=sender_defaults, required=True)
    sender_values = split_list(raw_sender)
    folders = prompt_folders(default_folders)
    rule = {
        "name": name,
        "server": {
            "folders": folders,
            "sender_substring": sender_values,
        },
        "actions": [prompt_rule_action()],
    }
    reply_defaults = cluster.get("examples", {}).get("reply_to_domains", []) or []
    if reply_defaults:
        reply_values = prompt_optional_list("replyto_substring", [str(value) for value in reply_defaults])
        if reply_values is not None:
            rule["server"]["replyto_substring"] = reply_values
    recipients_defaults = cluster.get("examples", {}).get("recipients", []) or []
    recipients_values = prompt_optional_list("recipients", [str(value) for value in recipients_defaults])
    if recipients_values is not None:
        rule["server"]["recipients"] = recipients_values
    return rule, ", ".join(folders)


def process_cluster(
    lens: str,
    cluster: Dict[str, Any],
    watch_rules: List[Dict[str, Any]],
    cleanup_rules: List[Dict[str, Any]],
    default_folders: Optional[str],
) -> Tuple[bool, Optional[str]]:
    print("\n=== Cluster ===")
    print(cluster_summary(cluster))
    for line in format_examples(cluster):
        print(f"  {line}")

    rule_name: Optional[str] = None

    watch_response = prompt_yes_no("Generate watch rule?", default=False)
    if watch_response == "q":
        return False, default_folders
    if watch_response == "y":
        rule_name = rule_name or rule_name_prompt(lens, cluster)
        if lens == "list_lens":
            rule = build_watch_rule_list(cluster, rule_name)
        else:
            rule = build_watch_rule_sender(cluster, rule_name)
        if rule:
            watch_rules.append(rule)

    cleanup_response = prompt_yes_no("Generate cleanup rule?", default=False)
    if cleanup_response == "q":
        return False, default_folders
    if cleanup_response == "y":
        rule_name = rule_name or rule_name_prompt(lens, cluster)
        if lens == "list_lens":
            rule, default_folders = build_cleanup_rule_list(cluster, rule_name, default_folders)
        else:
            rule, default_folders = build_cleanup_rule_sender(cluster, rule_name, default_folders)
        if rule:
            cleanup_rules.append(rule)

    return True, default_folders


def iter_clusters(indexes: Dict[str, Any]) -> List[Tuple[str, Dict[str, Any]]]:
    result: List[Tuple[str, Dict[str, Any]]] = []
    for lens in ("list_lens", "sender_lens"):
        clusters = indexes.get(lens, {}).get("clusters", []) or []
        for cluster in clusters:
            result.append((lens, cluster))
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate watch/cleanup rules from analyze output.")
    parser.add_argument("--analyze", required=True, help="Path to analyze JSON file")
    parser.add_argument("--watch-out", required=True, help="Path to write watch YAML")
    parser.add_argument("--cleanup-out", required=True, help="Path to write cleanup YAML")
    parser.add_argument(
        "--checkpoint",
        default=None,
        help="Path to checkpoint JSON (defaults to <watch-out>.checkpoint.json)",
    )
    args = parser.parse_args()

    checkpoint_path = args.checkpoint or f"{args.watch_out}.checkpoint.json"

    try:
        data = load_analyze(args.analyze)
    except (OSError, json.JSONDecodeError) as exc:
        print(f"Failed to load analyze file: {exc}", file=sys.stderr)
        return 1

    indexes = data.get("indexes", {})
    clusters = iter_clusters(indexes)

    checkpoint = load_checkpoint(checkpoint_path)
    resume_lens = checkpoint.get("lens") if checkpoint else None
    resume_cluster = checkpoint.get("cluster_id") if checkpoint else None
    resumed = resume_lens is None
    resume_found = False

    watch_rules: List[Dict[str, Any]] = []
    cleanup_rules: List[Dict[str, Any]] = []
    default_folders: Optional[str] = "INBOX"

    if checkpoint and resume_lens and resume_cluster:
        if any(lens == resume_lens and str(cluster.get("cluster_id", "")) == resume_cluster for lens, cluster in clusters):
            resume_found = True
        else:
            resumed = True
            print("Warning: checkpoint not found in analyze file; starting from beginning.", file=sys.stderr)

    for lens, cluster in clusters:
        cluster_id = str(cluster.get("cluster_id", ""))
        if not resumed:
            if lens == resume_lens and cluster_id == resume_cluster:
                resumed = True
                continue
            continue

        proceed, default_folders = process_cluster(
            lens,
            cluster,
            watch_rules,
            cleanup_rules,
            default_folders,
        )
        if not proceed:
            break
        save_checkpoint(checkpoint_path, lens, cluster_id)

    watch_config = {"rules": watch_rules}
    cleanup_config = {"rules": cleanup_rules}

    write_yaml(args.watch_out, watch_config)
    write_yaml(args.cleanup_out, cleanup_config)

    print(f"Wrote watch rules to {args.watch_out}")
    print(f"Wrote cleanup rules to {args.cleanup_out}")
    print(f"Checkpoint saved to {checkpoint_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
