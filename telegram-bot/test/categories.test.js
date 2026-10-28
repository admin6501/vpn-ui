import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import worker from '../worker.js';
function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 const prep=(q,a=[])=>({q,a,bind(...x){return prep(q,x)},async run(){const r=sql.prepare(q).run(...a);return {meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async all(){return {results:sql.prepare(q).all(...a)}},async first(){return sql.prepare(q).get(...a)??null}});
 const DB={prepare:q=>prep(q),async batch(ss){sql.exec('BEGIN');try{const result=[];for(const s of ss)result.push(/^\s*select/i.test(s.q)?await s.all():await s.run());sql.exec('COMMIT');return result}catch(e){sql.exec('ROLLBACK');throw e}}};
 sql.exec("INSERT INTO settings(key,value) VALUES('sub_base_url','https://sub.example/sub/'),('test_enabled','1')");let output=[],sent=[],seq=0,groups=[{id:1,name:'Alpha',inboundIds:[3,4]},{id:2,name:'Beta',inboundIds:[7]}];
 globalThis.fetch=async(url,opt={})=>{const u=new URL(url),p=u.pathname;
  if(u.hostname==='api.telegram.org'){const method=p.split('/').at(-1),body=JSON.parse(opt.body);output.push({method,body});return Response.json({ok:true,result:{message_id:3}})}
  if(p.endsWith('/login'))return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':'vpn-ui=test'}});
  if(p.endsWith('/clients/groups'))return Response.json({success:true,obj:groups});
  if(p.endsWith('/clients/assignable'))return Response.json({success:true,obj:[{inboundId:3,protocol:'vless'},{inboundId:4,protocol:'shadowsocks',method:'aes-128-gcm'},{inboundId:7,protocol:'trojan'}]});
  if(p.endsWith('/clients/list'))return Response.json({success:true,obj:{rows:[{email:'existing',subId:'old',enable:true,memberships:[{inboundId:3},{inboundId:4}]}],total:1}});
  if(p.endsWith('/addClient')){sent.push(new URLSearchParams(opt.body));return Response.json({success:true})}
  throw Error('unexpected '+p);
 };
 const env={DB,BOT_TOKEN:'token',WEBHOOK_SECRET:'secret',ADMIN_ID:'1',PANEL_URL:'https://panel.example/base',PANEL_USER:'rep',PANEL_PASS:'pass',CARD_NUMBER:'123',CARD_HOLDER:'owner'};
 async function emit(data){let pending=[];const req=new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'secret'},body:JSON.stringify({update_id:++seq,...data})});assert.equal((await worker.fetch(req,env,{waitUntil:p=>pending.push(p)})).status,200);await Promise.all(pending)}
 const message=(id,text)=>emit({message:{from:{id,first_name:'A'},chat:{id,type:'private'},message_id:++seq,text}});
 const cb=(id,data)=>emit({callback_query:{id:'cb'+ ++seq,from:{id,first_name:'A'},message:{chat:{id,type:'private'},message_id:1},data}});
 const last=()=>output.filter(x=>x.method==='sendMessage').at(-1)?.body;
 return {sql,output,sent,message,cb,last,setGroups(g){groups=g}};
}
async function category(f,name,group){await f.cb(1,'admin:category_add');await f.message(1,name);await f.cb(1,`admin:categorygroup:${group}`)}
async function plan(f,key,price,gb,days,categoryId){await f.cb(1,'admin:plan_add');await f.message(1,`${key} ${price} ${gb} ${days}`);await f.cb(1,`admin:planpick:${categoryId}`)}
test('category and plan control customer selection and preserve order group after later edits',async()=>{
 const f=fixture();await category(f,'A',1);await category(f,'B',2);await plan(f,'a10',100,10,14,1);await plan(f,'b20',200,20,40,2);
 assert.equal(f.sql.prepare('SELECT days FROM bot_plan_specs WHERE plan_key=?').get('a10').days,14);
 await f.cb(2,'buy');assert.ok(JSON.stringify(f.last().reply_markup).includes('buycat:1:0'));await f.cb(2,'buycat:1:0');let markup=JSON.stringify(f.last().reply_markup);assert.ok(markup.includes('plan_a10'));assert.ok(!markup.includes('plan_b20'));assert.ok(markup.includes('14 روز'));
 await f.cb(2,'plan_a10');await f.message(2,'Alice');const order=f.sql.prepare('SELECT * FROM orders').get();assert.equal(order.days,14);assert.equal(f.sql.prepare('SELECT group_ids FROM bot_order_routes WHERE order_id=?').get(order.id).group_ids,'1');
 await f.cb(1,'admin:category_rebind:1');await f.cb(1,'admin:categorygroup:2');assert.equal(f.sql.prepare('SELECT group_ids FROM bot_categories WHERE id=1').get().group_ids,'2');
 await f.message(2,'receipt');await f.cb(1,`approve_${order.id}`);assert.equal(f.sent.length,1);assert.equal(f.sent[0].get('id'),'3');assert.deepEqual(f.sent[0].getAll('groupIds'),['1']);const client=JSON.parse(f.sent[0].get('settings')).clients[0];assert.equal(client.renewalDuration,14*86400000);
});
test('test user chooses category and automatic inbound comes from that group',async()=>{
 const f=fixture();await category(f,'A',1);await category(f,'B',2);await f.cb(5,'test_service');assert.ok(JSON.stringify(f.last().reply_markup).includes('testcat:2:0'));await f.cb(5,'testcat:2:0');await f.cb(5,'testname:random');assert.equal(f.sent.length,1);assert.equal(f.sent[0].get('id'),'7');assert.deepEqual(f.sent[0].getAll('groupIds'),['2']);
});
test('invalidated group and foreign panel identity cannot issue or fulfill a new invoice',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'a10',100,10,14,1);f.setGroups([{id:2,name:'Beta',inboundIds:[7]}]);await f.cb(2,'plan_a10');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM orders').get().n,0);
 f.setGroups([{id:1,name:'Alpha',inboundIds:[3,4]}]);await f.cb(2,'plan_a10');await f.message(2,'Alice');const order=f.sql.prepare('SELECT id FROM orders').get();f.sql.exec("INSERT INTO settings(key,value) VALUES('panel_identity_v1','other')");await f.message(2,'receipt');await f.cb(1,`approve_${order.id}`);assert.equal(f.sent.length,0);assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=?').get(order.id).status,'pending');
});
test('renewal offers only plans matching the service group, volume and duration',async()=>{
 const f=fixture();await category(f,'A',1);await category(f,'B',2);await plan(f,'same',100,10,14,1);await plan(f,'wrongdays',150,10,30,1);await plan(f,'wronggroup',200,10,14,2);
 f.sql.prepare("INSERT INTO services(user_id,name,duration,panel_email,sub_id,volume_bytes) VALUES(2,'Alice','14 روزه','existing','old',?)").run(10*1024**3);
 await f.cb(2,'renew_1');const keys=JSON.stringify(f.last().reply_markup);assert.ok(keys.includes('rplan_same'));assert.ok(!keys.includes('rplan_wrongdays'));assert.ok(!keys.includes('rplan_wronggroup'));
 await f.cb(2,'rplan_wronggroup');await f.cb(2,'rplan_wrongdays');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM orders').get().n,0);
 await f.cb(2,'rplan_same');assert.equal(f.sql.prepare('SELECT days FROM orders').get().days,14);
});
test('renewal stops before panel write if group membership changes after invoice',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'same',100,10,14,1);
 f.sql.prepare("INSERT INTO services(user_id,name,duration,panel_email,sub_id,volume_bytes) VALUES(2,'Alice','14 روزه','existing','old',?)").run(10*1024**3);
 await f.cb(2,'renew_1');await f.cb(2,'rplan_same');const order=f.sql.prepare('SELECT id FROM orders').get();await f.message(2,'receipt');
 f.setGroups([{id:1,name:'Alpha',inboundIds:[3]}]);await f.cb(1,`approve_${order.id}`);
 assert.equal(f.sent.length,0);assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=?').get(order.id).status,'pending');
});
