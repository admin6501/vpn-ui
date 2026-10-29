import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import worker from '../worker.js';

function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 const prep=(q,a=[])=>({bind(...x){return prep(q,x)},async run(){const r=sql.prepare(q).run(...a);return {meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async all(){return {results:sql.prepare(q).all(...a)}},async first(){return sql.prepare(q).get(...a)??null}});
 const DB={prepare:q=>prep(q),batch:async ss=>Promise.all(ss.map(s=>s.all()))};let output=[], sequence=0;const chats=new Map();
 globalThis.fetch=async(url,opt)=>{const p=new URL(url).pathname;
  if(p.startsWith('/bot')){let method=p.split('/').at(-1),body=JSON.parse(opt.body);output.push({method,body});if(method==='getChat')return Response.json(chats.has(body.chat_id)?{ok:true,result:chats.get(body.chat_id)}:{ok:false,description:'chat unavailable'});return new Response(JSON.stringify(method==='getChatMember'?{ok:true,result:{status:'member'}}:{ok:true,result:{message_id:123}}));}
  if(p.endsWith('/login'))return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':'vpn-ui=test'}});
  if(p.endsWith('/clients/groups'))return new Response(JSON.stringify({success:true,obj:[{id:1,inboundIds:[3]}]}));
  if(p.endsWith('/clients/assignable'))return new Response(JSON.stringify({success:true,obj:[{inboundId:3,protocol:'vless'}]}));
  if(p.endsWith('/clients/list'))return new Response(JSON.stringify({success:true,obj:{rows:[{email:'active',subId:'s1',enable:true,up:0,down:0,totalGB:0,expiryTime:0},{email:'disabled',subId:'s2',enable:false,up:0,down:0,totalGB:0,expiryTime:0}],total:2}}));
  throw Error('unexpected fetch: '+p);
 };
 const env={DB,BOT_TOKEN:'token',WEBHOOK_SECRET:'secret',ADMIN_ID:'1',PANEL_URL:'https://panel.example/base',PANEL_USER:'u',PANEL_PASS:'p',CHANNEL_USERNAME:'@channel'};
 async function emit(data){let tasks=[];let req=new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'secret'},body:JSON.stringify({update_id:++sequence,...data})});assert.equal((await worker.fetch(req,env,{waitUntil:p=>tasks.push(p)})).status,200);await Promise.all(tasks)}
 const message=(uid,text,name='A')=>emit({message:{from:{id:uid,first_name:name},chat:{id:uid,type:'private'},message_id:++sequence,text}});
 const callback=(uid,data)=>emit({callback_query:{id:'cb'+ ++sequence,from:{id:uid,first_name:'A'},message:{chat:{id:uid,type:'private'},message_id:12},data}});
 const last=()=>output.filter(o=>o.method==='sendMessage').at(-1)?.body;
 return {sql,output,message,callback,last,chats};
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
 assert.ok(!JSON.stringify(f.last().reply_markup).includes('admin:help'));
 await f.callback(1,'admin:settings');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:setting:sub_base_url'));
 await f.callback(1,'admin:setting:test_days');await f.message(1,'45');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='test_days'").get().value,'45');
 await f.callback(1,'admin:setting:test_enabled');await f.callback(1,'admin:test:0');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='test_enabled'").get().value,'0');
 await f.callback(1,'admin:panelcheck');assert.ok(f.output.some(x=>x.body.text?.includes('اتصال به پنل برقرار')));
 await f.callback(1,'admin:stats');assert.ok(f.output.some(x=>x.body.text?.includes('مجموع فروش')));
});

test('buttons create, change and confirm deletion of plans',async()=>{
 const f=fixture();f.sql.exec("INSERT INTO bot_categories(id,name,group_ids) VALUES(1,'Test','1')");await f.callback(1,'admin:plan_add');await f.message(1,'p3 12345 3 30');await f.callback(1,'admin:planpick:1');assert.equal(f.sql.prepare("SELECT price FROM plans WHERE key='p3'").get().price,12345);
 await f.callback(1,'admin:price:p3');await f.message(1,'50000');assert.equal(f.sql.prepare("SELECT price FROM plans WHERE key='p3'").get().price,50000);
 await f.callback(1,'admin:editplan:p3');await f.message(1,'60000 7.5 45');await f.callback(1,'admin:planpick:1');assert.equal(f.sql.prepare("SELECT gb FROM plans WHERE key='p3'").get().gb,7.5);
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
 await f.callback(1,'admin:setting:test_days');await f.callback(1,'admin:stats');await f.message(1,'99');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='test_days'").get(),undefined);
});

test('fresh database has no sample records and customer sees an empty-plan message',async()=>{
 const f=fixture();
 for(const table of ['plans','services','orders','admins','test_claims','wallet_entries']) assert.equal(f.sql.prepare(`SELECT COUNT(*) AS n FROM ${table}`).get().n,0,table);
 await f.callback(2,'buy'); assert.ok(f.output.some(x=>x.body.text?.includes('دسته‌بندی قابل استفاده‌ای تنظیم نشده')));
 f.sql.exec("INSERT INTO plans(key,gb,price,sort) VALUES('mine',3,100,1)");
 f.sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 assert.equal(f.sql.prepare('SELECT COUNT(*) AS n FROM plans').get().n,1);
});

test('user directory paginates at ten and includes older service and order records',async()=>{
 const f=fixture();for(let i=10;i<33;i++)f.sql.prepare('INSERT INTO bot_users(user_id,first_name) VALUES(?,?)').run(i,`User${i}`);
 f.sql.prepare('INSERT INTO services(user_id,name) VALUES(?,?)').run(100,'old service');
 await f.callback(1,'admin:users:0');let markup=f.last().reply_markup.inline_keyboard;assert.equal(markup.filter(r=>r[0]?.callback_data?.startsWith('admin:user:')).length,10);assert.ok(JSON.stringify(markup).includes('admin:users:1'));assert.ok(JSON.stringify(markup).includes('admin:user:100'));
 await f.callback(1,'admin:users:1');markup=f.last().reply_markup.inline_keyboard;assert.equal(markup.filter(r=>r[0]?.callback_data?.startsWith('admin:user:')).length,10);
 await f.callback(1,'admin:user:100');assert.match(f.last().text,/شناسه.*100/);assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:uo:100:0'));
});
test('block only affects bot use, protects admins, and unblock restores access',async()=>{
 const f=fixture();await f.message(2,'/start');f.sql.prepare('INSERT INTO services(user_id,name) VALUES(2,?)').run('kept');
 await f.callback(9,'admin:ub:2:1');assert.equal(f.sql.prepare('SELECT blocked FROM bot_users WHERE user_id=2').get().blocked,0);
 await f.callback(1,'admin:ub:2:1');assert.equal(f.sql.prepare('SELECT blocked FROM bot_users WHERE user_id=2').get().blocked,1);
 await f.message(2,'/start');assert.match(f.last().text,/مسدود/);await f.callback(2,'buy');assert.ok(f.output.some(x=>x.method==='answerCallbackQuery'&&x.body.text?.includes('مسدود')));
 assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services').get().n,1);await f.callback(1,'admin:ub:2:0');await f.message(2,'/start');assert.match(f.last().text,/سلام A/);
 await f.callback(1,'admin:ub:1:1');assert.equal(f.sql.prepare('SELECT blocked FROM bot_users WHERE user_id=1').get().blocked,0);
});
test('admin sees all orders and wallet history in pages and current panel status',async()=>{
 const f=fixture();await f.message(2,'/start');for(let i=0;i<12;i++){f.sql.prepare("INSERT INTO orders(user_id,service_name,final_price,status,created_at) VALUES(2,?,?,?,?)").run(`S${i}`,100,'approved','today');f.sql.prepare("INSERT INTO wallet_entries(user_id,delta,reference,reason,created_at) VALUES(2,10,?,'topup','today')").run(`ref${i}`)}
 f.sql.exec("INSERT INTO services(user_id,name,panel_email,sub_id) VALUES(2,'Live','active','s1'),(2,'Disabled','disabled','s2'),(2,'Missing','missing','s3')");
 await f.callback(1,'admin:uo:2:0');assert.match(f.last().text,/#12/);assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:uo:2:1'));await f.callback(1,'admin:uo:2:1');assert.match(f.last().text,/#1/);
 await f.callback(1,'admin:ut:2:0');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:ut:2:1'));await f.callback(1,'admin:ut:2:1');assert.match(f.last().text,/شارژ/);
 await f.callback(1,'admin:us:2:0');assert.match(f.last().text,/Live — فعال/);assert.match(f.last().text,/Disabled — غیرفعال/);assert.match(f.last().text,/Missing — در پنل یافت نشد/);
 await f.callback(1,'admin:user:2');assert.match(f.last().text,/120 تومان/);
});
test('global trial reset requires confirmation and retains unresolved claims',async()=>{
 const f=fixture();f.sql.exec("INSERT INTO test_claims VALUES(10,'done|today'),(11,'processing|abc'),(12,'review|today'),(13,'legacy')");
 await f.callback(2,'admin:reset_all_confirm');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM test_claims').get().n,4);
 await f.callback(1,'admin:reset_all_tests');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM test_claims').get().n,4);
 await f.callback(1,'admin:reset_all_confirm');assert.deepEqual(f.sql.prepare('SELECT user_id FROM test_claims ORDER BY user_id').all().map(x=>x.user_id),[11,12]);assert.match(f.last().text,/2 کاربر/);
 await f.callback(1,'admin:reset_all_confirm');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM test_claims').get().n,2);
});

test('referral reward button sets amount, zero disables registration without consuming eligibility',async()=>{
 const f=fixture();await f.callback(1,'admin:root');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:setting:referral_discount'));
 await f.callback(1,'admin:setting:referral_discount');await f.message(1,'0');await f.callback(2,'enter_invite');assert.match(f.last().text,/غیرفعال/);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM referrals').get().n,0);
 await f.callback(1,'admin:setting:referral_discount');await f.message(1,'15000');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='referral_discount'").get().value,'15000');await f.callback(2,'enter_invite');await f.message(2,'3');assert.equal(f.sql.prepare('SELECT amount FROM discounts WHERE user_id=3').get().amount,15000);
});

test('admin selects custom or random names and random purchase skips name prompt',async()=>{
 const f=fixture();f.sql.exec("INSERT INTO plans(key,gb,price,sort) VALUES('p10',10,70000,1); INSERT INTO bot_categories(id,name,group_ids) VALUES(1,'Test','1'); INSERT INTO bot_plan_specs VALUES('p10',1,30); INSERT INTO settings(key,value) VALUES('sub_base_url','https://sub.example/sub/')");
 await f.callback(1,'admin:root');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:setting:name_mode'));
 await f.callback(5,'admin:namemode:random');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='name_mode'").get(),undefined);
 await f.callback(1,'admin:setting:name_mode');await f.callback(1,'admin:namemode:random');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='name_mode'").get().value,'random');
 assert.equal(f.last().reply_markup.inline_keyboard[0][0].callback_data,'admin:root');await f.callback(1,'admin:root');assert.match(f.last().text,/پنل مدیریت ربات/);
 await f.callback(2,'plan_p10');let row=f.sql.prepare('SELECT service_name FROM orders WHERE user_id=2').get();assert.match(row.service_name,/^Client[a-z]{12}$/);assert.match(f.last().text,/مبلغ قابل پرداخت/);
 await f.callback(2,'plan_p10');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM orders WHERE user_id=2').get().n,1);
 await f.callback(1,'admin:namemode:custom');assert.equal(f.last().reply_markup.inline_keyboard[0][0].callback_data,'admin:root');await f.callback(3,'plan_p10');assert.match(f.last().text,/نام.*سرویس/);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM orders WHERE user_id=3').get().n,0);await f.message(3,'MyName');assert.equal(f.sql.prepare('SELECT service_name FROM orders WHERE user_id=3').get().service_name,'MyName');
});
test('user profile links to safe wallet adjustment and complete ledger',async()=>{
 const f=fixture();await f.message(2,'/start');await f.callback(1,'admin:user:2');await f.callback(1,'admin:wallet_view:2');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:wallet_add:2'));
 await f.callback(1,'admin:wallet_add:2');await f.message(1,'100');assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM wallet_entries WHERE user_id=2').get().n,0);await f.callback(1,'admin:wallet_confirm');assert.equal(f.sql.prepare('SELECT SUM(delta) n FROM wallet_entries WHERE user_id=2').get().n,100);
 await f.callback(1,'admin:ut:2:0');assert.match(f.last().text,/100 تومان/);await f.callback(1,'admin:wallet_sub:2');await f.message(1,'101');await f.callback(1,'admin:wallet_confirm');assert.equal(f.sql.prepare('SELECT SUM(delta) n FROM wallet_entries WHERE user_id=2').get().n,100);
});

test('admin searches saved service names and panel emails and views panel details',async()=>{
 const f=fixture();for(let i=0;i<12;i++)f.sql.prepare('INSERT INTO services(user_id,name,panel_email,sub_id) VALUES(?,?,?,?)').run(2,`service${i}`,i===0?'active':`match${i}`,i===0?'s1':'x');
 await f.callback(2,'admin:service_search');await f.message(2,'service');assert.ok(!f.output.some(x=>x.body.text?.includes('نتیجه جستجوی سرویس')));
 await f.callback(1,'admin:service_search');await f.message(1,'service');assert.match(f.last().text,/نتیجه جستجوی سرویس/);assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:searchpage:1'));
 await f.callback(1,'admin:searchpage:1');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:svc:1'));await f.callback(1,'admin:svc:1');assert.match(f.last().text,/حساب پنل/);assert.match(f.last().text,/وضعیت حساب.*فعال/);
 await f.callback(1,'admin:service_search');await f.message(1,'active');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:svc:1'));
});

test('legacy user names are recovered from Telegram without altering block state',async()=>{
 const f=fixture();f.sql.prepare('INSERT INTO services(user_id,name) VALUES(?,?)').run(888,'old');f.sql.prepare('INSERT INTO bot_users(user_id,blocked) VALUES(888,1)').run();
 f.chats.set(888,{id:888,type:'private',first_name:'Ali <X>',username:'ali_user'});
 await f.callback(2,'admin:users:0');assert.equal(f.output.filter(x=>x.method==='getChat').length,0);
 await f.callback(1,'admin:users:0');const list=JSON.stringify(f.last().reply_markup);assert.ok(list.includes('Ali <X>'));assert.ok(list.includes('@ali_user'));
 const u=f.sql.prepare('SELECT first_name,username,blocked FROM bot_users WHERE user_id=888').get();assert.equal(u.first_name,'Ali <X>');assert.equal(u.username,'ali_user');assert.equal(u.blocked,1);
 await f.callback(1,'admin:user:888');assert.match(f.last().text,/Ali &lt;X&gt;/);assert.match(f.last().text,/@ali_user/);assert.match(f.last().text,/مسدود/);
});
test('unavailable legacy identity falls back to historical order username without inventing a name',async()=>{
 const f=fixture();f.sql.prepare("INSERT INTO orders(user_id,username,service_name) VALUES(?,?,'old')").run(777,'legacy_user');
 await f.callback(1,'admin:user:777');assert.match(f.last().text,/نام در دسترس نیست/);assert.match(f.last().text,/@legacy_user/);assert.match(f.last().text,/ممکن است تغییر کرده باشد/);
 f.chats.set(777,{id:999,type:'private',first_name:'Wrong',username:'wrong'});await f.callback(1,'admin:user:777');assert.doesNotMatch(f.last().text,/Wrong|@wrong/);
});

test('forced channel settings explain bot must be a channel administrator',async()=>{
 const f=fixture();await f.callback(1,'admin:setting:channel_username');assert.match(f.last().text,/ربات را به کانال اضافه و ادمین کانال کنید/);await f.callback(1,'admin:setting:channel_link');assert.match(f.last().text,/ادمین کانال/);
});
