#!/usr/bin/env python3

import argparse
import csv
from pathlib import Path

import genanki


PROJECT_DIR = Path(__file__).resolve().parent
DEFAULT_INPUT = PROJECT_DIR / "data" / "major-scales.tsv"
DEFAULT_OUTPUT = PROJECT_DIR / "anki-major-scales.apkg"

MODEL_ID = 1616089094
DECK_ID = 1877552195

FIELD_NAMES = [
    "ID",
    "Key",
    "PreviousKey",
    "NextKey",
    "AccidentalType",
    "AccidentalCount",
    "AddedAccidental",
    "KeySignature",
]

CARD_TEMPLATES = [
    {
        "name": "Accidental count",
        "qfmt": "{{Key}} major: how many sharps/flats?",
        "afmt": "{{FrontSide}}<hr id=answer>{{AccidentalCount}} {{AccidentalType}}",
    },
    {
        "name": "Added accidental",
        "qfmt": (
            "Moving from {{PreviousKey}} major to {{Key}} major in the "
            "circle-of-fifths sequence,<br>what accidental is added?"
        ),
        "afmt": "{{FrontSide}}<hr id=answer>{{AddedAccidental}}",
    },
    {
        "name": "Key signature",
        "qfmt": "{{Key}} major: what is the complete key signature?",
        "afmt": "{{FrontSide}}<hr id=answer>{{KeySignature}}",
    },
    {
        "name": "Major key",
        "qfmt": "{{KeySignature}} is the key signature of which major key?",
        "afmt": "{{FrontSide}}<hr id=answer>{{Key}} major",
    },
]

CARD_CSS = """
.card {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  font-size: 24px;
  text-align: center;
  color: #222;
  background: #fff;
}

.card.nightMode {
  color: #eee;
  background: #222;
}
"""


def read_notes(input_path):
    with input_path.open(encoding="utf-8", newline="") as input_file:
        reader = csv.DictReader(input_file, delimiter="\t")
        if reader.fieldnames != FIELD_NAMES:
            expected_columns = ", ".join(FIELD_NAMES)
            raise ValueError(f"TSV columns must be exactly: {expected_columns}")
        rows = list(reader)

    seen_ids = set()
    for line_number, row in enumerate(rows, start=2):
        if None in row or any(row[field_name] is None for field_name in FIELD_NAMES):
            raise ValueError(
                f"row {line_number} must contain exactly {len(FIELD_NAMES)} columns"
            )

        note_id = row["ID"]
        if not note_id.strip():
            raise ValueError(f"row {line_number} field ID must not be blank")
        if note_id in seen_ids:
            raise ValueError(f"duplicate ID: {note_id}")
        seen_ids.add(note_id)
    return rows


def build_deck(input_path, output_path):
    model = genanki.Model(
        MODEL_ID,
        "Major Scale Key Signature",
        fields=[{"name": field_name} for field_name in FIELD_NAMES],
        templates=CARD_TEMPLATES,
        css=CARD_CSS,
        sort_field_index=0,
    )
    deck = genanki.Deck(DECK_ID, "Major Scales")

    for row in read_notes(input_path):
        fields = [row[field_name] for field_name in FIELD_NAMES]
        deck.add_note(
            genanki.Note(
                model=model,
                fields=fields,
                guid=genanki.guid_for(row["ID"]),
            )
        )

    output_path.parent.mkdir(parents=True, exist_ok=True)
    genanki.Package(deck).write_to_file(str(output_path))


def parse_args():
    parser = argparse.ArgumentParser(description="Build the major-scales Anki deck")
    parser.add_argument("--input", type=Path, default=DEFAULT_INPUT)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    return parser.parse_args()


def main():
    args = parse_args()
    build_deck(args.input, args.output)


if __name__ == "__main__":
    main()
