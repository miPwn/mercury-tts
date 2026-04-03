import json
import pathlib
import tempfile
import unittest
import wave

from halo_cache import assemble_wav_playlist, read_cache_index_file, slugify_name, write_cache_index_file


def write_test_wav(path: pathlib.Path, sample_value: int, frame_count: int = 80) -> None:
    with wave.open(str(path), "wb") as target:
        target.setnchannels(1)
        target.setsampwidth(2)
        target.setframerate(16000)
        target.writeframes((sample_value.to_bytes(2, "little", signed=True)) * frame_count)


class CacheHelperTests(unittest.TestCase):
    def test_slugify_name(self):
        self.assertEqual(slugify_name("The Instruments We Build"), "the-instruments-we-build")
        self.assertEqual(slugify_name(""), "assembled")

    def test_index_round_trip(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            index_file = pathlib.Path(temp_dir) / "index" / "entry.json"
            payload = {"wav_path": "/tmp/demo.wav", "text": "example"}
            write_cache_index_file(index_file, payload)
            self.assertEqual(read_cache_index_file(index_file), payload)

    def test_assemble_wav_playlist(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            base = pathlib.Path(temp_dir)
            first = base / "a.wav"
            second = base / "b.wav"
            manifest = base / "playlist.txt"
            output = base / "assembled.wav"
            write_test_wav(first, 100, frame_count=40)
            write_test_wav(second, 200, frame_count=60)
            manifest.write_text(f"{first}\tchunk one\n{second}\tchunk two\n", encoding="utf-8")

            assemble_wav_playlist(manifest, output)

            with wave.open(str(output), "rb") as assembled:
                self.assertEqual(assembled.getnframes(), 100)
                self.assertEqual(assembled.getnchannels(), 1)


if __name__ == "__main__":
    unittest.main()