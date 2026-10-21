const assert = require('node:assert/strict');
const fs = require('node:fs');
const {chromium} = require('playwright');

// Run the shipped row menu, modal and handlers, rather than a second version of
// the renewal UI. Server-side accounting is covered by the transaction tests.
const source = fs.readFileSync('web/html/clients.html', 'utf8');
const translate = s => s.replace(/{{\s*i18n\s+"([^"]+)"\s*}}/g, '$1');
const renewIndex = source.indexOf('<a-menu-item key="renew">');
assert.ok(renewIndex > 0);
const menuStart = source.lastIndexOf('<a-dropdown', renewIndex);
const menu = source.slice(menuStart, source.indexOf('</a-dropdown>', renewIndex) + '</a-dropdown>'.length);
const modalStart = source.indexOf('<a-modal v-model="renewal.visible"');
const modal = source.slice(modalStart, source.indexOf('</a-modal>', modalStart) + '</a-modal>'.length);
function method(name) {
  const start = source.indexOf('      '+name);
  assert.ok(start > 0, name);
  return source.slice(start, source.indexOf('\n      },', start) + '\n      },'.length);
}

(async () => {
  const browser = await chromium.launch({executablePath:process.env.NODE_UI_CHROME || '/usr/bin/chromium', headless:true, args:['--no-sandbox']});
  try {
    const page = await browser.newPage({viewport:{width:390,height:844}});
    const errors=[]; page.on('pageerror', e=>errors.push(e.message));
    await page.setContent('<div id="renew-test">'+translate(menu+modal)+'</div>');
    await page.addStyleTag({path:'web/assets/ant-design-vue/antd.min.css'});
    for (const path of ['web/assets/vue/vue.min.js','web/assets/ant-design-vue/antd.min.js']) await page.addScriptTag({path});
    await page.addScriptTag({content:translate(`
      const themeSwitcher={currentTheme:'light'};
      const fixture={posts:[],reject:false,hold:null,reloads:0,balances:0};
      const HttpUtil={async post(path,body){fixture.posts.push({path,body});if(fixture.hold)await fixture.hold;return {success:!fixture.reject}}};
      const app=new Vue({el:'#renew-test',delimiters:['[[',']]'],
        data:{themeSwitcher,row:{email:'Renew Customer',totalGB:10737418240,expiryTime:Date.now()-1,renewalDuration:2592000000,enable:false,memberships:[]},
          subSettings:{enable:false},renewal:{visible:false,loading:false,row:null,duration:0,needsDays:false,days:null}},
        methods:{
          ${method('rowAction(key, row)')}
          ${method('openRenew(row)')}
          ${method('async renewAccount()')}
          isServed(){return false},isAccountFrozen(){return false},sizeFormat(){return '10 GB'},
          async getDBInbounds(){fixture.reloads++},async refreshResellerBalance(){fixture.balances++}
        }
      });
    `)});
    await page.locator('#renew-test > .ant-dropdown-trigger').click();
    await page.locator('.ant-dropdown-menu-item').filter({hasText:'pages.clients.renew'}).click();
    const dialog=page.locator('.ant-modal:visible');
    await dialog.waitFor();
    assert.match(await dialog.innerText(),/30/);
    assert.match(await dialog.innerText(),/10 GB/);
    assert.equal(await dialog.locator('input').count(),0,'known package must not ask for days again');
    await page.evaluate(()=>{fixture.hold=new Promise(resolve=>window.finishRenew=resolve)});
    await dialog.locator('.ant-modal-footer .ant-btn-primary').click();
    await page.waitForFunction(()=>app.renewal.loading);
    await page.evaluate(()=>app.renewAccount());
    assert.equal(await page.evaluate(()=>fixture.posts.length),1,'double click sent a duplicate renewal');
    await page.evaluate(()=>{finishRenew();fixture.hold=null});
    await page.waitForFunction(()=>!app.renewal.visible);
    assert.deepEqual(await page.evaluate(()=>fixture.posts[0]),{path:'/panel/api/inbounds/renewClient/Renew%20Customer',body:{renewalDuration:2592000000}});
    assert.equal(await page.evaluate(()=>fixture.reloads),1);
    assert.equal(await page.evaluate(()=>fixture.balances),1);

    // A legacy expired account asks once, validates before posting, and keeps its
    // draft if the server rejects the renewal (e.g. insufficient seller balance).
    await page.evaluate(()=>app.openRenew({...app.row,renewalDuration:0}));
    await dialog.locator('input').waitFor();
    await dialog.locator('.ant-modal-footer .ant-btn-primary').click();
    assert.equal(await page.evaluate(()=>fixture.posts.length),1);
    await dialog.locator('input').fill('45');
    await dialog.locator('input').press('Tab');
    await page.evaluate(()=>fixture.reject=true);
    await dialog.locator('.ant-modal-footer .ant-btn-primary').click();
    await page.waitForFunction(()=>fixture.posts.length===2&&!app.renewal.loading);
    assert.equal(await dialog.locator('input').inputValue(),'45');
    assert.equal(await dialog.isVisible(),true);
    assert.equal(await page.evaluate(()=>fixture.posts[1].body.renewalDuration),45*86400000);
    await page.evaluate(()=>fixture.reject=false);
    await dialog.locator('.ant-modal-footer .ant-btn-primary').click();
    await page.waitForFunction(()=>!app.renewal.visible);

    for (const row of [
      {expiryTime:0,totalGB:0,enable:false,renewalDuration:0},
      {expiryTime:0,totalGB:10737418240,enable:true,renewalDuration:0},
      {expiryTime:-2592000000,totalGB:0,enable:true,renewalDuration:0},
      {expiryTime:-432000000,totalGB:10737418240,enable:false,renewalDuration:2592000000},
    ]) {
      await page.evaluate(row=>app.openRenew({...app.row,...row}),row);
      await dialog.waitFor();
      assert.equal(await dialog.locator('input').count(),0);
      await dialog.locator('.ant-modal-footer .ant-btn-primary').click();
      await page.waitForFunction(()=>!app.renewal.visible);
      assert.equal(await page.evaluate(()=>fixture.posts.at(-1).body.renewalDuration),row.expiryTime===0?0:2592000000);
    }
    assert.deepEqual(errors,[]);
    console.log('Renewal mobile menu, legacy days, known/unlimited/frozen plans, loading guard, failed draft and refresh passed');
  } finally {await browser.close()}
})().catch(error=>{console.error(error);process.exit(1)});
