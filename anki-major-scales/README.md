# Anki Major Scales

This project builds an Anki deck for learning major scales and key signatures. The first prototype contains one source note for D major, from which Anki generates four card types:

1. Given a major key, recall how many sharps or flats it has.
2. Moving to that key in the circle-of-fifths sequence, recall the accidental that is added.
3. Given a major key, recall its complete key signature.
4. Given a key signature, recall its major key.

Each source note has a stable `ID`. The generator derives the Anki note GUID from that ID. Once an ID has been imported into Anki, it must not change: later packages use it to update the existing note without replacing that note or losing its review history. The generator's deck ID, model ID, field order, and card-template order must also remain stable.

Git is the source of truth for the deck data and generator. `anki-major-scales.apkg` is a generated artifact.

## Build and test

From this directory:

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements.txt
.venv/bin/python -m unittest -v test_build_deck.py
.venv/bin/python build_deck.py
```

The generator reads `data/major-scales.tsv` and writes `anki-major-scales.apkg`. The package contains no review history or media.

## Workflow

The intended workflow is:

```text
edit TSV -> build APKG -> import/update in Anki -> sync with AnkiWeb -> phone
```

Once this project is on the repository's `main` branch, AnkiMobile's **Add/Export > Download Link** can use:

```text
https://raw.githubusercontent.com/jrhy/sandbox/main/anki-major-scales/anki-major-scales.apkg
```
