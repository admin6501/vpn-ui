const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
(async () => {
 for (const [file, model] of [['account_info_modal.html','accountInfoModal'], ['membership_info_modal.html','membershipInfoModal']]) {
  const source = fs.readFileSync('web/html/modals/' + file, 'utf8');
  const script = source.slice(source.lastIndexOf('<script>') + 8, source.lastIndexOf('</script>'));
  let config, timer, cleared;
  const sandbox = {themeSwitcher:{}, console, Vue:function(c){ config=c; return c; },setInterval(fn,ms){assert.equal(ms,10000); timer=fn; return 7;},clearInterval(id){cleared=id;}};
  vm.runInNewContext(script, sandbox);
  let calls=0, resolve;
  const state={app:{loadOnlineState(){calls++; return new Promise(r=>resolve=r);}},...config.methods};
  config.watch[model+'.visible'].call(state,true);
  assert.equal(calls,1,'fetch immediately when Info opens');
  timer(); assert.equal(calls,1,'do not overlap a pending request');
  resolve(); await new Promise(setImmediate);
  timer(); assert.equal(calls,2,'refresh live state every 10 seconds');
  resolve(); await new Promise(setImmediate);
  config.watch[model+'.visible'].call(state,false);
  assert.equal(cleared,7,'stop polling after modal closes');
  config.beforeDestroy.call(state); assert.equal(cleared,7);
  if (model==='membershipInfoModal') assert.match(source,/onlineText/, 'inbound Info renders online state');
 }
 const page=fs.readFileSync('web/html/inbounds.html','utf8');
 const start=page.indexOf('      getClientCounts(dbInbound, inbound) {');
 const end=page.indexOf('\n      },',start);
 const count=vm.runInNewContext('(function(dbInbound,inbound){'+page.slice(start+'      getClientCounts(dbInbound, inbound) {'.length,end)+'\n})');
 const clients=[{email:'expired',enable:false},{email:'live',enable:true}];
 const stats=[{email:'expired',total:100,up:0,down:0,expiryTime:Date.now()-1000}];
 const context={expireDiff:86400000,trafficDiff:0,membershipOnline:(inbound,email)=>email==='live',membershipOnlineShared:()=>false};
 for (const enable of [true,false]) {
  const result=count.call(context,{enable,clientStats:stats},{clients});
  assert.deepEqual(Array.from(result.depleted),['expired'],'expired name survives disabled inbound');
  assert.equal(result.online.length,enable?1:0);
 }
 assert.match(page,/:title="accountNames\(dbInbound\)"/,'desktop/mobile summaries expose names on hover');
 assert.match(page,/accountSummary\.expiredEmails/,'header expiry popover exposes scoped names');
 const onlineStart=page.indexOf('      async refreshOnlineState() {');
 const onlineEnd=page.indexOf('\n      },',onlineStart);
 const refresh=vm.runInNewContext('(async function(){'+page.slice(onlineStart+'      async refreshOnlineState() {'.length,onlineEnd)+'\n})',{console});
 let calls=0, done;
 const liveState={getOnlineUsers(){calls++;return new Promise(r=>done=r)},async getOnlineMemberships(){calls++},async getLastOnlineMap(){calls++}};
 const pending=refresh.call(liveState);
 await refresh.call(liveState);assert.equal(calls,3,'page refresh requests must not overlap');
 done();await pending;
 assert.match(page,/this\.refreshOnlineState\(\);/,'page timer refreshes membership state without WebSocket');
 console.log('Info polling and expired-name hover regression checks passed.');
})().catch(error=>{console.error(error);process.exit(1);});
