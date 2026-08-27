# Anki Major Scales

This project builds an Anki deck for learning major scales and key signatures. The first prototype contains one source note for D major, from which Anki can generate four card types:

1. Given a major key, recall how many sharps or flats it has.
2. Moving to that key in the circle-of-fifths sequence, recall the accidental that is added.
3. Given a major key, recall its complete key signature.
4. Given a key signature, recall its major key.

Each source note has a stable `ID`. Once an ID has been imported into Anki, it must not change: later imports use it to update the existing note without replacing that note or losing its review history.

Git is the source of truth for the deck data. The intended workflow is:

```text
edit TSV -> import/update in Anki desktop -> sync with AnkiWeb -> phone
```
