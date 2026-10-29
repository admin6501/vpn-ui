import test from 'node:test';
import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync} from 'node:fs';
import {handleUpdate,runSubscriptionPolicies,prepareMigrationDestination,createMigrationDestination,importMigrationUsage,verifyMigrationDestination,readMigrationSnapshot,setMigrationAdmission} from '../worker.js';
import {encryptPanel,panelAccountKey} from '../panel-config.js';
function fixture(){
 const sql=new DatabaseSync(':memory:');sql.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));
 const prep=(q,a=[])=>({bind(...x){return prep(q,x)},execute(){const s=sql.prepare(q);if(s.columns().length)return {results:s.all(...a),meta:{changes:0}};const r=s.run(...a);return {results:[],meta:{changes:Number(r.changes),last_row_id:Number(r.lastInsertRowid)}}},async run(){return this.execute()},async all(){return this.execute()},async first(){return sql.prepare(q).get(...a)??null}});
 const DB={prepare:q=>prep(q),async batch(ss){sql.exec('BEGIN');try{const r=ss.map(s=>s.execute());sql.exec('COMMIT');return r}catch(e){sql.exec('ROLLBACK');throw e}}};
 sql.exec("INSERT INTO settings(key,value) VALUES('sub_base_url','https://sub-one.example/sub/')");
 const env={DB,BOT_TOKEN:'token',ADMIN_ID:'1',WEBHOOK_SECRET:'secret',PANEL_URL:'https://panel-one.example/base',PANEL_USER:'rep',PANEL_PASS:'pass',CARD_NUMBER:'123',CARD_HOLDER:'owner'};
 let seq=0,out=[],actions=[],accounts={one:[],two:[]};
 globalThis.fetch=async(url,opt={})=>{const u=new URL(url),p=u.pathname;
  if(u.hostname==='api.telegram.org'){const method=p.split('/').at(-1);out.push({method,body:JSON.parse(opt.body)});return Response.json({ok:true,result:{message_id:1}})}
  const target=u.hostname==='panel-one.example'?'one':u.hostname==='panel-two.example'?'two':null;if(!target)throw Error('unexpected host '+u.hostname);
  const inbound=target==='one'?3:8;
  if(p.endsWith('/login'))return new Response(JSON.stringify({success:true}),{headers:{'set-cookie':'vpn-ui=session'}});
  if(p.endsWith('/clients/groups'))return Response.json({success:true,obj:[{id:1,name:'Shared',inboundIds:[inbound]}]});
  if(p.endsWith('/clients/assignable'))return Response.json({success:true,obj:[{inboundId:inbound,protocol:'vless'}]});
  if(p.endsWith('/clients/list'))return Response.json({success:true,obj:{rows:accounts[target],total:accounts[target].length}});
  if(p.endsWith('/addClient')){const form=new URLSearchParams(opt.body),c=JSON.parse(form.get('settings')).clients[0];actions.push([target,'add',c.email]);accounts[target].push({...c,up:0,down:0,memberships:[{inboundId:inbound,enable:true}]});return Response.json({success:true})}
  if(p.includes('/migrationImportUsage/')){const email=decodeURIComponent(p.split('/').at(-1)),a=accounts[target].find(x=>x.email===email),v=JSON.parse(opt.body);if(!a||a.enable)return Response.json({success:false});Object.assign(a,{up:v.upload,down:v.download,allTime:v.allTime});return Response.json({success:true})}
  if(p.includes('/getClientTraffics/')){const email=decodeURIComponent(p.split('/').at(-1)),a=accounts[target].find(x=>x.email===email);return Response.json({success:!!a,obj:a})}
  if(p.endsWith('/bulkUpdateClients')){const data=JSON.parse(new URLSearchParams(opt.body).get('data'));for(const t of data.targets){const a=accounts[target].find(x=>x.email===t.email && x.memberships.some(m=>m.inboundId===t.inboundId));if(a)a.enable=data.op==='enable'}return Response.json({success:true,obj:{applied:data.targets.length}})}
  if(p.includes('/renewClient/')){actions.push([target,'renew']);return Response.json({success:true})}
  if(p.includes('/delClientByEmail/')){actions.push([target,'delete']);const email=decodeURIComponent(p.split('/').at(-1));accounts[target]=accounts[target].filter(a=>a.email!==email);return Response.json({success:true})}
  throw Error('unexpected '+u.href);
 };
 async function emit(update){await handleUpdate(env,{update_id:++seq,...update})}
 const msg=(id,text)=>emit({message:{from:{id,first_name:'A'},chat:{id,type:'private'},message_id:++seq,text}});
 const cb=(id,data)=>emit({callback_query:{id:String(++seq),from:{id,first_name:'A'},message:{chat:{id,type:'private'},message_id:1},data}});
 const last=()=>out.filter(x=>x.method==='sendMessage').at(-1)?.body;
 return {sql,env,msg,cb,last,out,actions,accounts:()=>accounts};
}

test('migration destination keeps original quota and deadline and starts disabled',async()=>{
 const f=fixture();f.env.cfg={sub_base_url:'https://sub-one.example/sub/'};
 const prepared=await prepareMigrationDestination(f.env,{name:'Alice',snapshot:{quota:1000,expiry:123456},renewalDuration:86400000,groupIds:'1'});
 assert.equal(prepared.client.enable,false);assert.equal(prepared.client.totalGB,1000);assert.equal(prepared.client.expiryTime,123456);assert.equal(prepared.client.renewalDuration,86400000);
 await createMigrationDestination(f.env,prepared);assert.equal(f.accounts().one[0].subId,prepared.subId);assert.equal(f.accounts().one[0].enable,false);
 await importMigrationUsage(f.env,prepared,{upload:100,download:200,allTime:500});assert.equal(f.accounts().one[0].down,200);assert.equal(f.accounts().one[0].allTime,500);
 const snapshot=await readMigrationSnapshot(f.env,{panel_email:'Alice',sub_id:prepared.subId});assert.equal(snapshot.allTime,500);assert.equal(snapshot.quota,1000);
 await verifyMigrationDestination(f.env,prepared,snapshot,false);
 await assert.rejects(verifyMigrationDestination(f.env,{...prepared,memberIds:[3,9]},snapshot,false),/تأیید نشد/);
 await setMigrationAdmission(f.env,{panel_email:'Alice',sub_id:prepared.subId},true);assert.equal(f.accounts().one[0].enable,true);
 await setMigrationAdmission(f.env,{panel_email:'Alice',sub_id:prepared.subId},false);assert.equal(f.accounts().one[0].enable,false);
 await assert.rejects(prepareMigrationDestination(f.env,{name:'Alice',snapshot:{quota:1000,expiry:123456},renewalDuration:86400000,groupIds:'1'}),/از قبل/);
});
test('two same-named groups keep purchase, subscription, renewal and deletion on their own panel',async()=>{
 const f=fixture();await f.cb(1,'admin:multipanel_add');await f.msg(1,'پنل ۲');await f.msg(1,'https://panel-two.example/base');await f.msg(1,'rep');await f.msg(1,'pass');await f.msg(1,'https://sub-two.example/sub/');
 const row=f.sql.prepare("SELECT key,value FROM settings WHERE key LIKE 'extra_panel:%'").get();assert.ok(row);assert.ok(!row.value.includes('pass'));const slot=row.key.split(':')[1];
 await f.cb(1,'admin:multipanels');assert.ok(JSON.stringify(f.last().reply_markup).includes(`admin:multipanel:${slot}`));await f.cb(1,`admin:multipanel:${slot}`);assert.match(f.last().text,/پنل ۲/);
 await f.cb(1,'admin:category_add');await f.msg(1,'Second');let markup=JSON.stringify(f.last().reply_markup);assert.ok(markup.includes('Shared — پنل ۱'));assert.ok(markup.includes('Shared — پنل ۲'));
 await f.cb(1,`admin:categorygroup:${slot}:1`);await f.cb(1,'admin:plan_add');await f.msg(1,'p10 100 10 30');await f.cb(1,'admin:planpick:1');
 f.accounts().one.push({email:'Alice',subId:'other',enable:true,memberships:[{inboundId:3}]});
 await f.cb(2,'plan_p10');await f.cb(2,'buyname:custom');await f.msg(2,'Alice');f.sql.exec("INSERT INTO wallet_entries(user_id,delta,reference,reason,created_at) VALUES(2,200,'test','admin_adjustment','now')");await f.cb(2,'wallet:pay:1');assert.deepEqual(f.actions[0],['two','add','Alice']);
 const svc=f.sql.prepare('SELECT * FROM services').get();assert.match(svc.sub_link,/^https:\/\/sub-two\.example\/sub\//);assert.ok(f.sql.prepare('SELECT value FROM settings WHERE key=?').get(`service_panel:${svc.id}`));
 await f.cb(1,`admin:mpsub:${slot}`);await f.msg(1,'https://new-sub-two.example/sub/');await f.cb(2,`svc_${svc.id}`);assert.match(f.last().text,/new-sub-two\.example/);await f.cb(2,`renew_${svc.id}`);assert.ok(JSON.stringify(f.last().reply_markup).includes('rplan_p10'));await f.cb(2,'rplan_p10');await f.cb(2,'wallet:pay:2');assert.ok(f.actions.some(([panel,action])=>panel==='two'&&action==='renew'));
 await f.cb(1,`admin:svcdeleteconfirm:${svc.id}`);assert.ok(f.actions.some(([panel,action])=>panel==='two'&&action==='delete'));assert.equal(f.accounts().one.length,1);assert.equal(f.accounts().two.length,0);
});
test('scheduled alerts stay bound to their own panel even when names and subscription ids match',async()=>{
 const f=fixture(),one=await panelAccountKey('https://panel-one.example/base','rep'),two=await panelAccountKey('https://panel-two.example/base','rep');
 const extra=await encryptPanel('token',{name:'پنل ۲',url:'https://panel-two.example/base',user:'rep',pass:'pass',sub:'https://sub-two.example/sub/'});
 f.sql.prepare('INSERT INTO settings(key,value) VALUES(?,?)').run('extra_panel:p123456789abc',extra);f.sql.exec("INSERT INTO settings(key,value) VALUES('alerts_enabled','1'),('service_panel:1','"+one+"'),('service_panel:2','"+two+"'); INSERT INTO services(id,user_id,name,panel_email,sub_id) VALUES(1,2,'One','same','shared'),(2,3,'Two','same','shared')");
 const common={email:'same',subId:'shared',enable:true,up:0,totalGB:1024**3,expiryTime:0,memberships:[{inboundId:3}]};f.accounts().one.push({...common,down:0.9*1024**3});f.accounts().two.push({...common,down:0.5*1024**3});
 await runSubscriptionPolicies(f.env);const messages=f.out.filter(x=>x.method==='sendMessage'&&[2,3].includes(x.body.chat_id));assert.equal(messages.length,2);assert.ok(messages.some(x=>x.body.chat_id===2&&x.body.text.includes('One')));assert.ok(messages.some(x=>x.body.chat_id===3&&x.body.text.includes('Two')));
});
test('deleting identical service identities on two panels only affects the selected panel and order',async()=>{
 const f=fixture(),one=await panelAccountKey('https://panel-one.example/base','rep'),two=await panelAccountKey('https://panel-two.example/base','rep');
 const extra=await encryptPanel('token',{name:'پنل ۲',url:'https://panel-two.example/base',user:'rep',pass:'pass',sub:'https://sub-two.example/sub/'});f.sql.prepare('INSERT INTO settings(key,value) VALUES(?,?)').run('extra_panel:p123456789abc',extra);
 for(const [id,identity] of [[1,one],[2,two]]){f.sql.prepare('INSERT INTO settings(key,value) VALUES(?,?)').run(`service_panel:${id}`,identity);f.sql.prepare('INSERT INTO services(id,user_id,name,panel_email,sub_id) VALUES(?,2,?,?,?)').run(id,'same','same','shared');f.sql.prepare("INSERT INTO orders(id,user_id,service_name,status) VALUES(?,2,'same','approved')").run(id);f.sql.prepare("INSERT INTO bot_order_routes(order_id,group_ids,anchor_id,panel_identity) VALUES(?,'1',0,?)").run(id,identity);f.sql.prepare("INSERT INTO provision_attempts(attempt_key,email,sub_id,inbound_id,created_at) VALUES(?,'same','shared',3,'now')").run(`order:${id}:x`)}
 const common={email:'same',subId:'shared',enable:true,up:0,down:0,totalGB:0,expiryTime:0,memberships:[{inboundId:3}]};f.accounts().one.push({...common});f.accounts().two.push({...common,memberships:[{inboundId:8}]});
 await f.cb(1,'admin:svcdeleteconfirm:2');assert.deepEqual(f.actions,[['two','delete']]);assert.equal(f.accounts().one.length,1);assert.equal(f.accounts().two.length,0);assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='hidden_order:2'").get().value,'1');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='hidden_order:1'").get(),undefined);
});
test('automatic expiry cleanup respects panel identity when two panels contain identical accounts',async()=>{
 const f=fixture(),one=await panelAccountKey('https://panel-one.example/base','rep'),two=await panelAccountKey('https://panel-two.example/base','rep');
 f.sql.prepare('INSERT INTO settings(key,value) VALUES(?,?)').run('extra_panel:p123456789abc',await encryptPanel('token',{name:'پنل ۲',url:'https://panel-two.example/base',user:'rep',pass:'pass',sub:'https://sub-two.example/sub/'}));
 f.sql.exec("INSERT INTO settings(key,value) VALUES('auto_delete_enabled','1'),('auto_delete_days','0'),('service_panel:1','"+one+"'),('service_panel:2','"+two+"'); INSERT INTO services(id,user_id,name,panel_email,sub_id) VALUES(1,2,'One','same','shared'),(2,3,'Two','same','shared')");
 const common={email:'same',subId:'shared',enable:true,up:0,down:0,totalGB:0,memberships:[{inboundId:3}]};f.accounts().one.push({...common,expiryTime:Date.now()+86400000});f.accounts().two.push({...common,expiryTime:Date.now()-86400000,memberships:[{inboundId:8}]});
 await runSubscriptionPolicies(f.env);assert.deepEqual(f.actions,[['two','delete']]);assert.equal(f.accounts().one.length,1);assert.equal(f.accounts().two.length,0);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services WHERE id=1').get().n,1);assert.equal(f.sql.prepare('SELECT COUNT(*) n FROM services WHERE id=2').get().n,0);
});
test('trial creation also uses its category panel and subscription base',async()=>{
 const f=fixture(),two=await panelAccountKey('https://panel-two.example/base','rep');
 f.sql.prepare('INSERT INTO settings(key,value) VALUES(?,?)').run('extra_panel:p123456789abc',await encryptPanel('token',{name:'پنل ۲',url:'https://panel-two.example/base',user:'rep',pass:'pass',sub:'https://sub-two.example/sub/'}));
 f.sql.prepare("INSERT INTO bot_categories(id,name,group_ids,panel_identity) VALUES(1,'Test','1',?)").run(two);f.sql.exec("INSERT INTO settings(key,value) VALUES('test_category_id','1'),('test_enabled','1')");
 await f.cb(4,'test_service');await f.cb(4,'testcat:1:0');await f.cb(4,'testname:custom');await f.msg(4,'Trial');await f.cb(4,'testname:make');assert.deepEqual(f.actions[0],['two','add','Trial']);assert.match(f.sql.prepare('SELECT sub_link FROM services WHERE user_id=4').get().sub_link,/^https:\/\/sub-two\.example/);
});
test('panel manager shows status and groups, supports HTTP and keeps bindings when address changes',async()=>{
 const f=fixture();await f.cb(1,'admin:multipanel_add');await f.msg(1,'پنل ۲');await f.msg(1,'http://panel-two.example/base');await f.msg(1,'rep');await f.msg(1,'pass');await f.msg(1,'http://sub-two.example/sub/');
 const row=f.sql.prepare("SELECT key FROM settings WHERE key LIKE 'extra_panel:%'").get(),slot=row.key.split(':')[1],old=await panelAccountKey('http://panel-two.example/base','rep');
 f.sql.prepare("INSERT INTO bot_categories(id,name,group_ids,panel_identity) VALUES(1,'Test','1',?)").run(old);await f.cb(1,`admin:multipanel:${slot}`);assert.match(f.last().text,/متصل/);assert.match(f.last().text,/تعداد گروه‌ها: 1/);assert.ok(JSON.stringify(f.last().reply_markup).includes(`admin:mpurl:${slot}`));
 await f.cb(1,`admin:mpurl:${slot}`);await f.msg(1,'https://panel-two.example/base');const identity=await panelAccountKey('https://panel-two.example/base','rep');assert.equal(f.sql.prepare('SELECT panel_identity FROM bot_categories WHERE id=1').get().panel_identity,identity);
 await f.cb(1,`admin:multipanel:${slot}`);assert.match(f.last().text,/https:\/\/panel-two/);await f.cb(1,'admin:settings');assert.ok(!JSON.stringify(f.last().reply_markup).includes('admin:setting:sub_base_url'));
});
test('panel deletion is refused while services or active orders are bound to it',async()=>{
 const f=fixture(),two=await panelAccountKey('https://panel-two.example/base','rep');f.sql.prepare('INSERT INTO settings(key,value) VALUES(?,?)').run('extra_panel:p123456789abc',await encryptPanel('token',{name:'پنل ۲',url:'https://panel-two.example/base',user:'rep',pass:'pass',sub:'https://sub-two.example/sub/'}));
 f.sql.prepare("INSERT INTO orders(id,user_id,status) VALUES(1,2,'pending')").run();f.sql.prepare("INSERT INTO bot_order_routes(order_id,group_ids,anchor_id,panel_identity) VALUES(1,'1',0,?)").run(two);
 await f.cb(1,'admin:mpdeleteyes:p123456789abc');assert.ok(f.sql.prepare("SELECT value FROM settings WHERE key='extra_panel:p123456789abc'").get());
 f.sql.exec("UPDATE orders SET status='approved'; INSERT INTO services(id,user_id,name,panel_email,sub_id) VALUES(1,2,'Alice','Alice','sid')");f.sql.prepare("INSERT INTO settings(key,value) VALUES('service_panel:1',?)").run(two);
 await f.cb(1,'admin:mpdeleteyes:p123456789abc');assert.ok(f.sql.prepare("SELECT value FROM settings WHERE key='extra_panel:p123456789abc'").get());
 f.sql.exec('DELETE FROM services');await f.cb(1,'admin:mpdeleteyes:p123456789abc');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='extra_panel:p123456789abc'").get(),undefined);
});
test('main panel uses the same manager and removal keeps it disabled despite environment defaults',async()=>{
 const f=fixture();await f.cb(1,'admin:multipanels');assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:multipanel:main'));
 await f.cb(1,'admin:multipanel:main');assert.match(f.last().text,/تعداد گروه‌ها: 1/);assert.ok(JSON.stringify(f.last().reply_markup).includes('admin:mpdelete:main'));
 await f.cb(1,'admin:mpdeleteyes:main');assert.equal(f.sql.prepare("SELECT value FROM settings WHERE key='panel_main_disabled'").get().value,'1');await f.cb(1,'admin:multipanels');assert.ok(!JSON.stringify(f.last().reply_markup).includes('admin:multipanel:main'));
});
