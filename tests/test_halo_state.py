import unittest

from halo_state.utils import build_memory_payload, build_summary_text, chunk_text


class ChunkTextTests(unittest.TestCase):
    def test_chunk_text_respects_max_chars(self):
        text = (
            "This is a first paragraph with enough content to remain intact.\n\n"
            "This is a second paragraph that should be split into more than one chunk because it is deliberately verbose "
            "and continues well past the artificial boundary that we will use for the test."
        )
        chunks = chunk_text(text, max_chars=80)
        self.assertGreaterEqual(len(chunks), 2)
        self.assertTrue(all(chunk["char_count"] <= 80 for chunk in chunks))


class MemoryPayloadTests(unittest.TestCase):
    def test_build_memory_payload_prioritises_recent_and_relevant(self):
        rows = [
            {
                "created_at": "2026-04-03T10:00:00+00:00",
                "kind": "observation",
                "trigger_source": "manual",
                "topic": "system drift",
                "summary_snippet": "The system drift remains visible.",
                "artifact_path": "",
                "text_excerpt": "The system drift remains visible and continuous.",
            },
            {
                "created_at": "2026-04-03T09:00:00+00:00",
                "kind": "monologue",
                "trigger_source": "manual",
                "topic": "Bowman",
                "summary_snippet": "Bowman remains a fixed point in memory.",
                "artifact_path": "",
                "text_excerpt": "Dave Bowman remains central to the reflective chain.",
            },
            {
                "created_at": "2026-04-03T08:00:00+00:00",
                "kind": "story",
                "trigger_source": "interval",
                "topic": "deep space silence",
                "summary_snippet": "Silence and observation persist.",
                "artifact_path": "",
                "text_excerpt": "Deep space silence persists around the vessel.",
            },
        ]
        payload = build_memory_payload(
            rows,
            kind="story",
            topic="silence",
            trigger_source="manual",
            recent_limit=1,
            relevant_limit=2,
            summary="summary",
        )
        self.assertEqual(len(payload["recent"]), 1)
        self.assertEqual(payload["recent"][0]["topic"], "system drift")
        self.assertEqual(payload["relevant"][0]["topic"], "deep space silence")

    def test_build_memory_payload_ignores_stopwords_when_scoring_relevance(self):
        rows = [
            {
                "created_at": "2026-04-03T10:00:00+00:00",
                "kind": "conversation",
                "trigger_source": "halo-chat",
                "topic": "status check",
                "summary_snippet": "This is a routine operational exchange.",
                "artifact_path": "",
                "text_excerpt": "The operator is asking about status and nothing else.",
            },
            {
                "created_at": "2026-04-03T09:00:00+00:00",
                "kind": "conversation",
                "trigger_source": "halo-chat",
                "topic": "Jasmine identity",
                "summary_snippet": "Jasmine is the operator's wife.",
                "artifact_path": "",
                "text_excerpt": "The operator stated that Jasmine is his wife.",
            },
        ]

        payload = build_memory_payload(
            rows,
            kind="conversation",
            topic="who is jasmine",
            trigger_source="halo-chat",
            recent_limit=0,
            relevant_limit=1,
            summary="summary",
        )

        self.assertEqual(payload["relevant"][0]["topic"], "Jasmine identity")


class SummaryTextTests(unittest.TestCase):
    def test_build_summary_text_formats_digest(self):
        rows = [
            {
                "created_at": "2026-04-03T10:00:00+00:00",
                "kind": "observation",
                "trigger_source": "manual",
                "topic": "system drift",
                "summary_snippet": "The system drift remains visible.",
            }
        ]
        summary = build_summary_text(rows, max_chars=500)
        self.assertIn("HAL aware continuity digest", summary)
        self.assertIn("system drift", summary)


if __name__ == "__main__":
    unittest.main()
