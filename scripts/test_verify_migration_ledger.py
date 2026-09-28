import hashlib
import importlib.util
import pathlib
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("ledger", pathlib.Path(__file__).with_name("verify-migration-ledger.py"))
ledger = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ledger)


class LedgerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name)
        self.git("init", "-q")
        self.git("config", "user.email", "test@example.invalid")
        self.git("config", "user.name", "test")
        directory = self.root / "backend/migrations"
        directory.mkdir(parents=True)
        sql = "\nCREATE TABLE example (id INT);\n"
        (directory / "001.sql").write_text(sql)
        self.git("add", ".")
        self.git("commit", "-qm", "fixture")
        self.base = self.git("rev-parse", "HEAD").strip()
        self.row = "001.sql\t" + hashlib.sha256(sql.strip().encode()).hexdigest() + "\n"

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.root), *args], text=True)

    def verify(self):
        # Run the real CLI in an isolated repository to test actual git-object reads.
        return subprocess.run(
            ["python3", str(pathlib.Path(ledger.__file__).resolve()), self.base, "ledger.tsv"],
            cwd=self.root, capture_output=True, text=True,
        )

    def test_exact_match(self):
        (self.root / "ledger.tsv").write_text(self.row)
        self.assertEqual(self.verify().returncode, 0)

    def test_mismatch_missing_extra_duplicate_and_invalid_rows_fail(self):
        for value in ["", self.row.replace(self.row.split("\t")[1][:64], "0" * 64),
                      self.row + self.row, self.row + "002.sql\t" + "0" * 64 + "\n", "bad"]:
            with self.subTest(value=value):
                (self.root / "ledger.tsv").write_text(value)
                self.assertNotEqual(self.verify().returncode, 0)

    def test_missing_commit_and_invalid_sha_fail(self):
        (self.root / "ledger.tsv").write_text(self.row)
        for base in ["0" * 40, "HEAD", ""]:
            with self.subTest(base=base):
                with self.assertRaises((ValueError, subprocess.CalledProcessError)):
                    ledger.verify(base, self.row)


if __name__ == "__main__":
    unittest.main()
