import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,writeFileSync,readFileSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {spawnSync} from 'node:child_process';
import {DatabaseSync} from 'node:sqlite';

function runServer(webhook=false,preseed=false){
 const dir=mkdtempSync(join(tmpdir(),'vpn-ui-bot-'));
 const config=join(dir,'config.json'), database=join(dir,'db.sqlite'), calls=join(dir,'calls');
 writeFileSync(config,JSON.stringify({BOT_TOKEN:'123:FakeToken',ADMIN_ID:'123'}));
 if(preseed){const db=new DatabaseSync(database);db.exec(readFileSync(new URL('./fixtures/clean-schema.sql',import.meta.url),'utf8'));db.exec("INSERT INTO plans(key,gb,price,sort) VALUES('mine',5,500,1); INSERT INTO settings(key,value) VALUES('test_enabled','1')");db.close();}
 writeFileSync(join(dir,'mock.mjs'), `import {appendFileSync} from 'node:fs'; let count=0; globalThis.fetch=async (url)=>{const m=new URL(url).pathname.split('/').pop();appendFileSync(process.env.CALLS,m+'\\n');if(m==='getWebhookInfo')return Response.json({ok:true,result:{url:process.env.WEBHOOK?'https://example.org/webhook':''}});if(m==='getUpdates'){if(!count++)return Response.json({ok:true,result:[{update_id:100,message:{from:{id:123,first_name:'Ali'},chat:{id:123,type:'private'},message_id:1,text:'/start'}}]});setTimeout(()=>process.kill(process.pid,'SIGTERM'),20);await new Promise(r=>setTimeout(r,60));return Response.json({ok:true,result:[]});}return Response.json({ok:true,result:{message_id:1}})};`);
 const result=spawnSync(process.execPath,['--import',join(dir,'mock.mjs'),'server.mjs'],{cwd:new URL('..',import.meta.url),env:{...process.env,BOT_CONFIG:config,BOT_DATABASE:database,CALLS:calls,WEBHOOK:webhook?'1':''},encoding:'utf8',timeout:10000});
 return {dir,database,calls,result};
}
test('server polls Telegram, handles start, and persists offset',()=>{
 const {dir,database,calls,result}=runServer();try{
  assert.equal(result.status,0,result.stderr);const db=new DatabaseSync(database);assert.equal(db.prepare('SELECT offset FROM server_poll_state').get().offset,101);db.close();assert.match(readFileSync(calls,'utf8'),/sendMessage/);const check=new DatabaseSync(database); for (const table of ['plans','services','orders','admins','wallet_entries','test_claims']) assert.equal(check.prepare(`SELECT COUNT(*) AS n FROM ${table}`).get().n,0,table); assert.equal(check.prepare("SELECT value FROM settings WHERE key='test_enabled'").get().value,'0'); assert.equal(check.prepare("SELECT value FROM settings WHERE key='referral_discount'").get().value,'0');check.close();
 }finally{rmSync(dir,{recursive:true,force:true})}
});
test('server does not disable an existing webhook',()=>{
 const {dir,calls,result}=runServer(true);try{assert.notEqual(result.status,0);assert.match(result.stderr,/webhook is active/);assert.doesNotMatch(readFileSync(calls,'utf8'),/deleteWebhook|getUpdates/)}finally{rmSync(dir,{recursive:true,force:true})}
});
test('server upgrades existing database without changing plans or settings',()=>{
 const {dir,database,result}=runServer(false,true);try{assert.equal(result.status,0,result.stderr);const db=new DatabaseSync(database);assert.equal(db.prepare('SELECT COUNT(*) AS n FROM plans').get().n,1);assert.equal(db.prepare("SELECT value FROM settings WHERE key='test_enabled'").get().value,'1');assert.ok(db.prepare("SELECT name FROM sqlite_master WHERE name='bot_users'").get());db.close()}finally{rmSync(dir,{recursive:true,force:true})}
});

test('shutdown interrupts long polling promptly',()=>{
 const dir=mkdtempSync(join(tmpdir(),'bot-stop-'));try{
  const config=join(dir,'config.json'), database=join(dir,'db.sqlite');writeFileSync(config,JSON.stringify({BOT_TOKEN:'123:FakeToken',ADMIN_ID:'123'}));
  writeFileSync(join(dir,'mock.mjs'),`globalThis.fetch=async (url,opt)=>{const method=new URL(url).pathname.split('/').pop();if(method==='getWebhookInfo')return Response.json({ok:true,result:{url:''}});if(method==='getUpdates'){setTimeout(()=>process.kill(process.pid,'SIGTERM'),30);return new Promise((resolve,reject)=>{const hold=setInterval(()=>{},1000);opt.signal.addEventListener('abort',()=>{clearInterval(hold);reject(Error('aborted'))},{once:true})})}throw Error('unexpected')};`);
  const started=Date.now();const result=spawnSync(process.execPath,['--import',join(dir,'mock.mjs'),'server.mjs'],{cwd:new URL('..',import.meta.url),env:{...process.env,BOT_CONFIG:config,BOT_DATABASE:database},encoding:'utf8',timeout:3000});
  assert.equal(result.status,0,result.stderr);assert.ok(Date.now()-started<2500);assert.match(result.stdout,/Bot stopped/);
 }finally{rmSync(dir,{recursive:true,force:true})}
});
