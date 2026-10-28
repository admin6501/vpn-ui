import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import worker from '../worker.js';

function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 const prep=(q,a=[])=>({bind(...x){return prep(q,x)},async run(){let r=sql.prepare(q).run(...a);return {meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async all(){return {results:sql.prepare(q).all(...a)}},async first(){return sql.prepare(q).get(...a)??null}});
 const env={DB:{prepare:q=>prep(q),batch:async ss=>Promise.all(ss.map(s=>s.all()))},BOT_TOKEN:'t',WEBHOOK_SECRET:'secret',ADMIN_ID:'1',PANEL_URL:'https://panel.example/base',PANEL_USER:'u',PANEL_PASS:'p'};
 sql.prepare('INSERT INTO services(user_id,name,volume,duration,sub_link,panel_email,inbound_id) VALUES(?,?,?,?,?,?,?)').run(2,'A','10 GB','30 days','https://sub.example/sub/token','customer',3);
 const active=new Map([[3,true],[4,false]]);let enabled=true,blocked=false,error=false,writes=[],out=[],seq=0;
 const account=()=>({email:'customer',enable:enabled,memberships:[{inboundId:3,protocol:'vless',remark:'first',enable:true},{inboundId:4,protocol:'shadowsocks',remark:'second',enable:true}]});
 globalThis.fetch=async(url,opt={})=>{const u=new URL(url),p=u.pathname;
  if(u.hostname==='api.telegram.org'){let method=p.split('/').at(-1),body=JSON.parse(opt.body);out.push({method,body});return new Response(JSON.stringify({ok:true,result:{message_id:1}}));}
  if(p.endsWith('/login'))return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':'vpn-ui=test'}});
  if(p.endsWith('/clients/list'))return new Response(JSON.stringify({success:true,obj:{rows:[account()],total:1}}));
  if(p.endsWith('/clients/inbounds'))return new Response(JSON.stringify({success:true,obj:[3,4].map(id=>({id,settings:JSON.stringify({clients:[{email:'customer',enable:!blocked&&active.get(id)}]})}))}));
  if(p.includes('/setMembershipEnable/')){let id=Number(p.split('/').at(-3));writes.push({id,enable:new URLSearchParams(opt.body).get('enable')});if(error)return new Response(JSON.stringify({success:false}));active.set(id,new URLSearchParams(opt.body).get('enable')==='true');return new Response(JSON.stringify({success:true}));}
  throw Error('Unexpected request '+p);
 };
 async function cb(uid,data){let tasks=[];const update={update_id:++seq,callback_query:{id:'cb'+seq,from:{id:uid},message:{chat:{id:uid,type:'private'},message_id:2},data}};const req=new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'secret'},body:JSON.stringify(update)});assert.equal((await worker.fetch(req,env,{waitUntil:p=>tasks.push(p)})).status,200);await Promise.all(tasks)}
 return {sql,cb,out,writes,active,setEnabled(v){enabled=v},setBlocked(v){blocked=v},setError(v){error=v}};
}

test('buttons reflect each client switch and toggle just that inbound',async()=>{
 const f=fixture();await f.cb(2,'cfgmanage:1:0');const msg=f.out.filter(x=>x.method==='sendMessage').at(-1).body;
 const kb=msg.reply_markup.inline_keyboard.flat();assert.ok(kb.some(x=>x.callback_data==='cfgset:1:3:0'&&x.text.includes('غیرفعال')));assert.ok(kb.some(x=>x.callback_data==='cfgset:1:4:1'&&x.text.includes('فعال')));
 await f.cb(2,'cfgset:1:3:0');assert.equal(f.active.get(3),false);assert.equal(f.active.get(4),false);assert.equal(f.writes.length,1);
 await f.cb(2,'cfgmanage:1:0');assert.ok(f.out.filter(x=>x.method==='sendMessage').at(-1).body.reply_markup.inline_keyboard.flat().some(x=>x.callback_data==='cfgset:1:3:1'));
 await f.cb(2,'cfgset:1:3:1');assert.equal(f.active.get(3),true);assert.equal(f.active.get(4),false);assert.deepEqual(f.writes.map(x=>x.enable),['false','true']);
 await f.cb(2,'cfgset:1:3:1');assert.equal(f.writes.length,2);
});
test('other users cannot see or change service and unowned inbound is rejected',async()=>{
 const f=fixture();await f.cb(99,'cfgmanage:1:0');await f.cb(99,'cfgset:1:3:0');await f.cb(2,'cfgset:1:99:0');assert.equal(f.writes.length,0);
});
test('disabled account, panel rejection and representative block never report successful activation',async()=>{
 const f=fixture();f.setEnabled(false);await f.cb(2,'cfgmanage:1:0');await f.cb(2,'cfgset:1:4:1');assert.equal(f.writes.length,0);
 f.setEnabled(true);f.setError(true);await f.cb(2,'cfgset:1:4:1');assert.equal(f.active.get(4),false);assert.ok(f.out.some(x=>x.body.text?.includes('نتیجهٔ تغییر وضعیت مشخص نیست')));
 f.setError(false);f.setBlocked(true);await f.cb(2,'cfgset:1:4:1');assert.ok(f.out.some(x=>x.body.text?.includes('تغییر در پنل تأیید نشد')));
});
test('locked service prevents simultaneous delete or toggle',async()=>{
 const f=fixture();f.sql.prepare('INSERT INTO service_operations VALUES(?,?,?)').run(1,'delete','now');await f.cb(2,'cfgset:1:3:0');assert.equal(f.writes.length,0);
});
