const {chromium}=require('playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try{
  const base=process.env.OUTBOUND_TABLE_URL,lang=process.env.NODE_UI_LANG;
  const config={log:{loglevel:'warning'},dns:{servers:['1.1.1.1']},routing:{domainStrategy:'AsIs',rules:[]},policy:{system:{statsOutboundUplink:true,statsOutboundDownlink:true}},inbounds:[],outbounds:[
   {tag:'direct',protocol:'freedom',settings:{}},
   {tag:'blocked',protocol:'blackhole',settings:{}},
   {tag:'long-outbound-name',protocol:'vless',settings:{vnext:[{address:'2001:db8:1234:5678:90ab:cdef:1234:5678',port:443,users:[]}]},streamSettings:{network:'tcp',security:'none'}}
  ]};
  for(const width of [1440,1024,768,390,320]){
   const context=await browser.newContext({viewport:{width,height:1000},locale:lang});
   try{
    await context.addCookies([{url:base,name:'lang',value:lang}]);
    const page=await context.newPage();page.setDefaultTimeout(15000);
    const errors=[];page.on('pageerror',e=>errors.push(e.message));
    await page.route('**/test/panel/**',async route=>{
     if(route.request().resourceType()==='document')return route.continue();
     const url=new URL(route.request().url());let obj=[],success=true;
     if(url.pathname.endsWith('/xray/'))obj=JSON.stringify({xraySetting:config,inboundTags:[],outboundTestUrl:'https://example.com/'});
     else if(url.pathname.endsWith('/getXrayResult'))obj='';
     else if(url.pathname.endsWith('/getOutboundsTraffic'))obj=[{tag:'direct',up:1234567890123456,down:Number.MAX_SAFE_INTEGER},{tag:'blocked',up:0,down:0},{tag:'long-outbound-name',up:1073741824,down:987654321098765}];
     else if(url.pathname.endsWith('/setting/all'))obj={};
     else if(url.pathname.endsWith('/custom-geo/aliases'))obj={};
     else if(/\/(update|resetOutboundsTraffic|restartXrayService)$/.test(url.pathname))throw Error('Layout test must not save or reset configuration');
     return route.fulfill({contentType:'application/json',body:JSON.stringify({success,msg:'',obj})});
    });
    await page.goto(base+'panel/xray',{waitUntil:'networkidle'});
    await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates.fetched&&app.outboundsTraffic.length===3);
    await page.evaluate(()=>{
     app.selectSection('tpl-outbound');
     app.$set(app.outboundTestStates,0,{testing:false,result:{success:true,delay:12345,statusCode:204,exit:{ipv4:'203.0.113.123',ipv6:'2001:db8:1234:5678:90ab:cdef:1234:5678',countryCode:'DE'}}});
     app.$set(app.outboundTestStates,1,{testing:false,result:{success:false,error:'Example failure'}});
     app.$set(app.outboundTestStates,2,{testing:false,result:{success:true,delay:25,statusCode:204}});
    });
    await page.locator('[data-section="tpl-outbound"] .ant-table-tbody > tr').first().waitFor();
    assert.equal(await page.locator('[data-section="tpl-outbound"] .ant-table-tbody > tr').nth(2).locator('td').nth(5).locator('.ant-tag-red').count(),0,'a successful request without exit metadata must not also show Fail');
    for(const unmeasured of [false,true]){
     if(unmeasured)await page.evaluate(()=>{app.outboundsTraffic=[];app.templateSettings.policy.system.statsOutboundUplink=false;app.templateSettings.policy.system.statsOutboundDownlink=false;});
     await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));
     const geometry=await page.evaluate(()=>{
      const scope=document.querySelector('[data-section="tpl-outbound"]');
      const rows=[...scope.querySelectorAll('.ant-table-tbody > tr')];
      const results=[];
      for(const row of rows){
       for(const column of [4,5]){
        const cell=row.querySelectorAll('td')[column],bounds=cell.getBoundingClientRect();
        for(const tag of cell.querySelectorAll('.ant-tag')){
         const rect=tag.getBoundingClientRect();
         results.push({column,text:tag.textContent.trim(),inside:rect.left>=bounds.left-1&&rect.right<=bounds.right+1,unclipped:tag.scrollWidth<=tag.clientWidth+1});
         const walker=document.createTreeWalker(tag,NodeFilter.SHOW_TEXT);
         for(let node=walker.nextNode();node;node=walker.nextNode()){
          if(!node.textContent.trim())continue;
          const range=document.createRange();range.selectNodeContents(node);
          for(const text of range.getClientRects())results.push({column,text:node.textContent.trim(),inside:text.left>=bounds.left-1&&text.right<=bounds.right+1,unclipped:true});
         }
        }
       }
      }
      const ips=[...scope.querySelectorAll('.ob-exit-ip')].map(ip=>{const rect=ip.getBoundingClientRect(),cell=ip.closest('td').getBoundingClientRect();return {text:ip.textContent,inside:rect.left>=cell.left-1&&rect.right<=cell.right+1,unclipped:ip.scrollWidth<=ip.clientWidth+1};});
      return {results,ips,documentWidth:document.documentElement.scrollWidth,viewport:innerWidth};
     });
     const problem=geometry.results.find(r=>!r.inside||!r.unclipped);
     assert.equal(problem,undefined,`${lang} ${width}px unmeasured=${unmeasured}: text crosses a cell boundary: ${JSON.stringify(problem)}`);
     assert.ok(geometry.ips.every(r=>r.inside&&r.unclipped),'exit addresses remain inside Test Result');
     assert.ok(geometry.documentWidth<=geometry.viewport+1,'only the table may scroll horizontally');
     if(!unmeasured){const text=await page.locator('[data-section="tpl-outbound"] .ant-table-tbody > tr').first().locator('td').nth(4).innerText();assert.ok(text.includes('1.10 PB')&&text.includes('8.00 PB'),'both complete byte values must stay visible');}
    }
    assert.deepEqual(errors,[]);
    console.log(`${lang} ${width}px: large/zero/unmeasured traffic, success/failure, IPv4/IPv6 and scroll boundaries passed`);
   }finally{await context.close()}
  }
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
