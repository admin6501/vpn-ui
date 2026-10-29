import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import {runSubscriptionPolicies} from '../worker.js';
const GB=1024**3, DAY=86400000;
function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 const prep=(q,a=[])=>({bind(...x){return prep(q,x)},execute(){const s=sql.prepare(q);if(s.columns().length)return {results:s.all(...a),meta:{changes:0}};const r=s.run(...a);return {results:[],meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async run(){return this.execute()},async all(){return this.execute()},async first(){return sql.prepare(q).get(...a)??null}});
 const DB={prepare:q=>prep(q),async batch(ss){sql.exec('BEGIN');try{const r=ss.map(x=>x.execute());sql.exec('COMMIT');return r}catch(e){sql.exec('ROLLBACK');throw e}}};
 const env={DB,BOT_TOKEN:'token',ADMIN_ID:'1',PANEL_URL:'https://panel.example/base',PANEL_USER:'admin',PANEL_PASS:'pass'};let accounts=[],sent=[],failDelete=false,failTelegram=false,listHook=null;
 const set=(k,v)=>sql.prepare('INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value').run(k,String(v));
 globalThis.fetch=async(url,opt={})=>{const u=new URL(url),p=u.pathname;
  if(u.hostname==='api.telegram.org'){const body=JSON.parse(opt.body);sent.push({method:p.split('/').at(-1),...body});return Response.json(failTelegram?{ok:false,description:'temporary failure'}:{ok:true,result:{message_id:1}})}
  if(p.endsWith('/login'))return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':'session=abc'}});
  if(p.endsWith('/clients/list')){if(listHook)listHook(u);return Response.json({success:true,obj:{rows:accounts,total:accounts.length}})}
  if(p.endsWith('/multiDeleteCapability'))return Response.json({success:true,obj:{preservesSharedTraffic:true}});
  if(p.includes('/delClientByEmail/')){if(failDelete)throw Error('unavailable');const id=Number(p.match(/inbounds\/(\d+)/)[1]),email=decodeURIComponent(p.split('/').at(-1));accounts=accounts.map(a=>a.email===email?{...a,memberships:a.memberships.filter(m=>m.inboundId!==id)}:a).filter(a=>a.memberships.length);return Response.json({success:true})}
  throw Error('unexpected '+p);
 };
 const add=(id=1,user=2,email='alice',sub='sid')=>sql.prepare('INSERT INTO services(id,user_id,name,panel_email,sub_id) VALUES(?,?,?,?,?)').run(id,user,email,email,sub);
 const account=(overrides={})=>({email:'alice',subId:'sid',enable:true,up:0,down:9.5*GB,totalGB:10*GB,expiryTime:Date.now()+0.5*DAY,memberships:[{inboundId:3},{inboundId:4}],...overrides});
 return {sql,env,set,add,account,sent,setAccounts(a){accounts=a},getAccounts(){return accounts},failDelete(){failDelete=true},setTelegramFailure(v){failTelegram=v},onList(fn){listHook=fn}};
}
test('alerts default off, notify once, respect unlimited limits and rearm after renewal',async()=>{
 const f=fixture();f.add();f.setAccounts([f.account()]);await runSubscriptionPolicies(f.env);assert.equal(f.sent.length,0);
 f.set('alerts_enabled',1);await runSubscriptionPolicies(f.env);assert.equal(f.sent.filter(x=>x.chat_id===2).length,1);assert.match(f.sent.at(-1).text,/زمان باقی‌مانده/);assert.match(f.sent.at(-1).text,/حجم باقی‌مانده/);
 await runSubscriptionPolicies(f.env);assert.equal(f.sent.length,1);
 f.setAccounts([f.account({up:0,down:0,expiryTime:Date.now()+30*DAY})]);await runSubscriptionPolicies(f.env);
 f.setAccounts([f.account({up:0,down:9.5*GB,expiryTime:Date.now()+0.5*DAY})]);await runSubscriptionPolicies(f.env);assert.equal(f.sent.length,2);
 f.setAccounts([f.account({totalGB:0,expiryTime:0})]);await runSubscriptionPolicies(f.env);assert.equal(f.sent.length,2);
});
test('concurrent policy runs do not duplicate alerts and explicit delivery failures can retry',async()=>{
 const f=fixture();f.add();f.setAccounts([f.account()]);f.set('alerts_enabled',1);f.setTelegramFailure(true);
 await Promise.all([runSubscriptionPolicies(f.env),runSubscriptionPolicies(f.env)]);assert.equal(f.sent.length,1);
 f.setTelegramFailure(false);await runSubscriptionPolicies(f.env);assert.equal(f.sent.length,2);await runSubscriptionPolicies(f.env);assert.equal(f.sent.length,2);
});
test('automatic deletion is off by default and only removes a safely verified overdue account',async()=>{
 const f=fixture();f.add();f.setAccounts([f.account({expiryTime:Date.now()-3*DAY})]);await runSubscriptionPolicies(f.env);assert.equal(f.getAccounts().length,1);
 f.set('auto_delete_enabled',1);f.set('auto_delete_days',7);await runSubscriptionPolicies(f.env);assert.equal(f.getAccounts().length,1);
 f.set('auto_delete_days',2);await runSubscriptionPolicies(f.env);assert.equal(f.getAccounts().length,0);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,0);
});
test('automatic deletion skips pending renewals, unlimited expiry and ambiguous identity',async()=>{
 const f=fixture();f.add();f.set('auto_delete_enabled',1);f.set('auto_delete_days',0);f.setAccounts([f.account({expiryTime:Date.now()-DAY})]);f.sql.exec("INSERT INTO orders(user_id,service_name,renew_service_id,status) VALUES(2,'alice',1,'pending')");await runSubscriptionPolicies(f.env);assert.equal(f.getAccounts().length,1);
 f.sql.exec("UPDATE orders SET status='approved'");f.setAccounts([f.account({expiryTime:0})]);await runSubscriptionPolicies(f.env);assert.equal(f.getAccounts().length,1);
 f.setAccounts([f.account({expiryTime:Date.now()-DAY})]);f.add(2,3);await runSubscriptionPolicies(f.env);assert.equal(f.getAccounts().length,1);
});
test('automatic deletion rechecks a renewed account immediately before deleting',async()=>{
 const f=fixture();f.add();f.set('auto_delete_enabled',1);f.set('auto_delete_days',0);f.setAccounts([f.account({expiryTime:Date.now()-DAY})]);let calls=0;
 f.onList(()=>{if(++calls===2)f.setAccounts([f.account({expiryTime:Date.now()+DAY})])});await runSubscriptionPolicies(f.env);assert.equal(f.getAccounts().length,1);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);
});
test('failed deletion retains a review lock and notifies the administrator once',async()=>{
 const f=fixture();f.add();f.set('auto_delete_enabled',1);f.set('auto_delete_days',0);f.setAccounts([f.account({expiryTime:Date.now()-DAY})]);f.failDelete();await runSubscriptionPolicies(f.env);assert.equal(f.sql.prepare('SELECT kind FROM service_operations').get().kind,'auto_delete_review');assert.equal(f.sent.filter(x=>x.chat_id===1).length,1);await runSubscriptionPolicies(f.env);assert.equal(f.sent.filter(x=>x.chat_id===1).length,1);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);
});
