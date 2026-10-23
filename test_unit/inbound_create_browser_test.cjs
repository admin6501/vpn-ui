'use strict';
const assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',args:['--no-sandbox']});
 try{
  const context=await browser.newContext({viewport:{width:1280,height:900}});
  const base=process.env.INBOUND_CREATE_URL;
  await context.addCookies([{url:base,name:'lang',value:process.env.NODE_UI_LANG}]);
  const page=await context.newPage();page.setDefaultTimeout(10000);const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const api=async(path,form)=>{const response=form===undefined?await context.request.get(base+path):await context.request.post(base+path,{form});const body=await response.json();assert.equal(body.success,true,JSON.stringify(body));return body.obj;};
  assert.deepEqual(await api('panel/groups/list'),[],'fresh panel has no groups');
  await page.goto(base+'panel/inbounds',{waitUntil:'networkidle'});
  await page.waitForFunction(()=>typeof app!=='undefined'&&Array.isArray(app.dbInbounds)&&app.loadingStates.fetched);
  async function create(){
   await page.evaluate(()=>{app.openAddInbound();inModal.inbound.protocol='shadowsocks';inModal.inbound.port=46741;inModal.dbInbound.remark='reusable-listener';inModal.dbInbound.enable=false;inModal.section='proto';});
   const modal=page.locator('#inbound-modal .ant-modal');await modal.waitFor();
   // The real Ant Select invokes the actual encryption-change handler.
   await modal.locator('[data-testid="inbound-shadowsocks-cipher"] .ant-select-selection').click();
   await page.locator('.ant-select-dropdown:visible .ant-select-dropdown-menu-item').filter({hasText:/^CHACHA20_IETF_POLY1305$/}).click();
   assert.equal(await page.evaluate(()=>inModal.inbound.settings.toJson().clients.length),0,'cipher selection cannot invent a default user');
   const response=page.waitForResponse(r=>r.url().endsWith('/panel/api/inbounds/add'));
   await modal.locator('.ant-modal-footer button.ant-btn-primary').click();
   const body=await(await response).json();assert.equal(body.success,true,JSON.stringify(body));
   assert.equal(JSON.parse(body.obj.settings).clients.length,0);
   await page.waitForFunction(()=>!inModal.visible);
   return body.obj;
  }
  const first=await create();assert.deepEqual(await api('panel/groups/list'),[],'listener can be created before groups');
  const group=await api('panel/groups/save',{name:'customer-group',inboundIds:String(first.id)});
  await api('panel/api/inbounds/saveAccount',{groupIds:String(group.id),settings:JSON.stringify({clients:[{email:'kept-browser-customer',id:'11111111-1111-4111-8111-111111111111',password:'kept-password',subId:'kept-browser-sub',totalGB:10737418240,enable:true}]})});
  const before=(await api('panel/api/clients/list')).rows.find(x=>x.email==='kept-browser-customer');assert.equal(before.memberships.length,1);
  await api('panel/api/inbounds/del/'+first.id,{});
  await page.evaluate(()=>app.getDBInbounds());
  const second=await create();assert.notEqual(first.id,second.id);
  assert.equal((await api('panel/groups/list'))[0].inboundIds.length,0,'old group does not silently acquire a different listener');
  await api('panel/groups/save',{id:String(group.id),name:group.name,inboundIds:String(second.id)});
  const after=(await api('panel/api/clients/list')).rows.find(x=>x.email==='kept-browser-customer');
  assert.equal(after.id,before.id);assert.equal(after.subId,before.subId);assert.equal(after.totalGB,before.totalGB);
  assert.equal(after.memberships.length,1);assert.equal(after.memberships[0].inboundId,second.id);
  assert.deepEqual(errors,[]);
  console.log('Fresh group-free creation, real cipher selection, grouped client, delete/recreate same listener and restored group assignment passed');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
