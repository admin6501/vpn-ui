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
  assert.deepEqual(errors,[]);
  console.log('Representative edit saved through the actual confirm button: '+process.env.REPRESENTATIVE_EDIT_KIND);
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
