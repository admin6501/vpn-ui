const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
function method(file,name){const s=fs.readFileSync(file,'utf8');const start=s.indexOf('      '+name+'() {');assert(start>=0);const end=s.indexOf('\n      },',start);return vm.runInNewContext('(function(){'+s.slice(start+('      '+name+'() {').length,end)+'\n})');}
const counts=method('web/html/index.html','coreStateCounts');
const healthy=method('web/html/index.html','allCoresHealthy');
const summary=method('web/html/index.html','healthSummary');
const state={coreStateKey:state=>state,coreStatuses:[{name:'xray',state:'running'},{name:'radius',state:'running'},...Array.from({length:12},(_,i)=>({name:'unused'+i,state:i%2?'not_installed':'idle'}))]};
state.coreStateCounts=counts.call(state);assert.equal(state.coreStateCounts.total,2);assert.equal(summary.call(state),'2 / 2');assert.equal(healthy.call(state),true);
state.coreStatuses.push({name:'openvpn',state:'stopped'});state.coreStateCounts=counts.call(state);assert.equal(summary.call(state),'2 / 3');assert.equal(healthy.call(state),false);
state.coreStatuses.push({name:'ikev2',state:'not_installed',enabledInbounds:1});state.coreStateCounts=counts.call(state);assert.equal(summary.call(state),'2 / 4');assert.equal(state.coreStateCounts.error,1);assert.equal(healthy.call(state),false);
state.coreStatuses=[];state.coreStateCounts=counts.call(state);assert.equal(summary.call(state),'-');assert.equal(healthy.call(state),false);
const rows=method('web/html/settings.html','sslRows');
const cert={identifiers:['panel.example.test'],issuer:'Test CA',keyMatchesLeaf:true,hasIntermediates:true};
const sslState={ssl:{status:{profiles:[],legacyRenewal:true,adoptable:[{certPath:'/cert/fullchain.pem',info:cert,hasAcmeState:true,usedByPanel:true,usedBySub:true}]}},sslHealthLevel:()=> 'ok',sslHealthText:()=> '30',sslLifePct:()=>100};
const listed=rows.call(sslState);assert.equal(listed.length,1);assert.equal(listed[0].kind,'external');assert.equal(listed[0].usedByPanel,true);assert.equal(listed[0].usedBySub,true);assert.equal(listed[0].autoRenew,true);assert.equal(listed[0].assignable,false);assert.equal(listed[0].deletable,false);
sslState.ssl.status.profiles=[{name:'default',active:cert,usedByPanel:true}];assert.equal(rows.call(sslState).length,2);
console.log('Overview service denominator and external SSL rows passed.');
