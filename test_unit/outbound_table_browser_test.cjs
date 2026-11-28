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
   const context=await browser.newContext({viewport:{width,height:1000},locale:lang,hasTouch:width<=390,isMobile:width<=390});
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
    await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates?.fetched&&app.outboundsTraffic.length===3);
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
    if(width<=390){
     const body=page.locator('.ob-table .ant-table-body');
     const initial=await body.evaluate(el=>({left:el.scrollLeft,width:el.clientWidth,scroll:el.scrollWidth,rtl:getComputedStyle(el).direction==='rtl'}));
     assert.ok(initial.scroll>initial.width,'the mobile table needs its own scroll area');
     await body.scrollIntoViewIfNeeded();
     const bounds=await body.boundingBox();
     const cdp=await context.newCDPSession(page);
     const y=Math.round(Math.max(0,bounds.y)+Math.min(70,bounds.height/2));
     const start=Math.round(bounds.x+bounds.width*(initial.rtl?0.20:0.80));
     const end=Math.round(bounds.x+bounds.width*(initial.rtl?0.80:0.20));
     await cdp.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:start,y}]});
     for(let step=1;step<=8;step++){
      await cdp.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:Math.round(start+(end-start)*step/8),y}]});
      await page.waitForTimeout(16);
     }
     await cdp.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});
     const scrollChanged = await page.waitForFunction(left => document.querySelector('.ob-table .ant-table-body').scrollLeft !== left, initial.left, { timeout: 3000 }).catch(() => null);
     assert.ok(scrollChanged, `${lang} ${width}px: swipe on ${JSON.stringify(bounds)} at ${start},${y} did not move the table`);
     const ping=page.locator('.ob-table .ant-table-tbody tr').first().locator('button:has(.anticon-thunderbolt)');
     await ping.scrollIntoViewIfNeeded();
     assert.ok(await ping.isVisible(),'ping control is reachable inside the mobile scroller');
    }
    if (lang === 'en-US' && width === 1440) {
     await page.evaluate(() => {
      app.templateSettings.outbounds.push({ tag: 'emergency', protocol: 'freedom', settings: {} });
      app.addBalancer();
      Object.assign(balancerModal.balancer, {
       tag: 'test-gateway', strategy: 'gateway', primaryTag: 'direct', selector: ['long-outbound-name'],
      });
      balancerModal.strategyChanged('gateway');
      Object.assign(balancerModal.gateway, { interval: 2, unit: 60, attempts: 3, emergencyTags: ['emergency'], emergencyAttempts: 2 });
      balancerModal.check();
     });
     const dialog = page.locator('#balancer-modal .ant-modal');
     await dialog.getByText('Master outbound', { exact: true }).waitFor();
     await dialog.getByText('Emergency probe interval', { exact: true }).waitFor();
     await dialog.locator('.ant-modal-footer .ant-btn-primary').click();
     const gateway = await page.evaluate(() => app.templateSettings.routing.balancers[0]);
     assert.equal(gateway.primaryTag, 'direct');
     assert.deepEqual(gateway.selector, ['long-outbound-name']);
     assert.equal(gateway.strategy.settings.intervalSeconds, 120);
     assert.equal(gateway.strategy.settings.emergencyAttempts, 2);
     assert.deepEqual(gateway.strategy.settings.emergencyTags, ['emergency']);
     await page.evaluate(() => app.editBalancer(0));
     await dialog.getByText('Master outbound', { exact: true }).waitFor();
     assert.equal(await page.evaluate(() => balancerModal.gateway.unit), 60);
     assert.equal(await page.evaluate(() => balancerModal.gateway.interval), 2);
     await page.evaluate(() => {
      if (!themeSwitcher.isDarkTheme) themeSwitcher.toggleTheme();
      balancerModal.balancer.strategy = 'failover';
      balancerModal.strategyChanged('failover');
     });
     const help = dialog.getByText('Uses the primary while it is healthy and selects the lowest-latency healthy backup if it fails.');
     await help.waitFor();
     const colors = await help.evaluate(el => {
      const expected = document.createElement('span');
      expected.style.color = 'var(--text-2)';
      el.append(expected);
      return { actual: getComputedStyle(el).color, expected: getComputedStyle(expected).color };
     });
     assert.equal(colors.actual, colors.expected, 'failover explanation uses the dark menu text color');
    }
    assert.deepEqual(errors,[]);
    console.log(`${lang} ${width}px: large/zero/unmeasured traffic, success/failure, IPv4/IPv6 and scroll boundaries passed`);
   }finally{await context.close()}
  }
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
