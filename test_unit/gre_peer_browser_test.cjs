const {chromium}=require('playwright');
const fs=require('node:fs');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try {
  const page=await browser.newPage({viewport:{width:390,height:844}});page.setDefaultTimeout(10000);
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const source=fs.readFileSync('web/html/modals/gre_config_modal.html','utf8');
  const markup=source.slice(0,source.indexOf('<script>')).replace(/{{\s*i18n\s+"([^"]+)"\s*}}/g,'$1').replace(/{{[\s\S]*?}}/g,'');
  const script=source.slice(source.indexOf('<script>')+8,source.lastIndexOf('</script>'));
  await page.setContent(markup);await page.addStyleTag({path:'web/assets/ant-design-vue/antd.min.css'});
  for(const path of ['web/assets/vue/vue.min.js','web/assets/ant-design-vue/antd.min.js']) await page.addScriptTag({path});
  await page.addScriptTag({content:`
   const PERMS={editClient:true};const app={reloads:0,async getDBInbounds(){this.reloads++}};
   const ClipboardManager={copyText:async()=>{}};
   const fixture={posts:[],reject:false,hold:null,ips:['','198.51.100.8']};
   const HttpUtil={
    async get(){return {success:true,obj:fixture.ips.map((ip,index)=>({peerIndex:index,peerIp:ip,dynamic:!ip,serverIp:'192.0.2.1',innerIp:'10.9.0.'+(16+index),gatewayIp:'10.9.0.1',mode:'ipsec',ipsecPsk:'fixture-key',configMikrotik:'src-address='+(ip||'YOUR-PUBLIC-IP')+'/32',configLinux:'local '+(ip||'YOUR-PUBLIC-IP')}))}},
    async post(path,body){fixture.posts.push({path,body});if(fixture.reject)return {success:false};if(fixture.hold)await fixture.hold;fixture.ips[body.peerIndex]=body.peerIp;return {success:true}}
   };
  `});
  await page.addScriptTag({content:script});
  await page.evaluate(()=>greConfigModal.show(27,{email:'gre customer',subId:''}));
  const first=page.getByTestId('gre-peer-ip-0'), second=page.getByTestId('gre-peer-ip-1');
  const save=page.getByTestId('gre-peer-save-0');await first.waitFor();
  assert.equal(await first.inputValue(),'');assert.equal(await save.isDisabled(),true);
  await first.fill('203.0.113.9');await second.fill('198.51.100.99');
  await page.evaluate(()=>{fixture.hold=new Promise(resolve=>window.finishSave=resolve)});
  await save.click();await page.waitForFunction(()=>greConfigModal.saving);
  assert.equal(await first.isDisabled(),true);assert.equal(await page.getByTestId('gre-peer-save-1').isDisabled(),true);
  await page.evaluate(()=>{finishSave();fixture.hold=null});
  await page.waitForFunction(()=>!greConfigModal.saving&&greConfigModal.peers[0].peerIp==='203.0.113.9');
  assert.equal(await second.inputValue(),'198.51.100.99','saving one peer must preserve other unsaved drafts');
  const post=await page.evaluate(()=>fixture.posts[0]);
  assert.equal(post.path,'/panel/api/inbounds/27/gre-peer-ip/gre%20customer');
  assert.deepEqual(post.body,{peerIndex:0,peerIp:'203.0.113.9',expectedIp:''});
  assert.equal(await page.locator('textarea').first().inputValue(),'src-address=203.0.113.9/32');
  await page.evaluate(()=>{greConfigModal.close();greConfigModal.show(27,{email:'gre customer'})});
  await page.waitForFunction(()=>!greConfigModal.loading);
  assert.equal(await first.inputValue(),'203.0.113.9','saved public IP survives reopening');
  await first.fill('invalid');await page.evaluate(()=>fixture.reject=true);await save.click();
  await page.waitForFunction(()=>fixture.posts.length===2&&!greConfigModal.saving);
  assert.equal(await first.inputValue(),'invalid','failed save preserves the draft');
  assert.equal(await page.locator('textarea').first().inputValue(),'src-address=203.0.113.9/32','failed save cannot change the generated recipe');
  await page.evaluate(()=>fixture.reject=false);await first.fill('');await save.click();
  await page.waitForFunction(()=>!greConfigModal.saving&&greConfigModal.peers[0].dynamic);
  assert.equal(await page.locator('textarea').first().inputValue(),'src-address=YOUR-PUBLIC-IP/32');
  await page.evaluate(()=>PERMS.editClient=false);
  // PERMS is intentionally nonreactive, just as it is fixed for a real session.
  await page.evaluate(()=>{greConfigModal.close();greConfigModal.show(27,{email:'gre customer'})});
  await page.waitForFunction(()=>!greConfigModal.loading);
  assert.equal(await first.isDisabled(),true);assert.equal(await save.count(),0);
  assert.deepEqual(errors,[]);
  console.log('GRE peer IP: mobile editor, persisted recipe refresh, slot isolation, stale-IP payload, failed-save draft, dynamic mode and read-only access passed');
 } finally {await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
