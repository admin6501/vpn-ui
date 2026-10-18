const { chromium } = require('playwright');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({executablePath: process.env.NODE_UI_CHROME || '/usr/bin/chromium', headless: true, args: ['--no-sandbox']});
  try {
    const expected = JSON.parse(process.env.SUBSCRIPTION_EXPECT);
    const context = await browser.newContext({locale: expected.lang, viewport: {width: 390, height: 844}});
    await context.addCookies([{name: 'lang', value: expected.lang, url: process.env.SUBSCRIPTION_BROWSER_URL}]);
    const page = await context.newPage();
    const errors = [];
    const failures = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('response', response => { if (response.status() >= 400) failures.push([response.status(), new URL(response.url()).pathname]); });
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()); });
    await page.goto(process.env.SUBSCRIPTION_BROWSER_URL, {waitUntil: 'networkidle'});
    assert.deepEqual(failures, [], 'all subscription assets must load');
    assert.deepEqual(errors, [], 'subscription must render without JavaScript errors');
    await page.locator('#app').waitFor({state: 'visible', timeout: 3000});
    const text = await page.locator('#app').innerText();
    const rows = page.locator('.ant-descriptions-row');
    const value = async index => (await rows.nth(index).locator('td').innerText()).trim();
    assert.equal(await value(2), '2.00GB', 'downloaded usage is visible');
    assert.equal(await value(3), '1.00GB', 'uploaded usage is visible');
    assert.equal(await value(4), '3.00GB', 'account usage is counted once across three inbounds');
    assert.equal(await value(5), expected.total > 0 ? (expected.total / 2**30).toFixed(2) + 'GB' : '∞', 'real quota is visible');
    assert.match(await value(-2), /[0-9۰-۹]/, 'last-online timestamp remains visible');
    assert.match(await value(-1), /[0-9۰-۹]/, 'expiry remains visible');
    const status = await value(1);
    const label = expected.lang === 'fa-IR' ? (expected.active ? 'فعال' : 'غیرفعال') : (expected.active ? 'Active' : 'Inactive');
    assert.equal(status.trim(), label, 'status must reflect enable, expiry and exhausted quota');
    assert.ok(!text.includes('[['), 'no unrendered bindings remain');
    if (expected.links) {
      for (const scheme of ['vmess://', 'ss://', 'wireguard://']) assert.ok(text.includes(scheme), scheme + ' is visible');
      const downloads = page.locator('a.sub-config-btn');
      assert.equal(await downloads.count(), 1, 'AWG config file is offered');
      const response = await context.request.get(await downloads.first().getAttribute('href'));
      assert.equal(response.status(), 200);
      assert.match(await response.text(), /\[Interface\]/);
      assert.match(response.headers()['content-disposition'], /attachment/);
    } else {
      for (const scheme of ['vmess://', 'ss://', 'wireguard://']) assert.ok(!text.includes(scheme), 'disabled membership stays out of config exports');
    }
    console.log('Subscription page: visible account-wide traffic, quota, status, three protocols, config downloads, locale and mobile rendering passed.');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exit(1); });
