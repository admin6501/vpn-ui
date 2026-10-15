const {chromium}=require('playwright');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.NODE_UI_CHROME||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 try {
  const page=await browser.newPage({viewport:{width:1280,height:900}});page.setDefaultTimeout(10000);
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.context().addCookies([{url:process.env.ADMIN_UI_URL,name:'lang',value:process.env.NODE_UI_LANG}]);
  await page.goto(process.env.ADMIN_UI_URL,{waitUntil:'networkidle'});
  await page.waitForFunction(()=>app.admins && app.admins.some(u=>u.isPanelOwner));
  await page.evaluate(()=>app.openEdit(app.admins.find(u=>u.isPanelOwner)));
  await page.locator('#admin-modal .ant-modal').waitFor();
  if(!await page.evaluate(()=>adminModal.canDemote && adminModal.admin.permissions.includes('manageNodes') && adminModal.isSelf))throw Error('self-demotion blocked or super-admin node permission missing');
  if(await page.locator('#admin-modal .ant-switch').nth(1).isDisabled())throw Error('owner super-admin switch is disabled');
  await page.evaluate(()=>adminModal.close());
  await page.evaluate(()=>app.openRole());await page.locator('#admin-role-modal .ant-modal').waitFor();
  await page.evaluate(()=>{app.roleModal.name='Location operator';app.roleModal.permissions=['manageNodes']});
  for(const slug of ['accessGroups','createGroup','editGroup','deleteGroup']){
   const label=await page.evaluate(slug=>PERMISSION_LABELS[slug],slug);
   if(!label||label===slug)throw Error('missing group permission label '+slug);
   await page.locator('#admin-role-modal .ant-checkbox-wrapper').filter({hasText:label}).locator('input').check();
  }
  await page.locator('#admin-role-modal .ant-modal-footer button.ant-btn-primary').click();
  await page.waitForFunction(()=>app.roles.length===1);
  await page.evaluate(()=>{app.openAdd();adminModal.admin.username='operator';adminModal.admin.password='browser-role-test-password'});
  await page.locator('#admin-modal .ant-select').click();
  await page.locator('.ant-select-dropdown:visible .ant-select-dropdown-menu-item').filter({hasText:'Location operator'}).click();
  if(!await page.evaluate(()=>adminModal.admin.permissions.includes('manageNodes')))throw Error('role selection did not inherit permissions');
  await page.locator('#admin-modal .ant-checkbox-wrapper').filter({hasText:'Admin group A'}).locator('input').check();
  const saved=page.waitForResponse(r=>r.url().endsWith('/admins/add'));

  await page.locator('#admin-modal .ant-modal-footer button.ant-btn-primary').click();
  const response=await saved;const payload=new URLSearchParams(response.request().postData());
  if(payload.has('inboundIds')||!payload.has('groupIds'))throw Error('admin form submitted individual inbounds');
  await page.waitForFunction(()=>app.admins.some(u=>u.username==='operator' && u.adminRoleName==='Location operator' && u.groupIds.length===1 && u.permissions.includes('accessGroups')));

  await page.evaluate(async()=>{await app.loadRoles()});
  if(!await page.evaluate(()=>app.roles[0].adminCount===1))throw Error('role membership count missing');
  const refusal=await page.evaluate(async()=>await HttpUtil.post('/panel/admins/roles/delete/'+app.roles[0].id));
  if(refusal.success)throw Error('assigned role was deleted');
  await page.evaluate(()=>{app.openRole(app.roles[0]);app.roleModal.permissions=['accessInbounds']});
  await page.locator('#admin-role-modal .ant-modal-footer button.ant-btn-primary').click();
  await page.waitForFunction(()=>app.admins.find(u=>u.username==='operator').permissions.includes('accessInbounds') && !app.admins.find(u=>u.username==='operator').permissions.includes('manageNodes'));
  await page.evaluate(()=>{const operator=app.admins.find(u=>u.username==='operator');app.openEdit(operator);adminModal.admin.adminRoleId=0});
  await page.locator('#admin-modal .ant-modal-footer button.ant-btn-primary').click();
  await page.waitForFunction(()=>app.admins.find(u=>u.username==='operator').adminRoleId===0);
  await page.evaluate(async()=>{await app.loadRoles();await HttpUtil.post('/panel/admins/roles/delete/'+app.roles[0].id);await app.loadRoles()});
  await page.waitForFunction(()=>app.roles.length===0);
  await page.evaluate(()=>app.openEdit(app.admins.find(u=>u.isPanelOwner)));
  await page.locator('#admin-modal .ant-switch').nth(1).click();
  if(await page.evaluate(()=>adminModal.admin.isSuperAdmin))throw Error('actual switch did not demote owner');
  await page.evaluate(()=>{adminModal.admin.permissions=['manageNodes']});
  await page.locator('#admin-modal .ant-modal-footer button.ant-btn-primary').click();
  await page.waitForFunction(()=>typeof app!=='undefined' && Array.isArray(app.admins) && app.admins.some(u=>u.isPanelOwner && !u.isSuperAdmin));
  if(await page.locator('.bo-rail-item[href$="/panel/admins"]').count()!==1)throw Error('owner lost administrator management after demotion');
  await page.evaluate(()=>app.openRole());await page.locator('#admin-role-modal .ant-modal').waitFor();
  if(errors.length)throw Error(errors.join('\n'));
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
