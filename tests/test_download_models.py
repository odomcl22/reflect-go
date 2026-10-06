"""Exercise interrupted and corrupt model downloads without network access."""
import hashlib
import importlib.util
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("download_models", Path(__file__).resolve().parents[1] / "scripts/download_models.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class Response(io.BytesIO):
    def __init__(self, data, status=200, **headers):
        super().__init__(data)
        self.status = status
        self.headers = headers


class Downloads(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.folder = Path(self.tmp.name)
        self.data = b"GGUF model fixture for a resumable download"
        self.item = {"filename": "example.gguf", "size_bytes": len(self.data), "sha256": hashlib.sha256(self.data).hexdigest(), "url": "https://example.test/model"}
        self.target, self.part = module.paths(self.folder, self.item)

    def test_interrupted_download_resumes_and_publishes_after_verification(self):
        self.part.write_bytes(self.data[:9])
        with patch.object(module.urllib.request, "urlopen", return_value=Response(self.data[9:], 206, **{"Content-Range": f"bytes 9-{len(self.data)-1}/{len(self.data)}"})) as request:
            module.download(self.folder, self.item)
        self.assertEqual(request.call_args.args[0].headers["Range"], f"bytes=9-{len(self.data)-1}")
        self.assertEqual(self.target.read_bytes(), self.data)

    def test_bounded_ranges_cover_the_whole_file(self):
        window = 10
        replies = [Response(self.data[start:start+window], 206, **{"Content-Range": f"bytes {start}-{min(start+window,len(self.data))-1}/{len(self.data)}"}) for start in range(0, len(self.data), window)]
        with patch.object(module, "WINDOW", window), patch.object(module.urllib.request, "urlopen", side_effect=replies) as request:
            module.download(self.folder, self.item)
        self.assertEqual(request.call_count, len(replies))
        self.assertEqual(self.target.read_bytes(), self.data)
        self.assertFalse(self.part.exists())

    def test_server_ignoring_resume_cannot_corrupt_partial(self):
        self.part.write_bytes(self.data[:9])
        with patch.object(module.urllib.request, "urlopen", return_value=Response(self.data)), self.assertRaisesRegex(ValueError, "resume range"):
            module.download(self.folder, self.item)
        self.assertEqual(self.part.read_bytes(), self.data[:9])
        self.assertFalse(self.target.exists())

    def test_early_eof_is_resumed_automatically(self):
        replies = [Response(self.data[:9]), Response(self.data[9:], 206, **{"Content-Range": f"bytes 9-{len(self.data)-1}/{len(self.data)}"})]
        with patch.object(module.urllib.request, "urlopen", side_effect=replies) as request, patch.object(module.time, "sleep"):
            module.download(self.folder, self.item)
        self.assertEqual(request.call_count, 2)
        self.assertEqual(self.target.read_bytes(), self.data)

    def test_wrong_range_cannot_append(self):
        self.part.write_bytes(self.data[:9])
        with patch.object(module.urllib.request, "urlopen", return_value=Response(self.data[9:], 206, **{"Content-Range": f"bytes 8-{len(self.data)-1}/{len(self.data)}"})), self.assertRaisesRegex(ValueError, "byte range"):
            module.download(self.folder, self.item)
        self.assertEqual(self.part.read_bytes(), self.data[:9])

    def test_checksum_mismatch_is_never_published(self):
        bad = b"B" * len(self.data)
        with patch.object(module.urllib.request, "urlopen", return_value=Response(bad)), self.assertRaisesRegex(ValueError, "Checksum mismatch"):
            module.download(self.folder, self.item)
        self.assertFalse(self.target.exists())
        self.assertEqual(self.part.read_bytes(), bad)

    def test_existing_file_is_never_overwritten(self):
        self.target.write_bytes(b"unrelated existing model")
        with self.assertRaisesRegex(ValueError, "Refusing to overwrite"):
            module.download(self.folder, self.item)
        self.assertEqual(self.target.read_bytes(), b"unrelated existing model")

    def test_insufficient_space_stops_before_network(self):
        with patch.object(module.shutil, "disk_usage", return_value=module.shutil._ntuple_diskusage(100, 99, 1)), patch.object(module.urllib.request, "urlopen") as request, self.assertRaisesRegex(OSError, "Insufficient space"):
            module.download(self.folder, self.item)
        request.assert_not_called()
        self.assertFalse(self.part.exists())


if __name__ == "__main__":
    unittest.main()
