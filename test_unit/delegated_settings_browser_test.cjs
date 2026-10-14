'use strict';
const assert = require('node:assert/strict');
const {chromium} = require('playwright');
(async () => {
 const browser = await chromium.launch({headless:true, executablePath:process.env.NODE_UI_CHROME || undefined, args:['--no-sandbox']});
 try {
  const page = await browser.newPage(); const errors=[]; const requests=[];
  page.on('pageerror',error=>errors.push(String(error)));
  page.on('request',request=>requests.push(request.url()));
  await page.goto(process.env.SETTINGS_UI_URL,{waitUntil:'networkidle'});
  await page.waitForFunction(()=>typeof app !== 'undefined' && app.loadingStates && app.loadingStates.fetched);
  await page.waitForFunction(()=>app.ssl && app.ssl.status !== null);
  assert.equal(await page.locator('.bo-secnav-sec').count(),7,'the permission grants every settings section');
  assert.equal(requests.some(url=>url.includes('/ssl/status')),true);
  assert.equal(requests.some(url=>url.endsWith('/setting/service')),true);
  assert.equal(await page.evaluate(()=>app.settingKeys.includes('tgBotToken')),true);
  assert.equal(await page.evaluate(()=>app.allSetting.tgBotToken),'browser-private-token');
  await page.evaluate(()=>{app.allSetting.pageSize=35;app.allSetting.subTitle='Delegated save';app.allSetting.subAnnounce='';app.allSetting.tgBotToken='browser-updated-token';app.allSetting.ldapPassword='browser-updated-password';});
  const responsePromise=page.waitForResponse(r=>r.url().endsWith('/setting/update'));
  await page.evaluate(()=>app.updateAllSetting());
  assert.equal((await (await responsePromise).json()).success,true,'settings save must pass the real permission check');
  await page.reload({waitUntil:'networkidle'});
  await page.waitForFunction(()=>typeof app !== 'undefined' && app.loadingStates && app.loadingStates.fetched);
  assert.equal(await page.evaluate(()=>app.allSetting.pageSize),35);
  assert.equal(await page.evaluate(()=>app.allSetting.subTitle),'Delegated save');
  assert.equal(await page.evaluate(()=>app.allSetting.tgBotToken),'browser-updated-token');
  assert.equal(await page.evaluate(()=>app.allSetting.ldapPassword),'browser-updated-password');
  assert.deepEqual(errors,[]);
  console.log('Settings load, save, reload and protected-field preservation passed: '+process.env.SETTINGS_UI_OWNER);
 } finally {await browser.close()}
})().catch(error=>{console.error(error);process.exit(1)});
