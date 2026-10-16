const {chromium}=require('playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try {
  const page=await browser.newPage();
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  for(const name of ['clients','inbounds']) {
   await page.goto(process.env.OVERVIEW_SSL_URL+name,{waitUntil:'networkidle'});
   await page.waitForFunction(()=>typeof app!=='undefined'&&!!app.$el);
   const result=await page.evaluate(async name=>{
    clearInterval(app.liveStateTimer);clearInterval(app.accountSummaryTimer);
    app.isRefreshEnabled=false;app.toggleRefresh();
    const sleep=ms=>new Promise(r=>setTimeout(r,ms));
    // Online state may be unavailable while the main database API is healthy.
    const waiting=[];
    HttpUtil.post=()=>new Promise(r=>waiting.push(r));
    let releaseInbound;
    const record={id:91,userId:1,tag:'refresh',remark:'saved',protocol:'vmess',port:24443,enable:true,
      settings:JSON.stringify({clients:[]}),streamSettings:JSON.stringify({network:'tcp',security:'none'}),clientStats:[]};
    const list={rows:[{id:777,email:'saved',enable:true,up:0,down:0,totalGB:0,expiryTime:0,memberships:[]}],total:1};
    HttpUtil.get=async url=>{
     if(url.endsWith('/clients/inbounds'))return new Promise(r=>releaseInbound=r);
     return {success:true,obj:url.includes('/clients/list')?list:url.endsWith('/inbounds/list')?[record]:{}};
    };
    let done=false;
    const refresh=app.getDBInbounds().then(()=>done=true);
    await sleep(30);
    const earlyRows=name==='clients'?app.clients[0]?.email==='saved':app.dbInbounds[0]?.remark==='saved';
    if(releaseInbound)releaseInbound({success:true,obj:[]});
    await sleep(30);
    const finishedWithoutOnline=done;
    for(const resolve of waiting)resolve({success:true,obj:[]});
    await refresh;await sleep(20);
    // A save occurring during an older refresh needs a second, fresh read.
    let releaseFirst,calls=0,active=0,maxActive=0,version='before';
    HttpUtil.post=async()=>({success:true,obj:[]});
    const endpoint=name==='clients'?'/clients/inbounds':'/inbounds/list';
    HttpUtil.get=async url=>{
     if(url.endsWith(endpoint)) {
      calls++;active++;maxActive=Math.max(maxActive,active);
      const snapshot=version;
      if(calls===1)await new Promise(r=>releaseFirst=r);
      active--;
      return {success:true,obj:name==='clients'?[]:[{...record,remark:snapshot}]};
     }
     return {success:true,obj:url.includes('/clients/list')?{...list,rows:[{...list.rows[0],email:version}]}:{}};
    };
    const old=app.getDBInbounds();await sleep(10);version='after';
    const writes=Array.from({length:10},()=>app.getDBInbounds());
    releaseFirst();await Promise.all([old,...writes]);
    const finalValue=name==='clients'?app.clients[0].email:app.dbInbounds[0].remark;
    // Polling recovers even when the socket remains open without delivering data.
    app.refreshInterval=1000;app.isRefreshEnabled=true;
    for(let i=0;i<10;i++)app.startDataRefreshLoop(true);
    await sleep(1200);
    const pollCalls=calls-2;
    app.isRefreshEnabled=false;app.toggleRefresh();
    const stoppedCalls=calls;await sleep(1100);
    const stopped=calls===stoppedCalls;
    const socketSnapshot=JSON.stringify(app.dbInbounds);
    if(name==='inbounds')window.wsClient.emit('inbounds',[{...record,remark:'must-not-refresh'}]);
    const preferenceRespected=name!=='inbounds'||JSON.stringify(app.dbInbounds)===socketSnapshot;
    app.$destroy();
    return {earlyRows,finishedWithoutOnline,callsBeforePolling:stoppedCalls-pollCalls,maxActive,finalValue,pollCalls,stopped,preferenceRespected};
   },name);
   assert.equal(result.earlyRows,true,name+': table blocked by unrelated request');
   assert.equal(result.finishedWithoutOnline,true,name+': spinner blocked by online API');
   assert.equal(result.callsBeforePolling,2,name+': overlapping refreshes not coalesced');
   assert.equal(result.maxActive,1,name+': simultaneous full payload requests');
   assert.equal(result.finalValue,'after',name+': post-save refresh reused old response');
   assert.equal(result.pollCalls,2,name+': duplicated or missing polling chain');
   assert.equal(result.stopped,true,name+': disabled polling continued');
   assert.equal(result.preferenceRespected,true,name+': socket ignored disabled refresh');
  }
  assert.deepEqual(errors,[]);
  console.log('Panel refresh: delayed APIs, immediate rows, post-save invalidation, single polling chain, connected silent socket, disable and cleanup passed in '+process.env.NODE_UI_LANG);
 } finally {await browser.close()}
})().catch(error=>{console.error(error);process.exit(1)});
