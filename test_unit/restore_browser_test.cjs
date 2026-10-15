'use strict';
const assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try{
  const page=await browser.newPage();page.setDefaultTimeout(20000);
  const errors=[],uploads=[],restarts=[];let rejectUpload=false;
  page.on('pageerror',error=>errors.push(error.message));
  await page.context().addCookies([{url:process.env.OVERVIEW_SSL_URL,name:'lang',value:process.env.NODE_UI_LANG}]);
  await page.route('**/panel/api/server/importDB',async route=>{
   uploads.push(route.request().postDataBuffer().toString());
   await route.fulfill({contentType:'application/json',body:JSON.stringify({success:!rejectUpload,msg:rejectUpload?'restore-fixture-rejected':'restore-fixture-complete',obj:rejectUpload?null:'restored'})});
  });
  await page.route('**/panel/setting/restartPanel',async route=>{
   restarts.push(route.request().url());
   await route.fulfill({status:401,contentType:'application/json',body:'{"success":false,"msg":"imported credentials invalidated the previous session"}'});
  });
  await page.goto(process.env.OVERVIEW_SSL_URL+'overview',{waitUntil:'networkidle'});
  for(const [name,marker] of [['first.db','first-backup-fixture'],['second.db','second-backup-fixture']]){
   try { await page.waitForFunction(()=>typeof app!=='undefined'&&!!app.$el); }
   catch(error) { throw Error(JSON.stringify({upload:name,url:page.url(),errors,restarts,uploads:uploads.length,reason:error.message})); }
   await page.evaluate(()=>{PromiseUtil.sleep=async()=>{}});
   const chooser=page.waitForEvent('filechooser');
   await page.evaluate(()=>app.importDatabase());
   const picker=await chooser;
   const navigation=page.waitForEvent('domcontentloaded');
   await picker.setFiles({name,mimeType:'application/octet-stream',buffer:Buffer.from('SQLite format 3\0'+marker)});
   await navigation;
   assert.equal(uploads.length,name==='first.db'?1:2);
   assert(uploads[uploads.length-1].includes(marker),'the second upload reused the first file');
   assert.equal(restarts.length,0,'restore depends on a second authenticated restart request');
  }
  await page.waitForFunction(()=>typeof app!=='undefined'&&!!app.$el);
  rejectUpload=true;
  const chooser=page.waitForEvent('filechooser');await page.evaluate(()=>app.importDatabase());
  const response=page.waitForResponse(r=>r.url().endsWith('/server/importDB'));
  await(await chooser).setFiles({name:'invalid.db',mimeType:'application/octet-stream',buffer:Buffer.from('invalid-fixture')});
  await response;
  await page.waitForFunction(()=>!app.loadingStates.spinning);
  await page.getByText('restore-fixture-rejected',{exact:true}).waitFor();
  assert.equal(restarts.length,0);
  assert.deepEqual(errors,[]);
  console.log('Two different restore uploads complete without a second authenticated request; failures remain visible.');
 }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exit(1)});
