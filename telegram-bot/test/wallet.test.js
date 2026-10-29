import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import worker from '../worker.js';
import {walletBalance,adjustWallet,reserveWalletPayment,completeWalletPayment,recoverWalletPayments} from '../wallet.js';

function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 sql.exec("INSERT INTO plans(key,gb,price,sort) VALUES('p10',10,70000,1); INSERT INTO bot_categories(id,name,group_ids) VALUES(1,'Test','1'); INSERT INTO bot_plan_specs VALUES('p10',1,30)");
 const prep=(q,a=[])=>({q,a,bind(...x){return prep(q,x)},async run(){const before=Number(sql.prepare('SELECT total_changes() AS n').get().n);const r=sql.prepare(q).run(...a);const changes=Number(sql.prepare('SELECT total_changes() AS n').get().n)-before;return {meta:{changes,last_row_id:Number(r.lastInsertRowid)}}},async all(){return {results:sql.prepare(q).all(...a),meta:{changes:0}}},async first(){return sql.prepare(q).get(...a)??null}});
 let queue=Promise.resolve(); const DB={prepare:q=>prep(q),batch(ss){const work=async()=>{sql.exec('BEGIN');try{let r=[];for(const s of ss){r.push(/^\s*select\b/i.test(s.q)?await s.all():await s.run())}sql.exec('COMMIT');return r}catch(e){sql.exec('ROLLBACK');throw e}};const next=queue.then(work);queue=next.catch(()=>{});return next}};
 const env={DB,BOT_TOKEN:'token',WEBHOOK_SECRET:'secret',ADMIN_ID:'1',PANEL_URL:'https://panel.example/base',PANEL_USER:'u',PANEL_PASS:'p',CARD_NUMBER:'1111',CARD_HOLDER:'Test'};
 sql.prepare("INSERT INTO settings(key,value) VALUES('panel_inbound_id','3'),('sub_base_url','https://sub.example/sub/')").run();
 let output=[],seq=0,mode='good',calls=0;
 globalThis.fetch=async(url,opt={})=>{let u=new URL(url),p=u.pathname;
  if(u.hostname==='api.telegram.org'){const method=p.split('/').at(-1),body=JSON.parse(opt.body);output.push({method,body});return new Response(JSON.stringify({ok:true,result:{message_id:8}}));}
  if(p.endsWith('/login'))return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':'vpn-ui=x'}});
  if(p.endsWith('/clients/groups')) {if(mode==='preflight')throw Error('connection lost before submit');return new Response(JSON.stringify({success:true,obj:[{id:1,inboundIds:[3]}]}));}
  if(p.endsWith('/clients/assignable'))return new Response(JSON.stringify({success:true,obj:[{inboundId:3,protocol:'vless'}]}));
  if(p.endsWith('/addClient')){calls++;if(mode==='ambiguous')throw Error('connection lost');return new Response(JSON.stringify({success:true}));}
  if(p.endsWith('/clients/list'))return new Response(JSON.stringify({success:true,obj:{rows:[{email:'old',subId:'sid',enable:true,memberships:[{inboundId:3}]}],total:1}}));
  if(p.endsWith('/renewClient/old')){calls++;if(mode==='ambiguous')throw Error('connection lost');return new Response(JSON.stringify({success:true}));}
  throw Error('unexpected endpoint '+p);
 };
 async function emit(data){let tasks=[];let req=new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'secret'},body:JSON.stringify({update_id:++seq,...data})});assert.equal((await worker.fetch(req,env,{waitUntil:p=>tasks.push(p)})).status,200);await Promise.all(tasks)}
 const message=(uid,text)=>emit({message:{from:{id:uid},chat:{id:uid,type:'private'},message_id:++seq,text}});
 const cb=(uid,data)=>emit({callback_query:{id:'cb'+ ++seq,from:{id:uid},message:{chat:{id:uid,type:'private'},message_id:1},data}});
 const order=(user=2,price=70,renew=null)=>{const r=sql.prepare("INSERT INTO orders(user_id,username,service_name,volume,price,final_price,duration,gb,days,status,renew_service_id) VALUES(?,'u','A','10 گیگ',?,?,'۱ ماهه (۳۰ روزه)',10,30,'draft',?)").run(user,price,price,renew);sql.prepare("INSERT INTO bot_order_routes VALUES(?,'1',3,'')").run(r.lastInsertRowid);return r};
 return {sql,env,out:output,message,cb,order,setMode(v){mode=v},get calls(){return calls}};
}

test('top-up receipt and duplicate admin approvals credit exactly once',async()=>{
 const f=fixture();await f.cb(2,'wallet:home');await f.cb(2,'wallet:topup');await f.message(2,'150');await f.message(2,'receipt');assert.equal(f.sql.prepare('SELECT status FROM wallet_topups').get().status,'pending');
 await f.cb(3,'topup:approve:1');assert.equal(await walletBalance(f.env,2),0);
 await Promise.all([f.cb(1,'topup:approve:1'),f.cb(1,'topup:approve:1')]);assert.equal(await walletBalance(f.env,2),150);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM wallet_entries').get().n,1);
});
test('admin configures top-up limits and customers can only request amounts within them',async()=>{
 const f=fixture();
 await f.cb(1,'admin:settings');assert.ok(JSON.stringify(f.out.findLast(x=>x.method==='sendMessage').body.reply_markup).includes('admin:setting:wallet_topup_min'));
 await f.cb(1,'admin:setting:wallet_topup_min');await f.message(1,'100');await f.cb(1,'admin:setting:wallet_topup_max');await f.message(1,'500');
 await f.cb(1,'admin:setting:wallet_topup_min');await f.message(1,'600');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='wallet_topup_min'").get().value,'100');
 await f.cb(2,'wallet:topup');assert.match(f.out.findLast(x=>x.method==='sendMessage').body.text,/100 تومان/);assert.match(f.out.findLast(x=>x.method==='sendMessage').body.text,/500 تومان/);
 await f.message(2,'99');await f.message(2,'501');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM wallet_topups').get().n,0);
 await f.message(2,'100');assert.equal(f.sql.prepare('SELECT amount FROM wallet_topups').get().amount,100);
 await f.cb(3,'wallet:topup');await f.message(3,'500');assert.equal(f.sql.prepare('SELECT amount FROM wallet_topups WHERE user_id=3').get().amount,500);
 await f.message(1,'/set wallet_topup_max 50');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='wallet_topup_max'").get().value,'500');
 await f.message(1,'/set wallet_topup_max 0');await f.cb(4,'wallet:topup');assert.match(f.out.findLast(x=>x.method==='sendMessage').body.text,/بدون سقف/);await f.message(4,'1000');assert.equal(f.sql.prepare('SELECT amount FROM wallet_topups WHERE user_id=4').get().amount,1000);
});
test('admin adjustments are confirmed, audited and cannot overdraw',async()=>{
 const f=fixture();await f.cb(1,'admin:wallet');await f.message(1,'2');await f.cb(1,'admin:wallet_add:2');await f.message(1,'100');assert.equal(await walletBalance(f.env,2),0);await f.cb(1,'admin:wallet_confirm');assert.equal(await walletBalance(f.env,2),100);await f.cb(1,'admin:wallet_confirm');assert.equal(await walletBalance(f.env,2),100);
 await f.cb(1,'admin:wallet_sub:2');await f.message(1,'101');await f.cb(1,'admin:wallet_confirm');assert.equal(await walletBalance(f.env,2),100);
 f.sql.prepare("INSERT INTO admins(user_id,added_at) VALUES(5,'x')").run();await f.cb(5,'admin:wallet_add:2');await f.message(5,'10');await f.cb(5,'admin:wallet_confirm');assert.equal(await walletBalance(f.env,2),110);
 await f.cb(6,'admin:wallet_add:2');await f.message(6,'10');await f.cb(6,'admin:wallet_confirm');assert.equal(await walletBalance(f.env,2),110);
});
test('wallet payment debits once and succeeds for new service',async()=>{
 const f=fixture();await adjustWallet(f.env,2,100,1,'seed');f.order();await Promise.all([f.cb(2,'wallet:pay:1'),f.cb(2,'wallet:pay:1')]);assert.equal(f.calls,1);assert.equal(await walletBalance(f.env,2),30);assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'approved');assert.equal(f.sql.prepare('SELECT state FROM wallet_payments').get().state,'settled');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);
});
test('insufficient wallet, duplicate order, other user, and receipt competition never debit',async()=>{
 const f=fixture();await adjustWallet(f.env,2,50,1,'seed');f.order();await f.cb(3,'wallet:pay:1');await f.cb(2,'wallet:pay:1');assert.equal(await walletBalance(f.env,2),50);assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'draft');assert.equal(f.calls,0);
 f.sql.prepare("UPDATE orders SET status='pending' WHERE id=1").run();await f.cb(2,'wallet:pay:1');assert.equal(await walletBalance(f.env,2),50);
});
test('unknown panel result refunds immediately and blocks duplicate fulfillment',async()=>{
 const f=fixture();await adjustWallet(f.env,2,100,1,'seed');f.order();f.setMode('ambiguous');await f.cb(2,'wallet:pay:1');assert.equal(await walletBalance(f.env,2),100);assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'needs_review_refunded');assert.equal(f.sql.prepare('SELECT state FROM wallet_payments').get().state,'refunded');assert.equal(f.calls,1);await f.cb(2,'wallet:pay:1');assert.equal(f.calls,1);assert.ok(f.out.some(x=>x.body.text?.includes('به کیف پول برگشت')));assert.ok(f.out.some(x=>x.body.chat_id===1&&x.body.text?.includes('نیاز به بررسی')));
});
test('wallet can renew an existing service with preserved identity',async()=>{
 const f=fixture();await adjustWallet(f.env,2,100,1,'seed');f.sql.prepare("INSERT INTO services(user_id,name,volume,duration,sub_link,panel_email,inbound_id,sub_id,volume_bytes) VALUES(2,'A','10 گیگ','۱ ماهه (۳۰ روزه)','https://sub.example/sub/sid','old',3,'sid',?)").run(10*1024**3);f.order(2,70,1);await f.cb(2,'wallet:pay:1');assert.equal(await walletBalance(f.env,2),30);assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'approved');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);
});
test('interrupted payment is refunded once by recovery and remains reviewable',async()=>{
 const f=fixture();await adjustWallet(f.env,2,100,1,'seed');f.order();const clock=Date.now;try {Date.now=()=>1000;assert.ok(await reserveWalletPayment(f.env,1,2));}finally{Date.now=clock;}
 assert.deepEqual(await recoverWalletPayments(f.env),[{orderId:1,userId:2}]);assert.deepEqual(await recoverWalletPayments(f.env),[]);assert.equal(await walletBalance(f.env,2),100);assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'needs_review_refunded');
});

test('refunded review is visible to admin and closes only with confirmation',async()=>{
 const f=fixture();await adjustWallet(f.env,2,100,1,'seed');f.order();f.setMode('ambiguous');await f.cb(2,'wallet:pay:1');
 await f.cb(2,'admin:wallet_reviews');assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'needs_review_refunded');
 await f.cb(1,'admin:wallet_reviews');assert.ok(f.out.some(x=>x.body.text?.includes('نیازمند بررسی')));await f.cb(1,'admin:wallet_review:1');await f.cb(1,'admin:wallet_close:1');assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'needs_review_refunded');await f.cb(1,'admin:wallet_close_yes:1');assert.equal(f.sql.prepare('SELECT status FROM orders').get().status,'wallet_review_resolved');assert.equal(await walletBalance(f.env,2),100);assert.equal(f.sql.prepare('SELECT actor_id FROM wallet_review_resolutions').get().actor_id,1);
});

test('topup rejection and immutable ledger do not change funds',async()=>{
 const f=fixture();await f.cb(2,'wallet:topup');await f.message(2,'50');await f.message(2,'receipt');await f.cb(1,'topup:reject:1');await f.cb(1,'topup:approve:1');assert.equal(await walletBalance(f.env,2),0);assert.equal(f.sql.prepare('SELECT status FROM wallet_topups').get().status,'rejected');
 await adjustWallet(f.env,2,10,1,'seed');assert.throws(()=>f.sql.prepare('UPDATE wallet_entries SET delta=999').run(),/WALLET_IMMUTABLE/);assert.throws(()=>f.sql.prepare('DELETE FROM wallet_entries').run(),/WALLET_IMMUTABLE/);assert.equal(await walletBalance(f.env,2),10);
});

test('failure before dispatch refunds and permits a new order',async()=>{
 const f=fixture();await adjustWallet(f.env,2,100,1,'seed');f.order();f.setMode('preflight');await f.cb(2,'wallet:pay:1');assert.equal(await walletBalance(f.env,2),100);assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,'wallet_failed_refunded');
 f.setMode('good');f.order();await f.cb(2,'wallet:pay:2');assert.equal(await walletBalance(f.env,2),30);assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=2').get().status,'approved');
});

test('uncertain refunded order blocks another wallet purchase until admin reviews it',async()=>{
 const f=fixture();await adjustWallet(f.env,2,100,1,'seed');f.order();f.setMode('ambiguous');await f.cb(2,'wallet:pay:1');f.setMode('good');f.order();await f.cb(2,'wallet:pay:2');assert.equal(f.calls,1);assert.equal(await walletBalance(f.env,2),100);assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=2').get().status,'draft');
 await f.cb(1,'admin:wallet_close_yes:1');await f.cb(2,'wallet:pay:2');assert.equal(f.calls,2);assert.equal(await walletBalance(f.env,2),30);assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=2').get().status,'approved');
});
test('customer buys through plan and invoice buttons using wallet balance',async()=>{
 const f=fixture();await adjustWallet(f.env,2,100000,1,'seed');await f.cb(2,'buy');await f.cb(2,'buycat:1:0');await f.cb(2,'plan_p10');await f.message(2,'Amir');
 assert.ok(f.out.some(x=>JSON.stringify(x.body.reply_markup||{}).includes('wallet:pay:1')));await f.cb(2,'wallet:pay:1');
 assert.equal(await walletBalance(f.env,2),30000);assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,'approved');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);
});
