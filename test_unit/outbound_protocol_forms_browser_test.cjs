const {chromium}=require('playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try{
  const base=process.env.OUTBOUND_TABLE_URL,lang=process.env.NODE_UI_LANG;
  const config={log:{loglevel:'warning'},dns:{servers:['1.1.1.1']},routing:{domainStrategy:'AsIs',rules:[]},policy:{system:{}},inbounds:[],outbounds:[{tag:'direct',protocol:'freedom',settings:{}}]};
  const context=await browser.newContext({viewport:{width:390,height:900},locale:lang});
  await context.addCookies([{url:base,name:'lang',value:lang}]);
  const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.route('**/test/panel/**',route=>{
   if(route.request().resourceType()==='document')return route.continue();
   const endpoint=new URL(route.request().url()).pathname;let obj=[];
   if(/\/(update|save|reg|resetOutboundsTraffic|restartXrayService)$/.test(endpoint))throw Error('Form audit cannot save, register, restart or reset');
   if(endpoint.endsWith('/xray/'))obj=JSON.stringify({xraySetting:config,inboundTags:[],outboundTestUrl:'https://example.com/'});
   else if(endpoint.endsWith('/getXrayResult'))obj='';
   else if(endpoint.endsWith('/setting/all')||endpoint.endsWith('/custom-geo/aliases'))obj={};
   return route.fulfill({contentType:'application/json',body:JSON.stringify({success:true,msg:'',obj})});
  });
  await page.goto(base+'panel/xray',{waitUntil:'networkidle'});
  await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates&&app.loadingStates.fetched);
  const protocols=await page.evaluate(()=>Object.values(Protocols));
  for(const protocol of protocols){
   await page.evaluate(protocol=>{app.addOutbound();outModal.outbound.protocol=protocol;outModal.outbound.tag='form-check';},protocol);
   await page.locator('#out-modal .ant-modal:visible').waitFor();
   await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));
   const modal=page.locator('#out-modal .ant-modal');
   const bounds=await modal.boundingBox();assert.ok(bounds.width<=390,'modal must fit mobile viewport');
   await modal.locator('.ant-tabs-tab').filter({hasText:/^JSON$/}).click();
   await page.waitForTimeout(60);
   assert.deepEqual(errors,[],protocol+' JSON editor and form render without JS errors');
   await page.evaluate(()=>outModal.close());
   console.log('Rendered '+protocol+' form and JSON view without saving');
  }
  assert.equal(await page.evaluate(()=>app.templateSettings.outbounds.length),1,'form inspection must not add an outbound');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
