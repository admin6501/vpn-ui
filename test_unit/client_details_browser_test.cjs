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
  const page=await browser.newPage();page.setDefaultTimeout(10000);const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const factor=template('web/html/modals/two_factor_modal.html');
  const member=template('web/html/modals/membership_info_modal.html');
  const account=template('web/html/modals/account_info_modal.html');
  const hysteria=template('web/html/form/stream/stream_hysteria.html');
  await page.setContent(factor.markup+member.markup+account.markup+'<div id="hy-test">'+hysteria.markup+'</div>');
  await page.addStyleTag({path:'web/assets/ant-design-vue/antd.min.css'});
  for (const path of ['web/assets/moment/moment.min.js','web/assets/vue/vue.min.js','web/assets/ant-design-vue/antd.min.js','web/assets/otpauth/otpauth.umd.min.js','web/assets/qrcode/qrious2.min.js','web/assets/js/util/index.js','web/assets/uri/URI.min.js','web/assets/js/model/inbound.js','web/assets/js/model/dbinbound.js','web/assets/js/util/export.js']) await page.addScriptTag({path});
  await page.addScriptTag({content:'const themeSwitcher={currentTheme:"light"}; const app={};'});
  await page.addScriptTag({content:factor.script});
  await page.addScriptTag({content:member.script});
  await page.addScriptTag({content:account.script});
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
  await page.setViewportSize({width:390,height:844});
  await page.evaluate(()=>{
   const client={id:'ssh-login',email:'tracking',password:'ssh-password',enable:true};
   const endpoints=[{dest:'edge.one.example',port:2223},{dest:'edge.two.example',port:2224}];
   const records=[0,1,2].map((count,index)=>new DBInbound({id:index+1,protocol:'ssh',remark:'SSH-'+count,
    listen:'panel.example',port:2222,clientStats:[],settings:JSON.stringify({clients:[client],externalProxy:endpoints.slice(0,count)})}));
   records.push(new DBInbound({id:4,protocol:'vless',remark:'VLESS',listen:'panel.example',port:443,clientStats:[],
    settings:JSON.stringify({clients:[{...client,id:'56cd771a-e16a-4878-9b8c-82f1a1f1c5a7'}]}),
    streamSettings:JSON.stringify({network:'tcp',security:'none',externalProxy:endpoints})}));
   accountInfoModalApp.app={dbInbounds:records,accountInbounds:()=>records,
    accountClientIn:(row,record)=>record.toInbound().clients,
    getInboundClients:record=>record.toInbound().clients,
    membershipEnabled:()=>true,membershipLampClass:()=>'',membershipOnline:()=>false,
    isAccountFrozen:()=>false,getInboundSumStats:()=>0,getSumStats:()=>0,loadOnlineState:async()=>{}};
   HttpUtil.get=async function(path){
    const record=records.find(record=>record.id===Number(path.split('/')[4]));
    const eps=record.toInbound().settings.externalProxy;
    return {success:true,obj:(eps.length?eps:[{dest:record.address,port:record.port}]).map(ep=>({host:ep.dest,port:ep.port,link:'ssh://'+btoa('ssh-login:ssh-password@'+ep.dest+':'+ep.port)}))};
   };
   accountInfoModal.show({email:'tracking',enable:true});
  });
  const expanded=page.locator('#account-info-modal .ant-modal');await expanded.waitFor();
  for(const [id,hostnames] of [[1,['panel.example:2222']],[2,['edge.one.example:2223']],[3,['edge.one.example:2223','edge.two.example:2224']],[4,['edge.one.example:2223','edge.two.example:2224']]]) {
   await expanded.locator('.ai-item-h').nth(id-1).locator('.ai-item-nm').click();
   await page.waitForFunction(id=>Array.isArray(accountInfoModal.cards[id])&&!accountInfoModal.loading[id],id).catch(async error=>{console.log(await page.evaluate(()=>({open:accountInfoModal.open,cardIds:Object.keys(accountInfoModal.cards),loading:accountInfoModal.loading})));throw error;});
   const body=expanded.locator('.ai-item-b');
   const text=await body.innerText();
   for(const host of hostnames)assert.ok(text.includes(host),'missing advertised endpoint '+host+' on inbound '+id+'; '+text);
   if(id!==1)assert.doesNotMatch(text,/panel.example/,'Account Info must not advertise the panel over external proxies');
   if(id<=3){assert.match(text,/ssh-login/);assert.match(text,/ssh-password/);assert.ok(await body.locator('canvas').count()>0,'SSH must retain its QR');}
  }
  await page.evaluate(()=>accountInfoModal.close());
  const obfs=page.locator('#hy-test input').first();await obfs.fill('obfs @?#% exact');
  assert.equal(await page.evaluate(()=>hyState.inbound.stream.finalmask.udp[0].settings.password),'obfs @?#% exact');
  await obfs.fill('');assert.equal(await page.evaluate(()=>hyState.inbound.stream.finalmask.udp.length),0);
  const confText='[Interface]\nPrivateKey = fixture-private\nAddress = 10.10.0.2/32\n\n[Peer]\nPublicKey = fixture-public\nEndpoint = fixture.example:51820\nAllowedIPs = 0.0.0.0/0\n';
  const downloaded=page.waitForEvent('download');
  await page.evaluate(config=>AccountExport.txt([{email:'wg-client',configText:config}], 'wg-client.txt'),confText);
  const file=await downloaded;
  assert.equal(file.suggestedFilename(),'wg-client.conf','WireGuard download must be .conf');
  assert.equal(fs.readFileSync(await file.path(),'utf8'),confText,'WireGuard .conf must contain raw importable INI without report decorations');
  assert.deepEqual(errors,[]);
  console.log('Browser: Account Info mobile expanders (0/1/2 proxies), Membership Info, SSH credentials/QR, 2FA and Hysteria obfs passed');
 } finally {await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
