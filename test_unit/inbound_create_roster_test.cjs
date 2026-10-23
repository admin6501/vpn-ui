'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const context = vm.createContext({atob,btoa,window:{btoa,atob,crypto:require('node:crypto').webcrypto,location:{hostname:'panel.example.test'}},document:{addEventListener(){}},console});
vm.runInContext(fs.readFileSync('web/assets/js/util/index.js','utf8')+'\n'+fs.readFileSync('web/assets/js/model/inbound.js','utf8')+'\nglobalThis.Model=Inbound; globalThis.Ciphers=SSMethods; globalThis.ProtocolSet=Protocols;',context);
function method(file,name,indent) {
 const source=fs.readFileSync(file,'utf8');
 const start=source.indexOf(' '.repeat(indent)+name);assert.ok(start>=0,name);
 const end=source.indexOf('\n'+' '.repeat(indent)+'},',start);assert.ok(end>start,name);
 return vm.runInContext('({'+source.slice(start,end+indent+5)+'})',context);
}
const change=method('web/html/modals/inbound_modal.html','SSMethodChange()',12).SSMethodChange;
const {Model,Ciphers,ProtocolSet}=context;
for(const protocol of Object.values(ProtocolSet)) {
 const inbound=new Model(); inbound.startEmpty=true; inbound.protocol=protocol;
 const settings=inbound.settings?.toJson();
 if(settings && Object.hasOwn(settings,'clients'))assert.equal(settings.clients.length,0,protocol+': protocol selection must not create accounts');
}
for(const cipher of Object.values(Ciphers)) {
 const inbound=new Model(); inbound.startEmpty=true; inbound.protocol='shadowsocks';inbound.settings.method=cipher;
 change.call({inModal:{inbound}});
 assert.equal(inbound.settings.toJson().clients.length,0,cipher+': changing encryption must not generate an account');
 // Hidden Shadowsocks client storage is still serialized in single-user mode.
 inbound.settings.shadowsockses.push(new Model.ShadowsocksSettings.Shadowsocks());
 inbound.clearClients();
 assert.equal(inbound.settings.toJson().clients.length,0,cipher+': clearing must include hidden client storage');
}
const clone=method('web/html/inbounds.html','async cloneInbound(',6).cloneInbound;
const add=method('web/html/inbounds.html','async addInbound(',6).addInbound;
context.inModal={};
(async()=>{
 for(const protocol of ['vmess','vless','shadowsocks','trojan','wg-c','awg','gre']) {
  const base=new Model();base.protocol=protocol;
  let sent;
  const app={validateInboundForm:()=>true,submit:async(path,data)=>{sent={path,data}}};
  await clone.call(app,base,{remark:'same-name',port:24443,nodeId:0});
  assert.equal(JSON.parse(sent.data.settings).clients?.length||0,0,protocol+': clone must create listener without accounts');
  // An old default or reused form state must not leak into a fresh creation.
  await add.call(app,base,{remark:'same-name',port:24443,nodeId:0});
  assert.equal(JSON.parse(sent.data.settings).clients?.length||0,0,protocol+': final create payload must contain no accounts');
 }
 console.log('Fresh protocol/cipher selection, hidden Shadowsocks lists, cloned/reused create forms keep empty account rosters');
})().catch(error=>{console.error(error);process.exitCode=1});
