import test from 'node:test';
import assert from 'node:assert/strict';
import worker, {panelCreate, panelRenew, panelDelete} from '../worker.js';
const json=(obj,status=200,headers={})=>new Response(JSON.stringify(obj),{status,headers:{'content-type':'application/json',...headers}});
const env={PANEL_URL:'https://panel.example/base',PANEL_USER:'reseller',PANEL_PASS:'secret',cfg:{panel_group_ids:'1',panel_inbound_id:'3',sub_base_url:'https://sub.example/sub/'}};
function server({groups=[{id:1,inboundIds:[3,4]}],inbounds=[{inboundId:3,protocol:'vless'},{inboundId:4,protocol:'shadowsocks',method:'aes-128-gcm'}],account,deleteResult={success:true},createResult={success:true},multiSafe=false}={}) {
  let calls=[];
  globalThis.fetch=async (url,opt={})=>{const path=new URL(url).pathname;calls.push({path,body:opt.body});
    if(path.endsWith('/login')) return json({success:true},200,{'set-cookie':'vpn-ui=example; Path=/; HttpOnly'});
    if(path.endsWith('/multiDeleteCapability')) return json({success:true,obj:{preservesSharedTraffic:multiSafe}});
    if(path.endsWith('/clients/groups')) return json({success:true,obj:groups});
    if(path.endsWith('/clients/assignable')) return json({success:true,obj:inbounds});
    if(path.endsWith('/addClient')) return json(createResult);
    if(path.endsWith('/renewClient/customer')) return json({success:true});
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
test('renewal only accepts existing package and does not change quota',async()=>{
 let calls=server();let svc={panel_email:'customer',volume_bytes:10,duration:'۱ ماهه (۳۰ روزه)'};
 await assert.rejects(panelRenew(env,svc,20,30),/همان حجم/);assert.equal(calls.length,0);
 await panelRenew(env,svc,10,30);assert.equal(calls.filter(x=>x.path.includes('renewClient')).length,1);assert.ok(!calls.some(x=>x.path.includes('bulkUpdate')));
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
 let stored=[];const withDB={...env,DB:{prepare:()=>({bind:(...args)=>({run:async()=>{stored.push(args);return {meta:{changes:1}}}})})}};
 const calls=server();const original=globalThis.fetch;
 globalThis.fetch=async(url,opt)=>{if(new URL(url).pathname.endsWith('/addClient')){calls.push({path:new URL(url).pathname});throw Error('connection lost')};return original(url,opt)};
 await assert.rejects(panelCreate(withDB,{name:'Amir',bytes:1,days:1,attemptKey:'order:19'}));
 assert.equal(stored.length,1);assert.equal(stored[0][0],'order:19');assert.ok(stored[0][1].startsWith('Amir_'));assert.equal(calls.filter(x=>x.path.endsWith('addClient')).length,1);
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
