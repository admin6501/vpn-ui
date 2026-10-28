import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import worker from '../worker.js';

function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 const prep=(q,a=[])=>({bind(...x){return prep(q,x)},async run(){const r=sql.prepare(q).run(...a);return {meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async all(){return {results:sql.prepare(q).all(...a)}},async first(){return sql.prepare(q).get(...a)??null}});
 const DB={prepare:q=>prep(q),batch:async ss=>Promise.all(ss.map(s=>s.all()))};let output=[], sequence=0;
 globalThis.fetch=async(url,opt)=>{const p=new URL(url).pathname;
  if(p.startsWith('/bot')){let method=p.split('/').at(-1),body=JSON.parse(opt.body);output.push({method,body});return new Response(JSON.stringify(method==='getChatMember'?{ok:true,result:{status:'member'}}:{ok:true,result:{message_id:123}}));}
  if(p.endsWith('/login'))return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':'vpn-ui=test'}});
  if(p.endsWith('/clients/groups'))return new Response(JSON.stringify({success:true,obj:[{id:1,inboundIds:[3]}]}));
  if(p.endsWith('/clients/assignable'))return new Response(JSON.stringify({success:true,obj:[{inboundId:3,protocol:'vless'}]}));
  throw Error('unexpected fetch: '+p);
 };
 const env={DB,BOT_TOKEN:'token',WEBHOOK_SECRET:'secret',ADMIN_ID:'1',PANEL_URL:'https://panel.example/base',PANEL_USER:'u',PANEL_PASS:'p',CHANNEL_USERNAME:'@channel'};
 async function emit(data){let tasks=[];let req=new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'secret'},body:JSON.stringify({update_id:++sequence,...data})});assert.equal((await worker.fetch(req,env,{waitUntil:p=>tasks.push(p)})).status,200);await Promise.all(tasks)}
 const message=(uid,text,name='A')=>emit({message:{from:{id:uid,first_name:name},chat:{id:uid,type:'private'},message_id:++sequence,text}});
 const callback=(uid,data)=>emit({callback_query:{id:'cb'+ ++sequence,from:{id:uid,first_name:'A'},message:{chat:{id:uid,type:'private'},message_id:12},data}});
 const last=()=>output.filter(o=>o.method==='sendMessage').at(-1)?.body;
 return {sql,output,message,callback,last};
}

test('start text can be edited, is safely displayed, validated and reset',async()=>{
 const f=fixture(); await f.message(2,'/start'); assert.match(f.last().text,/سلام A/);
 await f.callback(1,'admin:settings'); assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:setting:start_text'));
 await f.callback(1,'admin:setting:start_text'); await f.message(1,'خوش آمدی {name}\nپیام <b>خام</b>');
 assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='start_text'").get().value,'خوش آمدی {name}\nپیام <b>خام</b>');
 await f.message(2,'/start','A$&<X>'); assert.ok(f.last().text.includes('خوش آمدی A$&amp;&lt;X&gt;\nپیام &lt;b&gt;خام&lt;/b&gt;'));
 await f.callback(2,'check_join'); assert.ok(f.last().text.includes('خوش آمدی A'));
 await f.callback(1,'admin:setting:start_text'); await f.message(1,'x'.repeat(701)); assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='start_text'").get().value,'خوش آمدی {name}\nپیام <b>خام</b>');
 await f.message(1,'-'); await f.message(2,'/start'); assert.match(f.last().text,/سلام A/);
});

test('custom start text is used with main menu photo',async()=>{
 const f=fixture(); f.sql.prepare("INSERT INTO settings(key,value) VALUES('main_menu_image_url',?)").run('https://example.com/image.jpg');
 f.sql.prepare("INSERT INTO settings(key,value) VALUES('start_text',?)").run('درود {name}');
 await f.message(2,'/start'); const photo=f.output.findLast(o=>o.method==='sendPhoto'); assert.ok(photo); assert.ok(photo.body.caption.includes('درود A'));
});

test('admin menu exposes every section, settings edit and toggle',async()=>{
 const f=fixture();await f.message(1,'/start');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:root'));
 await f.callback(1,'admin:root');for(const name of ['admin:settings','admin:plans','admin:admins','admin:stats','admin:resettest','admin:panelcheck'])assert.ok(JSON.stringify(f.last().reply_markup).includes(name));
 await f.callback(1,'admin:settings');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:setting:sub_base_url'));
 await f.callback(1,'admin:setting:duration_days');await f.message(1,'45');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='duration_days'").get().value,'45');
 await f.callback(1,'admin:setting:test_enabled');await f.callback(1,'admin:test:0');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='test_enabled'").get().value,'0');
 await f.callback(1,'admin:panelcheck');assert.ok(f.output.some(x=>x.body.text?.includes('اتصال به پنل برقرار')));
 await f.callback(1,'admin:stats');assert.ok(f.output.some(x=>x.body.text?.includes('مجموع فروش')));
});

test('buttons create, change and confirm deletion of plans',async()=>{
 const f=fixture();await f.callback(1,'admin:plan_add');await f.message(1,'p3 12345 3');assert.equal(f.sql.prepare("SELECT price FROM plans WHERE key='p3'").get().price,12345);
 await f.callback(1,'admin:price:p3');await f.message(1,'50000');assert.equal(f.sql.prepare("SELECT price FROM plans WHERE key='p3'").get().price,50000);
 await f.callback(1,'admin:editplan:p3');await f.message(1,'60000 7.5');assert.equal(f.sql.prepare("SELECT gb FROM plans WHERE key='p3'").get().gb,7.5);
 await f.callback(1,'admin:delplan:p3');assert.ok(f.sql.prepare("SELECT key FROM plans WHERE key='p3'").get());
 await f.callback(1,'admin:confirmplan:p3');assert.equal(f.sql.prepare("SELECT key FROM plans WHERE key='p3'").get(),undefined);
});

test('owner can add/remove secondary admins, forged admin actions are denied',async()=>{
 const f=fixture();await f.callback(1,'admin:add_admin');await f.message(1,'55');assert.ok(f.sql.prepare('SELECT user_id FROM admins WHERE user_id=55').get());
 await f.callback(55,'admin:rmconfirm:1');assert.ok(f.output.some(x=>x.method==='answerCallbackQuery'&&x.body.show_alert));
 await f.callback(66,'admin:setting:test_enabled');await f.callback(66,'admin:test:0');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='test_enabled'").get(),undefined);
 await f.callback(1,'admin:rmadmin:55');assert.ok(f.sql.prepare('SELECT user_id FROM admins WHERE user_id=55').get());
 await f.callback(1,'admin:rmconfirm:55');assert.equal(f.sql.prepare('SELECT user_id FROM admins WHERE user_id=55').get(),undefined);
});

test('reset test requires confirmation, cancel and navigation clear wizard state',async()=>{
 const f=fixture();f.sql.prepare("INSERT INTO test_claims VALUES(?,?)").run(77,'today');
 await f.callback(1,'admin:resettest');await f.message(1,'77');assert.ok(f.sql.prepare('SELECT user_id FROM test_claims WHERE user_id=77').get());
 await f.callback(1,'admin:root');await f.callback(1,'admin:reset_confirm');assert.ok(f.sql.prepare('SELECT user_id FROM test_claims WHERE user_id=77').get());
 await f.callback(1,'admin:resettest');await f.message(1,'77');await f.callback(1,'admin:reset_confirm');assert.equal(f.sql.prepare('SELECT user_id FROM test_claims WHERE user_id=77').get(),undefined);
 await f.callback(1,'admin:setting:duration_days');await f.callback(1,'admin:stats');await f.message(1,'99');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='duration_days'").get(),undefined);
});
