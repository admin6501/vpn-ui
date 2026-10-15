'use strict';
const assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',args:['--no-sandbox']});
 try{
  const page=await browser.newPage({viewport:{width:1280,height:900}});page.setDefaultTimeout(10000);
  const errors=[];page.on('pageerror',e=>errors.push(String(e)));
  await page.goto(process.env.REPRESENTATIVE_EDIT_URL,{waitUntil:'networkidle'});
  await page.waitForFunction(()=>typeof app!=='undefined'&&Array.isArray(app.resellers)&&app.resellers.length===1);
  await page.evaluate(()=>app.openEdit(app.resellers[0]));
  await page.locator('#reseller-modal .ant-modal').waitFor();
  if(process.env.REPRESENTATIVE_EDIT_KIND==='owner'){
   await page.waitForFunction(()=>resellerModal.availableGroups.length===2);
   await page.locator('#reseller-modal .ant-checkbox-wrapper').filter({hasText:'Additional group'}).locator('input[type=checkbox]').check();
   assert.equal(await page.evaluate(()=>resellerModal.reseller.groupIds.length),2);
  }
  await page.locator('#reseller-modal input').nth(2).fill('Edited in browser');
  await page.evaluate(()=>resellerModal.reseller.twoFactorRequired=true);
  const initial=await page.evaluate(()=>({...resellerModal.reseller}));
  await page.locator('#reseller-modal input').nth(0).fill('delegated-manager');
  const rejected=page.waitForResponse(r=>r.url().includes('/resellers/update/'));
  await page.locator('#reseller-modal .ant-modal-footer button.ant-btn-primary').click();
  const failure=await(await rejected).json();
  assert.equal(failure.success,false);
  await page.locator('#reseller-modal .ant-alert-error').filter({hasText:failure.msg}).waitFor();
  assert.equal(await page.evaluate(()=>resellerModal.visible&&!resellerModal.confirmLoading),true);
  assert.equal(await page.locator('#reseller-modal input').nth(2).inputValue(),'Edited in browser');
  await page.locator('#reseller-modal input').nth(0).fill(initial.username);
  const response=page.waitForResponse(r=>r.url().includes('/resellers/update/'));
  await page.locator('#reseller-modal .ant-modal-footer button.ant-btn-primary').click();
  const savedResponse=await response;
  const payload=new URLSearchParams(savedResponse.request().postData());
  assert.equal(payload.has('inboundIds'),false);assert.equal(payload.has('groupIds'),true);
  const result=await savedResponse.json();
  assert.equal(result.success,true,JSON.stringify({result,errors}));
  await page.waitForFunction(()=>!resellerModal.visible&&app.resellers[0].nickname==='Edited in browser'&&app.resellers[0].twoFactorRequired);
  await page.reload({waitUntil:'networkidle'});
  await page.waitForFunction(()=>Array.isArray(app.resellers)&&app.resellers[0]&&app.resellers[0].nickname==='Edited in browser'&&app.resellers[0].twoFactorRequired);
  if(process.env.REPRESENTATIVE_EDIT_KIND==='owner'){
   const groups=await page.evaluate(()=>app.allGroups);
   assert.equal(groups.length,2);
   const originalRole=await page.evaluate(()=>resellerModal.roles[0]);
   await page.evaluate(()=>app.openAdd());
   await page.waitForFunction(()=>resellerModal.visible&&!resellerModal.rolesLoading);
   assert.deepEqual(await page.evaluate(()=>resellerModal.reseller.groupIds),groups.map(g=>g.id),'the sole default role must select its groups');
   await page.evaluate(()=>resellerModal.close());
   const roles=await page.evaluate(async ids=>{
    const result=[];
    for(const [name,allowed] of [['Restricted A',[ids[0]]],['Restricted B',[ids[1]]],['No groups',[]]]){
     const msg=await HttpUtil.post('/panel/resellers/roles/add',{
      name,permissions:JSON.stringify({users:{read:{scope:1},create:true,update:{scope:1}}}),
      limits:'{}',features:'{}',access:JSON.stringify({allowed_group_ids:allowed}),
     });
     if(!msg.success)throw Error(JSON.stringify(msg));
     result.push(msg.obj);
    }
    return result;
   },groups.map(g=>g.id));
   await page.evaluate(()=>app.openAdd());
   await page.waitForFunction(()=>resellerModal.visible&&!resellerModal.rolesLoading&&resellerModal.roles.length===4);
   const choose=async name=>{
    await page.locator('#reseller-modal .ant-select').click();
    await page.locator('.ant-select-dropdown:visible .ant-select-dropdown-menu-item').filter({hasText:name}).click();
   };
   await choose(roles[0].name);
   assert.deepEqual(await page.evaluate(()=>resellerModal.reseller.groupIds),[groups[0].id]);
   assert.equal(await page.locator('#reseller-modal .ant-checkbox-wrapper').filter({hasText:groups[0].name}).locator('input').isChecked(),true);
   await page.locator('#reseller-modal .ant-checkbox-wrapper').filter({hasText:groups[0].name}).locator('input').uncheck();
   await choose(roles[1].name);
   assert.deepEqual(await page.evaluate(()=>resellerModal.reseller.groupIds),[groups[1].id],'switching roles must replace previous defaults');
   assert.equal(await page.locator('#reseller-modal .ant-checkbox-wrapper').filter({hasText:groups[1].name}).locator('input').isChecked(),true);
   await choose(roles[2].name);
   assert.deepEqual(await page.evaluate(()=>resellerModal.reseller.groupIds),[]);
   await choose(originalRole.name);
   assert.deepEqual(await page.evaluate(()=>resellerModal.reseller.groupIds),groups.map(g=>g.id));
   await page.locator('#reseller-modal .ant-checkbox-wrapper').filter({hasText:groups[1].name}).locator('input').uncheck();
   await page.evaluate(()=>{resellerModal.reseller.username='role-group-defaults';resellerModal.reseller.password='role-group-password'});
   const created=page.waitForResponse(r=>r.url().endsWith('/resellers/add'));
   await page.locator('#reseller-modal .ant-modal-footer button.ant-btn-primary').click();
   const createdResponse=await created;
   assert.equal((await createdResponse.json()).success,true);
   assert.deepEqual(new URLSearchParams(createdResponse.request().postData()).getAll('groupIds'),[String(groups[0].id)]);
   await page.waitForFunction(()=>!resellerModal.visible&&app.resellers.some(r=>r.username==='role-group-defaults'));
   await page.reload({waitUntil:'networkidle'});
   await page.waitForFunction(()=>app.resellers&&app.resellers.some(r=>r.username==='role-group-defaults'));
   await page.evaluate(()=>app.openEdit(app.resellers.find(r=>r.username==='role-group-defaults')));
   await page.waitForFunction(()=>resellerModal.visible&&!resellerModal.rolesLoading);
   assert.deepEqual(await page.evaluate(()=>resellerModal.reseller.groupIds),[groups[0].id],'opening an existing representative must preserve the manually chosen subset');
   assert.equal(await page.locator('#reseller-modal .ant-checkbox-wrapper').filter({hasText:groups[1].name}).locator('input').isChecked(),false);
  }
  assert.deepEqual(errors,[]);
  console.log('Representative edit saved through the actual confirm button: '+process.env.REPRESENTATIVE_EDIT_KIND);
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
