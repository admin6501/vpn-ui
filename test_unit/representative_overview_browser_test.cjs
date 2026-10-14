'use strict';
const assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.NODE_UI_CHROME||undefined,args:['--no-sandbox']});
 try {
  const page=await browser.newPage();let updateRequests=0;const errors=[];
  page.on('request',r=>{if(/\/checkUpdate|\/updateProgress/.test(r.url()))updateRequests++});
  page.on('pageerror',e=>errors.push(String(e)));
  await page.goto(process.env.REPRESENTATIVE_OVERVIEW_URL,{waitUntil:'networkidle'});
  await page.evaluate(()=>{app.loadingStates.fetched=true});
  const quota=page.locator('[data-testid="representative-quota"]');await quota.waitFor();
  await page.evaluate(()=>app.fetchQuotaBalance());
  assert.match(await quota.innerText(),/75(?:\.00)?\s*GB/);
  assert.match(await quota.innerText(),/100(?:\.00)?\s*GB/);
  assert.equal(await page.locator('button').filter({hasText:'pages.index.checkUpdate'}).count(),0);
  await page.evaluate(()=>app.checkForUpdate(true));
  assert.equal(updateRequests,0,'representative must never query update endpoints');
  assert.deepEqual(errors,[],'representative overview must mount without errors');
  console.log('Representative quota and blocked update controls verified in Chromium');
 } finally {await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
