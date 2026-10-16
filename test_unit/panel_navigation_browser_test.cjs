const {chromium}=require('playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try {
  const page=await browser.newPage();page.setDefaultTimeout(10000);
  const panel=process.env.PANEL_NAV_URL, source=process.env.PANEL_NAV_SOURCE;
  await page.goto(source);await page.locator('#clients').click();
  assert.equal(await page.locator('body').innerText(),'Login','external links must not authenticate anonymous visitors');
  await page.goto(panel+'/private/test-session');
  for(const id of ['root','clients','root','clients']){
   await page.goto(source);
   const result=page.waitForResponse(r=>r.url().startsWith(panel+'/private/panel/')&&r.status()===200);
   await page.locator('#'+id).click();const response=await result;
   assert.equal(response.headers()['x-test-fetch-site'],'cross-site','exercise real cross-site browser navigation metadata');
   assert.equal(await page.locator('body').innerText(),'Authenticated panel');
  }
  // App-opened tabs and script redirects may carry cross-site metadata without
  // Sec-Fetch-User. They must open on the first attempt, without a manual reload.
  for(const target of ['root','clients','security','groups']) {
   const loaded=page.waitForResponse(r=>r.url()===panel+'/private/panel/'+(target==='root'?'':target));
   await page.goto(source+'?auto='+target);
   const response=await loaded;
   assert.equal(response.status(),200,'first navigation without user-activation metadata returned 403');
   assert.equal(response.headers()['x-test-fetch-site'],'cross-site');
   assert.equal(response.headers()['x-test-fetch-user'] || '','','exercise a navigation without Sec-Fetch-User');
   await page.waitForURL(panel+'/private/panel/'+(target==='root'?'':target));
   assert.equal(await page.locator('body').innerText(),'Authenticated panel');
  }
  for(const path of ['/private','/private/','/private/panel','/private/panel/clients','/private/panel/security','/private/panel/groups']){
   const response=await page.goto(panel+path);assert.equal(response.status(),200);
   assert.equal(await page.locator('body').innerText(),'Authenticated panel','pasted link stays usable with an existing session');
  }
  await page.goto(source);
  const rejected=page.waitForResponse(r=>r.url()===panel+'/private/panel/clients');
  await page.evaluate(url=>{const f=document.createElement('iframe');f.src=url;document.body.appendChild(f);},panel+'/private/panel/clients');
  const response=await rejected;assert.equal(response.status(),403,'cross-site iframes stay blocked');
  assert.equal(response.headers()['x-vpn-ui-error'],'origin-rejected');
  console.log('Chromium: external links, pasted links, app/script redirects without activation, existing-session redirects, anonymous login and iframe protection passed');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
