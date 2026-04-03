import pathlib
import tempfile
import unittest

from halo_review import (
    build_review_precheck_prompts,
    build_review_prompts,
    parse_review_precheck_decision,
    prepare_review_source,
)


class ReviewPreparationTests(unittest.TestCase):
    def test_prepare_review_source_normalizes_text(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = pathlib.Path(temp_dir) / "essay.txt"
            path.write_text(
                "This is a valid source for review.   It has spacing issues.\r\n\r\n"
                "It is still coherent enough to discuss in character.",
                encoding="utf-8",
            )
            payload = prepare_review_source(path, max_chars=2000)

        self.assertTrue(payload["accepted"])
        self.assertNotIn("\r", payload["text"])
        self.assertIn("spacing issues.", payload["text"])

    def test_prepare_review_source_rejects_empty_material(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = pathlib.Path(temp_dir) / "empty.txt"
            path.write_text("\n\n   \n", encoding="utf-8")
            payload = prepare_review_source(path, max_chars=2000)

        self.assertFalse(payload["accepted"])


class ReviewPrecheckTests(unittest.TestCase):
    def test_precheck_prompt_requests_only_pass_fail(self):
        prompts = build_review_precheck_prompts(
            {"title": "moon", "source_name": "moon.txt", "word_count": 80, "text": "The moon has jam upon its face."}
        )
        self.assertIn("Return exactly one token: pass or fail.", prompts["system_prompt"])
        self.assertIn("Do not judge literary quality", prompts["system_prompt"])

    def test_parse_review_precheck_decision_accepts_plain_token(self):
        self.assertEqual(parse_review_precheck_decision("pass"), "pass")
        self.assertEqual(parse_review_precheck_decision(" fail\n"), "fail")

    def test_parse_review_precheck_decision_accepts_json_category(self):
        self.assertEqual(parse_review_precheck_decision('{"category":"pass"}'), "pass")


class ReviewPromptTests(unittest.TestCase):
    def test_review_prompt_enforces_canon(self):
        payload = {
            "accepted": True,
            "title": "statecraft",
            "source_name": "statecraft.txt",
            "word_count": 320,
            "truncated": False,
            "text": "The author argues with confidence but not quite with rigor.",
        }
        prompts = build_review_prompts("You are HAL.", payload, "HAL review mode is active.")

        self.assertIn("Remain entirely in character", prompts["system_prompt"])
        self.assertIn("not primarily summarising", prompts["system_prompt"])
        self.assertIn("Source text:", prompts["user_prompt"])


if __name__ == "__main__":
    unittest.main()