import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import worker, {panelCall} from '../worker.js';
import {decryptPanel} from '../panel-config.js';

function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 const prep=(q,a=[])=>({bind(...x){return prep(q,x)},async run(){let r=sql.prepare(q).run(...a);return {meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async all(){return {results:sql.prepare(q).all(...a)}},async first(){return sql.prepare(q).get(...a)??null}});
 const DB={prepare:q=>prep(q),batch:async ss=>Promise.all(ss.map(s=>s.all()))};let out=[],seq=0,acceptPass='pass-秘密',panelRequests=[];
 globalThis.fetch=async(url,opt={})=>{let u=new URL(url),p=u.pathname;
  if(u.hostname==='api.telegram.org'){const method=p.split('/').at(-1),body=JSON.parse(opt.body);out.push({method,body});return new Response(JSON.stringify({ok:true,result:{message_id:1}}));}
  assert.equal(u.hostname,'panel.example');panelRequests.push({path:p,body:opt.body});
  if(p.endsWith('/login')){let data=new URLSearchParams(opt.body);return new Response(JSON.stringify({success:data.get('username')==='rep'&&data.get('password')===acceptPass}),{headers:{'set-cookie':'vpn-ui=secret-session; Path=/'}});}
  if(p.endsWith('/clients/groups'))return new Response(JSON.stringify({success:true,obj:[{id:9,name:'Group A',inboundIds:[31,32]}]}));
  if(p.endsWith('/clients/assignable'))return new Response(JSON.stringify({success:true,obj:[{inboundId:31,remark:'A',protocol:'vless'},{inboundId:32,remark:'B',protocol:'shadowsocks'}]}));
  throw Error('unexpected');
 };
 const env={DB,BOT_TOKEN:'bot-secret-token',WEBHOOK_SECRET:'secret',ADMIN_ID:'1'};
 async function emit(data){let tasks=[];let req=new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'secret'},body:JSON.stringify({update_id:++seq,...data})});assert.equal((await worker.fetch(req,env,{waitUntil:p=>tasks.push(p)})).status,200);await Promise.all(tasks)}
 const message=(uid,text)=>emit({message:{from:{id:uid},chat:{id:uid,type:'private'},message_id:++seq,text}});
 const cb=(uid,data)=>emit({callback_query:{id:'cb'+ ++seq,from:{id:uid},message:{chat:{id:uid,type:'private'},message_id:1},data}});
 return {sql,out,env,message,cb,panelRequests,setPass(p){acceptPass=p}};
}

test('owner adds panel and subscription, then assigns a group to a category; credentials encrypted',async()=>{
 const f=fixture();await f.cb(1,'admin:settings');assert.ok(JSON.stringify(f.out.at(-2)).includes('admin:panelsetup') || f.out.some(x=>JSON.stringify(x.body).includes('admin:panelsetup')));
 await f.cb(1,'admin:panelsetup');await f.message(1,'https://panel.example:2053/path');await f.message(1,'rep');await f.message(1,'pass-秘密');
 const row=f.sql.prepare("SELECT value FROM settings WHERE key='panel_credentials_v1'").get();assert.ok(row);assert.ok(!row.value.includes('pass-秘密')&&!row.value.includes('rep'));
 const saved=await decryptPanel(f.env.BOT_TOKEN,row.value);assert.deepEqual(saved,{url:'https://panel.example:2053/path',user:'rep',pass:'pass-秘密'});
 assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='panel_draft:1'").get(),undefined);
 assert.ok(!JSON.stringify(f.out).includes('pass-秘密'));assert.ok(f.out.filter(x=>x.method==='deleteMessage').length>=3);
 await f.message(1,'https://sub.example:2096/sub/');
 let settings=Object.fromEntries(f.sql.prepare("SELECT key,value FROM settings WHERE key IN ('panel_group_ids','panel_inbound_id','sub_base_url')").all().map(x=>[x.key,x.value]));assert.deepEqual(settings,{panel_group_ids:'',panel_inbound_id:'',sub_base_url:'https://sub.example:2096/sub/'});
 await f.cb(1,'admin:category_add');await f.message(1,'Premium');await f.cb(1,'admin:categorygroup:9');assert.equal(f.sql.prepare('SELECT group_ids FROM bot_categories').get().group_ids,'9');
 await f.cb(1,'admin:panelcheck');assert.ok(f.out.some(x=>x.body.text?.includes('اتصال به پنل برقرار')));
});

test('bad credentials do not replace panel; password is retryable without storing plaintext',async()=>{
 const f=fixture();await f.cb(1,'admin:panelsetup');await f.message(1,'https://panel.example/path');await f.message(1,'rep');await f.message(1,'wrong-secret');
 assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='panel_credentials_v1'").get(),undefined);assert.ok(!JSON.stringify(f.out).includes('wrong-secret'));assert.ok(!JSON.stringify(f.sql.prepare('SELECT * FROM states').all()).includes('wrong-secret'));assert.ok(!JSON.stringify(f.sql.prepare('SELECT * FROM settings').all()).includes('wrong-secret'));
 await f.message(1,'pass-秘密');assert.ok(f.sql.prepare("SELECT value FROM settings WHERE key='panel_credentials_v1'").get());
});

test('non-owner cannot configure panel and owner may switch with existing services',async()=>{
 const f=fixture();f.sql.prepare('INSERT INTO admins VALUES(?,?)').run(5,'now');await f.cb(5,'admin:panelsetup');assert.ok(f.out.some(x=>x.method==='answerCallbackQuery'&&x.body.show_alert));
 f.sql.prepare("INSERT INTO services(user_id,name,panel_email) VALUES(2,'x','user')").run();await f.cb(1,'admin:panelsetup');await f.message(1,'https://panel.example/path');await f.message(1,'rep');await f.message(1,'pass-秘密');
 assert.ok(f.sql.prepare("SELECT value FROM settings WHERE key='panel_credentials_v1'").get());assert.ok(f.panelRequests.length>0);assert.ok(f.sql.prepare("SELECT * FROM services WHERE user_id=2").get());
});

test('invalid HTTP URL does not persist credentials',async()=>{
 const f=fixture();await f.cb(1,'admin:panelsetup');await f.message(1,'http://panel.example/path');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='panel_credentials_v1'").get(),undefined);assert.ok(f.out.some(x=>x.body.text?.includes('HTTPS')));
});

test('rotating bot token permits re-entering the same account without clearing service settings',async()=>{
 const f=fixture();await f.cb(1,'admin:panelsetup');await f.message(1,'https://panel.example/path');await f.message(1,'rep');await f.message(1,'pass-秘密');
 f.sql.prepare("INSERT INTO services(user_id,name,panel_email) VALUES(2,'x','customer')").run();
 f.sql.prepare("UPDATE settings SET value='9' WHERE key='panel_group_ids'").run();f.sql.prepare("UPDATE settings SET value='31' WHERE key='panel_inbound_id'").run();f.env.BOT_TOKEN='rotated-token';
 await f.cb(1,'admin:panelsetup');await f.message(1,'https://panel.example/path');await f.message(1,'rep');await f.message(1,'pass-秘密');
 const cfg=await decryptPanel(f.env.BOT_TOKEN,f.sql.prepare("SELECT value FROM settings WHERE key='panel_credentials_v1'").get().value);assert.equal(cfg.user,'rep');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='panel_group_ids'").get().value,'9');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='panel_inbound_id'").get().value,'31');
});

test('panel sessions are bound to panel credentials and redirects are disabled',async()=>{
 let logins=0; const seen=[];
 globalThis.fetch=async(url,opt={})=>{assert.equal(opt.redirect,'error');let path=new URL(url).pathname;if(path.endsWith('/login')){logins++;const data=new URLSearchParams(opt.body);return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':`vpn-ui=${data.get('username')}; Path=/`}});} seen.push(opt.headers.cookie);return new Response(JSON.stringify({success:true,obj:[]}));};
 const a={PANEL_URL:'https://panel.example/path',PANEL_USER:'alpha',PANEL_PASS:'one'},b={...a,PANEL_USER:'beta',PANEL_PASS:'two'};
 await panelCall(a,'GET','/panel/api/clients/groups');await panelCall(b,'GET','/panel/api/clients/groups');assert.equal(logins,2);assert.deepEqual(seen,['vpn-ui=alpha','vpn-ui=beta']);
});
