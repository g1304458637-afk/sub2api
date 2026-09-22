import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parent / "hubu-compose-image.py"


class HubuComposeImageTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.compose = Path(self.temp.name) / "compose.yml"
        self.compose.write_text(
            "services:\n"
            "  backend:\n"
            "    image: sub2api-hubu:old\n"
            "    volumes:\n"
            "      - ./data:/app/data\n"
            "  postgres:\n"
            "    image: postgres:18-alpine\n"
            "    volumes:\n"
            "      - ./postgres_data:/var/lib/postgresql\n",
            encoding="utf-8",
        )

    def tearDown(self):
        self.temp.cleanup()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, str(SCRIPT), *args], text=True, capture_output=True)

    def test_updates_only_backend_image(self):
        result = self.run_tool("set", str(self.compose), "sub2api-hubu:old", "ghcr.io/example/sub2api:hubu-sha-123")
        self.assertEqual(0, result.returncode, result.stderr)
        contents = self.compose.read_text(encoding="utf-8")
        self.assertIn("image: ghcr.io/example/sub2api:hubu-sha-123", contents)
        self.assertIn("image: postgres:18-alpine", contents)
        self.assertIn("./postgres_data:/var/lib/postgresql", contents)
        self.assertEqual("ghcr.io/example/sub2api:hubu-sha-123", self.run_tool("get", str(self.compose)).stdout.strip())

    def test_refuses_unexpected_image_without_changing_file(self):
        before = self.compose.read_text(encoding="utf-8")
        result = self.run_tool("set", str(self.compose), "not-the-current-image", "ghcr.io/example/sub2api:next")
        self.assertNotEqual(0, result.returncode)
        self.assertEqual(before, self.compose.read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
