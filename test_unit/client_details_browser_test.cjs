const {chromium}=require('playwright');
const fs=require('node:fs');
const assert=require('node:assert/strict');
function template(file) {
 const s=fs.readFileSync(file,'utf8');
 const start=s.indexOf('<script>');
 const markup=(start>=0?s.slice(0,start):s).replace(/{{define[^}]*}}|{{end}}/g,'').replace(/{{\s*i18n\s+"([^"]+)"\s*}}/g,'$1');
 const script=start>=0?s.slice(start+8,s.lastIndexOf('</script>')):'';
 return {markup,script};
}
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try {
  const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const factor=template('web/html/modals/two_factor_modal.html');
  const member=template('web/html/modals/membership_info_modal.html');
  const hysteria=template('web/html/form/stream/stream_hysteria.html');
  await page.setContent(factor.markup+member.markup+'<div id="hy-test">'+hysteria.markup+'</div>');
  await page.addStyleTag({path:'web/assets/ant-design-vue/antd.min.css'});
  for (const path of ['web/assets/moment/moment.min.js','web/assets/vue/vue.min.js','web/assets/ant-design-vue/antd.min.js','web/assets/otpauth/otpauth.umd.min.js','web/assets/qrcode/qrious2.min.js','web/assets/js/util/index.js','web/assets/js/model/inbound.js']) await page.addScriptTag({path});
  await page.addScriptTag({content:'const themeSwitcher={currentTheme:"light"}; const app={};'});
  await page.addScriptTag({content:factor.script});
  await page.addScriptTag({content:member.script});
  await page.addScriptTag({content:'const hyState={inbound:new Inbound()}; const hyApp=new Vue({el:"#hy-test",data:hyState});'});
  await page.evaluate(()=>{window.saved=[];twoFactorModal.show({token:'JBSWY3DPEHPK3PXP',confirm:async(success,code,password)=>{saved.push({code,password});return saved.length>1;}})});
  const inputs=page.locator('#two-factor-modal .ant-modal input');
  await inputs.nth(0).fill('current-password');await inputs.nth(1).fill('۱۲۳۴۵۶');
  await page.locator('#two-factor-modal .ant-modal-footer .ant-btn-primary').click();
  await page.waitForFunction(()=>saved.length===1 && !twoFactorModal.submitting);
  assert.equal(await inputs.nth(0).inputValue(),'current-password','failed enrollment preserves password for retry');
  assert.equal(await page.evaluate(()=>twoFactorModal.visible),true);
  await page.locator('#two-factor-modal .ant-modal-footer .ant-btn-primary').click();
  await page.waitForFunction(()=>!twoFactorModal.visible);
  assert.equal(await page.evaluate(()=>saved[1].code),'۱۲۳۴۵۶');
  await page.evaluate(()=>{
   membershipInfoModal.record={id:1,protocol:'ssh',address:'panel.example',port:22};
   membershipInfoModal.client={email:'tracking',enable:true};
   membershipInfoModal.cards=[{server:'relay.example',port:2222,username:'ssh-login',password:'ssh-password',qr:'ssh://credentials',showCredentials:true}];
   membershipInfoModal.visible=true;
  });
  const detail=page.locator('#membership-info-modal .ant-modal');await detail.waitFor();
  assert.match(await detail.innerText(),/ssh-login/);assert.match(await detail.innerText(),/ssh-password/);
  assert.match(await detail.innerText(),/relay.example:2222/);assert.doesNotMatch(await detail.innerText(),/panel.example/);
  await page.evaluate(()=>membershipInfoModal.close());
  const obfs=page.locator('#hy-test input').first();await obfs.fill('obfs @?#% exact');
  assert.equal(await page.evaluate(()=>hyState.inbound.stream.finalmask.udp[0].settings.password),'obfs @?#% exact');
  await obfs.fill('');assert.equal(await page.evaluate(()=>hyState.inbound.stream.finalmask.udp.length),0);
  assert.deepEqual(errors,[]);
  console.log('Browser: 2FA submission/retry, SSH QR credentials, external address and Hysteria obfs field passed');
 } finally {await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
