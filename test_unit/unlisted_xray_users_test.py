import hashlib
import json
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "scripts/check-unlisted-xray-users.py"


class ReadOnlyUserComparison(unittest.TestCase):
    def test_known_missing_and_incomplete_inputs(self):
        with tempfile.TemporaryDirectory() as directory:
            dbpath, config = Path(directory) / "panel.db", Path(directory) / "config.json"
            db = sqlite3.connect(dbpath)
            db.execute("CREATE TABLE accounts(email TEXT)")
            db.execute("INSERT INTO accounts VALUES (' Alice ')")
            db.commit()
            db.close()
            before = hashlib.sha256(dbpath.read_bytes()).digest()
            command = [sys.executable, str(SCRIPT), "--db", str(dbpath), "--config", str(config)]

            def check(clients):
                config.write_text(json.dumps({"inbounds": [{"tag": "test", "settings": {"clients": clients}}]}))
                return subprocess.run(command, text=True, capture_output=True)

            self.assertEqual(check([{"email": "ALICE"}]).returncode, 0)
            found = check([{"email": "alice"}, {"email": "unknown", "password": "do-not-print"}, {"id": "private-uuid"}])
            self.assertEqual(found.returncode, 1)
            self.assertIn('"email": "unknown"', found.stdout)
            self.assertNotIn("do-not-print", found.stdout + found.stderr)
            self.assertNotIn("private-uuid", found.stdout + found.stderr)
            self.assertIn("2", found.stdout)
            config.write_text("{}")
            self.assertEqual(subprocess.run(command, capture_output=True).returncode, 2)
            config.unlink()
            self.assertEqual(subprocess.run(command, capture_output=True).returncode, 2)
            self.assertEqual(hashlib.sha256(dbpath.read_bytes()).digest(), before)
            missing = Path(directory) / "missing.db"
            config.write_text('{"inbounds": []}')
            self.assertEqual(subprocess.run(command[:3] + [str(missing)] + command[4:], capture_output=True).returncode, 2)
            self.assertFalse(missing.exists(), "read-only check created a database")


if __name__ == "__main__":
    unittest.main()
