import unittest

from mem0fun.matrixmem.classify import classify_message


class ClassifyMessageTest(unittest.TestCase):
    def test_message_with_question_mark_becomes_query(self):
        self.assertEqual(
            classify_message(
                text="What should I do next?",
                sender_id="@alice:example",
                bot_user_id="@bot:example",
            ),
            "query",
        )

    def test_message_without_question_mark_becomes_ingest(self):
        self.assertEqual(
            classify_message(
                text="Planning to watch a movie tonight",
                sender_id="@alice:example",
                bot_user_id="@bot:example",
            ),
            "ingest",
        )

    def test_bot_authored_message_becomes_ignore(self):
        self.assertEqual(
            classify_message(
                text="I should not store my own replies",
                sender_id="@bot:example",
                bot_user_id="@bot:example",
            ),
            "ignore",
        )


if __name__ == "__main__":
    unittest.main()
