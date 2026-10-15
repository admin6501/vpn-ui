'use strict';
const assert = require('node:assert/strict');
const {chromium} = require('playwright');
(async () => {
 const browser = await chromium.launch({headless:true, executablePath:process.env.NODE_UI_CHROME || undefined, args:['--no-sandbox']});
 try {
  const page = await browser.newPage({viewport:{width:1280,height:900}});
  const errors=[]; const failedAssets=[];
  page.on('pageerror',e=>errors.push(String(e)));
  page.on('response',r=>{if(r.url().includes('/assets/') && r.status()>=400) failedAssets.push(r.url())});
  const base=process.env.SECURITY_UI_BASE;
  const required=process.env.SECURITY_UI_REQUIRED==='true';
  // Only the final landing URL is relevant here; account APIs are covered by the Go tests.
  await page.route('**/panel/clients',route=>route.fulfill({contentType:'text/html',body:'<p>Enrollment completed</p>'}));
  let login=await page.request.post(base+'login',{form:{username:'browser-representative',password:'browser-password'}});
  assert.equal((await login.json()).success,true);
  await page.goto(base+'panel/security',{waitUntil:'networkidle'});
  await page.waitForFunction(()=>typeof app!=='undefined');
  assert.equal(await page.evaluate(()=>app.required),required);
  assert.equal(await page.locator('a[href$="panel/settings"]').count(),0,'personal security must not expose shared settings');
  await page.locator('.ant-card .ant-switch').click();
  await page.waitForFunction(()=>twoFactorModal.visible && !!document.getElementById('twofactor-qrcode'));
  const secret=await page.evaluate(()=>twoFactorModal.token);
  assert.ok(secret.length>=32);
  assert.equal(await page.evaluate(()=>twoFactorModal.totpObject.label),'browser-representative');
  await page.locator('input[autocomplete="current-password"]').fill('browser-password');
  await page.locator('input[autocomplete="one-time-code"]').fill('invalid');
  await page.evaluate(()=>twoFactorModal.ok());
  assert.equal(await page.evaluate(()=>twoFactorModal.visible),true,'bad enrollment must keep modal open');
  await page.locator('input[autocomplete="one-time-code"]').fill(await page.evaluate(()=>twoFactorModal.totpObject.generate()));
  await page.evaluate(()=>twoFactorModal.ok());
  if(required) {
   await page.waitForURL(base+'panel/clients');
   await page.goto(base+'panel/security',{waitUntil:'networkidle'});
   assert.equal(await page.locator('.ant-card .ant-switch').isDisabled(),true);
   const denied=await page.request.post(base+'panel/setting/twoFactor',{headers:{'X-Requested-With':'XMLHttpRequest'},form:{enable:false,code:await page.evaluate(secret=>new OTPAuth.TOTP({secret}).generate(),secret)}});
   assert.equal(denied.status(),200);
   assert.equal((await denied.json()).success,false);
  } else {
   await page.waitForFunction(()=>app.enabled && !twoFactorModal.visible);
   await page.reload({waitUntil:'networkidle'});
   assert.equal(await page.evaluate(()=>app.enabled),true);
   await page.locator('.ant-card .ant-switch').click();
   await page.locator('input[autocomplete="one-time-code"]').fill(await page.evaluate(secret=>new OTPAuth.TOTP({secret}).generate(),secret));
   await page.evaluate(()=>twoFactorModal.ok());
   await page.waitForFunction(()=>!app.enabled && !twoFactorModal.visible);
  }
  assert.deepEqual(failedAssets,[]);
  assert.deepEqual(errors,[]);
  console.log('Representative QR enrollment, reload and '+(required?'required policy':'voluntary disable')+' passed');
 } finally {await browser.close()}
})().catch(error=>{console.error(error);process.exit(1)});
