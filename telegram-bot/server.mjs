import { DatabaseSync } from 'node:sqlite';
import { readFileSync, mkdirSync, chmodSync, existsSync } from 'node:fs';
import { dirname } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import worker, { handleUpdate, runSubscriptionPolicies } from './worker.js';
import { migratePlanModel } from './plan-migration.mjs';
import { processMessageJobs, queuedMessagePayload } from './messages.js';

const config=JSON.parse(readFileSync(process.env.BOT_CONFIG || '/etc/vpn-ui-bot/config.json','utf8'));
if (!/^\d+:[A-Za-z0-9_-]+$/.test(config.BOT_TOKEN) || !/^\d+(,\d+)*$/.test(config.ADMIN_ID)) throw Error('Invalid bot token or admin ID');
const path=process.env.BOT_DATABASE || '/var/lib/vpn-ui-bot/bot.sqlite';
mkdirSync(dirname(path),{recursive:true,mode:0o700});
const freshDatabase=!existsSync(path);
const db=new DatabaseSync(path); chmodSync(path,0o600);
db.exec('PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;');
// The schema contains only CREATE IF NOT EXISTS statements; existing data remains intact.
db.exec(readFileSync(new URL('./schema.sql',import.meta.url),'utf8'));
migratePlanModel(db);
if(freshDatabase) db.exec("INSERT INTO settings(key,value) VALUES ('referral_discount','0'),('test_enabled','0'),('panel_group_ids','')");
db.exec('CREATE TABLE IF NOT EXISTS server_poll_state (id INTEGER PRIMARY KEY CHECK(id=1), offset INTEGER NOT NULL); INSERT OR IGNORE INTO server_poll_state VALUES(1,0)');
class Statement {
 constructor(sql,args=[]){this.sql=sql;this.args=args}
 bind(...args){return new Statement(this.sql,args)}
 async run(){return this.execute()}
 async all(){return this.execute()}
 async first(){return db.prepare(this.sql).get(...this.args) ?? null}
 execute(){const s=db.prepare(this.sql); if(s.columns().length) return {results:s.all(...this.args),meta:{changes:0}}; const r=s.run(...this.args);return {results:[],meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}}
}
const DB={prepare(sql){return new Statement(sql)},async batch(statements){db.exec('BEGIN IMMEDIATE');try {const results=statements.map(s=>s.execute());db.exec('COMMIT');return results}catch(e){db.exec('ROLLBACK');throw e}}};
const env={...config,DB,SERVER_MODE:true};
let stopping=false;
const shutdown=new AbortController();
const stop=()=>{stopping=true;shutdown.abort()};
process.on('SIGTERM',stop); process.on('SIGINT',stop);
async function telegram(method, data={}){
 const r=await fetch(`https://api.telegram.org/bot${config.BOT_TOKEN}/${method}`,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(data),signal:AbortSignal.any([shutdown.signal,AbortSignal.timeout(40000)])});
 const obj=await r.json();if(!obj.ok)throw Error(`Telegram ${method}: ${obj.description || r.status}`);return obj.result;
}
const info=await telegram('getWebhookInfo');
if(info.url) throw Error('A Telegram webhook is active. Remove it explicitly before using server polling.');
console.log('Bot started using long polling.');
let messageTask=null;
const sendQueued=async(userId,body,signal)=>{
 const p=queuedMessagePayload(userId,body);
 const response=await fetch(`https://api.telegram.org/bot${config.BOT_TOKEN}/${p.method}`,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(p.params),signal:AbortSignal.any([signal,AbortSignal.timeout(10000)])});
 return response.json();
};
const tickMessages=()=>{if(stopping||messageTask)return;messageTask=processMessageJobs(env,sendQueued,{signal:shutdown.signal}).catch(e=>console.error('Message queue error:',e.message)).finally(()=>{messageTask=null})};
const messageTimer=setInterval(tickMessages,5000);messageTimer.unref();tickMessages();
let policyTask=null;
const tickPolicies=()=>{if(stopping||policyTask)return;policyTask=runSubscriptionPolicies(env,{signal:shutdown.signal}).catch(e=>{if(!stopping)console.error('Subscription policy error:',e.message)}).finally(()=>{policyTask=null})};
const policyTimer=setInterval(tickPolicies,300000);policyTimer.unref();tickPolicies();
let nextRecovery=0;
while(!stopping){
 try {
  if(Date.now()>=nextRecovery){let tasks=[];await worker.scheduled({},env,{waitUntil(p){tasks.push(p)}});await Promise.all(tasks);nextRecovery=Date.now()+300000}
  const offset=db.prepare('SELECT offset FROM server_poll_state WHERE id=1').get().offset;
  const updates=await telegram('getUpdates',{offset,timeout:25,allowed_updates:['message','callback_query']});
  for(const update of updates){
   if(stopping)break;
   await handleUpdate(env,update);
   db.prepare('UPDATE server_poll_state SET offset=? WHERE id=1').run(update.update_id+1);
  }
 }catch(e){if(!stopping){console.error('Polling/recovery error:',e.message);await delay(5000,undefined,{signal:shutdown.signal}).catch(()=>{})}}
}
clearInterval(messageTimer);clearInterval(policyTimer);if(messageTask)await messageTask;if(policyTask)await policyTask;db.close();console.log('Bot stopped.');
