"""Deterministic message classification for the Matrix memory bot."""


def classify_message(*, text, sender_id, bot_user_id):
    """Classify a Matrix message as query, ingest, or ignore."""
    if sender_id == bot_user_id:
        return "ignore"
    if "?" in text:
        return "query"
    return "ingest"
