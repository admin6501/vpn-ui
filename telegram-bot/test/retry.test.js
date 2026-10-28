import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import worker from '../worker.js';

function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 const prep=(q,a=[])=>({bind(...x){return prep(q,x)},async run(){let r=sql.prepare(q).run(...a);return {meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async all(){return {results:sql.prepare(q).all(...a)}},async first(){return sql.prepare(q).get(...a)??null}});
 const env={DB:{prepare:q=>prep(q),batch:async ss=>Promise.all(ss.map(s=>s.all()))},BOT_TOKEN:'t',WEBHOOK_SECRET:'secret',ADMIN_ID:'1',PANEL_URL:'https://panel.example/base',PANEL_USER:'u',PANEL_PASS:'p'};
 sql.prepare('INSERT INTO settings(key,value) VALUES(?,?)').run('panel_inbound_id','3');sql.prepare('INSERT INTO settings(key,value) VALUES(?,?)').run('sub_base_url','https://sub.example/sub/');
 let stage='good',calls=0,seq=0,output=[];
 globalThis.fetch=async(url,opt)=>{const p=new URL(url).pathname;
  if(p.startsWith('/bot')){const method=p.split('/').at(-1),body=JSON.parse(opt.body);output.push({method,body});if(stage==='receiptfail' && method==='copyMessage')return new Response(JSON.stringify({ok:false,description:'blocked'}));if(stage==='deliveryfail' && method==='sendMessage' && body.chat_id===2 && body.text?.includes('سرویس تست شما ساخته شد'))throw Error('telegram unavailable');return new Response(JSON.stringify({ok:true,result:{message_id:8}}));}
  if(p.endsWith('/login'))return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':'vpn-ui=test'}});
  if(p.endsWith('/clients/groups')) {if(stage==='preflight')throw Error('network down');return new Response(JSON.stringify({success:true,obj:[{id:1,inboundIds:[3]}]}));}
  if(p.endsWith('/clients/assignable'))return new Response(JSON.stringify({success:true,obj:[{inboundId:3,protocol:'vless'}]}));
  if(p.endsWith('/addClient')){calls++;if(stage==='ambiguous')throw Error('connection lost');if(stage==='rejected')return new Response(JSON.stringify({success:false,msg:'no capacity'}));return new Response(JSON.stringify({success:true}));}
  throw Error('unexpected '+p);
 };
 async function emit(data){const tasks=[];let req=new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'secret'},body:JSON.stringify({update_id:++seq,...data})});assert.equal((await worker.fetch(req,env,{waitUntil:p=>tasks.push(p)})).status,200);await Promise.all(tasks)}
 const message=(uid,text)=>emit({message:{from:{id:uid,first_name:'A'},chat:{id:uid,type:'private'},message_id:6,text}});
 const cb=(uid,data)=>emit({callback_query:{id:'cb'+ ++seq,from:{id:uid},message:{chat:{id:uid,type:'private'},message_id:6},data}});
 return {sql,cb,message,output,setStage(v){stage=v},get calls(){return calls}};
}

test('trial preflight failure releases the claim and next attempt can succeed exactly once',async()=>{
 const f=fixture();f.setStage('preflight');await f.cb(2,'testname:random');assert.equal(f.calls,0);assert.equal(f.sql.prepare('SELECT * FROM test_claims').get(),undefined);
 f.setStage('good');await f.cb(2,'testname:random');assert.equal(f.calls,1);assert.match(f.sql.prepare('SELECT created_at FROM test_claims').get().created_at,/^done\|/);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);
 await f.cb(2,'testname:random');assert.equal(f.calls,1);
});

test('unknown and explicit rejection after dispatch stay reviewable, never auto-create twice',async()=>{
 for(const mode of ['ambiguous','rejected']){const f=fixture();f.setStage(mode);await f.cb(2,'testname:random');assert.equal(f.calls,1);assert.match(f.sql.prepare('SELECT created_at FROM test_claims').get().created_at,/^review\|/);await f.cb(2,'testname:random');assert.equal(f.calls,1);assert.ok(f.output.some(x=>x.body.text?.includes('نیاز به بررسی')));}
});

test('telegram notification failure keeps created trial and cannot cause duplicate',async()=>{
 const f=fixture();f.setStage('deliveryfail');await f.cb(2,'testname:random');assert.equal(f.calls,1);assert.match(f.sql.prepare('SELECT created_at FROM test_claims').get().created_at,/^done\|/);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);f.setStage('good');await f.cb(2,'testname:random');assert.equal(f.calls,1);
});

test('paid order preflight error remains retryable but post-dispatch error requires review',async()=>{
 for(const [mode,want] of [['preflight','pending'],['ambiguous','needs_review']]){const f=fixture();f.sql.prepare("INSERT INTO orders(user_id,username,service_name,volume,price,final_price,duration,gb,days,status) VALUES(2,'u','A','10 گیگ',10,10,'۱ ماهه (۳۰ روزه)',10,30,'pending')").run();
  f.setStage(mode);await f.cb(1,'approve_1');assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,want);
  f.setStage('good');await f.cb(1,'approve_1');assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,want==='pending'?'approved':'needs_review');assert.equal(f.calls,1);
 }
});

test('receipt delivery failure is not falsely acknowledged and resending works',async()=>{
 const f=fixture();f.sql.prepare("INSERT INTO orders(user_id,username,service_name,volume,price,final_price,duration,gb,days,status) VALUES(2,'u','A','10 گیگ',10,10,'30 روزه',10,30,'draft')").run();
 f.sql.prepare('INSERT INTO states(user_id,data) VALUES(?,?)').run(2,JSON.stringify({step:'receipt',order_id:1}));
 f.setStage('receiptfail');await f.message(2,'receipt');assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,'needs_fix');assert.ok(f.output.some(x=>x.body.text?.includes('ارسال فیش به ادمین ناموفق')));
 f.setStage('good');await f.message(2,'receipt');assert.equal(f.sql.prepare('SELECT status FROM orders WHERE id=1').get().status,'pending');assert.ok(f.output.some(x=>x.body.text?.includes('فیش شما دریافت شد')));
});

test('concurrent trial callbacks allow only one create',async()=>{
 const f=fixture();await Promise.all([f.cb(2,'testname:random'),f.cb(2,'testname:random')]);assert.equal(f.calls,1);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);
});

test('trial asks for custom or random name and uses the selected name',async()=>{
 const f=fixture();await f.cb(2,'test_service');assert.ok(JSON.stringify(f.output).includes('testname:custom'));assert.ok(JSON.stringify(f.output).includes('testname:random'));assert.equal(f.calls,0);
 await f.cb(2,'testname:custom');await f.message(2,'invalid 22');assert.equal(f.calls,0);await f.message(2,'MyTest');assert.equal(f.calls,0);await f.cb(2,'testname:make');assert.equal(f.calls,1);assert.equal(f.sql.prepare('SELECT name FROM services WHERE user_id=2').get().name,'MyTest');
});
