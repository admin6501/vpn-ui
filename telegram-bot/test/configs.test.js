import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import {PNG} from 'pngjs';
import jsQR from 'jsqr';
import worker, {subscriptionLinks,linkFingerprint} from '../worker.js';
import {qrPng} from '../qr.js';

const content='vless://00000000-0000-4000-8000-000000000000@example.com:443?type=tcp#%D8%AA%D8%B3%D8%AA';
function decode(blob){return blob.arrayBuffer().then(buf=>{const p=PNG.sync.read(Buffer.from(buf));return jsQR(new Uint8ClampedArray(p.data),p.width,p.height)?.data;});}
function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 const prep=(q,a=[])=>({bind(...x){return prep(q,x)},async run(){let r=sql.prepare(q).run(...a);return {meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async all(){return {results:sql.prepare(q).all(...a)}},async first(){return sql.prepare(q).get(...a)??null}});
 const env={DB:{prepare:q=>prep(q),batch:async ss=>Promise.all(ss.map(s=>s.all()))},BOT_TOKEN:'test',WEBHOOK_SECRET:'secret',ADMIN_ID:'1',PANEL_URL:'https://panel.example/path',PANEL_USER:'r',PANEL_PASS:'p'};
 sql.prepare('INSERT INTO services(user_id,name,volume,duration,sub_link,panel_email,inbound_id) VALUES(?,?,?,?,?,?,?)').run(2,'A','10 GB','30 days','https://sub.example/sub/unique','example',3);
 let out=[],subCalls=0,seq=0,panelMode='found',links=[content,'trojan://password@example.net:443#server'];
 globalThis.fetch=async(url,opt={})=>{const u=new URL(url);
  if(u.hostname==='sub.example'){subCalls++;assert.equal(u.pathname,'/sub/unique/raw');assert.equal(opt.redirect,'error');return new Response(Buffer.from(links.join('\n')).toString('base64'));}
  if(u.hostname==='panel.example') { if(panelMode==='offline') throw Error('offline'); if(u.pathname.endsWith('/login')) return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':'vpn-ui=test'}}); if(u.pathname.endsWith('/clients/list'))return new Response(JSON.stringify({success:true,obj:{rows:panelMode==='missing'?[]:[{email:'example',subId:panelMode==='mismatch'?'wrong':'unique',memberships:[],enable:true}],total:1}}));throw Error('unexpected panel endpoint'); }
  assert.equal(u.hostname,'api.telegram.org');const method=u.pathname.split('/').at(-1);
  const body=opt.body instanceof FormData?opt.body:JSON.parse(opt.body);out.push({method,body});return new Response(JSON.stringify({ok:true,result:{message_id:1}}));
 };
 async function cb(uid,data){let tasks=[];const update={update_id:++seq,callback_query:{id:'cb'+seq,from:{id:uid},message:{chat:{id:uid,type:'private'},message_id:2},data}};const req=new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'secret'},body:JSON.stringify(update)});assert.equal((await worker.fetch(req,env,{waitUntil:p=>tasks.push(p)})).status,200);await Promise.all(tasks)}
 return {sql,env,out,cb,setLinks(v){links=v},setPanelMode(v){panelMode=v},get subCalls(){return subCalls}};
}

test('QR PNG can be scanned back to the exact configuration',async()=>{const b=await qrPng(content);assert.equal(b.type,'image/png');assert.equal(await decode(b),content);await assert.rejects(qrPng('x'.repeat(2049)),/طولانی/)});
test('list, link and QR buttons use subscription data and preserve owner isolation',async()=>{
 const f=fixture();await f.cb(3,'cfglist:1:0');assert.equal(f.subCalls,0);
 await f.cb(2,'cfglist:1:0');assert.equal(f.subCalls,1);const list=f.out.filter(x=>x.method==='sendMessage').at(-1).body;const buttons=list.reply_markup.inline_keyboard.flat();
 const textCb=buttons.find(b=>b.callback_data?.startsWith('cfgtxt:')).callback_data;const qrCb=buttons.find(b=>b.callback_data?.startsWith('cfgqr:')).callback_data;
 await f.cb(2,textCb);assert.ok(f.out.some(x=>x.method==='sendMessage' && x.body.text?.includes(content)));
 await f.cb(2,qrCb);const photo=f.out.find(x=>x.method==='sendPhoto');assert.ok(photo);assert.equal(await decode(photo.body.get('photo')),content);
 f.setLinks(['vless://new@example.com:443']);await f.cb(2,qrCb);assert.equal(f.out.filter(x=>x.method==='sendPhoto').length,1);assert.ok(f.out.some(x=>x.method==='answerCallbackQuery'&&x.body.text?.includes('تغییر')));
});
test('empty subscriptions and response errors do not fabricate configs',async()=>{
 const f=fixture();f.setLinks([]);await f.cb(2,'cfglist:1:0');assert.ok(f.out.some(x=>x.body.text?.includes('کانفیگ تکی فعالی')));
 assert.equal((await subscriptionLinks({sub_link:'https://sub.example/sub/unique'})).length,0);
 assert.equal((await subscriptionLinks({sub_link:'http://sub.example/sub/unique'})).length,0);
 await assert.rejects(subscriptionLinks({sub_link:'ftp://sub.example/sub/unique'}),/معتبر/);
});
test('legacy service with an existing individual config and no sub remains usable',async()=>{
 assert.deepEqual(await subscriptionLinks({config_link:content}),[content]);
 await assert.rejects(subscriptionLinks({}),/ثبت نشده/);
});

test('service missing from connected panel is reported and does not expose old links',async()=>{
 const f=fixture();f.setPanelMode('missing');await f.cb(2,'svc_1');assert.ok(f.out.some(x=>x.body.text?.includes('سرویس در پنل متصل فعلی یافت نشد')));await f.cb(2,'cfglist:1:0');assert.equal(f.subCalls,0);
});
test('panel unavailability is not falsely reported as an absent service',async()=>{
 const f=fixture();f.setPanelMode('offline');await f.cb(2,'svc_1');assert.ok(f.out.some(x=>x.body.text?.includes('بررسی سرویس در پنل ناموفق')));assert.ok(!f.out.some(x=>x.body.text?.includes('سرویس در پنل متصل فعلی یافت نشد')));
});
test('a same-named account with different subscription id is not mistaken for the service',async()=>{
 const f=fixture();f.sql.prepare("UPDATE services SET sub_id='unique' WHERE id=1").run();f.setPanelMode('mismatch');await f.cb(2,'svc_1');
 assert.ok(f.out.some(x=>x.body.text?.includes('سرویس در پنل متصل فعلی یافت نشد')));await f.cb(2,'cfglist:1:0');assert.equal(f.subCalls,0);
});
test('a matched service uses the currently configured subscription base',async()=>{
 const f=fixture();f.sql.prepare("UPDATE services SET sub_id='unique',sub_link='https://old.example/sub/unique' WHERE id=1").run();f.sql.prepare("INSERT INTO settings(key,value) VALUES('sub_base_url','https://sub.example/sub/')").run();
 await f.cb(2,'svc_1');assert.ok(f.out.some(x=>x.body.text?.includes('https://sub.example/sub/unique')));await f.cb(2,'cfglist:1:0');assert.equal(f.subCalls,1);
});
test('stale management buttons cannot delete or enable an unrelated same-named service',async()=>{
 const f=fixture();f.sql.prepare("UPDATE services SET sub_id='unique' WHERE id=1").run();f.setPanelMode('mismatch');
 await f.cb(2,'cfgmanage:1:0');assert.ok(f.out.some(x=>x.body.text?.includes('فقط برای ادمین')));
 await f.cb(1,'admin:sc:1:0');assert.ok(f.out.some(x=>x.body.text?.includes('سرویس پیدا نشد')));
 await f.cb(2,'delyes_1');assert.ok(f.sql.prepare('SELECT id FROM services WHERE id=1').get());assert.equal(f.subCalls,0);
});
