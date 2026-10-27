const {chromium}=require('playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try{
  const context=await browser.newContext({viewport:{width:1280,height:900}});
  const base=process.env.INBOUND_CREATE_URL;await context.addCookies([{url:base,name:'lang',value:process.env.NODE_UI_LANG}]);
  const page=await context.newPage();page.setDefaultTimeout(15000);const errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  await page.route('**/panel/api/inbounds/**',route=>{
   if(route.request().method()!=='GET'&&/\/(add|update|del|saveAccount|reset)/.test(route.request().url()))throw Error('Form audit cannot save, delete or reset');
   return route.continue();
  });
  await page.goto(base+'panel/inbounds',{waitUntil:'networkidle'});
  await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates&&app.loadingStates.fetched);
  const protocols=await page.evaluate(()=>Object.values(Protocols));
  for(const protocol of protocols){
   await page.evaluate(protocol=>{app.openAddInbound();inModal.inbound.protocol=protocol;inModal.dbInbound.enable=false;inModal.section='proto';},protocol);
   await page.locator('#inbound-modal .ant-modal:visible').waitFor();
   await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));
   for(const section of ['identity','proto','transport','security','network','access','quota','reach','sniff']){
    await page.evaluate(section=>{inModal.section=section},section);
    await page.evaluate(()=>new Promise(r=>requestAnimationFrame(r)));
   }
   assert.deepEqual(errors,[],protocol+' renders without JS errors');
   await page.evaluate(()=>{inModal.visible=false});
   console.log('Rendered '+protocol+' protocol and option sections without saving');
  }
  const rows=await context.request.get(base+'panel/api/inbounds/list');const body=await rows.json();
  assert.equal(body.obj.length,0,'form inspection must leave the isolated database unchanged');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
