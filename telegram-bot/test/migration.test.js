import test from 'node:test';
import assert from 'node:assert/strict';
import { advanceMigration, migrationSnapshot } from '../migration.js';
import { migrationJournal } from '../migration-journal.js';
import { DatabaseSync } from 'node:sqlite';
import { readFileSync } from 'node:fs';

function fixture(failure = '') {
  let job = { id: 7, state: 'ready', next: 0, destination: { email: 'alice', subId: 'planned' } };
  const calls = [];
  const journal = {
    async get() { return structuredClone(job); },
    async compareAndSet(id, before, after) {
      if (id !== job.id || JSON.stringify(before) !== JSON.stringify(job)) return false;
      job = structuredClone(after); return true;
    },
  };
  const names = ['freezeSource', 'snapshotSource', 'createDestination', 'importUsage', 'verifyDestination', 'activateDestination', 'verifyActiveDestination', 'deleteSource', 'commitDestination'];
  const adapter = Object.fromEntries(names.map(name => [name, async state => {
    calls.push(name);
    if (name === failure) throw new Error('connection lost');
    if (name === 'createDestination') assert.equal(state.destination.subId, 'planned');
    return name === 'snapshotSource' ? { snapshot: { up: 5, down: 15, quota: 100, expiry: 123 } } : true;
  }]));
  return { journal, adapter, calls, get: () => job };
}

test('migration verifies and activates destination before deleting source', async () => {
  const f = fixture(); const result = await advanceMigration(f.journal, f.adapter, 7);
  assert.equal(result.state, 'complete');
  assert.deepEqual(f.calls, ['freezeSource','snapshotSource','createDestination','importUsage','verifyDestination','activateDestination','verifyActiveDestination','deleteSource','commitDestination']);
  assert.equal(result.snapshot.quota, 100);
  await advanceMigration(f.journal, f.adapter, 7); assert.equal(f.calls.length, 9);
});

test('ambiguous creation or deletion stays locked and cannot be blindly repeated', async () => {
  for (const [failure, step] of [['createDestination','create_destination'], ['deleteSource','delete_source']]) {
    const f = fixture(failure);
    await assert.rejects(advanceMigration(f.journal, f.adapter, 7), /connection lost/);
    assert.equal(f.get().state, 'review'); assert.equal(f.get().uncertain, step);
    const count = f.calls.length;
    await assert.rejects(advanceMigration(f.journal, f.adapter, 7), /requires review/);
    assert.equal(f.calls.length, count);
    if (failure === 'createDestination') assert.ok(!f.calls.includes('deleteSource'));
  }
});

test('verification failure cannot delete source and read may be retried', async () => {
  const f = fixture('verifyDestination');
  await assert.rejects(advanceMigration(f.journal, f.adapter, 7), /connection lost/);
  assert.equal(f.get().state, 'ready'); assert.equal(f.get().next, 4);
  assert.ok(!f.calls.includes('deleteSource'));
});

test('persistent journal locks the service and only releases completed jobs', async () => {
  const sql = new DatabaseSync(':memory:');
  sql.exec(readFileSync(new URL('../schema.sql', import.meta.url), 'utf8'));
  sql.exec("INSERT INTO services(id,user_id,name) VALUES(1,42,'alice')");
  const prep = (q, args = []) => ({
    bind(...values) { return prep(q, values); },
    execute() { const r = sql.prepare(q).run(...args); return { meta: { changes: Number(r.changes) } }; },
    async run() { return this.execute(); },
    async first() { return sql.prepare(q).get(...args) ?? null; },
  });
  const db = { prepare: q => prep(q), async batch(statements) {
    sql.exec('BEGIN'); try { const results = statements.map(s => s.execute()); sql.exec('COMMIT'); return results; }
    catch (e) { sql.exec('ROLLBACK'); throw e; }
  } };
  const journal = migrationJournal(db), id = '12345678-1234-1234-1234-123456789abc';
  const initial = await journal.create(id, 1, { destination: 'two' });
  await assert.rejects(journal.create('22345678-1234-1234-1234-123456789abc', 1, {}));
  await assert.rejects(journal.releaseComplete(id), /not complete/);
  const review = { ...initial, state: 'review', uncertain: 'delete_source' };
  assert.equal(await journal.compareAndSet(id, initial, review), true);
  assert.equal(await journal.compareAndSet(id, initial, review), false);
  const done = { ...review, state: 'complete', uncertain: null };
  assert.equal(await journal.compareAndSet(id, review, done), true);
  assert.equal(await journal.releaseComplete(id), true);
  assert.equal(sql.prepare('SELECT COUNT(*) AS n FROM service_operations').get().n, 0);
  sql.close();
});

test('concurrent starts dispatch only one source freeze', async () => {
  const f = fixture();
  let release;
  const barrier = new Promise(resolve => { release = resolve; });
  f.adapter.freezeSource = async () => { f.calls.push('freezeSource'); await barrier; return true; };
  const first = advanceMigration(f.journal, f.adapter, 7);
  await new Promise(resolve => setImmediate(resolve));
  await assert.rejects(advanceMigration(f.journal, f.adapter, 7), /requires review/);
  release();
  await first;
  assert.equal(f.calls.filter(x => x === 'freezeSource').length, 1);
});

test('snapshot keeps full original quota and absolute deadline, including unlimited quota', () => {
  const account = { email: 'Alice', subId: 'secret', enable: false, up: 10, down: 20, allTime: 50, totalGB: 100, expiryTime: 1234 };
  assert.deepEqual(migrationSnapshot(account, 'alice', 'secret'), { upload: 10, download: 20, allTime: 50, quota: 100, expiry: 1234 });
  assert.equal(migrationSnapshot({ ...account, totalGB: 0, expiryTime: 0 }, 'alice', 'secret').quota, 0);
  for (const bad of [{ ...account, enable: true }, { ...account, subId: 'wrong' }, { ...account, up: -1 }, { ...account, expiryTime: -1 }]) {
    assert.throws(() => migrationSnapshot(bad, 'alice', 'secret'));
  }
});
