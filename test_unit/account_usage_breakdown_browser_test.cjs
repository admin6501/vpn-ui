const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require('playwright');

function template(path) {
  const source = fs.readFileSync(path, 'utf8')
    .replace(/{{\s*i18n\s+"([^"]+)"\s*}}/g, '$1');
  const start = source.indexOf('<script>');
  return {
    markup: source.slice(0, start).replace(/{{define[^}]*}}|{{end}}/g, ''),
    script: source.slice(start + 8, source.lastIndexOf('</script>')),
  };
}

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.NODE_UI_CHROME || '/usr/bin/chromium',
    headless: true, args: ['--no-sandbox'],
  });
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const account = template('web/html/modals/account_info_modal.html');
    const member = template('web/html/modals/membership_info_modal.html');
    const mixin = template('web/html/component/clientActions.html');
    await page.setContent(account.markup + member.markup);
    for (const path of ['web/assets/ant-design-vue/antd.min.css', 'web/assets/css/tokens.css',
      'web/assets/css/custom.css', 'web/assets/css/components.css', 'web/assets/css/ant-bridge.css']) {
      await page.addStyleTag({ path });
    }
    for (const path of ['web/assets/moment/moment.min.js', 'web/assets/vue/vue.min.js',
      'web/assets/ant-design-vue/antd.min.js', 'web/assets/js/util/index.js']) {
      await page.addScriptTag({ path });
    }
    await page.addScriptTag({ content: `
      const themeSwitcher = {currentTheme: 'dark'};
      const RESELLER = {isReseller: false};
      const ProtocolLabels = {vmess:'VMess', shadowsocks:'Shadowsocks', awg:'AmneziaWG'};
      const AccountExport = {buildCards: async () => [], ovpnTransports: () => []};
      document.body.className = 'dark';
    ` });
    await page.addScriptTag({ content: mixin.script });
    await page.addScriptTag({ content: account.script });
    await page.addScriptTag({ content: member.script });
    await page.addScriptTag({ content: `
      const MB = 1024 * 1024;
      const pool = 72620 * MB;
      const measured = [0, 554 * MB, Math.floor(26026 * MB / 100)];
      const total = pool + measured.reduce((a,b) => a+b, 0);
      const client = {email:'mycon', enable:true};
      const records = ['vmess','shadowsocks','awg'].map((protocol,index) => ({
        id:index+1, protocol, remark:['VMess1','Shadowsocks1','Amnezia1'][index],
        clientStats:[{email:'mycon',up:0,down:total,
          inboundUp:0,inboundDown:measured[index]+(index<2?pool:0),
          sharedUp:0,sharedDown:index<2?pool:0,shared:index<2}],
      }));
      const app = {...ClientActionsMixin.methods, onlineClients:[],
        subSettings:{enable:false}, accountInbounds:()=>records, accountClientIn:()=>[client],
        membershipEnabled:()=>true, membershipLampClass:()=>'', membershipOnline:()=>false,
        membershipOnlineText:()=> 'offline', isAccountFrozen:()=>false, loadOnlineState:async()=>{}};
      accountInfoModalApp.app = app;
      membershipInfoModalApp.app = app;
      accountInfoModal.show({email:'mycon',enable:true,up:0,down:total});
    ` });
    const modal = page.locator('#account-info-modal .ant-modal');
    await modal.waitFor();
    assert.deepEqual(await page.evaluate(() => records.map(record => SizeFormatter.sizeFormat(app.getInboundSumStats(record, 'mycon')))),
      ['70.92 GB', '71.46 GB', '260.26 MB'], 'fixture reproduces the reported misleading figures');
    for (const [width, height, dir] of [[390,844,'rtl'], [1280,900,'ltr']]) {
      await page.setViewportSize({width,height});
      await page.evaluate(dir => document.documentElement.dir = dir, dir);
      const figures = await modal.locator('.ai-item-use').allTextContents();
      assert.deepEqual(figures.map(s=>s.trim()), ['0 B','554.00 MB','260.26 MB']);
      assert.match(await modal.locator('.ai-usage-note').innerText(), /70\.92 GB/);
      assert.equal(await modal.locator('.ai-usage-note').count(), 1, 'pool appears once');
      assert.match(await modal.locator('.ai-meter-v').first().innerText(), /71\.71 GB/);
      assert.equal(await page.evaluate(() => accountInfoModalApp.sharedBytes), 72620*1024*1024);
      assert.equal(await page.evaluate(() => measured.reduce((a,b)=>a+b,0) + accountInfoModalApp.sharedBytes),
        await page.evaluate(() => accountInfoModalApp.usedBytes), 'breakdown reconciles with account total');
    }
    await modal.locator('.ai-item-nm').nth(1).click();
    assert.match(await modal.locator('.ai-item-b').innerText(), /554\.00 MB/);
    await page.evaluate(() => {
      accountInfoModal.close();
      membershipInfoModal.record = records[1];
      membershipInfoModal.client = client;
      membershipInfoModal.visible = true;
    });
    const detail = page.locator('#membership-info-modal .ant-modal');
    await detail.waitFor();
    assert.match(await detail.innerText(), /554\.00 MB/);
    assert.match(await detail.innerText(), /70\.92 GB/);
    await page.evaluate(() => {
      membershipInfoModal.close();
      // A reset retains lifetime usage but has no current shared traffic.
      for (const record of records) {
        record.clientStats = [{email:'mycon',up:0,down:0,inboundUp:0,inboundDown:0,
          sharedUp:0,sharedDown:0,sharedAllTime:pool,shared:true}];
        delete record._clientStatsMap;
      }
      accountInfoModal.show({email:'mycon',enable:true,up:0,down:0});
    });
    await modal.waitFor();
    assert.equal(await modal.locator('.ai-usage-note').count(), 0, 'lifetime pool is not billed current usage');
    assert.deepEqual((await modal.locator('.ai-item-use').allTextContents()).map(s=>s.trim()), ['0 B','0 B','0 B']);
    assert.deepEqual(errors, []);
    console.log('Account/membership usage: shared pool counted once, measured shares reconcile, mobile RTL/desktop and reset passed');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exit(1); });
