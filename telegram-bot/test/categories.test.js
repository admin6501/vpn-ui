import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import worker from '../worker.js';
function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 const prep=(q,a=[])=>({q,a,bind(...x){return prep(q,x)},execute(){const stmt=sql.prepare(q);if(stmt.columns().length)return {results:stmt.all(...a),meta:{changes:0}};const r=stmt.run(...a);return {results:[],meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async run(){return this.execute()},async all(){return this.execute()},async first(){return sql.prepare(q).get(...a)??null}});
 const DB={prepare:q=>prep(q),async batch(ss){sql.exec('BEGIN');try{const result=ss.map(s=>s.execute());sql.exec('COMMIT');return result}catch(e){sql.exec('ROLLBACK');throw e}}};
 sql.exec("INSERT INTO settings(key,value) VALUES('sub_base_url','https://sub.example/sub/'),('test_enabled','1')");let output=[],sent=[],seq=0,groups=[{id:1,name:'Alpha',inboundIds:[3,4]},{id:2,name:'Beta',inboundIds:[7]}], accounts=[{email:'existing',subId:'old',enable:true,totalGB:10*1024**3,expiryTime:Date.now()+86400000,up:100,down:200,memberships:[{inboundId:3,protocol:'vless',clientId:'uuid-existing'},{inboundId:4,protocol:'shadowsocks',clientId:'existing'}]}];
 globalThis.fetch=async(url,opt={})=>{const u=new URL(url),p=u.pathname;
  if(u.hostname==='api.telegram.org'){const method=p.split('/').at(-1),body=JSON.parse(opt.body);output.push({method,body});return Response.json({ok:true,result:{message_id:3}})}
  if(p.endsWith('/login'))return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':'vpn-ui=test'}});
  if(p.endsWith('/resellerBalance'))return Response.json({success:true,obj:{isReseller:false}});
  if(p.endsWith('/clients/groups'))return Response.json({success:true,obj:groups});
  if(p.endsWith('/clients/assignable'))return Response.json({success:true,obj:[{inboundId:3,protocol:'vless'},{inboundId:4,protocol:'shadowsocks',method:'aes-128-gcm'},{inboundId:7,protocol:'trojan'}]});
  if(p.endsWith('/clients/list'))return Response.json({success:true,obj:{rows:accounts,total:accounts.length}});
  if(p.endsWith('/clients/inbounds'))return Response.json({success:true,obj:[3,4,7].map(id=>({id,protocol:id===3?'vless':id===4?'shadowsocks':'trojan',settings:JSON.stringify({clients:accounts.filter(a=>a.memberships.some(m=>m.inboundId===id)).map(a=>({email:a.email,id:'uuid-existing',password:'secret',subId:a.subId,enable:a.enable,totalGB:a.totalGB,expiryTime:a.expiryTime}))})}))});
  if(p.endsWith('/multiDeleteCapability'))return Response.json({success:true,obj:{preservesSharedTraffic:true}});
  if(p.includes('/delClientByEmail/')){const id=Number(p.match(/inbounds\/(\d+)/)[1]);accounts=accounts.map(a=>({...a,memberships:a.memberships.filter(m=>m.inboundId!==id)})).filter(a=>a.memberships.length);return Response.json({success:true})}
  if(p.endsWith('/addClient')){sent.push(new URLSearchParams(opt.body));return Response.json({success:true})}
  if(p.includes('/updateClient/')){const c=JSON.parse(new URLSearchParams(opt.body).get('settings')).clients[0],a=accounts.find(x=>x.email===c.email);Object.assign(a,{totalGB:c.totalGB,expiryTime:c.expiryTime,enable:c.enable});sent.push('renew-existing');return Response.json({success:true})}
  throw Error('unexpected '+p);
 };
 const env={DB,BOT_TOKEN:'token',WEBHOOK_SECRET:'secret',ADMIN_ID:'1',PANEL_URL:'https://panel.example/base',PANEL_USER:'rep',PANEL_PASS:'pass',CARD_NUMBER:'123',CARD_HOLDER:'owner'};
 async function emit(data){let pending=[];const req=new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'secret'},body:JSON.stringify({update_id:++seq,...data})});assert.equal((await worker.fetch(req,env,{waitUntil:p=>pending.push(p)})).status,200);await Promise.all(pending)}
 const message=(id,text)=>emit({message:{from:{id,first_name:'A'},chat:{id,type:'private'},message_id:++seq,text}});
 const cb=(id,data)=>emit({callback_query:{id:'cb'+ ++seq,from:{id,first_name:'A'},message:{chat:{id,type:'private'},message_id:1},data}});
 const photo=id=>emit({message:{from:{id,first_name:'A'},chat:{id,type:'private'},message_id:++seq,photo:[{file_id:'receipt'}]}});
 const last=()=>output.filter(x=>x.method==='sendMessage').at(-1)?.body;
 return {sql,output,sent,message,photo,cb,last,accounts:()=>accounts,setGroups(g){groups=g},setAccounts(a){accounts=a}};
}
async function category(f,name,group){await f.cb(1,'admin:category_add');await f.message(1,name);await f.cb(1,`admin:categorygroup:${group}`)}
async function plan(f,key,price,gb,days,categoryId){await f.cb(1,'admin:plan_add');await f.message(1,`${key} ${price} ${gb} ${days}`);await f.cb(1,`admin:planpick:${categoryId}`)}
test('checkout displays card details only after card payment selection and accepts a photo',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'a10',100,10,14,1);await f.cb(2,'plan_a10');await f.message(2,'Alice');
 assert.ok(JSON.stringify(f.last().reply_markup).includes('card:pay:1'));assert.ok(JSON.stringify(f.last().reply_markup).includes('wallet:pay:1'));assert.ok(!f.last().text.includes('شماره کارت'));
 await f.message(2,'receipt');assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'draft');await f.cb(2,'card:pay:1');assert.match(f.last().text,/123/);assert.match(f.last().text,/100 تومان/);await f.message(2,'text');assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'draft');await f.photo(2);assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'pending');
});
test('duplicate chosen names are rejected before ordering and the submitted name reaches the panel unchanged',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'a10',100,10,14,1);await f.cb(2,'plan_a10');await f.message(2,'Existing');assert.match(f.last().text,/نام دیگری/);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM orders').get().n,0);
 await f.message(2,'Alice');await f.cb(2,'card:pay:1');await f.photo(2);await f.cb(1,'approve_1');assert.equal(JSON.parse(f.sent[0].get('settings')).clients[0].email,'Alice');
});
test('admin deletes the confirmed multi-inbound service and hides its linked orders while preserving accounting',async()=>{
 const f=fixture();f.sql.exec("INSERT INTO services(id,user_id,name,panel_email,sub_id) VALUES(1,2,'Alice','existing','old'); INSERT INTO orders(id,user_id,service_name,status) VALUES(1,2,'Alice','approved'); INSERT INTO orders(id,user_id,service_name,status,renew_service_id) VALUES(2,2,'Alice','approved',1); INSERT INTO provision_attempts VALUES('order:1:abc','existing','old',3,'today')");
 await f.cb(2,'admin:svcdeleteconfirm:1');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);
 await f.cb(1,'admin:svc:1');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:svcdelete:1'));await f.cb(1,'admin:svcdelete:1');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);
 await f.cb(1,'admin:svcdeleteconfirm:1');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,0);assert.equal(f.sql.prepare("SELECT COUNT(*) n FROM settings WHERE key LIKE 'hidden_order:%'").get().n,2);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM orders').get().n,2);
});
test('category and plan control customer selection and preserve order group after later edits',async()=>{
 const f=fixture();await category(f,'A',1);await category(f,'B',2);await plan(f,'a10',100,10,14,1);await plan(f,'b20',200,20,40,2);
 assert.equal(f.sql.prepare('SELECT days FROM bot_plan_specs WHERE plan_key=?').get('a10').days,14);
 await f.cb(2,'buy');assert.ok(JSON.stringify(f.last().reply_markup).includes('buycat:1:0'));await f.cb(2,'buycat:1:0');let markup=JSON.stringify(f.last().reply_markup);assert.ok(markup.includes('plan_a10'));assert.ok(!markup.includes('plan_b20'));assert.ok(markup.includes('14 روز'));
 await f.cb(2,'plan_a10');await f.message(2,'Alice');const order=f.sql.prepare('SELECT * FROM orders').get();assert.equal(order.days,14);assert.equal(f.sql.prepare('SELECT group_ids FROM bot_order_routes WHERE order_id=?').get(order.id).group_ids,'1');
 await f.cb(1,'admin:category_rebind:1');await f.cb(1,'admin:categorygroup:2');assert.equal(f.sql.prepare('SELECT group_ids FROM bot_categories WHERE id=1').get().group_ids,'2');
 await f.cb(2,'card:pay:1');await f.photo(2);await f.cb(1,`approve_${order.id}`);assert.equal(f.sent.length,1);assert.equal(f.sent[0].get('id'),'3');assert.deepEqual(f.sent[0].getAll('groupIds'),['1']);const client=JSON.parse(f.sent[0].get('settings')).clients[0];assert.equal(client.renewalDuration,14*86400000);
});
test('admin selects one trial category; stale buttons cannot issue a trial on another group',async()=>{
 const f=fixture();await category(f,'A',1);await category(f,'B',2);
 await f.cb(5,'test_service');assert.match(f.last().text,/آماده نیست/);
 await f.cb(1,'admin:settings');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:testcategories:0'));
 await f.cb(1,'admin:testcategories:0');await f.cb(1,'admin:testcategory:2');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='test_category_id'").get().value,'2');
 await f.cb(5,'test_service');const markup=JSON.stringify(f.last().reply_markup);assert.ok(markup.includes('testcat:2:0'));assert.ok(!markup.includes('testcat:1:0'));
 await f.cb(5,'testcat:1:0');await f.cb(5,'testname:random');assert.equal(f.sent.length,0);
 await f.cb(5,'testcat:2:0');await f.cb(1,'admin:testcategory:1');await f.cb(5,'testname:random');assert.equal(f.sent.length,0);
 await f.cb(1,'admin:testcategory:2');await f.cb(5,'testcat:2:0');await f.cb(5,'testname:random');assert.equal(f.sent.length,1);assert.equal(f.sent[0].get('id'),'7');assert.deepEqual(f.sent[0].getAll('groupIds'),['2']);
});
test('invalidated group and foreign panel identity cannot issue or fulfill a new invoice',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'a10',100,10,14,1);f.setGroups([{id:2,name:'Beta',inboundIds:[7]}]);await f.cb(2,'plan_a10');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM orders').get().n,0);
 f.setGroups([{id:1,name:'Alpha',inboundIds:[3,4]}]);await f.cb(2,'plan_a10');await f.message(2,'Alice');const order=f.sql.prepare('SELECT id FROM orders').get();f.sql.exec("INSERT INTO settings(key,value) VALUES('panel_identity_v1','other')");await f.cb(2,'card:pay:1');await f.photo(2);await f.cb(1,`approve_${order.id}`);assert.equal(f.sent.length,0);assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=?').get(order.id).status,'pending');
});
test('renewal offers all plans of the service group regardless of prior package',async()=>{
 const f=fixture();await category(f,'A',1);await category(f,'B',2);await plan(f,'same',100,10,14,1);await plan(f,'wrongdays',150,10,30,1);await plan(f,'wronggroup',200,10,14,2);
 f.sql.prepare("INSERT INTO services(user_id,name,duration,panel_email,sub_id,volume_bytes) VALUES(2,'Alice','14 روزه','existing','old',?)").run(10*1024**3);
 await f.cb(2,'renew_1');const keys=JSON.stringify(f.last().reply_markup);assert.ok(keys.includes('rplan_same'));assert.ok(keys.includes('rplan_wrongdays'));assert.ok(!keys.includes('rplan_wronggroup'));
 await f.cb(2,'rplan_wronggroup');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM orders').get().n,0);
 await f.cb(2,'rplan_wrongdays');assert.equal(f.sql.prepare('SELECT days FROM orders').get().days,30);
});
test('card and wallet renewals add a different package without losing remaining quota or time',async()=>{
 for(const payment of ['card','wallet']){
  const f=fixture();await category(f,'A',1);await plan(f,'small',70,5,7,1);
  f.sql.prepare("INSERT INTO services(user_id,name,duration,panel_email,sub_id,volume_bytes) VALUES(2,'Alice','30 روزه','existing','old',?)").run(10*1024**3);
  const original=f.accounts()[0],expiry=original.expiryTime,usage=original.up+original.down;
  await f.cb(2,'renew_1');await f.cb(2,'rplan_small');
  if(payment==='card'){await f.cb(2,'card:pay:1');await f.photo(2);await f.cb(1,'approve_1')}
  else {f.sql.exec("INSERT INTO wallet_entries(user_id,delta,reference,reason,created_at) VALUES(2,100,'seed','admin_adjustment','now')");await f.cb(2,'wallet:pay:1')}
  const after=f.accounts()[0];assert.equal(after.totalGB,15*1024**3);assert.equal(after.expiryTime,expiry+7*86400000);assert.equal(after.up+after.down,usage);assert.equal(after.subId,'old');assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'approved');
 }
});
test('renewal keeps the assigned category when multiple categories share a group',async()=>{
 const f=fixture();await category(f,'First',1);await category(f,'Second',1);await plan(f,'first',100,10,30,1);await plan(f,'second',70,5,7,2);
 f.sql.prepare("INSERT INTO services(id,user_id,name,duration,panel_email,sub_id,volume_bytes) VALUES(1,2,'Alice','30 روزه','existing','old',?)").run(10*1024**3);
 f.sql.exec("INSERT INTO settings(key,value) VALUES('service_category:1','2')");await f.cb(2,'renew_1');const keys=JSON.stringify(f.last().reply_markup);assert.ok(keys.includes('rplan_second'));assert.ok(!keys.includes('rplan_first'));
 await f.cb(2,'renewcat:1:1');await f.cb(2,'rplan_first');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM orders').get().n,0);
 f.sql.exec("DELETE FROM settings WHERE key='service_category:1'");await f.cb(2,'renew_1');assert.ok(JSON.stringify(f.last().reply_markup).includes('renewcat:1:2'));await f.cb(2,'renewcat:1:2');assert.ok(JSON.stringify(f.last().reply_markup).includes('rplan_second'));
});
test('renewal invoice is not applied if the service category changes before approval',async()=>{
 const f=fixture();await category(f,'First',1);await category(f,'Second',1);await plan(f,'first',100,10,30,1);await plan(f,'second',70,5,7,2);
 f.sql.prepare("INSERT INTO services(id,user_id,name,duration,panel_email,sub_id,volume_bytes) VALUES(1,2,'Alice','30 روزه','existing','old',?)").run(10*1024**3);
 f.sql.exec("INSERT INTO settings(key,value) VALUES('service_category:1','1')");await f.cb(2,'renew_1');await f.cb(2,'rplan_first');await f.cb(2,'card:pay:1');await f.photo(2);
 f.sql.exec("UPDATE settings SET value='2' WHERE key='service_category:1'");await f.cb(1,'approve_1');
 assert.equal(f.sent.length,0);assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'pending');assert.equal(f.accounts()[0].totalGB,10*1024**3);
});
test('renewal stops before panel write if group membership changes after invoice',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'same',100,10,14,1);
 f.sql.prepare("INSERT INTO services(user_id,name,duration,panel_email,sub_id,volume_bytes) VALUES(2,'Alice','14 روزه','existing','old',?)").run(10*1024**3);
 await f.cb(2,'renew_1');await f.cb(2,'rplan_same');const order=f.sql.prepare('SELECT id FROM orders').get();await f.cb(2,'card:pay:1');await f.photo(2);
 f.setGroups([{id:1,name:'Alpha',inboundIds:[3]}]);await f.cb(1,`approve_${order.id}`);
 assert.equal(f.sent.length,0);assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=?').get(order.id).status,'pending');
});

test('admin changes only plan category and leaves existing invoice route untouched',async()=>{
 const f=fixture();await category(f,'A',1);await category(f,'B',2);await plan(f,'a10',100,10,14,1);
 await f.cb(2,'plan_a10');await f.message(2,'Alice');const order=f.sql.prepare('SELECT id FROM orders').get();
 await f.cb(1,'admin:plan:a10');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:planmove:a10'));
 await f.cb(2,'admin:planmove:a10');await f.cb(2,'admin:planpick:2');assert.equal(f.sql.prepare("SELECT category_id FROM bot_plan_specs WHERE plan_key='a10'").get().category_id,1);
 await f.cb(1,'admin:planmove:a10');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:planpick:2'));
 f.sql.exec("UPDATE plans SET price=150,gb=12 WHERE key='a10'; UPDATE bot_plan_specs SET days=21 WHERE plan_key='a10'");
 await f.cb(1,'admin:planpick:2');assert.deepEqual({...f.sql.prepare("SELECT p.price,p.gb,s.days,s.category_id FROM plans p JOIN bot_plan_specs s ON s.plan_key=p.key WHERE p.key='a10'").get()},{price:150,gb:12,days:21,category_id:2});
 assert.equal(f.sql.prepare('SELECT group_ids FROM bot_order_routes WHERE order_id=?').get(order.id).group_ids,'1');
 await f.cb(1,'admin:planpick:1');assert.equal(f.sql.prepare("SELECT category_id FROM bot_plan_specs WHERE plan_key='a10'").get().category_id,2);
});

test('admin creates coupon and customer applies it to wallet purchase',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'a10',100,10,14,1);
 await f.cb(1,'admin:coupon_new');await f.cb(1,'admin:coupon_type:percent');await f.message(1,'SAVE25 25 1 1 30');assert.equal(f.sql.prepare("SELECT value FROM bot_coupons WHERE code='SAVE25'").get().value,25);
 await f.cb(2,'plan_a10');await f.message(2,'Alice');await f.cb(2,'coupon:enter:1');await f.message(2,'save25');assert.deepEqual({...f.sql.prepare('SELECT discount,final_price FROM orders WHERE id=1').get()},{discount:25,final_price:75});
 f.sql.exec("INSERT INTO wallet_entries(user_id,delta,reference,reason,created_at) VALUES(2,100,'testcredit','admin_adjustment','now')");await f.cb(2,'wallet:pay:1');assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,'approved');assert.equal(f.sql.prepare('SELECT state FROM bot_coupon_uses').get().state,'consumed');assert.equal(f.sql.prepare('SELECT SUM(delta) n FROM wallet_entries WHERE user_id=2').get().n,25);
});
test('admin rejection of a paid receipt releases the coupon without claiming money was refunded',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'a10',100,10,14,1);await f.cb(1,'admin:coupon_new');await f.cb(1,'admin:coupon_type:fixed');await f.message(1,'OFF10 10 1 0 0');
 await f.cb(2,'plan_a10');await f.message(2,'Alice');await f.cb(2,'coupon:enter:1');await f.message(2,'OFF10');await f.cb(2,'card:pay:1');await f.photo(2);assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,'pending');
 await f.cb(1,'reject_order:1');assert.equal(f.sql.prepare('SELECT state FROM bot_coupon_uses').get().state,'reserved');await f.cb(1,'reject_order_yes:1');assert.equal(f.sql.prepare('SELECT state FROM bot_coupon_uses').get().state,'released');assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,'rejected');assert.ok(f.output.some(x=>x.body.text?.includes('وضعیت وجه را جداگانه بررسی')));
 await f.cb(3,'plan_a10');await f.message(3,'Bob');await f.cb(3,'coupon:enter:2');await f.message(3,'OFF10');assert.equal(f.sql.prepare('SELECT final_price FROM orders WHERE id=2').get().final_price,90);
});
test('expired discounted receipt is sent for manual review and never fulfills automatically',async()=>{
 const f=fixture(),real=Date.now;try{let now=1700000000000;Date.now=()=>now;await category(f,'A',1);await plan(f,'a10',100,10,14,1);await f.cb(1,'admin:coupon_new');await f.cb(1,'admin:coupon_type:fixed');await f.message(1,'OFF10 10 1 1 0');
 f.sql.exec("INSERT INTO discounts(user_id,amount,used,created_at) VALUES(2,40,0,'today')");await f.cb(2,'plan_a10');await f.message(2,'Alice');await f.cb(2,'coupon:enter:1');await f.message(2,'OFF10');await f.cb(2,'card:pay:1');now+=31*60*1000;await f.photo(2);assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,'cancelled');assert.equal(f.sql.prepare('SELECT used FROM discounts').get().used,0);assert.equal(f.sql.prepare('SELECT state FROM bot_coupon_uses').get().state,'released');assert.equal(f.sent.length,0);assert.ok(f.output.some(x=>x.body.text?.includes('فیش برای بررسی به مدیریت ارسال شد')));assert.ok(f.output.some(x=>x.body.chat_id===1&&x.body.text?.includes('مبلغ فاکتور قبلی: 50 تومان')));
 }finally{Date.now=real}
});
test('coupon management supports limits, unlimited and disabling and deleting without erasing history',async()=>{
 const f=fixture();await f.cb(1,'admin:coupon_type:fixed');await f.message(1,'CODE50 50 0 0 0');let row=f.sql.prepare("SELECT * FROM bot_coupons WHERE code='CODE50'").get();assert.equal(row.total_limit,null);assert.equal(row.user_limit,null);assert.equal(row.expires_ms,null);
 await f.cb(1,`admin:coupon_toggle:${row.id}`);assert.equal(f.sql.prepare('SELECT enabled FROM bot_coupons WHERE id=?').get(row.id).enabled,0);await f.cb(1,`admin:coupon_toggle:${row.id}`);await f.cb(1,`admin:coupon_delete:${row.id}`);assert.equal(f.sql.prepare('SELECT deleted FROM bot_coupons WHERE id=?').get(row.id).deleted,0);await f.cb(1,`admin:coupon_delete_yes:${row.id}`);assert.equal(f.sql.prepare('SELECT deleted FROM bot_coupons WHERE id=?').get(row.id).deleted,1);
});

test('discount code applies to matching service renewal and is consumed after fulfillment',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'same',100,10,14,1);await f.cb(1,'admin:coupon_type:fixed');await f.message(1,'RENEW10 10 0 1 30');
 f.sql.prepare("INSERT INTO services(user_id,name,duration,panel_email,sub_id,volume_bytes) VALUES(2,'Alice','14 روزه','existing','old',?)").run(10*1024**3);
 await f.cb(2,'renew_1');await f.cb(2,'rplan_same');await f.cb(2,'coupon:enter:1');await f.message(2,'RENEW10');assert.equal(f.sql.prepare('SELECT final_price FROM orders').get().final_price,90);
 f.sql.exec("INSERT INTO wallet_entries(user_id,delta,reference,reason,created_at) VALUES(2,100,'testcredit','admin_adjustment','now')");await f.cb(2,'wallet:pay:1');assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'approved');assert.equal(f.sql.prepare('SELECT state FROM bot_coupon_uses').get().state,'consumed');assert.ok(f.sent.includes('renew-existing'));
});

test('manual receipt approval consumes the coupon and another user cannot apply it to someone else order',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'a10',100,10,14,1);await f.cb(1,'admin:coupon_type:percent');await f.message(1,'SALE20 20 0 1 0');
 await f.cb(2,'plan_a10');await f.message(2,'Alice');await f.cb(3,'coupon:enter:1');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM bot_coupon_uses').get().n,0);
 await f.cb(2,'coupon:enter:1');await f.message(2,'SALE20');await f.cb(2,'card:pay:1');await f.photo(2);await f.cb(1,'approve_1');assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'approved');assert.equal(f.sql.prepare('SELECT state FROM bot_coupon_uses').get().state,'consumed');
});

test('referral gift and coupon stack on remaining amount and both release on cancellation',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'a10',100,10,14,1);await f.cb(1,'admin:coupon_type:percent');await f.message(1,'SAVE25 25 1 1 0');f.sql.exec("INSERT INTO discounts(user_id,amount,used,created_at) VALUES(2,40,0,'today')");
 await f.cb(2,'plan_a10');await f.message(2,'Alice');assert.deepEqual({...f.sql.prepare('SELECT discount,final_price FROM orders WHERE id=1').get()},{discount:40,final_price:60});
 await f.cb(2,'coupon:enter:1');await f.message(2,'SAVE25');assert.deepEqual({...f.sql.prepare('SELECT discount,final_price FROM orders WHERE id=1').get()},{discount:55,final_price:45});
 await f.cb(2,'main_menu');assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,'cancelled');assert.equal(f.sql.prepare('SELECT used FROM discounts').get().used,0);assert.equal(f.sql.prepare('SELECT state FROM bot_coupon_uses WHERE order_id=1').get().state,'released');
 await f.cb(2,'plan_a10');await f.message(2,'Alice');await f.cb(2,'coupon:enter:2');await f.message(2,'SAVE25');f.sql.exec("INSERT INTO wallet_entries(user_id,delta,reference,reason,created_at) VALUES(2,100,'refcredit','admin_adjustment','now')");await f.cb(2,'wallet:pay:2');
 assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=2').get().status,'approved');assert.equal(f.sql.prepare('SELECT state FROM bot_order_referrals WHERE order_id=2').get().state,'consumed');assert.equal(f.sql.prepare('SELECT state FROM bot_coupon_uses WHERE order_id=2').get().state,'consumed');assert.equal(f.sql.prepare('SELECT SUM(delta) n FROM wallet_entries WHERE user_id=2').get().n,55);
});
test('failed wallet fulfillment refunds balance and releases both referral and code',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'a10',100,10,14,1);await f.cb(1,'admin:coupon_type:fixed');await f.message(1,'SAVE10 10 1 1 0');f.sql.exec("INSERT INTO discounts(user_id,amount,used,created_at) VALUES(2,40,0,'today'); INSERT INTO wallet_entries(user_id,delta,reference,reason,created_at) VALUES(2,100,'failcredit','admin_adjustment','now')");
 await f.cb(2,'plan_a10');await f.message(2,'Alice');await f.cb(2,'coupon:enter:1');await f.message(2,'SAVE10');f.setGroups([]);await f.cb(2,'wallet:pay:1');
 assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,'wallet_failed_refunded');assert.equal(f.sql.prepare('SELECT used FROM discounts').get().used,0);assert.equal(f.sql.prepare('SELECT state FROM bot_coupon_uses').get().state,'released');assert.equal(f.sql.prepare('SELECT SUM(delta) n FROM wallet_entries WHERE user_id=2').get().n,100);
});

test('concurrent invoice creation reserves a referral reward at most once',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'a10',100,10,14,1);f.sql.exec("INSERT INTO discounts(user_id,amount,used,created_at) VALUES(2,40,0,'today')");await f.cb(2,'plan_a10');await Promise.all([f.message(2,'Alice'),f.message(2,'Bob')]);
 assert.equal(f.sql.prepare("SELECT COUNT(*) n FROM bot_order_referrals WHERE state='reserved'").get().n,1);assert.equal(f.sql.prepare('SELECT SUM(discount) n FROM orders').get().n,40);assert.equal(f.sql.prepare('SELECT used FROM discounts').get().used,1);
});
test('concurrent random name selections create at most one invoice',async()=>{
 const f=fixture();await category(f,'A',1);await plan(f,'a10',100,10,14,1);await f.cb(2,'plan_a10');await Promise.all([f.cb(2,'buyname:random'),f.cb(2,'buyname:random')]);
 assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM orders WHERE user_id=2').get().n,1);
});
