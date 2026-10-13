const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync('web/assets/js/util/export.js','utf8');
const sandbox = {console, location:{hostname:'panel.example'}, SizeFormatter:{sizeFormat:String}, Protocols:Object.fromEntries(['L2TP','PPTP','OPENVPN','OPENCONNECT','SSTP','IKEV2','SSH','WGC','AWG','GRE','MTPROTO','NAIVE'].map(p=>[p,p.toLowerCase()])), HttpUtil:{async get(){return {success:true,obj:[{host:'ssh.edge.example',port:2222,link:'ssh://credential'}]}}}};
vm.runInNewContext(source+'\nglobalThis.exports=AccountExport',sandbox);
const exporter=sandbox.exports;
function fixture(protocol, endpoints=[]) {
 const client={id:'ssh-login',email:'tracking-label',password:'ssh-password',enable:true};
 const inbound={settings:{},stream:{network:'tcp',externalProxy:endpoints},port:443,
 genLink:(host,port)=>'vless://uuid@'+host+':'+port,
 genAllLinks(){return endpoints.length ? endpoints.map(e=>({link:'vless://uuid@'+e.dest+':'+e.port})) : [{link:'vless://uuid@panel.example:443'}]}};
 const record={id:1,protocol,address:'panel.example',port:443,toInbound:()=>inbound};
 return {dbInbounds:[record],getInboundClients:()=>[client],getSumStats:()=>0};
}
(async()=>{
 const targets=[{inboundId:1,email:'tracking-label'}];
 const ssh=await exporter.buildCards(fixture('ssh'),targets);
 assert.equal(ssh[0].username,'ssh-login'); assert.equal(ssh[0].password,'ssh-password');
 assert.equal(ssh[0].server,'ssh.edge.example');assert.equal(ssh[0].showCredentials,true);
 assert.equal(ssh[0].uuid,'');assert.equal(ssh[0].qr,'ssh://credential');
 const cards=await exporter.buildCards(fixture('vless',[{dest:'edge.one',port:8443},{dest:'edge.two',port:9443}]),targets);
 assert.equal(cards.length,2);assert.equal(cards[0].server,'edge.one');assert.equal(cards[1].port,'9443');
 assert.equal(cards[1].link,'vless://uuid@edge.two:9443');
 const info=fs.readFileSync('web/html/modals/membership_info_modal.html','utf8');
 let config;const ctx={themeSwitcher:{},Vue:function(c){config=c;return c;}};
 vm.runInNewContext(info.slice(info.lastIndexOf('<script>')+8,info.lastIndexOf('</script>')),ctx);
 assert.equal(config.computed.endpoints.call({cards,record:{address:'panel.example',port:443}})[1].server,'edge.two');
 assert.match(info,/!card.qr \|\| card.showCredentials/);
 const modal=fs.readFileSync('web/html/modals/two_factor_modal.html','utf8');
 vm.runInNewContext(modal.slice(modal.indexOf('<script>')+8,modal.lastIndexOf('</script>'))+'\nglobalThis.modal=twoFactorModal',ctx);
 let resolve,calls=0;const m=ctx.modal;m.visible=true;m.enteredCode='123456';m.enteredPassword='password';m.totpObject={generate(){throw Error('client clock must not validate enrollment')}};
 m.confirm=async(success,code,password)=>{calls++;assert.equal(code,'123456');assert.equal(password,'password');return new Promise(r=>resolve=r)};
 const pending=m.ok();await m.ok();assert.equal(calls,1);resolve(false);await pending;
 assert.equal(m.visible,true);assert.equal(m.enteredPassword,'password');
 m.confirm=async()=>true;await m.ok();assert.equal(m.visible,false);assert.equal(m.enteredPassword,'');
 const downloaded=[];
 sandbox.FileManager={downloadTextFile(content,filename,options){downloaded.push({content,filename,options})}};
 const confText='[Interface]\nPrivateKey = fixture\n[Peer]\nPublicKey = fixture\n';
 exporter.txt([{email:'wg',configText:confText}], 'wg.txt');
 assert.equal(downloaded[0].filename,'wg.conf');assert.equal(downloaded[0].content,confText);
 assert.equal(downloaded[0].options.type,'application/octet-stream');
 exporter.txt([{email:'wg',configText:confText},{email:'wg',configText:confText}], 'wg');
 assert.deepEqual(downloaded.slice(1).map(file=>file.filename),['wg-peer-1.conf','wg-peer-2.conf']);
 console.log('External endpoint cards, SSH credentials and server-verified async 2FA passed');
})().catch(e=>{console.error(e);process.exit(1)});
