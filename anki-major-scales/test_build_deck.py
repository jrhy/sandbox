import json
import sqlite3
import subprocess
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path


PROJECT_DIR = Path(__file__).resolve().parent
BUILD_SCRIPT = PROJECT_DIR / "build_deck.py"
DATA_FILE = PROJECT_DIR / "data" / "major-scales.tsv"
COMMITTED_PACKAGE = PROJECT_DIR / "anki-major-scales.apkg"
MODEL_ID = "1616089094"
DECK_ID = "1877552195"

EXPECTED_NOTES = [
    ("IQR5>Y_{V&", ["major-C", "C", "", "G", "sharps or flats", "0", "", "No sharps or flats"]),
    ("npg;8f/RV0", ["major-G", "G", "C", "D", "sharp", "1", "F♯", "F♯"]),
    ("Q*+WPc1<9p", ["major-D", "D", "G", "A", "sharps", "2", "C♯", "F♯ C♯"]),
    ("NV.4JBo[N+", ["major-A", "A", "D", "E", "sharps", "3", "G♯", "F♯ C♯ G♯"]),
    ("cF`)>s*bvK", ["major-E", "E", "A", "B", "sharps", "4", "D♯", "F♯ C♯ G♯ D♯"]),
    ("Ky9gUu)osc", ["major-B", "B", "E", "F♯", "sharps", "5", "A♯", "F♯ C♯ G♯ D♯ A♯"]),
    ("jq-t>e:.U3", ["major-F-sharp", "F♯", "B", "C♯", "sharps", "6", "E♯", "F♯ C♯ G♯ D♯ A♯ E♯"]),
    ("Eo`L#V;zG5", ["major-F", "F", "C", "B♭", "flat", "1", "B♭", "B♭"]),
    ("NbfCrX/gh:", ["major-B-flat", "B♭", "F", "E♭", "flats", "2", "E♭", "B♭ E♭"]),
    ("xtFD`#FvGs", ["major-E-flat", "E♭", "B♭", "A♭", "flats", "3", "A♭", "B♭ E♭ A♭"]),
    ("hV5(vJqu1[", ["major-A-flat", "A♭", "E♭", "D♭", "flats", "4", "D♭", "B♭ E♭ A♭ D♭"]),
    ("tt?T)lJzpl", ["major-D-flat", "D♭", "A♭", "G♭", "flats", "5", "G♭", "B♭ E♭ A♭ D♭ G♭"]),
    ("AL^[C>.5H^", ["major-G-flat", "G♭", "D♭", "C♭", "flats", "6", "C♭", "B♭ E♭ A♭ D♭ G♭ C♭"]),
]


def package_contract(package_path):
    with tempfile.TemporaryDirectory() as temp_dir:
        with zipfile.ZipFile(package_path) as package:
            collection_path = Path(temp_dir) / "collection.anki2"
            collection_path.write_bytes(package.read("collection.anki2"))
            media = json.loads(package.read("media"))

        with sqlite3.connect(collection_path) as collection:
            models_json, decks_json = collection.execute(
                "SELECT models, decks FROM col"
            ).fetchone()
            models = json.loads(models_json)
            decks = json.loads(decks_json)
            model = models[MODEL_ID]
            deck = decks[DECK_ID]
            return {
                "notes": collection.execute(
                    "SELECT guid, flds FROM notes ORDER BY guid"
                ).fetchall(),
                "card_ordinals": collection.execute(
                    "SELECT ord FROM cards ORDER BY ord"
                ).fetchall(),
                "review_count": collection.execute(
                    "SELECT COUNT(*) FROM revlog"
                ).fetchone()[0],
                "model_id": model["id"],
                "model_name": model["name"],
                "fields": [field["name"] for field in model["flds"]],
                "templates": [
                    (template["name"], template["qfmt"], template["afmt"])
                    for template in model["tmpls"]
                ],
                "css": model["css"],
                "deck_id": deck["id"],
                "deck_name": deck["name"],
                "media": media,
            }


class BuildDeckTest(unittest.TestCase):
    def test_builds_major_scale_notes_and_cards(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            output_path = Path(temp_dir) / "anki-major-scales.apkg"
            result = subprocess.run(
                [
                    sys.executable,
                    str(BUILD_SCRIPT),
                    "--input",
                    str(DATA_FILE),
                    "--output",
                    str(output_path),
                ],
                cwd=PROJECT_DIR,
                capture_output=True,
                text=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)

            with zipfile.ZipFile(output_path) as package:
                self.assertIn("collection.anki2", package.namelist())
                self.assertIn("media", package.namelist())
                collection_path = Path(temp_dir) / "collection.anki2"
                collection_path.write_bytes(package.read("collection.anki2"))

            with sqlite3.connect(collection_path) as collection:
                self.assertEqual(
                    collection.execute("PRAGMA integrity_check").fetchone()[0],
                    "ok",
                )
                actual_notes = {
                    fields.split("\x1f")[0]: (guid, fields.split("\x1f"))
                    for guid, fields in collection.execute(
                        "SELECT guid, flds FROM notes"
                    )
                }
                self.assertEqual(
                    actual_notes,
                    {fields[0]: (guid, fields) for guid, fields in EXPECTED_NOTES},
                )
                self.assertEqual(
                    collection.execute("SELECT COUNT(*) FROM notes").fetchone()[0],
                    13,
                )
                self.assertEqual(
                    collection.execute("SELECT COUNT(*) FROM cards").fetchone()[0],
                    51,
                )
                card_ordinals_by_id = {}
                for fields, ordinal in collection.execute(
                    "SELECT n.flds, c.ord FROM notes n "
                    "JOIN cards c ON c.nid = n.id ORDER BY n.id, c.ord"
                ):
                    note_id = fields.split("\x1f")[0]
                    card_ordinals_by_id.setdefault(note_id, []).append(ordinal)
                self.assertEqual(
                    card_ordinals_by_id["major-C"],
                    [0, 2, 3],
                )
                for note_id in set(actual_notes) - {"major-C"}:
                    self.assertEqual(card_ordinals_by_id[note_id], [0, 1, 2, 3])
                self.assertEqual(
                    collection.execute("SELECT COUNT(*) FROM revlog").fetchone()[0],
                    0,
                )

                models_json, decks_json = collection.execute(
                    "SELECT models, decks FROM col"
                ).fetchone()
                models = json.loads(models_json)
                self.assertEqual(set(models), {MODEL_ID})
                model = models[MODEL_ID]
                self.assertEqual(
                    [field["name"] for field in model["flds"]],
                    [
                        "ID",
                        "Key",
                        "PreviousKey",
                        "NextKey",
                        "AccidentalType",
                        "AccidentalCount",
                        "AddedAccidental",
                        "KeySignature",
                    ],
                )
                self.assertEqual(
                    [
                        (template["name"], template["qfmt"], template["afmt"])
                        for template in model["tmpls"]
                    ],
                    [
                        (
                            "Accidental count",
                            "{{Key}} major: how many sharps/flats?",
                            "{{FrontSide}}<hr id=answer>"
                            "{{AccidentalCount}} {{AccidentalType}}",
                        ),
                        (
                            "Added accidental",
                            "{{#PreviousKey}}Moving from {{PreviousKey}} major to "
                            "{{Key}} major in the circle-of-fifths sequence,<br>"
                            "what accidental is added?{{/PreviousKey}}",
                            "{{FrontSide}}<hr id=answer>{{AddedAccidental}}",
                        ),
                        (
                            "Key signature",
                            "{{Key}} major: what is the complete key signature?",
                            "{{FrontSide}}<hr id=answer>{{KeySignature}}",
                        ),
                        (
                            "Major key",
                            "{{KeySignature}} is the key signature of which major key?",
                            "{{FrontSide}}<hr id=answer>{{Key}} major",
                        ),
                    ],
                )

                decks = json.loads(decks_json)
                self.assertEqual(decks[DECK_ID]["name"], "Major Scales")

    def test_committed_package_matches_generated_package(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            output_path = Path(temp_dir) / "anki-major-scales.apkg"
            result = subprocess.run(
                [
                    sys.executable,
                    str(BUILD_SCRIPT),
                    "--input",
                    str(DATA_FILE),
                    "--output",
                    str(output_path),
                ],
                cwd=PROJECT_DIR,
                capture_output=True,
                text=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                package_contract(COMMITTED_PACKAGE),
                package_contract(output_path),
            )

    def test_rejects_changed_tsv_columns(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            input_path = Path(temp_dir) / "invalid.tsv"
            output_path = Path(temp_dir) / "anki-major-scales.apkg"
            input_path.write_text(
                "ID\tKey\tPreviousKey\tNextKey\tAccidentalType\t"
                "AccidentalCount\tAddedAccidental\n"
                "major-D\tD\tG\tA\tsharps\t2\tC♯\n",
                encoding="utf-8",
            )

            result = subprocess.run(
                [
                    sys.executable,
                    str(BUILD_SCRIPT),
                    "--input",
                    str(input_path),
                    "--output",
                    str(output_path),
                ],
                cwd=PROJECT_DIR,
                capture_output=True,
                text=True,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("TSV columns must be exactly", result.stderr)
            self.assertFalse(output_path.exists())

    def test_rejects_duplicate_ids(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            input_path = Path(temp_dir) / "duplicate.tsv"
            output_path = Path(temp_dir) / "anki-major-scales.apkg"
            input_path.write_text(
                "ID\tKey\tPreviousKey\tNextKey\tAccidentalType\t"
                "AccidentalCount\tAddedAccidental\tKeySignature\n"
                "major-D\tD\tG\tA\tsharps\t2\tC♯\tF♯ C♯\n"
                "major-D\tD\tG\tA\tsharps\t2\tC♯\tF♯ C♯\n",
                encoding="utf-8",
            )

            result = subprocess.run(
                [
                    sys.executable,
                    str(BUILD_SCRIPT),
                    "--input",
                    str(input_path),
                    "--output",
                    str(output_path),
                ],
                cwd=PROJECT_DIR,
                capture_output=True,
                text=True,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("duplicate ID: major-D", result.stderr)
            self.assertFalse(output_path.exists())

    def test_rejects_malformed_rows(self):
        header = (
            "ID\tKey\tPreviousKey\tNextKey\tAccidentalType\t"
            "AccidentalCount\tAddedAccidental\tKeySignature\n"
        )
        cases = [
            (
                "extra column",
                "major-D\tD\tG\tA\tsharps\t2\tC♯\tF♯ C♯\textra\n",
                "row 2 must contain exactly 8 columns",
            ),
            (
                "missing column",
                "major-D\tD\tG\tA\tsharps\t2\tC♯\n",
                "row 2 must contain exactly 8 columns",
            ),
            (
                "blank ID",
                "\tD\tG\tA\tsharps\t2\tC♯\tF♯ C♯\n",
                "row 2 field ID must not be blank",
            ),
        ]

        for name, row, expected_error in cases:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temp_dir:
                input_path = Path(temp_dir) / "invalid.tsv"
                output_path = Path(temp_dir) / "anki-major-scales.apkg"
                input_path.write_text(header + row, encoding="utf-8")

                result = subprocess.run(
                    [
                        sys.executable,
                        str(BUILD_SCRIPT),
                        "--input",
                        str(input_path),
                        "--output",
                        str(output_path),
                    ],
                    cwd=PROJECT_DIR,
                    capture_output=True,
                    text=True,
                )

                self.assertNotEqual(result.returncode, 0)
                self.assertIn(expected_error, result.stderr)
                self.assertFalse(output_path.exists())


if __name__ == "__main__":
    unittest.main()
