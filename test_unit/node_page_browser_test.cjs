const {chromium}=require('playwright');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try{
  const page=await browser.newPage({viewport:{width:390,height:844}});
  page.setDefaultTimeout(10000);
  const errors=[];page.on('pageerror',err=>{errors.push(err.message);console.error('Page error:',err.message)});
  await page.goto(process.env.NODE_UI_URL,{waitUntil:'networkidle'});
  await page.locator('#app:not([v-cloak])').waitFor({state:'visible'});
  await page.locator('.bo-topbar-title').waitFor({state:'visible'});
  if(await page.locator('a-sidebar').count())throw Error('sidebar component not mounted');
  await page.locator('button.ant-btn-primary').first().click();
  await page.locator('.ant-modal:visible').waitFor({state:'visible'});
  const inputs=page.locator('.ant-modal:visible input');
  await inputs.nth(0).fill('Germany');await inputs.nth(1).fill('de.example.com');
  await page.locator('.ant-modal:visible .ant-modal-footer button.ant-btn-primary').click();
  await page.locator('.ant-modal:visible textarea').waitFor({state:'visible'});
  const command=await page.locator('.ant-modal:visible textarea').inputValue();
  if(!command.includes('install-node.sh')||!command.includes('/test/'))throw Error('pairing command missing web path');
  if(errors.length)throw Error(errors.join('\n'));
  console.log('Node page mounted, rendered sidebar and created pairing command without JavaScript errors.');
 }finally{await browser.close();}
})().catch(err=>{console.error(err);process.exit(1)});
