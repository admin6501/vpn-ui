import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import worker from '../worker.js';
import {processMessageJobs, queuedMessagePayload} from '../messages.js';
function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 const prep=(q,a=[])=>({q,a,bind(...x){return prep(q,x)},async run(){const r=sql.prepare(q).run(...a);return {meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async all(){return {results:sql.prepare(q).all(...a)}},async first(){return sql.prepare(q).get(...a)??null}});
 let queue=Promise.resolve();const DB={prepare:q=>prep(q),batch(ss){const fn=async()=>{sql.exec('BEGIN');try{const result=[];for(const s of ss)result.push(/^\s*(SELECT|WITH)/i.test(s.q)&&/^\s*SELECT/i.test(s.q)?await s.all():await s.run());sql.exec('COMMIT');return result}catch(e){sql.exec('ROLLBACK');throw e}};const next=queue.then(fn);queue=next.catch(()=>{});return next}};
 const env={DB,BOT_TOKEN:'token',WEBHOOK_SECRET:'secret',ADMIN_ID:'1'},out=[];let seq=0,failUser=0;
 globalThis.fetch=async(url,opt)=>{const method=new URL(url).pathname.split('/').at(-1),body=JSON.parse(opt.body);out.push({method,body});if(method==='sendMessage'&&body.chat_id===failUser)return Response.json({ok:false,description:'blocked'});return Response.json({ok:true,result:{message_id:1}})};
 async function emit(data){const tasks=[];const req=new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'secret'},body:JSON.stringify({update_id:++seq,...data})});assert.equal((await worker.fetch(req,env,{waitUntil:p=>tasks.push(p)})).status,200);await Promise.all(tasks)}
 const message=(uid,text)=>emit({message:{from:{id:uid,first_name:'A'},chat:{id:uid,type:'private'},message_id:++seq,text}});
 const media=(uid,content)=>emit({message:{from:{id:uid,first_name:'A'},chat:{id:uid,type:'private'},message_id:++seq,...content}});
 const cb=(uid,data)=>emit({callback_query:{id:'cb'+ ++seq,from:{id:uid,first_name:'A'},message:{chat:{id:uid,type:'private'},message_id:1},data}});
 return {sql,env,out,message,media,cb,setFail(id){failUser=id}};
}
test('broadcast previews and sends photos, voice and forwards with original Telegram content',async()=>{
 for(const [content,method] of [[{photo:[{file_id:'image'}],caption:'caption'},'copyMessage'],[{voice:{file_id:'voice'}},'copyMessage'],[{text:'forwarded',forward_origin:{type:'user'}},'forwardMessage']]){
  const f=fixture();await f.message(2,'/start');await f.cb(1,'admin:broadcast');await f.media(1,content);
  const job=f.sql.prepare('SELECT * FROM bot_message_jobs').get();assert.equal(job.status,'draft');assert.ok(f.out.some(x=>x.method===method&&x.body.chat_id===1));
  await f.cb(1,`admin:message_send:${job.id}`);const sent=[];await processMessageJobs(f.env,async(id,body)=>{sent.push(queuedMessagePayload(id,body));return {ok:true}});
  assert.ok(sent.some(x=>x.method===method&&x.params.chat_id===2&&x.params.from_chat_id===1));
 }
});
test('broadcast refuses media albums rather than silently sending only one item',async()=>{
 const f=fixture();await f.cb(1,'admin:broadcast');await f.media(1,{photo:[{file_id:'a'}],media_group_id:'group'});assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM bot_message_jobs').get().n,0);assert.ok(f.out.some(x=>x.body.text?.includes('آلبوم')));
});
test('broadcast requires creator confirmation, excludes blocked users and reports outcomes once',async()=>{
 const f=fixture();f.sql.exec("INSERT INTO bot_users(user_id,first_name,blocked) VALUES(2,'A',0),(3,'B',0),(4,'C',1),(5,'D',0); INSERT INTO admins(user_id,added_at) VALUES(9,'now')");
 await f.cb(1,'admin:broadcast');await f.message(1,'سلام <دوست>');assert.equal(f.sql.prepare('SELECT status FROM bot_message_jobs WHERE id=1').get().status,'draft');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM bot_message_targets').get().n,0);
 await f.cb(2,'admin:message_send:1');await f.cb(9,'admin:message_send:1');assert.equal(f.sql.prepare('SELECT status FROM bot_message_jobs WHERE id=1').get().status,'draft');
 await f.cb(1,'admin:message_send:1');await f.cb(1,'admin:message_send:1');assert.equal(f.sql.prepare('SELECT status FROM bot_message_jobs WHERE id=1').get().status,'queued');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM bot_message_targets WHERE user_id=4').get().n,0);
 const sends=[];await processMessageJobs(f.env,async(id,text)=>{sends.push([id,text]);if(id===5)throw Error('timeout');return {ok:id!==3}});const statuses=f.sql.prepare('SELECT user_id,status FROM bot_message_targets ORDER BY user_id').all();assert.ok(statuses.some(r=>r.user_id===2&&r.status==='sent'));assert.ok(statuses.some(r=>r.user_id===3&&r.status==='failed'));assert.ok(statuses.some(r=>r.user_id===5&&r.status==='uncertain'));assert.equal(f.sql.prepare('SELECT status FROM bot_message_jobs WHERE id=1').get().status,'done');const count=sends.length;await processMessageJobs(f.env,async()=>{throw Error('duplicate')});assert.equal(sends.length,count);
});
test('private message targets only the selected user after preview and confirmation',async()=>{
 const f=fixture();await f.message(2,'/start');await f.cb(1,'admin:user:2');await f.cb(1,'admin:message_user:2');await f.message(1,'پیام خصوصی');assert.equal(f.sql.prepare('SELECT status FROM bot_message_jobs').get().status,'draft');await f.cb(1,'admin:message_send:1');const sends=[];await processMessageJobs(f.env,async(id,text)=>{sends.push([id,text]);return {ok:true}});assert.ok(sends.some(([id,text])=>id===2&&text==='پیام خصوصی'));assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM bot_message_targets').get().n,1);
});
test('stale delivery claim is marked uncertain instead of being resent',async()=>{
 const f=fixture();await f.message(2,'/start');await f.cb(1,'admin:message_user:2');await f.message(1,'message');await f.cb(1,'admin:message_send:1');f.sql.exec("UPDATE bot_message_targets SET status='sending',claimed_ms=1 WHERE user_id=2");const sends=[];await processMessageJobs(f.env,async(id,text)=>{sends.push([id,text]);return {ok:true}},{now:()=>200000});assert.equal(f.sql.prepare('SELECT status FROM bot_message_targets').get().status,'uncertain');assert.ok(!sends.some(([id])=>id===2));
});
test('wallet adjustment notifies customer; notification failure does not roll back balance',async()=>{
 const f=fixture();await f.message(2,'/start');await f.cb(1,'admin:wallet_add:2');await f.message(1,'100');await f.cb(1,'admin:wallet_confirm');assert.ok(f.out.some(x=>x.method==='sendMessage'&&x.body.chat_id===2&&x.body.text.includes('افزایش یافت')));
 f.setFail(2);await f.cb(1,'admin:wallet_sub:2');await f.message(1,'20');await f.cb(1,'admin:wallet_confirm');assert.equal(f.sql.prepare('SELECT SUM(delta) n FROM wallet_entries WHERE user_id=2').get().n,80);assert.ok(f.out.some(x=>x.body.chat_id===1&&x.body.text?.includes('ارسال اعلان به کاربر ناموفق')));
});
