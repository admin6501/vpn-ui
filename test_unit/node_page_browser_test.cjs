const {chromium}=require('playwright');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try{
  const page=await browser.newPage({viewport:{width:390,height:844}});
  page.setDefaultTimeout(10000);
  await page.context().addCookies([{url:process.env.NODE_UI_URL,name:"lang",value:process.env.NODE_UI_LANG || "en-US"}]);
  const errors=[];page.on('pageerror',err=>{errors.push(err.message);console.error('Page error:',err.message)});
  await page.goto(process.env.NODE_UI_URL,{waitUntil:'networkidle'});
  await page.locator('#app:not([v-cloak])').waitFor({state:'visible'});
  await page.locator('.bo-topbar-title').waitFor({state:'visible'});
  if(await page.locator('a-sidebar').count())throw Error('sidebar component not mounted');
  if(await page.locator('.bo-rail-item[href$="/panel/nodes"]').count()!==1 || await page.locator('.bo-rail-item[href$="/panel/admins"]').count()!==0)throw Error('delegated node menu missing or root administrator menu exposed');
  await page.locator('button.ant-btn-primary').first().click();
  await page.locator('.ant-modal:visible').waitFor({state:'visible'});
  const componentLocale=await page.evaluate(()=>PanelComponentLocales[LanguageManager.getLanguage()]);
  const okText=await page.locator('.ant-modal:visible .ant-modal-footer button.ant-btn-primary').innerText();
  if(okText.replace(/\s/g,'')!==componentLocale.Modal.okText.replace(/\s/g,''))throw Error('component dialog language differs from panel language: '+okText);
  if(await page.evaluate(()=>moment.locale())!=='en')throw Error('component locales changed default wire date formatting');
  const inputs=page.locator('.ant-modal:visible input');
  await inputs.nth(0).fill('Germany');await inputs.nth(1).fill('de.example.com');
  await page.locator('.ant-modal:visible .ant-modal-footer button.ant-btn-primary').click();
  await page.locator('.ant-modal:visible textarea').waitFor({state:'visible'});
  const command=await page.locator('.ant-modal:visible textarea').inputValue();
  if(!command.includes('install-node.sh')||!command.includes('/test/'))throw Error('pairing command missing web path');

  await page.locator('.ant-modal:visible').filter({has:page.locator('textarea')}).locator('.ant-modal-close').click();
  // Real library components must inherit the selected panel language too.
  await page.evaluate(()=>{
   const el=document.createElement('div');el.id='locale-probe';document.body.appendChild(el);
   window.componentProbe=new Vue({el:'#locale-probe',data:{value:moment('2026-10-11')},template:'<div id="locale-probe"><a-date-picker v-model="value"></a-date-picker><a-pagination :total="100"></a-pagination></div>'});
  });
  const datePlaceholder=await page.locator('#locale-probe .ant-calendar-picker input').getAttribute('placeholder');
  if(datePlaceholder!==componentLocale.DatePicker.lang.placeholder)throw Error('date picker language differs');
  const nextPage=await page.locator('#locale-probe .ant-pagination-next').getAttribute('title');
  if(nextPage!==componentLocale.Pagination.next_page)throw Error('pagination language differs');
  await page.locator('#locale-probe .ant-calendar-picker input').click();
  const today=await page.locator('.ant-calendar:visible .ant-calendar-today-btn').innerText();
  if(today!==componentLocale.DatePicker.lang.today)throw Error('calendar language differs');
  await page.keyboard.press('Escape');
  await page.evaluate(()=>{const el=componentProbe.$el;componentProbe.$destroy();el.remove();delete window.componentProbe});
  // A synthetic agent report exercises the UI; requests still use the actual
  // controller/database. The real-agent integration test verifies execution.
  let currentCore='xray',submitted=null;
  const meta=await page.request.get(new URL('1/xray/meta',process.env.NODE_UI_URL).href);
  const defaults=(await meta.json()).obj.defaults;
  await page.route('**/panel/nodes/*/config',async route=>{
   const form=new URLSearchParams(route.request().postData());
   currentCore=form.get('core');if(form.get('text'))submitted=form.get('text');
   await route.continue();
  });
  await page.route('**/panel/nodes/list',async route=>{
   const response=await route.fetch();const body=await response.json();
   for(const item of body.obj||[]){item.paired=true;item.online=true;item.report={catalog:[{name:'xray',title:'Xray'},{name:'awg',title:'AmneziaWG'}],coreActionSupported:true,coreActionGeneration:String(item.node.coreActionGeneration),coreActionError:'',cores:[{name:'xray',state:'running',detail:''}],controlSupported:true,runtimeRunning:!item.node.paused,controlGeneration:String(item.node.controlGeneration),configGeneration:String(item.node.configGeneration),installed:['xray','openvpn'],configError:'',configTargets:[currentCore==='xray' ? {core:'xray',inbound:0,file:'config.json',label:'Xray template',generated:JSON.stringify({...defaults,inbounds:[...(defaults.inbounds || []),{tag:'turkey-node-vless',protocol:'vless',port:24443,settings:{clients:[]}}]}),current:submitted || JSON.stringify(defaults)} : {core:'openvpn',inbound:0,file:'server.conf',label:'server.conf',format:'conf',current:'# fixture\nport 1194'}]};}
   await route.fulfill({response,json:body});
  });
  await page.evaluate(()=>app.load());
  const card=page.locator('.ant-card').filter({has:page.locator('.ant-card-head-title',{hasText:'Germany'})}).last();
  const buttons=card.locator('.ant-card-body button.ant-btn');
  await card.locator('button[data-node-action="cores"]').click(); // Core installer
  await page.waitForFunction(()=>app.coresVisible===true);
  const corePicker=page.locator('.ant-modal:visible');
  await corePicker.waitFor({state:'visible'});
  const xrayChoice=corePicker.locator('.ant-checkbox-wrapper').filter({hasText:'Xray'});
  const xrayCount=await xrayChoice.count();
  if(xrayCount!==1)throw Error('Expected one Xray option in the node installer; found '+xrayCount);
  if(!(await xrayChoice.innerText()).includes('VLESS'))throw Error('Xray protocol description disappeared');
  await xrayChoice.click();
  if(await page.evaluate(()=>app.selected.filter(name=>name==='xray').length)!==1)throw Error('Xray checkbox did not select exactly one core');
  await corePicker.locator('.ant-modal-close').click();
  await buttons.nth(2).click(); // Stop
  await page.waitForFunction(()=>app.nodes[0].node.paused===true);
  await buttons.nth(1).click(); // Start
  await page.waitForFunction(()=>app.nodes[0].node.paused===false && app.nodes[0].node.controlGeneration===2);
  await buttons.nth(3).click(); // Restart
  await page.waitForFunction(()=>app.nodes[0].node.controlGeneration===3);
  await buttons.nth(4).click(); // Configuration
  await page.waitForURL('**/nodes/1/xray');
  await page.waitForFunction(()=>typeof app!=='undefined' && !!app.loadingStates && app.loadingStates.fetched);
  if(await page.locator('a-sidebar').count())throw Error('node Xray sidebar not mounted');
  const centralRequests=[];
  page.on('request',req=>{if(req.url().includes('/panel/xray/') || req.url().includes('/panel/api/server/restartXrayService'))centralRequests.push(req.url())});
  // Changing a graphical option updates the same model as the main Xray page.
  await page.locator('.ant-collapse-content-active .ant-select').nth(1).click();
  await page.locator('.ant-select-dropdown:visible .ant-select-dropdown-menu-item').filter({hasText:/^IPOnDemand$/}).click();
  await page.waitForFunction(()=>!app.saveBtnDisable);
  await page.locator('button.ant-btn-primary').first().click();
  await page.waitForFunction(()=>!app.nodeConfigPending && app.oldXraySetting===app.xraySetting);
  if(!submitted || JSON.parse(submitted).routing.domainStrategy!=='IPOnDemand')throw Error('graphical node edit not submitted');
  if(!await page.evaluate(async()=>{try{await axios.post('/panel/xray/update',{xraySetting:'{}'});return false}catch(error){return true}}))throw Error('node editor did not reject central write');
  if(centralRequests.length)throw Error('node editor called central Xray API');
  await page.evaluate(async()=>{app.selectSection('tpl-advanced');await app.$nextTick();app.advSettings='nodeGeneratedConfig';app.changeCode();});
  await page.waitForFunction(()=>app.cm && app.cm.getValue().includes('turkey-node-vless'));
  if(!await page.evaluate(()=>app.cm.getOption('readOnly')===true))throw Error('generated runtime config is editable');
  await page.evaluate(async()=>{app.nodeGeneratedConfig=JSON.stringify({inbounds:[{tag:'updated-turkey-inbound'}]});await app.$nextTick();});
  await page.waitForFunction(()=>app.cm.getValue().includes('updated-turkey-inbound'));
  if(!await page.evaluate(()=>!app.xraySetting.includes('turkey-node-vless')))throw Error('runtime inbounds leaked into editable template');

  await page.goto(process.env.NODE_UI_URL,{waitUntil:'networkidle'});
  await page.waitForFunction(()=>app.nodes.length===1);
  // Daemon cores reuse the main panel CodeMirror file editor.
  await page.evaluate(()=>{app.current=app.nodes[0];app.configureCore('openvpn')});
  await page.locator('.ant-modal:visible .CodeMirror').waitFor({state:'visible'});
  if(!await page.evaluate(()=>app.configCm.getValue().includes('fixture')))throw Error('CodeMirror config did not fill');
  await page.locator('.ant-modal:visible .ant-modal-close').click();
  await buttons.nth(5).click(); // Reconnect
  await page.waitForFunction(()=>app.nodes[0].node.controlGeneration===4);
  await buttons.nth(6).click(); // Cores
  const coreRow=page.locator('.ant-modal:visible .ant-list-item').filter({hasText:'xray'}).last();
  await coreRow.locator('button.ant-btn').nth(1).click();
  await page.waitForFunction(()=>app.nodes[0].node.coreActionGeneration===1);
  await coreRow.locator('button.ant-btn').nth(4).click();
  await page.locator('.ant-popover:visible button.ant-btn-primary').click();
  await page.waitForFunction(()=>app.nodes[0].node.coreActionGeneration===2);
  if(errors.length)throw Error(errors.join('\n'));
  console.log('Node page mounted, rendered sidebar and created pairing command without JavaScript errors.');
 }finally{await browser.close();}
})().catch(err=>{console.error(err);process.exit(1)});
