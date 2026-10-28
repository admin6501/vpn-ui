import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, copyFileSync, rmSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { DatabaseSync } from 'node:sqlite';
function fixture(schemaChanged=false) {
 const dir=mkdtempSync(join(tmpdir(),'bot-menu-'));
 const app=join(dir,'app'), data=join(dir,'data'), conf=join(dir,'conf'), bin=join(dir,'bin'), repo=join(dir,'vpn-ui-main/telegram-bot');
 for(const p of [app,data,conf,bin,repo]) mkdirSync(p,{recursive:true});
 const files=['worker.js','qr.js','panel-config.js','wallet.js','server.mjs','schema.sql','package.json','package-lock.json','manage-server.sh'];
 for(const file of files){copyFileSync(resolve(file),join(repo,file));copyFileSync(resolve(file),join(app,file));}
 writeFileSync(join(app,'worker.js'),'// old version\n');
 if(schemaChanged)writeFileSync(join(app,'schema.sql'),'-- old schema\n');
 writeFileSync(join(conf,'config.json'),'{}');const db=new DatabaseSync(join(data,'bot.sqlite'));db.exec('CREATE TABLE keepme(v); INSERT INTO keepme VALUES(42)');db.close();
 assert.equal(spawnSync('tar',['-czf',join(dir,'repo.tar.gz'),'-C',dir,'vpn-ui-main']).status,0);
 const fake=(name,body)=>writeFileSync(join(bin,name),'#!/usr/bin/env bash\n'+body+'\n',{mode:0o755});
 fake('curl','cp "$MOCK_ARCHIVE" "${@: -1}"');fake('npm','exit 0');fake('systemctl','echo "$*" >> "$MOCK_LOG"; if [[ "${MOCK_FAIL_START:-}" == 1 && "$1" == start && ! -e "$MOCK_LOG.failed" ]]; then touch "$MOCK_LOG.failed"; exit 1; fi; exit 0');
 let script=readFileSync('manage-server.sh','utf8');
 script=script.replace('APP=/opt/vpn-ui-telegram-bot',`APP=${app}`).replace('CONF=/etc/vpn-ui-bot',`CONF=${conf}`).replace('DATA=/var/lib/vpn-ui-bot',`DATA=${data}`).replace('NODE=/opt/vpn-ui-bot-node/bin/node',`NODE=${process.execPath}`).replace('[[ $EUID == 0 ]]', '[[ 1 == 1 ]]').replaceAll('/opt/.vpn-ui-bot-stage.',`${dir}/.stage.`).replaceAll('/opt/vpn-ui-bot-node',`${dir}/runtime`).replaceAll('/etc/systemd/system/vpn-ui-telegram-bot.service',`${dir}/service`).replaceAll('/usr/local/bin/vpn-ui-bot',`${dir}/link`);
 const menu=join(dir,'menu.sh');writeFileSync(menu,script);
 const run=(input,extra={})=>spawnSync('bash',[menu],{input,encoding:'utf8',timeout:20000,env:{...process.env,PATH:`${bin}:${process.env.PATH}`,MOCK_ARCHIVE:join(dir,'repo.tar.gz'),MOCK_LOG:join(dir,'calls'),...extra}});
 return {dir,app,data,conf,run};
}
test('update preserves database and creates backup before replacing code',()=>{
 const f=fixture();try{const result=f.run('9\n\n0\n');assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/آپدیت انجام شد/);assert.match(readFileSync(join(f.app,'worker.js'),'utf8'),/export default/);const db=new DatabaseSync(join(f.data,'bot.sqlite'));assert.equal(db.prepare('SELECT v FROM keepme').get().v,42);db.close();assert.ok(existsSync(join(f.data,'backups')))}finally{rmSync(f.dir,{recursive:true,force:true})}
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
test('sample cleanup deletes only matching plans and leaves services',()=>{
 const f=fixture();try{const db=new DatabaseSync(join(f.data,'bot.sqlite'));db.exec("CREATE TABLE plans(key TEXT PRIMARY KEY,gb REAL,price INTEGER,sort INTEGER); INSERT INTO plans VALUES('p10',10,70000,1),('p20',20,42,2),('mine',5,70000,3); CREATE TABLE services(id INTEGER); INSERT INTO services VALUES(1)");db.close();const result=f.run('11\nREMOVE\n\n0\n');assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/1 بسته/);const after=new DatabaseSync(join(f.data,'bot.sqlite'));assert.equal(after.prepare('SELECT count(*) n FROM plans').get().n,2);assert.equal(after.prepare('SELECT count(*) n FROM services').get().n,1);after.close()}finally{rmSync(f.dir,{recursive:true,force:true})}
});

test('database statistics report counts without exposing client identifiers',()=>{
 const f=fixture();try{const db=new DatabaseSync(join(f.data,'bot.sqlite'));db.exec(readFileSync('schema.sql','utf8'));db.exec("INSERT INTO services(user_id,name) VALUES(987654321,'private-client')");db.close();const result=f.run('12\n\n0\n');assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/سرویس‌های ثبت‌شده در ربات: 1/);assert.doesNotMatch(result.stdout,/987654321|private-client/)}finally{rmSync(f.dir,{recursive:true,force:true})}
});
