import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, copyFileSync, rmSync, existsSync, readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { DatabaseSync } from 'node:sqlite';
function fixture(schemaChanged=false) {
 const dir=mkdtempSync(join(tmpdir(),'bot-menu-'));
 const app=join(dir,'app'), data=join(dir,'data'), conf=join(dir,'conf'), bin=join(dir,'bin'), repo=join(dir,'vpn-ui-main/telegram-bot');
 for(const p of [app,data,conf,bin,repo]) mkdirSync(p,{recursive:true});
 const files=['worker.js','qr.js','panel-config.js','wallet.js','coupons.js','messages.js','migration.js','migration-journal.js','migration-commit.js','bulk-migration.js','server.mjs','plan-migration.mjs','schema.sql','package.json','package-lock.json','manage-server.sh'];
 for(const file of files){copyFileSync(resolve(file),join(repo,file));copyFileSync(resolve(file),join(app,file));}
 writeFileSync(join(app,'worker.js'),'// old version\n');
 if(schemaChanged)writeFileSync(join(app,'schema.sql'),'-- old schema\n');
 writeFileSync(join(conf,'config.json'),'{}');const db=new DatabaseSync(join(data,'bot.sqlite'));db.exec('CREATE TABLE keepme(v); INSERT INTO keepme VALUES(42)');db.close();
 assert.equal(spawnSync('tar',['-czf',join(dir,'repo.tar.gz'),'-C',dir,'vpn-ui-main']).status,0);
 const fake=(name,body)=>writeFileSync(join(bin,name),'#!/usr/bin/env bash\n'+body+'\n',{mode:0o755});
 fake('curl','cp "$MOCK_ARCHIVE" "${@: -1}"');fake('npm','ln -s \"$MOCK_NODE_MODULES\" node_modules');fake('systemctl','echo "$*" >> "$MOCK_LOG"; if [[ "${MOCK_INTERRUPT_ON:-}" == "$1" && ! -e "$MOCK_LOG.interrupted" ]]; then touch "$MOCK_LOG.interrupted"; kill -TERM "$PPID"; exit 0; fi; if [[ "${MOCK_FAIL_START:-}" == 1 && "$1" == start && ! -e "$MOCK_LOG.failed" ]]; then touch "$MOCK_LOG.failed"; exit 1; fi; exit 0');
 let script=readFileSync('manage-server.sh','utf8');
 script=script.replace('APP=/opt/vpn-ui-telegram-bot',`APP=${app}`).replace('CONF=/etc/vpn-ui-bot',`CONF=${conf}`).replace('DATA=/var/lib/vpn-ui-bot',`DATA=${data}`).replace('NODE=/opt/vpn-ui-bot-node/bin/node',`NODE=${process.execPath}`).replace('[[ $EUID == 0 ]]', '[[ 1 == 1 ]]').replaceAll('/opt/.vpn-ui-bot-stage.',`${dir}/.stage.`).replaceAll('/opt/vpn-ui-bot-node',`${dir}/runtime`).replaceAll('/etc/systemd/system/vpn-ui-telegram-bot.service',`${dir}/service`).replaceAll('/etc/systemd/system/vpn-ui-bot-backup.service',`${dir}/backup.service`).replaceAll('/etc/systemd/system/vpn-ui-bot-backup.timer',`${dir}/backup.timer`).replaceAll('/usr/local/bin/vpn-ui-bot',`${dir}/link`);
 const menu=join(dir,'menu.sh');writeFileSync(menu,script);
 const run=(input,extra={})=>spawnSync('bash',[menu],{input,encoding:'utf8',timeout:20000,env:{...process.env,PATH:`${bin}:${process.env.PATH}`,MOCK_ARCHIVE:join(dir,'repo.tar.gz'),MOCK_NODE_MODULES:resolve('node_modules'),MOCK_LOG:join(dir,'calls'),...extra}});
 const runAuto=(extra={})=>spawnSync('bash',[menu,'--auto-backup'],{encoding:'utf8',timeout:20000,env:{...process.env,PATH:`${bin}:${process.env.PATH}`,MOCK_LOG:join(dir,'calls'),...extra}});
 return {dir,app,data,conf,repo,run,runAuto,repack(){assert.equal(spawnSync('tar',['-czf',join(dir,'repo.tar.gz'),'-C',dir,'vpn-ui-main']).status,0)}};
}
test('update preserves database and creates backup before replacing code',()=>{
 const f=fixture();try{const result=f.run('9\n\n0\n');assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/آپدیت انجام شد/);assert.match(readFileSync(join(f.app,'worker.js'),'utf8'),/export default/);const db=new DatabaseSync(join(f.data,'bot.sqlite'));assert.equal(db.prepare('SELECT v FROM keepme').get().v,42);db.close();assert.ok(existsSync(join(f.data,'backups')))}finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('update includes future runtime modules without a hard-coded file list',()=>{
 const f=fixture();try{writeFileSync(join(f.repo,'future-runtime.js'),'export const ready=true;\n');writeFileSync(join(f.repo,'worker.js'),"import './future-runtime.js';\n"+readFileSync(join(f.repo,'worker.js'),'utf8'));f.repack();const result=f.run('9\n\n0\n');assert.match(result.stdout,/آپدیت انجام شد/);assert.ok(existsSync(join(f.app,'future-runtime.js')))}finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('missing imported module blocks update before stopping the existing service',()=>{
 const f=fixture();try{writeFileSync(join(f.repo,'worker.js'),"import './missing-module.js';\nexport default {};\n");f.repack();const result=f.run('9\n\n0\n');assert.match(result.stdout,/آماده‌سازی ناموفق بود/);assert.equal(readFileSync(join(f.app,'worker.js'),'utf8'),'// old version\n');assert.ok(!existsSync(join(f.dir,'calls')));assert.ok(existsSync(join(f.data,'bot.sqlite')))}finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('missing server-only import is detected before service stop',()=>{
 const f=fixture();try{const server=join(f.repo,'server.mjs');writeFileSync(server,"import './missing-server-module.js';\n"+readFileSync(server,'utf8'));f.repack();const result=f.run('9\n\n0\n');assert.match(result.stdout,/آماده‌سازی ناموفق بود/);assert.equal(readFileSync(join(f.app,'worker.js'),'utf8'),'// old version\n');assert.ok(!existsSync(join(f.dir,'calls')))}finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('restore validates input, saves existing database, and restores service data',()=>{
 const f=fixture();try{
  const source=join(f.dir,'saved.sqlite'),db=new DatabaseSync(source);db.exec(readFileSync('schema.sql','utf8'));db.exec("INSERT INTO services(user_id,name) VALUES(123,'restored')");db.close();
  const result=f.run(`12\n${source}\nRESTORE\n\n0\n`);assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/ریستور انجام شد/);
  const live=new DatabaseSync(join(f.data,'bot.sqlite'));assert.equal(live.prepare('SELECT name FROM services').get().name,'restored');live.close();
  const files=readdirSync(join(f.data,'backups'));assert.equal(files.length,1);const old=new DatabaseSync(join(f.data,'backups',files[0]));assert.equal(old.prepare('SELECT v FROM keepme').get().v,42);old.close();
 }finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('restore rejects incompatible database without stopping service or changing data',()=>{
 const f=fixture();try{const source=join(f.dir,'invalid.sqlite');writeFileSync(source,'not a database');const result=f.run(`12\n${source}\n\n0\n`);assert.match(result.stderr,/بکاپ سازگار/);assert.ok(!existsSync(join(f.dir,'calls')));const db=new DatabaseSync(join(f.data,'bot.sqlite'));assert.equal(db.prepare('SELECT v FROM keepme').get().v,42);db.close()}finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('failed restart after restore rolls back the original database',()=>{
 const f=fixture();try{const source=join(f.dir,'saved.sqlite'),db=new DatabaseSync(source);db.exec(readFileSync('schema.sql','utf8'));db.close();const result=f.run(`12\n${source}\nRESTORE\n\n0\n`,{MOCK_FAIL_START:'1'});assert.match(result.stdout,/دیتابیس قبلی برگردانده/);const live=new DatabaseSync(join(f.data,'bot.sqlite'));assert.equal(live.prepare('SELECT v FROM keepme').get().v,42);live.close()}finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('interrupted restore does not lose the original database and restarts the bot',()=>{
 const f=fixture();try{const source=join(f.dir,'saved.sqlite'),db=new DatabaseSync(source);db.exec(readFileSync('schema.sql','utf8'));db.close();f.run(`12\n${source}\nRESTORE\n\n0\n`,{MOCK_INTERRUPT_ON:'stop'});const live=new DatabaseSync(join(f.data,'bot.sqlite'));assert.equal(live.prepare('SELECT v FROM keepme').get().v,42);live.close();assert.match(readFileSync(join(f.dir,'calls'),'utf8'),/start vpn-ui-telegram-bot.service/)}finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('backup schedule accepts minutes or hours and can be disabled',()=>{
 const f=fixture();try{let result=f.run('13\n1\n2\n2\n\n0\n');assert.match(result.stdout,/بکاپ خودکار فعال شد/);assert.match(readFileSync(join(f.dir,'backup.timer'),'utf8'),/OnUnitActiveSec=7200s/);assert.match(readFileSync(join(f.dir,'backup.service'),'utf8'),/--auto-backup/);result=f.run('13\n2\n\n0\n');assert.match(result.stdout,/غیرفعال شد/);assert.match(readFileSync(join(f.dir,'calls'),'utf8'),/disable --now vpn-ui-bot-backup.timer/)}finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('automatic backup sends a SQLite snapshot to configured owner IDs and cleans its temporary file',()=>{
 const f=fixture();try{
  writeFileSync(join(f.conf,'config.json'),JSON.stringify({BOT_TOKEN:'123:secret_token',ADMIN_ID:'11,22'}));
  writeFileSync(join(f.dir,'sitecustomize.py'),`import io,os,urllib.request\ndef mock(request,timeout):\n    with open(os.environ['MOCK_REQUESTS'],'ab') as f: f.write(request.data+b'\\nEND-REQUEST\\n')\n    return io.BytesIO(b'{"ok":true}')\nurllib.request.urlopen=mock\n`);
  const result=f.runAuto({PYTHONPATH:f.dir,MOCK_REQUESTS:join(f.dir,'requests')});assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/ارسال شد/);assert.doesNotMatch(result.stdout+result.stderr,/secret_token/);
  const data=readFileSync(join(f.dir,'requests'));assert.ok(data.includes(Buffer.from('SQLite format 3')));assert.ok(data.includes(Buffer.from('\r\n\r\n11\r\n')));assert.ok(data.includes(Buffer.from('\r\n\r\n22\r\n')));assert.ok(!readdirSync(f.data).some(x=>x.startsWith('.telegram-backup-')));
 }finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('automatic backup reports Telegram failure and removes the temporary snapshot',()=>{
 const f=fixture();try{
  writeFileSync(join(f.conf,'config.json'),JSON.stringify({BOT_TOKEN:'123:secret_token',ADMIN_ID:'11'}));
  writeFileSync(join(f.dir,'sitecustomize.py'),`import urllib.request\ndef mock(request,timeout): raise OSError('connection failed')\nurllib.request.urlopen=mock\n`);
  const result=f.runAuto({PYTHONPATH:f.dir});assert.notEqual(result.status,0);assert.match(result.stderr,/ناموفق/);assert.doesNotMatch(result.stderr,/secret_token/);assert.ok(!readdirSync(f.data).some(x=>x.startsWith('.telegram-backup-')));
 }finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('update stops before replacing code if schema differs',()=>{
 const f=fixture(true);try{const result=f.run('9\n\n0\n');assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/ساختار دیتابیس/);assert.equal(readFileSync(join(f.app,'worker.js'),'utf8'),'// old version\n')}finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('uninstall keeps data unless separately confirmed',()=>{
 const f=fixture();try{const result=f.run('10\nREMOVE\n\n');assert.equal(result.status,0,result.stderr);assert.equal(existsSync(f.app),false);assert.equal(existsSync(join(f.data,'bot.sqlite')),true);assert.equal(existsSync(join(f.conf,'config.json')),true)}finally{rmSync(f.dir,{recursive:true,force:true})}
});

test('failed start restores the old code and keeps database',()=>{
 const f=fixture();try{const result=f.run('9\n\n0\n',{MOCK_FAIL_START:'1'});assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/نسخه قبلی برگردانده/);assert.equal(readFileSync(join(f.app,'worker.js'),'utf8'),'// old version\n');assert.ok(existsSync(join(f.data,'bot.sqlite')))}finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('data deletion requires explicit separate confirmation',()=>{
 const f=fixture();try{const result=f.run('10\nREMOVE\nDELETE\n');assert.equal(result.status,0,result.stderr);assert.equal(existsSync(f.data),false);assert.equal(existsSync(f.conf),false)}finally{rmSync(f.dir,{recursive:true,force:true})}
});

test('reviewed seed-only update accepts previous schema and preserves existing plans',()=>{
 const f=fixture();try{copyFileSync(resolve('test/fixtures/previous-schema.sql'),join(f.app,'schema.sql'));const db=new DatabaseSync(join(f.data,'bot.sqlite'));db.exec("CREATE TABLE plans(key TEXT PRIMARY KEY,gb REAL,price INTEGER,sort INTEGER); INSERT INTO plans VALUES('custom',4,55,1)");db.close();const result=f.run('9\n\n0\n');assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/آپدیت انجام شد/);const after=new DatabaseSync(join(f.data,'bot.sqlite'));assert.equal(after.prepare('SELECT count(*) n FROM plans').get().n,1);after.close()}finally{rmSync(f.dir,{recursive:true,force:true})}
});
test('database statistics report counts without exposing client identifiers',()=>{
 const f=fixture();try{const db=new DatabaseSync(join(f.data,'bot.sqlite'));db.exec(readFileSync('schema.sql','utf8'));db.exec("INSERT INTO services(user_id,name) VALUES(987654321,'private-client')");db.close();const result=f.run('11\n\n0\n');assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/سرویس‌های ثبت‌شده در ربات: 1/);assert.doesNotMatch(result.stdout,/987654321|private-client/)}finally{rmSync(f.dir,{recursive:true,force:true})}
});

test('update accepts clean previous schema without modifying existing data',()=>{
 const f=fixture();try{copyFileSync(resolve('test/fixtures/clean-schema.sql'),join(f.app,'schema.sql'));const result=f.run('9\n\n0\n');assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/آپدیت انجام شد/);const db=new DatabaseSync(join(f.data,'bot.sqlite'));assert.equal(db.prepare('SELECT v FROM keepme').get().v,42);db.close()}finally{rmSync(f.dir,{recursive:true,force:true})}
});

test('interrupted update during stop or startup restores code and restarts service',()=>{
 for(const phase of ['stop','start']){const f=fixture();try{const result=f.run('9\n\n0\n',{MOCK_INTERRUPT_ON:phase});assert.equal(result.status,0,result.stderr);assert.equal(readFileSync(join(f.app,'worker.js'),'utf8'),'// old version\n');assert.ok(existsSync(join(f.data,'bot.sqlite')));const calls=readFileSync(join(f.dir,'calls'),'utf8');assert.match(calls,/start vpn-ui-telegram-bot.service/)}finally{rmSync(f.dir,{recursive:true,force:true})}}
});

test('updater accepts the previous user-directory schema and preserves the database',()=>{
 const f=fixture();try{copyFileSync(resolve('test/fixtures/user-directory-schema.sql'),join(f.app,'schema.sql'));const result=f.run('9\n\n0\n');assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/آپدیت انجام شد/);const db=new DatabaseSync(join(f.data,'bot.sqlite'));assert.equal(db.prepare('SELECT v FROM keepme').get().v,42);db.close()}finally{rmSync(f.dir,{recursive:true,force:true})}
});

test('updater accepts category-era schema and preserves the database',()=>{
 const f=fixture();try{copyFileSync(resolve('test/fixtures/category-schema.sql'),join(f.app,'schema.sql'));const result=f.run('9\n\n0\n');assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/آپدیت انجام شد/);const db=new DatabaseSync(join(f.data,'bot.sqlite'));assert.equal(db.prepare('SELECT v FROM keepme').get().v,42);db.close()}finally{rmSync(f.dir,{recursive:true,force:true})}
});
