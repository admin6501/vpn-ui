const {chromium}=require('playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try {
  const page=await browser.newPage();page.setDefaultTimeout(10000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.addInitScript(()=>{window.__liveIntervals=new Map();const set=window.setInterval.bind(window),clear=window.clearInterval.bind(window);window.setInterval=(fn,ms,...rest)=>{const id=set(fn,ms,...rest);__liveIntervals.set(id,{fn,ms});return id};window.clearInterval=id=>{__liveIntervals.delete(id);return clear(id)}});
  await page.context().addCookies([{url:process.env.OVERVIEW_SSL_URL,name:'lang',value:process.env.NODE_UI_LANG}]);
  await page.goto(process.env.OVERVIEW_SSL_URL+'clients',{waitUntil:'networkidle'});
  await page.waitForFunction(()=>typeof app!=='undefined'&&!!app.$el);
  await page.evaluate(async()=>{
   window.live=true;
   HttpUtil.get=async url=>({success:true,obj:url.includes('/clients/list')?{rows:[],total:0,online:live?1:0,enabled:1,disabled:0}:[]});
   HttpUtil.post=async url=>({success:true,obj:url.endsWith('/onlines')?(live?['live-user']:[]):url.endsWith('/onlineMemberships')?(live?['91:live-user']:[]):{}});
   await app.refreshLiveState();await app.$nextTick();
  });
  assert.equal(await page.evaluate(()=>app.clientCounts.online),1);
  assert.equal(await page.evaluate(()=>app.isRefreshEnabled),false,'fixture disables full table refresh');
  assert.equal(await page.evaluate(()=>__liveIntervals.get(app.liveStateTimer).ms),10000);
  await page.evaluate(async()=>{live=false;await app.getDBInbounds();await app.$nextTick()});
  assert.equal(await page.evaluate(()=>app.clientCounts.online),0,'real offline state remains visible');
  await page.evaluate(async()=>{live=true;app.clients=[{id:777,email:"keep-row",enable:true,up:0,down:0,totalGB:0,expiryTime:0,memberships:[]}];await __liveIntervals.get(app.liveStateTimer).fn();await app.$nextTick()});
  assert.equal(await page.evaluate(()=>app.clientCounts.online),1,'live timer recovers after edit without page reload');
  assert.equal(await page.evaluate(()=>app.clients[0].email),'keep-row','live timer replaced table rows while full refresh is off');
  // A delayed pre-edit zero answer must not replace a newer successful answer.
  await page.evaluate(async()=>{
   const original=HttpUtil.get;
   let resolve,first=true;
   HttpUtil.get=url=>{if(first){first=false;return new Promise(r=>resolve=r)}return Promise.resolve({success:true,obj:{rows:[],total:0,online:2,enabled:2,disabled:0}})};
   const old=app.loadClients();await app.loadClients();resolve({success:true,obj:{rows:[],total:0,online:0,enabled:0,disabled:0}});await old;
   HttpUtil.get=original;
  });
  assert.equal(await page.evaluate(()=>app.clientCounts.online),2,'stale list answer reset fresh count');
  assert.equal(await page.evaluate(()=>{const id=app.liveStateTimer;app.$destroy();return !__liveIntervals.has(id)}),true,'live timer cleaned up');

  await page.goto(process.env.OVERVIEW_SSL_URL+'inbounds',{waitUntil:'networkidle'});
  await page.waitForFunction(()=>typeof app!=='undefined'&&!!app.$el);
  await page.evaluate(async()=>{
   window.record={id:91,userId:1,tag:'live-test',remark:'LIVE-ROW',protocol:'vless',port:24443,enable:true,settings:JSON.stringify({clients:[{id:'11111111-2222-3333-4444-555555555555',email:'live-user',enable:true}],decryption:'none'}),streamSettings:JSON.stringify({network:'tcp',security:'none'}),clientStats:[{email:'live-user',enable:true,total:0,expiryTime:0,up:0,down:0}]};
   HttpUtil.get=async url=>({success:true,obj:url.endsWith('/list')?[record]:{total:1,online:1,depleted:0,expired:0,expiring:0}});
   HttpUtil.post=async url=>({success:true,obj:url.endsWith('/onlines')?['live-user']:url.endsWith('/onlineMemberships')?['91:live-user']:{}});
   await app.getDBInbounds();app.loading(false);await app.$nextTick();
  });
  await page.locator('.bo-il-online').first().waitFor({state:'visible'});
  assert.equal(await page.evaluate(()=>app.clientCount[91].online.length),1);
  for(let i=0;i<3;i++) {
   await page.evaluate(async()=>{record={...record,remark:'LIVE-ROW-EDITED'};await app.getDBInbounds();await app.$nextTick()});
   assert.equal(await page.evaluate(()=>app.clientCount[91].online.length),1,'edit lost membership count');
   assert.equal(await page.locator('.bo-il-online').first().isVisible(),true,'Vue did not repaint online pill after edit');
  }
  await page.evaluate(async()=>{
   const original=HttpUtil.get;HttpUtil.get=async()=>{throw Error('fixture request failure')};
   try{await app.getDBInbounds()}catch{}finally{HttpUtil.get=original}
  });
  assert.equal(await page.evaluate(()=>app.refreshing),false,'failed request stranded loading state');
  assert.deepEqual(errors,[]);
  console.log('Browser: live counters recover after edits, stale replies ignored, reactive inbound counts and timer cleanup passed in '+process.env.NODE_UI_LANG);
 } finally {await browser.close()}
})().catch(error=>{console.error(error);process.exit(1)});
