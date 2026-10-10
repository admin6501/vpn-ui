import importlib.util
from pathlib import Path
import sqlite3
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('repair', Path(__file__).with_name('repair.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class RepairTest(unittest.TestCase):
    def test_uploaded_database_copy_and_idempotence(self):
        # Set externally for private validation; private database never enters git.
        import os
        source = os.environ.get('VPNUI_PRIVATE_REPAIR_TEST_DB')
        if not source:
            self.skipTest('Private database not configured')
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'copy.db'
            with sqlite3.connect('file:' + source + '?mode=ro', uri=True) as original, sqlite3.connect(path) as db:
                original.backup(db)
            with sqlite3.connect(path) as db:
                snapshots = {t: db.execute('SELECT * FROM ' + t).fetchall() for t in
                             ('accounts', 'settings', 'users', 'pasar_guard_subscriptions', 'pasar_guard_secrets', 'account_inbounds')}
            before = module.repair(path)
            self.assertEqual(before['missing_rows_recoverable'], 433)
            result = module.repair(path, True)
            self.assertEqual(result['missing_rows_restored'], 433)
            self.assertEqual(module.repair(path, True)['missing_rows_restored'], 0)
            with sqlite3.connect(path) as db:
                self.assertEqual(db.execute('SELECT COUNT(*) FROM client_traffics WHERE total>0').fetchone()[0], 430)
                self.assertEqual(db.execute('SELECT COUNT(*) FROM accounts a JOIN client_traffics t ON a.email=t.email WHERE a.total_gb!=t.total').fetchone()[0], 0)
                for table, rows in snapshots.items():
                    self.assertEqual(rows, db.execute('SELECT * FROM ' + table).fetchall(), table)


if __name__ == '__main__':
    unittest.main()
