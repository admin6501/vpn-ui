'use strict';
const assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',args:['--no-sandbox']});
 try {
  const page=await browser.newPage({viewport:{width:1280,height:900}});page.setDefaultTimeout(15000);
  const errors=[];page.on('pageerror',e=>errors.push(String(e)));
  await page.goto(process.env.GROUPS_URL,{waitUntil:'networkidle'});
  await page.waitForFunction(()=>typeof app!=='undefined'&&Array.isArray(app.groups)&&app.groups.length===3);
  await page.getByText('Original backup group',{exact:true}).waitFor();
  await page.getByText('PasarGuard #27',{exact:true}).waitFor();
  await page.evaluate(()=>app.edit(app.groups[0]));
  await page.locator('.ant-modal:visible input').first().fill('Shared group');
  // Use the real Ant Select to include the second inbound.
  await page.locator('.ant-modal:visible .ant-select-selection').click();
  await page.locator('.ant-select-dropdown-menu-item').filter({hasText:'Second inbound'}).click();
  await page.keyboard.press('Escape');
  const saved=page.waitForResponse(r=>r.url().endsWith('/groups/save'));
  await page.locator('.ant-modal:visible .ant-modal-footer .ant-btn-primary').click();
  assert.equal((await(await saved).json()).success,true);
  await page.waitForFunction(()=>!app.form.visible&&app.groups[0].name==='Shared group'&&app.groups[0].inboundIds.length===2);
  const base=process.env.GROUPS_URL.replace(/panel\/groups$/,'');
  assert.ok((await page.title()).startsWith('برند Nava & Friends'));
  assert.match(await page.locator('.bo-rail-brand').innerText(), /برند Nava & Friends/);
  const list=await page.evaluate(async base=>(await(await fetch(base+'panel/api/clients/list')).json()).obj,base);
  const original=list.rows.find(x=>x.email==='existing-user');assert.equal(original.memberships.length,2);assert.equal(original.groupIds.length,1);
  await page.goto(base+'panel/clients',{waitUntil:'networkidle'});
  await page.waitForFunction(()=>typeof app!=='undefined'&&Array.isArray(app.clients)&&app.clients.some(x=>x.email==='existing-user')&&app.assignable.length===3);
  await page.evaluate(()=>app.openAdd());
  await page.locator('#client-membership-modal .ant-modal').waitFor();
  await page.locator('#client-membership-modal input').first().fill('new-group-user');
  await page.evaluate(()=>clientMembershipModal.tab='inbounds');
  const choice=page.locator('#client-membership-modal .bo-cf-ib').filter({hasText:'Shared group'});
  await choice.locator('input[type=checkbox]').check();
  assert.equal(await page.evaluate(()=>clientMembershipModal.selected.length),2);
  assert.equal(await page.locator('#client-membership-modal .bo-cf-ib').count(),3);
  const created=page.waitForResponse(r=>r.url().endsWith('/inbounds/saveAccount'));
  await page.locator('#client-membership-modal .ant-modal-footer .ant-btn-primary').click();
  const response=await(await created).json();assert.equal(response.success,true,JSON.stringify(response));
  await page.waitForFunction(()=>!clientMembershipModal.visible);
  await page.reload({waitUntil:'networkidle'});
  await page.waitForFunction(()=>Array.isArray(app.clients)&&app.clients.some(x=>x.email==='new-group-user'&&x.memberships.length===2&&x.groupIds.length===1));
  for (const enable of [false,true]) {
   const saved=page.waitForResponse(r=>r.url().endsWith('/inbounds/saveAccount'));
   await page.evaluate(enable=>app.toggleEnable(app.clients.find(x=>x.email==='new-group-user'),enable),enable);
   const response=await(await saved).json();assert.equal(response.success,true,JSON.stringify(response));
   await page.waitForFunction(enable=>app.clients.some(x=>x.email==='new-group-user'&&x.enable===enable&&x.memberships.length===2&&x.groupIds.length===1),enable);
  }
  await page.request.post(base+'fixtures/traffic');
  await page.reload({waitUntil:'networkidle'});
  await page.waitForFunction(()=>app.clients.some(x=>x.email==='new-group-user'&&x.up===1024&&x.down===2048));
  await page.setViewportSize({width:390,height:844});
  await page.evaluate(()=>app.openEdit(app.clients.find(x=>x.email==='new-group-user')));
  const edit=page.locator('#client-membership-modal .ant-modal');await edit.waitFor();
  const bounds=await edit.boundingBox(), button=await edit.getByTestId('client-edit-reset-traffic').boundingBox();
  assert.ok(button&&button.x>=bounds.x&&button.x+button.width<=bounds.x+bounds.width,'reset button fits the mobile edit form');
  await page.evaluate(()=>{clientMembershipModal.totalGb=17;clientMembershipModal.client.email='unsaved-name';});
  let resets=0;page.on('request',r=>{if(r.url().includes('/resetClientTraffic/'))resets++;});
  await edit.getByTestId('client-edit-reset-traffic').click();
  const confirm=page.locator('.ant-modal-confirm .ant-modal-content');await confirm.waitFor();
  await confirm.locator('.ant-modal-confirm-btns button').first().click();
  assert.equal(resets,0,'cancelling the confirmation must not reset anything');
  await edit.getByTestId('client-edit-reset-traffic').click();await confirm.waitFor();
  const reset=page.waitForResponse(r=>r.url().endsWith('/resetClientTraffic/new-group-user'));
  await confirm.locator('.ant-modal-confirm-btns .ant-btn-primary').click();
  assert.equal((await(await reset).json()).success,true);
  await page.waitForFunction(()=>!clientMembershipModal.resettingTraffic&&app.clients.some(x=>x.email==='new-group-user'&&x.up===0&&x.down===0));
  assert.equal(resets,1,'one account reset across all memberships');
  assert.equal(await edit.isVisible(),true,'reset keeps the edit form open');
  assert.deepEqual(await page.evaluate(()=>[clientMembershipModal.totalGb,clientMembershipModal.client.email]),[17,'unsaved-name'],'reset preserves unsaved edits');
  await page.evaluate(()=>clientMembershipModal.close());
  await page.setViewportSize({width:1280,height:900});
  const ownership=await page.request.post(base+'fixtures/representative');assert.equal(ownership.ok(),true);
  for (const actor of ['delegate','representative']) {
   await page.setExtraHTTPHeaders({'X-Test-Actor':actor});
   await page.reload({waitUntil:'networkidle'});
   await page.waitForFunction(()=>Array.isArray(app.clients)&&app.clients.some(x=>x.email==='new-group-user'));
   for (const enable of [false,true]) {
    const saved=page.waitForResponse(r=>r.url().includes('/inbounds/updateClient/'));
    await page.evaluate(enable=>app.toggleEnable(app.clients.find(x=>x.email==='new-group-user'),enable),enable);
    const response=await(await saved).json();assert.equal(response.success,true,JSON.stringify({actor,response}));
    await page.waitForFunction(enable=>app.clients.some(x=>x.email==='new-group-user'&&x.enable===enable&&x.memberships.length===2&&x.groupIds.length===1),enable);
   }
  }
  const hostile="<b>[[constructor.constructor('window.brandExecuted=1')()]]</b>";
  await page.request.post(base+'fixtures/brand',{form:{brandName:hostile}});
  await page.setExtraHTTPHeaders({'X-Test-Actor':'anonymous'});
  await page.goto(base,{waitUntil:'networkidle'});
  assert.equal(await page.locator('.lgt-brand').innerText(),'['+hostile+']');
  assert.equal(await page.evaluate(()=>window.brandExecuted),undefined,'brand text cannot execute HTML or Vue expressions');
  assert.ok((await page.title()).startsWith(hostile));
  assert.deepEqual(errors,[]);
  console.log('Group membership and account controls passed; mobile edit-form reset preserves drafts and lifetime usage; custom branding renders safely in English and Persian.');
 } finally {await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
