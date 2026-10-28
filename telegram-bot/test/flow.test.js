import test from 'node:test';
import assert from 'node:assert/strict';
import { DatabaseSync } from 'node:sqlite';
import { readFileSync } from 'node:fs';
import worker from '../worker.js';

test('full purchase and duplicate approval produce one panel user', async()=>{
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 sql.exec("INSERT INTO plans(key,gb,price,sort) VALUES('p10',10,70000,1)");
 const prep=(query,args=[])=>({bind(...xs){return prep(query,xs)},async run(){const r=sql.prepare(query).run(...args);return {meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async all(){return {results:sql.prepare(query).all(...args)}},async first(){return sql.prepare(query).get(...args) ?? null}});
 const DB={prepare:q=>prep(q),batch:async stmts=>Promise.all(stmts.map(s=>s.all()))};
 sql.prepare('INSERT INTO settings(key,value) VALUES(?,?)').run('panel_inbound_id','3');
 sql.prepare('INSERT INTO settings(key,value) VALUES(?,?)').run('sub_base_url','https://sub.example/sub/');
 const out=[];let creates=0;
 globalThis.fetch=async(url,opt)=>{
  const p=new URL(url).pathname;
  if(p.startsWith('/bot')) {const method=p.split('/').at(-1);const body=JSON.parse(opt.body);out.push({method,body});return new Response(JSON.stringify(method==='getChatMember'?{ok:true,result:{status:'member'}}:{ok:true,result:{message_id:3}}));}
  if(p.endsWith('/login'))return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':'vpn-ui=example'}});
  if(p.endsWith('/clients/groups'))return new Response(JSON.stringify({success:true,obj:[{id:1,inboundIds:[3,4]}]}));
  if(p.endsWith('/clients/assignable'))return new Response(JSON.stringify({success:true,obj:[{inboundId:3,protocol:'vless'},{inboundId:4,protocol:'shadowsocks',method:'aes-128-gcm'}]}));
  if(p.endsWith('/addClient')) {creates++;return new Response(JSON.stringify({success:true}));}
  throw Error('Unexpected route '+p);
 };
 const env={DB,BOT_TOKEN:'token',WEBHOOK_SECRET:'secret',ADMIN_ID:'1',PANEL_URL:'https://panel.example/base',PANEL_USER:'reseller',PANEL_PASS:'password',CHANNEL_USERNAME:'@channel'};
 let id=0; async function update(data){let tasks=[];const req=new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'secret'},body:JSON.stringify({update_id:++id,...data})});assert.equal((await worker.fetch(req,env,{waitUntil:p=>tasks.push(p)})).status,200);await Promise.all(tasks)}
 const message=text=>({message:{from:{id:2,first_name:'Client'},chat:{id:2,type:'private'},message_id:++id,text}});
 const cb=(data,uid=2)=>({callback_query:{id:`cb${++id}`,from:{id:uid},message:{chat:{id:uid,type:'private'},message_id:33},data}});
 await update(message('/start'));await update(cb('buy'));await update(cb('plan_p10'));await update(message('Amir'));await update(message('receipt'));
 const order=sql.prepare('SELECT * FROM orders').get();assert.equal(order.status,'pending');
 await update(cb(`approve_${order.id}`,1));await update(cb(`approve_${order.id}`,1));
 assert.equal(creates,1);assert.equal(sql.prepare('SELECT status FROM orders').get().status,'approved');assert.equal(sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);
 assert.ok(out.some(x=>x.method==='sendMessage'&&x.body.chat_id===2&&x.body.text.includes('سرویس شما ساخته شد')));
});
