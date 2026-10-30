import test from 'node:test';
import assert from 'node:assert/strict';
import worker, {panelCreate, panelRenew, panelDelete} from '../worker.js';
const json=(obj,status=200,headers={})=>new Response(JSON.stringify(obj),{status,headers:{'content-type':'application/json',...headers}});
const env={PANEL_URL:'https://panel.example/base',PANEL_USER:'reseller',PANEL_PASS:'secret',cfg:{panel_group_ids:'1',panel_inbound_id:'3',sub_base_url:'https://sub.example/sub/'}};
function server({groups=[{id:1,inboundIds:[3,4]}],inbounds=[{inboundId:3,protocol:'vless'},{inboundId:4,protocol:'shadowsocks',method:'aes-128-gcm'}],account,deleteResult={success:true},createResult={success:true},multiSafe=false}={}) {
  let calls=[];
  globalThis.fetch=async (url,opt={})=>{const path=new URL(url).pathname;calls.push({path,body:opt.body});
    if(path.endsWith('/login')) return json({success:true},200,{'set-cookie':'vpn-ui=example; Path=/; HttpOnly'});
    if(path.endsWith('/resellerBalance'))return json({success:true,obj:{isReseller:false}});
    if(path.endsWith('/multiDeleteCapability')) return json({success:true,obj:{preservesSharedTraffic:multiSafe}});
    if(path.endsWith('/clients/groups')) return json({success:true,obj:groups});
    if(path.endsWith('/clients/assignable')) return json({success:true,obj:inbounds});
    if(path.endsWith('/addClient')) return json(createResult);
    if(path.endsWith('/clients/inbounds'))return json({success:true,obj:account?.memberships?.map(m=>({id:m.inboundId,protocol:m.protocol,settings:JSON.stringify({clients:[{id:m.clientId,email:account.email,subId:account.subId,enable:account.enable,totalGB:account.totalGB,expiryTime:account.expiryTime,password:'keep-secret',custom:'keep-field'}]})}))||[]});
    if(path.includes('/updateClient/')){const client=JSON.parse(new URLSearchParams(opt.body).get('settings')).clients[0];Object.assign(account,{totalGB:client.totalGB,expiryTime:client.expiryTime,enable:client.enable});return json({success:true})}
    if(path.endsWith('/clients/list')) return json({success:true,obj:{rows:account?[account]:[]}});
    if(path.includes('/delClientByEmail/')) { if (deleteResult.success) { const id=Number(path.split('/').at(-3)); account.memberships=account.memberships.filter(m=>m.inboundId!==id);if(!account.memberships.length)account=null; } return json(deleteResult); }
    throw Error('Unexpected test request '+path);
  };return calls;
}

test('mixed VLESS and Shadowsocks group: complete credentials sent and proper form',async()=>{
  const calls=server();let result=await panelCreate(env,{name:'Amir',bytes:1073741824,days:30});
  const form=new URLSearchParams(calls.find(c=>c.path.endsWith('/addClient')).body);
  let client=JSON.parse(form.get('settings')).clients[0];
  assert.match(client.id,/^[0-9a-f-]{36}$/);assert.equal(client.uuid,client.id);assert.ok(client.password.length>=20);
  assert.equal(form.get('groupIds'),'1');assert.equal(form.get('id'),'3');assert.equal(client.totalGB,1073741824);assert.equal(client.renewalDuration,30*86400000);assert.ok(result.link.startsWith('https://sub.example/sub/'));
});
test('unsupported Shadowsocks 2022 chacha is rejected before any write',async()=>{
 const calls=server({inbounds:[{inboundId:3,protocol:'vless'},{inboundId:4,protocol:'shadowsocks',method:'2022-blake3-chacha20-poly1305'}]});
 await assert.rejects(panelCreate(env,{name:'Amir',bytes:1,days:1}),/2022/);assert.ok(!calls.some(x=>x.path.endsWith('addClient')));
});
test('unknown or inaccessible group member and wrong anchor are rejected',async()=>{
 let calls=server({inbounds:[{inboundId:3,protocol:'vless'}]});await assert.rejects(panelCreate(env,{name:'Amir',bytes:1,days:1}),/دسترس/);assert.ok(!calls.some(x=>x.path.endsWith('addClient')));
 calls=server({groups:[{id:1,inboundIds:[4]}],inbounds:[{inboundId:4,protocol:'vmess'}]});await assert.rejects(panelCreate(env,{name:'Amir',bytes:1,days:1}),/عضو/);assert.ok(!calls.some(x=>x.path.endsWith('addClient')));
});
test('early renewal adds the selected package while retaining usage and client fields',async()=>{
 const expiry=Date.now()+7*86400000,account={email:'customer',subId:'sid',enable:true,totalGB:10,expiryTime:expiry,up:2,down:1,memberships:[{inboundId:3,protocol:'vless',clientId:'old-id'}]};
 let calls=server({account});let svc={panel_email:'customer',sub_id:'sid'};
 await panelRenew(env,svc,5,30);const edits=calls.filter(x=>x.path.includes('/updateClient/'));assert.equal(edits.length,1);
 const client=JSON.parse(new URLSearchParams(edits[0].body).get('settings')).clients[0];assert.equal(client.totalGB,15);assert.equal(client.expiryTime,expiry+30*86400000);assert.equal(client.subId,'sid');assert.equal(client.password,'keep-secret');assert.equal(client.custom,'keep-field');assert.equal(account.up+account.down,3);assert.ok(!calls.some(x=>x.path.includes('renewClient')||x.path.includes('resetClient')));
});
test('expired and overused service receives the full new allowance; unlimited stays unlimited',async()=>{
 const now=Date.now(),base={email:'customer',subId:'sid',enable:false,totalGB:10,expiryTime:now-86400000,up:8,down:4,memberships:[{inboundId:3,protocol:'vless',clientId:'old-id'}]};
 server({account:base});await panelRenew(env,{panel_email:'customer',sub_id:'sid'},5,7);assert.equal(base.totalGB,17);assert.ok(base.expiryTime>=now+7*86400000);assert.equal(base.enable,true);assert.equal(base.up+base.down,12);
 const unlimited={email:'customer',subId:'sid',enable:true,totalGB:0,expiryTime:0,up:8,down:4,memberships:[{inboundId:3,protocol:'vless',clientId:'old-id'}]};server({account:unlimited});await panelRenew(env,{panel_email:'customer',sub_id:'sid'},5,7);assert.equal(unlimited.totalGB,0);assert.equal(unlimited.expiryTime,0);
});
test('a changed panel value prevents renewal before dispatch',async()=>{
 const expiry=Date.now()+86400000,account={email:'customer',subId:'sid',enable:true,totalGB:10,expiryTime:expiry,up:0,down:0,memberships:[{inboundId:3,protocol:'vless',clientId:'old-id'}]};
 const calls=server({account}),realFetch=globalThis.fetch;globalThis.fetch=async(url,opt)=>{if(new URL(url).pathname.endsWith('/clients/inbounds')){const r=await realFetch(url,opt);account.totalGB=11;return r}return realFetch(url,opt)};
 let dispatched=false;await assert.rejects(panelRenew(env,{panel_email:'customer',sub_id:'sid'},5,7,()=>{dispatched=true}),/تغییر کرد/);assert.equal(dispatched,false);assert.ok(!calls.some(x=>x.path.includes('/updateClient/')));
});
test('renewal addresses each managed protocol by the panel client identity',async()=>{
 const passwordKey=new Set(['trojan','l2tp','pptp','openvpn','openconnect','sstp','ikev2','anytls','naive']),authKey=new Set(['hysteria','hysteria2']);
 const protocols=['vless','vmess','shadowsocks','trojan','anytls','tuic','naive','hysteria','hysteria2','l2tp','pptp','openvpn','openconnect','sstp','ikev2','ssh','wg-c','awg','wireguard','gre','mtproto'];
 for(const protocol of protocols){
  const expiry=Date.now()+86400000,client={id:'client-id',password:'password-key',auth:'auth-key',email:'customer',subId:'sid',enable:true,totalGB:10,expiryTime:expiry,custom:'preserved'},key=passwordKey.has(protocol)?client.password:authKey.has(protocol)?client.auth:protocol==='shadowsocks'?client.email:client.id;
  const account={email:'customer',subId:'sid',enable:true,totalGB:10,expiryTime:expiry,up:1,down:2,memberships:[{inboundId:3,protocol,clientId:key}]};let edited=0;
  globalThis.fetch=async(url,opt={})=>{const p=new URL(url).pathname;if(p.endsWith('/login'))return json({success:true},200,{'set-cookie':'vpn-ui=example'});if(p.endsWith('/resellerBalance'))return json({success:true,obj:{isReseller:false}});if(p.endsWith('/clients/list'))return json({success:true,obj:{rows:[account],total:1}});if(p.endsWith('/clients/inbounds'))return json({success:true,obj:[{id:3,protocol,settings:JSON.stringify({clients:[client]})}]});if(p.includes('/updateClient/')){assert.equal(decodeURIComponent(p.split('/').at(-1)),key,protocol);const next=JSON.parse(new URLSearchParams(opt.body).get('settings')).clients[0];assert.equal(next.custom,'preserved',protocol);assert.equal(next.password,client.password,protocol);assert.equal(next.auth,client.auth,protocol);Object.assign(account,{totalGB:next.totalGB,expiryTime:next.expiryTime,enable:next.enable});edited++;return json({success:true})}throw Error('unexpected endpoint')};
  await panelRenew(env,{panel_email:'customer',sub_id:'sid'},5,7);assert.equal(edited,1,protocol);assert.equal(account.totalGB,15,protocol);
 }
});
test('slash password uses email-based client edit after checking capability',async()=>{
 const expiry=Date.now()+86400000,client={email:'customer',password:'secret/part',subId:'sid',enable:true,totalGB:10,expiryTime:expiry},account={email:'customer',subId:'sid',enable:true,totalGB:10,expiryTime:expiry,up:1,down:2,memberships:[{inboundId:3,protocol:'trojan',clientId:'secret/part'}]};let edits=0;
 globalThis.fetch=async(url,opt={})=>{const p=new URL(url).pathname;if(p.endsWith('/login'))return json({success:true},200,{'set-cookie':'vpn-ui=example'});if(p.endsWith('/resellerBalance'))return json({success:true,obj:{isReseller:false}});if(p.endsWith('/clients/list'))return json({success:true,obj:{rows:[account],total:1}});if(p.endsWith('/clients/inbounds'))return json({success:true,obj:[{id:3,protocol:'trojan',settings:JSON.stringify({clients:[client]})}]});if(p.endsWith('/updateClientByEmailCapability'))return json({success:true,obj:{supported:true}});if(p.endsWith('/updateClientByEmail')){const next=JSON.parse(new URLSearchParams(opt.body).get('settings')).clients[0];assert.equal(next.password,'secret/part');Object.assign(account,{totalGB:next.totalGB,expiryTime:next.expiryTime,enable:next.enable});edits++;return json({success:true})}throw Error('unexpected '+p)};
 await panelRenew(env,{panel_email:'customer',sub_id:'sid'},5,7);assert.equal(edits,1);assert.equal(account.totalGB,15);
});
test('slash password refuses an old panel before submitting the edit',async()=>{
 const expiry=Date.now()+86400000,client={email:'customer',password:'secret/part',subId:'sid',enable:true,totalGB:10,expiryTime:expiry},account={email:'customer',subId:'sid',enable:true,totalGB:10,expiryTime:expiry,up:1,down:2,memberships:[{inboundId:3,protocol:'trojan',clientId:'secret/part'}]};let dispatched=false;
 globalThis.fetch=async(url)=>{const p=new URL(url).pathname;if(p.endsWith('/login'))return json({success:true},200,{'set-cookie':'vpn-ui=example'});if(p.endsWith('/resellerBalance'))return json({success:true,obj:{isReseller:false}});if(p.endsWith('/clients/list'))return json({success:true,obj:{rows:[account],total:1}});if(p.endsWith('/clients/inbounds'))return json({success:true,obj:[{id:3,protocol:'trojan',settings:JSON.stringify({clients:[client]})}]});if(p.endsWith('/updateClientByEmailCapability'))return json({success:false},404);throw Error('unexpected '+p)};
 await assert.rejects(panelRenew(env,{panel_email:'customer',sub_id:'sid'},5,7,()=>{dispatched=true}),/پنل باید/);assert.equal(dispatched,false);assert.equal(account.totalGB,10);
});
test('representative renewals require a guarded panel and send the expected expiry',async()=>{
 const expiry=Date.now()+86400000,client={id:'client-id',email:'customer',subId:'sid',enable:true,totalGB:10,expiryTime:expiry},account={email:'customer',subId:'sid',enable:true,totalGB:10,expiryTime:expiry,up:1,down:2,memberships:[{inboundId:3,protocol:'vless',clientId:'client-id'}]};let guarded=false,edits=0;
 globalThis.fetch=async(url,opt={})=>{const p=new URL(url).pathname;if(p.endsWith('/login'))return json({success:true},200,{'set-cookie':'vpn-ui=example'});if(p.endsWith('/resellerBalance'))return json({success:true,obj:{isReseller:true}});if(p.endsWith('/clients/list'))return json({success:true,obj:{rows:[account],total:1}});if(p.endsWith('/clients/inbounds'))return json({success:true,obj:[{id:3,protocol:'vless',settings:JSON.stringify({clients:[client]})}]});if(p.endsWith('/updateClientByEmailCapability'))return json({success:true,obj:{supported:true,guardedExpiry:guarded}});if(p.includes('/updateClient/')){const form=new URLSearchParams(opt.body),next=JSON.parse(form.get('settings')).clients[0];assert.equal(Number(form.get('expectedExpiry')),expiry+7*86400000);Object.assign(account,{totalGB:next.totalGB,expiryTime:next.expiryTime,enable:next.enable});edits++;return json({success:true})}throw Error('unexpected '+p)};
 let dispatched=false;await assert.rejects(panelRenew(env,{panel_email:'customer',sub_id:'sid'},5,7,()=>{dispatched=true}),/پنل باید/);assert.equal(dispatched,false);guarded=true;
 await panelRenew(env,{panel_email:'customer',sub_id:'sid'},5,7);assert.equal(edits,1);assert.equal(account.totalGB,15);
});
test('delete refuses missing/multiple memberships and unsuccessful panel response',async()=>{
 let svc={panel_email:'customer',inbound_id:3};let calls=server({account:{email:'customer',memberships:[{inboundId:3},{inboundId:4}]}});
 await assert.rejects(panelDelete(env,svc),/چندورودی/);assert.ok(!calls.some(x=>x.path.includes('delClientByEmail')));
 calls=server({account:{email:'customer',memberships:[{inboundId:3}]},deleteResult:{success:false,msg:'denied'}});
 await assert.rejects(panelDelete(env,svc),/حذف ورودی/);assert.ok(calls.some(x=>x.path.includes('delClientByEmail')));
 calls=server({account:{email:'customer',memberships:[{inboundId:3}]}});await panelDelete(env,svc);assert.ok(calls.some(x=>x.path.includes('delClientByEmail')));
});
test('webhook requires secret and deduplicates update ids',async()=>{
 let set=new Set(),count=0;const db={prepare(){return {bind(id){return {run:async()=>({meta:{changes:set.has(id)?0:(set.add(id),1)}})}}}}};
 const ctx={waitUntil(p){count++;return p}};const e={WEBHOOK_SECRET:'s',DB:db};
 let missing=await worker.fetch(new Request('https://worker.example/webhook',{method:'POST',body:'{}'}),{...e,WEBHOOK_SECRET:''},ctx);assert.equal(missing.status,403);
 const request=()=>new Request('https://worker.example/webhook',{method:'POST',headers:{'X-Telegram-Bot-Api-Secret-Token':'s'},body:JSON.stringify({update_id:42})});
 assert.equal((await worker.fetch(request(),e,ctx)).status,200);assert.equal((await worker.fetch(request(),e,ctx)).status,200);assert.equal(count,1);
 assert.equal((await worker.fetch(new Request('https://worker.example/setup?key=s'),e,ctx)).status,200);assert.equal(count,1);
});

test('Shadowsocks 2022 generates a padded key of the correct size', async()=>{
 for (const [method,size] of [['2022-blake3-aes-128-gcm',16],['2022-blake3-aes-256-gcm',32]]) {
  const calls=server({inbounds:[{inboundId:3,protocol:'vless'},{inboundId:4,protocol:'shadowsocks',method}]});
  await panelCreate(env,{name:'Test',bytes:1,days:1});
  const form=new URLSearchParams(calls.find(c=>c.path.endsWith('/addClient')).body);
  assert.equal(Buffer.from(JSON.parse(form.get('settings')).clients[0].password,'base64').length,size);
 }
});
test('creation records a recoverable identity before dispatch and does not retry an unknown response',async()=>{
 let stored=[];const withDB={...env,DB:{prepare:()=>({bind:(...args)=>({run:async()=>{stored.push(args);return {meta:{changes:1}}}})}),batch:async statements=>Promise.all(statements.map(s=>s.run()))}};
 const calls=server();const original=globalThis.fetch;
 globalThis.fetch=async(url,opt)=>{if(new URL(url).pathname.endsWith('/addClient')){calls.push({path:new URL(url).pathname});throw Error('connection lost')};return original(url,opt)};
 await assert.rejects(panelCreate(withDB,{name:'Amir',bytes:1,days:1,attemptKey:'order:19'}));
 assert.equal(stored.length,2);assert.equal(stored[0][0],'order:19');assert.equal(stored[0][1],'Amir');assert.equal(stored[1][0],'attempt_panel:order:19');assert.equal(calls.filter(x=>x.path.endsWith('addClient')).length,1);
});

test('all 21 managed protocols produce the panel credential fields',async()=>{
 const protocols=['vmess','vless','tuic','shadowsocks','trojan','anytls','naive','l2tp','pptp','openvpn','openconnect','sstp','ikev2','ssh','hysteria','hysteria2','mtproto','wg-c','awg','wireguard','gre'];
 for (const protocol of protocols) {
  const calls=server({groups:[{id:1,inboundIds:[3]}],inbounds:[{inboundId:3,protocol,method:protocol==='shadowsocks'?'aes-128-gcm':''}]});
  await panelCreate(env,{name:'Test',bytes:1,days:1});
  const form=new URLSearchParams(calls.find(c=>c.path.endsWith('/addClient')).body);const c=JSON.parse(form.get('settings')).clients[0];
  assert.match(c.uuid,/^[0-9a-f-]{36}$/);assert.ok(c.password);assert.match(c.vpnUsername,/^u[a-f0-9]+$/);assert.ok(c.auth);assert.match(c.secret,/^[a-f0-9]{32}$/);
  if(['vmess','vless','tuic'].includes(protocol))assert.equal(c.id,c.uuid);
  if(['l2tp','pptp','openvpn','openconnect','sstp','ikev2','ssh'].includes(protocol))assert.equal(c.id,c.vpnUsername);
  if(['mtproto','wg-c','awg','wireguard','gre'].includes(protocol))assert.equal(c.id,c.email);
 }
});

test('multi-inbound deletion verifies capability and removes every membership',async()=>{
 let svc={panel_email:'customer',inbound_id:3};
 let calls=server({multiSafe:true,account:{email:'customer',memberships:[{inboundId:3},{inboundId:4},{inboundId:5}]}});
 await panelDelete(env,svc);const removed=calls.filter(x=>x.path.includes('delClientByEmail'));assert.equal(removed.length,3);
});
test('multi-inbound failure does not report success or remove remaining memberships',async()=>{
 let svc={panel_email:'customer',inbound_id:3};
 let calls=server({multiSafe:true,deleteResult:{success:false},account:{email:'customer',memberships:[{inboundId:3},{inboundId:4}]}});
 await assert.rejects(panelDelete(env,svc),/حذف ورودی/);assert.equal(calls.filter(x=>x.path.includes('delClientByEmail')).length,1);
});
test('live account display combines usage once and lists all memberships',async()=>{
 const {liveServiceDetails}=await import('../worker.js');
 const text=liveServiceDetails({enable:true,up:1024,down:2048,totalGB:1073741824,expiryTime:0,memberships:[{inboundId:3,remark:'A & B',protocol:'vless',enable:true},{inboundId:4,remark:'SS',protocol:'shadowsocks',enable:false}]});
 assert.match(text,/3\.00 KB/);assert.match(text,/A &amp; B/);assert.match(text,/ورودی‌ها \(2\)/);assert.match(text,/خاموش/);
});
