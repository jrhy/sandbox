from __future__ import annotations

from collections.abc import Sequence
from typing import Any


FALLBACK_QUERY_ANSWER = "I found related memories, but couldn't synthesize an answer."


class NoUsableContextError(RuntimeError):
    pass


def build_query_prompt(
    query: str,
    *,
    memories: Sequence[Any],
    relations: Sequence[Any],
) -> str:
    lines = [
        "You are a concise memory assistant.",
        "Answer the user's question using the retrieved memories and graph relations.",
        f"Question: {query}",
        "",
        "Retrieved memories:",
    ]
    lines.extend(_format_block(memories))
    lines.extend(["", "Graph relations:"])
    lines.extend(_format_block(relations))
    lines.extend(["", "Answer:"])
    return "\n".join(lines)


def synthesize_query_answer(prompt: str) -> str:
    question = _extract_question(prompt)
    memories = _extract_section(prompt, "Retrieved memories:")
    relations = _extract_section(prompt, "Graph relations:")
    if not memories and not relations:
        raise NoUsableContextError("No retrieved context to synthesize")

    opening = _build_question_opening(question)
    core_items = memories if memories else relations
    memory_summary = _summarize_items(core_items, max_items=3)
    answer = f"{opening} {memory_summary}."

    if memories and relations:
        relation_summary = _summarize_items(relations, max_items=2)
        answer += f" Related graph context: {relation_summary}."
    return answer


def _format_block(items: Sequence[Any]) -> list[str]:
    if not items:
        return ["- (none)"]

    lines: list[str] = []
    for item in items:
        lines.append(f"- {_format_item(item)}")
    return lines


def _format_item(item: Any) -> str:
    if isinstance(item, str):
        return item.strip()
    if isinstance(item, dict):
        source = item.get("source")
        relation = item.get("relation")
        target = item.get("target")
        if source is not None and relation is not None and target is not None:
            return f"{source} -> {relation} -> {target}"

        memory = item.get("memory")
        if memory is not None:
            return str(memory)
    return str(item)


def _extract_section(prompt: str, heading: str) -> list[str]:
    lines = prompt.splitlines()
    try:
        start = lines.index(heading) + 1
    except ValueError:
        return []

    extracted: list[str] = []
    section_headings = {"Retrieved memories:", "Graph relations:", "Answer:"}
    for line in lines[start:]:
        stripped = line.strip()
        if stripped in section_headings and stripped != heading:
            break
        if not stripped or stripped == "- (none)":
            continue
        if stripped.startswith("- "):
            extracted.append(stripped[2:])
            continue
        extracted.append(stripped)
    return extracted


def _extract_question(prompt: str) -> str:
    lines = prompt.splitlines()
    for line in lines:
        if line.startswith("Question: "):
            return line[len("Question: ") :].strip()
    return ""


def _build_question_opening(question: str) -> str:
    normalized = question.strip().lower()
    if any(
        phrase in normalized
        for phrase in (
            "should i",
            "should i do",
            "should i consider",
            "what should i",
            "what should we",
            "what should i think about",
        )
    ):
        prefix = "You should consider"
    elif normalized.startswith(
        (
            "what ",
            "which ",
            "how ",
            "where ",
            "when ",
            "why ",
            "tell me ",
            "give me ",
            "can you ",
            "could you ",
            "do i ",
            "do we ",
            "is there ",
            "are there ",
        )
    ):
        prefix = "A good place to start is"
    else:
        prefix = "A good place to start is"

    focus = _question_focus(question)
    if focus:
        return f"{focus}, {prefix.lower()}"
    return prefix


def _question_focus(question: str) -> str:
    normalized = question.strip().rstrip("?").lower()
    if "tonight" in normalized:
        return "For tonight"
    if "today" in normalized:
        return "For today"
    if "this weekend" in normalized:
        return "For this weekend"

    marker = "going to "
    if marker in normalized:
        start = normalized.index(marker) + len(marker)
        tail = question.strip()[start:]
        for separator in (",", "?", ".", "!", ";"):
            tail = tail.split(separator, 1)[0]
        tail = tail.strip()
        if tail:
            return f"For your trip to {tail}"

    return ""


def _strip_ending(text: str) -> str:
    return text.rstrip().rstrip(".")


def _summarize_items(items: Sequence[str], *, max_items: int) -> str:
    selected = [_strip_ending(item) for item in items[:max_items]]
    if not selected:
        return ""
    if len(selected) == 1:
        summary = selected[0]
    if len(selected) == 2:
        summary = f"{selected[0]} and {selected[1]}"
    if len(selected) >= 3:
        summary = f"{selected[0]}, {selected[1]}, and {selected[2]}"

    remainder = len(items) - len(selected)
    if remainder > 0:
        summary += f", plus {remainder} more"
    return summary
