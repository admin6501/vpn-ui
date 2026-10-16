// Exercise the real INFO/export builder and the Clients credential-copy method.
const fs=require('node:fs');
global.window={location:{hostname:'vpn.example.com'},crypto:require('node:crypto').webcrypto};
window.btoa=global.btoa;window.atob=global.atob;
global.location=window.location;
global.document={addEventListener(){}};
(0,eval)(['web/assets/js/util/index.js','web/assets/js/model/inbound.js','web/assets/js/model/dbinbound.js','web/assets/js/util/export.js'].map(p=>fs.readFileSync(p,'utf8')).join('\n')+'\nglobalThis.naming={DBInbound,AccountExport};');
const template=fs.readFileSync('web/html/clients.html','utf8');
const start=template.indexOf('      membershipCredentials(dbInbound, client) {');
const end=template.indexOf('\n      },',start);
const credentials=(0,eval)('({'+template.slice(start,end)+'\n}})').membershipCredentials;
(async()=>{
 const fixtures=JSON.parse(fs.readFileSync(0,'utf8'));
 const results=[];
 for(const f of fixtures){
  const record=new naming.DBInbound(f.inbound);
  const client=record.toInbound().clients.find(c=>c.email===f.email);
  const app={dbInbounds:[record],remarkModel:f.remarkModel,getInboundClients:r=>r.toInbound().clients,getSumStats:()=>0};
  const cards=await naming.AccountExport.buildCards(app,[{inboundId:record.id,email:f.email}]);
  results.push({info:cards.map(c=>c.link),credentials:credentials.call(app,record,client).map(c=>c.value),qr:record.toInbound().genAllLinks(record.remark,f.remarkModel,client).map(l=>l.link)});
 }
 process.stdout.write(JSON.stringify(results));
})().catch(e=>{console.error(e);process.exit(1)});
