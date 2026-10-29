"""One observation model shared by Markdown, JSON, and reference validation."""

import hashlib
import json
import math
import secrets
from collections import Counter
import xml.etree.ElementTree as ET
from .transport import Error

ACTION_ROLES = {
    "Button",
    "Cell",
    "Switch",
    "TextField",
    "SecureTextField",
    "SearchField",
    "Link",
    "Slider",
    "SegmentedControl",
    "Key",
}
CONTAINER_ROLES = {
    "Application",
    "NavigationBar",
    "Table",
    "CollectionView",
    "ScrollView",
    "Toolbar",
    "TabBar",
    "Cell",
    "Outline",
    "SplitGroup",
}
ATTRIBUTES = (
    "type",
    "name",
    "label",
    "value",
    "enabled",
    "selected",
    "visible",
    "accessible",
    "x",
    "y",
    "width",
    "height",
)


def bounds(attributes):
    """WDA XML geometry and native rect predicates use the same wdRect fields."""
    try:
        rect = {key: float(attributes[key]) for key in ("x", "y", "width", "height")}
    except (KeyError, ValueError):
        return None
    return rect if all(math.isfinite(value) for value in rect.values()) else None


def tree_model(source):
    try:
        root = ET.fromstring(source)
    except ET.ParseError as exc:
        raise Error(f"Invalid accessibility XML: {exc}") from exc
    nodes = []
    signature = []
    next_group = 0

    def visit(node, ancestors, containers, visible=True, row_has_button=False):
        nonlocal next_group
        attrs = {key: node.get(key, "") for key in ATTRIBUTES}
        signature.append((node.tag, attrs))
        # XCTest can mark a collection hidden while its on-screen cells are visible.
        # Honor explicit per-element flags; inherit only when the flag is absent.
        if attrs["visible"]:
            visible = attrs["visible"] == "true"
        role = (attrs["type"] or node.tag).removeprefix("XCUIElementType")
        label = attrs["label"] or attrs["name"]
        context = ancestors
        container = None
        # Keep semantic structure and named regions, but skip layout-only wrappers.
        # A hidden container can still have explicitly visible children (Bluefire).
        if len(node) and (role in CONTAINER_ROLES or (role == "Other" and label)):
            next_group += 1
            container = {
                "id": f"g{next_group}",
                "parent_id": containers[-1]["id"] if containers else None,
                "role": role,
                "name": attrs["name"],
                "label": label or role,
            }
            containers = containers + [container]
        if role == "Cell":
            row_has_button = any(
                child.get("type") == "XCUIElementTypeButton"
                and (child.get("label") or child.get("name")) == label
                and child.get("visible") != "false"
                for child in node.iter()
            )
        redundant = (
            (role == "Cell" and row_has_button)
            or (
                role == "StaticText"
                and label in ancestors
                and attrs["value"] in {"", label}
            )
            or (role == "Button" and label == "chevron" and row_has_button)
        )
        if (
            visible
            and not redundant
            and role not in {"Application", "Window", "Other"}
            and (label or attrs["value"] or role in ACTION_ROLES)
        ):
            nodes.append(
                {
                    "label": label,
                    "role": role,
                    "value": attrs["value"],
                    "enabled": attrs["enabled"] != "false",
                    "selected": attrs["selected"] == "true",
                    "actionable": role in ACTION_ROLES,
                    "context": ancestors,
                    "attributes": attrs,
                    "bounds": bounds(attrs),
                    "_containers": containers,
                    "_container_id": container["id"] if container else None,
                }
            )
        if label and (role in CONTAINER_ROLES or role == "Other"):
            context = (
                ancestors
                if ancestors and ancestors[-1] == label
                else ancestors + [label]
            )
        for child in node:
            visit(child, context, containers, visible, row_has_button)
        signature.append(None)  # Preserve hierarchy, not just a flat attribute list.

    visit(root, [], [])

    # UIKit can expose an accessible button plus an identical non-accessible child.
    # Keep the real accessibility target while retaining both in the fingerprint.
    def identity(element):
        return tuple(
            element["attributes"][key] for key in ATTRIBUTES if key != "accessible"
        )

    accessible_targets = {
        identity(e)
        for e in nodes
        if e["actionable"] and e["attributes"]["accessible"] == "true"
    }
    nodes = [
        e
        for e in nodes
        if not (
            e["actionable"]
            and e["attributes"]["accessible"] == "false"
            and identity(e) in accessible_targets
        )
    ]
    digest = hashlib.sha256(json.dumps(signature, sort_keys=True).encode()).hexdigest()
    return nodes, digest


def observe(source, device, session):
    nodes, digest = tree_model(source)
    observation_id = "s" + secrets.token_hex(8)
    groups = {}
    for index, node in enumerate(nodes, 1):
        node["ref"] = f"{observation_id}:{index}"
        containers = node.pop("_containers")
        for container in containers:
            groups.setdefault(container["id"], dict(container))
        node["group_id"] = containers[-1]["id"] if containers else None
        container_id = node.pop("_container_id")
        if container_id:
            groups[container_id]["ref"] = node["ref"]
    return {
        "device": device,
        "session": session,
        "observation": observation_id,
        "fingerprint": digest,
        "elements": nodes,
        "groups": list(groups.values()),
    }


def resolve(observation, reference, device, session, source):
    if (
        not observation
        or observation["device"] != device
        or observation["session"] != session
    ):
        raise Error(
            "Reference belongs to another device/session or was invalidated; run screen"
        )
    target = next((e for e in observation["elements"] if e["ref"] == reference), None)
    if target is None:
        raise Error("Unknown or stale reference; use the latest screen observation")
    if tree_model(source)[1] != observation["fingerprint"]:
        raise Error("Screen changed; reference rejected. Run screen again")
    if not target["actionable"] or not target["enabled"]:
        raise Error("Element is not enabled/actionable")
    return target


def predicate(target):
    # Exact native predicate; duplicate results are rejected, never picked by index.
    def quote(value):
        return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'

    attrs = target["attributes"]
    clauses = []
    for key in ("type", "name", "label", "value"):
        value = attrs[key]
        # XML omits nil native attributes and cannot distinguish nil from "".
        # Empty is still a constraint; omitting it silently broadens the query.
        clauses.append(
            f"{key} == {quote(value)}" if value else f'({key} == nil OR {key} == "")'
        )
    rect = bounds(attrs)
    if rect:
        clauses.extend(f"rect.{key} == {value!r}" for key, value in rect.items())
    clauses += ["visible == 1", "enabled == 1"]
    if attrs.get("accessible") in {"true", "false"}:
        clauses.append(f"accessible == {int(attrs['accessible'] == 'true')}")
    return " AND ".join(clauses)


def public(observation):
    return {
        **{k: v for k, v in observation.items() if k not in {"fingerprint", "session"}},
        "elements": [
            {k: v for k, v in e.items() if k != "attributes"}
            for e in observation["elements"]
        ],
    }


def element_text(element, labels, heading=False):
    detail = [element["role"]]
    if element["value"] and element["value"] != element["label"]:
        detail.append(" ".join(element["value"].splitlines()))
    if element["selected"]:
        detail.append("selected")
    if not element["enabled"]:
        detail.append("disabled")
    if not element["actionable"]:
        detail.append("text/context")
    label = " ".join(element["label"].splitlines()) or f"Unlabeled {element['role']}"
    rect = element.get("bounds")
    if rect and (
        not element["label"] or labels[(element["role"], element["label"])] > 1
    ):
        detail.append(
            f"at ({rect['x']:g}, {rect['y']:g}), {rect['width']:g}×{rect['height']:g}"
        )
    title = f"[{element['ref']}] {label}"
    if heading:
        title = f"**{title}**"
    return f"{title} — {', '.join(detail)}"


def group_path(group_id, groups):
    path = []
    seen = set()
    while group_id is not None:
        if group_id in seen or group_id not in groups:
            raise Error("Invalid observation group hierarchy; run screen again")
        seen.add(group_id)
        group = groups[group_id]
        path.append(group)
        group_id = group["parent_id"]
    return list(reversed(path))


def markdown(observation):
    lines = [
        f"Device: {observation['device']}",
        f"Observation: {observation['observation']}",
        "",
    ]
    elements = observation["elements"]
    labels = Counter((e["role"], e["label"]) for e in elements)
    by_ref = {e["ref"]: e for e in elements}
    groups = {g["id"]: g for g in observation.get("groups", [])}
    container_refs = {g["ref"] for g in groups.values() if "ref" in g}
    active = []
    for element in elements:
        path = group_path(element.get("group_id"), groups)
        ids = [g["id"] for g in path]
        common = 0
        while common < min(len(active), len(ids)) and active[common] == ids[common]:
            common += 1
        for depth, group in enumerate(path[common:], common):
            owner = by_ref.get(group.get("ref"))
            if owner:
                title = element_text(owner, labels, heading=True)
            else:
                label = " ".join(group["label"].splitlines())
                title = f"**{label}**"
                if label != group["role"]:
                    title += f" — {group['role']}"
            lines.append("  " * depth + "- " + title)
        active = ids
        if element["ref"] in container_refs:
            continue  # The actual element reference was rendered in its section heading.
        text = element_text(element, labels)
        # Older saved observations had only textual context, without group IDs.
        if not groups and element.get("context"):
            text += " (" + " › ".join(element["context"]) + ")"
        lines.append("  " * len(path) + "- " + text)
    return "\n".join(lines)
