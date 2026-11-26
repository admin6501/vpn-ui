const { chromium } = require('playwright');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({executablePath: process.env.NODE_UI_CHROME || '/usr/bin/chromium', headless: true, args: ['--no-sandbox']});
  try {
    const expected = JSON.parse(process.env.SUBSCRIPTION_EXPECT);
    const context = await browser.newContext({locale: expected.lang, viewport: {width: 390, height: 844}});
    await context.addCookies([{name: 'lang', value: 'en-US', url: process.env.SUBSCRIPTION_BROWSER_URL}]);
    const page = await context.newPage();
    const errors = [];
    const failures = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('response', response => { if (response.status() >= 400) failures.push([response.status(), new URL(response.url()).pathname]); });
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()); });
    const url = new URL(process.env.SUBSCRIPTION_BROWSER_URL);
    if (expected.lang !== 'fa-IR') url.searchParams.set('lang', expected.lang);
    await page.goto(url.href, {waitUntil: 'networkidle'});
    assert.equal(await page.locator('html').getAttribute('lang'), expected.lang);
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
    assert.match(await value(-2), /\p{Decimal_Number}/u, 'last-online timestamp remains visible');
    assert.match(await value(-1), /\p{Decimal_Number}/u, 'expiry remains visible');
    const status = await value(1);
    const label = expected.status;
    assert.equal(status.trim(), label, 'status must reflect enable, expiry and exhausted quota');
    assert.ok(!text.includes('[['), 'no unrendered bindings remain');
    if (expected.links) {
      for (const scheme of ['vmess://', 'ss://']) assert.ok(text.includes(scheme), scheme + ' is visible');
      assert.ok(!text.includes('wireguard://'), 'AWG must not be rendered as a plain WireGuard link');
      const awg = page.locator('.sub-native-card').filter({hasText: 'AmneziaWG'});
      assert.match(await awg.innerText(), /awg-relay\.example\.test[\s\S]*55080/);
      if (expected.checkNodeLabels) {
        for (const proto of ['hysteria2', 'tuic', 'anytls', 'naive']) {
          const label = page.locator('.qr-box .ant-tag').filter({hasText: 'Node ' + proto});
          assert.match(await label.innerText(), /7\.00GB📊.*(?:\d+D|\d+H)⏳/);
        }
      }
      const title = page.locator('.ant-divider bdi');
      if (expected.lang === 'fa-IR') {
        const heading = await title.evaluate(element => {
          const label = element.textContent.trim();
          const range = document.createRange();
          range.setStart(element.firstChild, 0);
          range.setEnd(element.firstChild, 1);
          const first = range.getBoundingClientRect();
          const vpn = label.indexOf('VPN');
          range.setStart(element.firstChild, vpn);
          range.setEnd(element.firstChild, vpn + 3);
          return { label, direction: getComputedStyle(element).direction, vpnBeforeReadingStart: range.getBoundingClientRect().right < first.left };
        });
        assert.deepEqual(heading, { label: 'مشخصات اتصال پروتکل‌های VPN', direction: 'rtl', vpnBeforeReadingStart: true });
      } else if (expected.lang === 'en-US') {
        assert.equal(await title.evaluate(element => getComputedStyle(element).direction), 'ltr');
      }
      const downloads = page.locator('a.sub-config-btn');
      assert.equal(await downloads.count(), 1, 'AWG config file is offered');
      const response = await context.request.get(await downloads.first().getAttribute('href'));
      assert.equal(response.status(), 200);
      const config = await response.text();
      assert.match(config, /\[Interface\]/);
      assert.match(config, /^Endpoint = awg-relay\.example\.test:55080$/m);
      assert.match(response.headers()['content-disposition'], /attachment/);
    } else {
      for (const scheme of ['vmess://', 'ss://', 'wireguard://']) assert.ok(!text.includes(scheme), 'disabled membership stays out of config exports');
      assert.equal(await page.locator('a.sub-config-btn').count(), 0, 'manually disabled profiles cannot be downloaded');
    }
    assert.equal(await page.getByRole('link', {name: 'v2rayNG'}).getAttribute('href'), 'https://github.com/2dust/v2rayNG/releases');
    const themeColors = await page.locator('.sub-native-card').first().evaluate(card => {
      const reference = document.createElement('span');
      reference.style.color = 'var(--text-2)';
      document.body.append(reference);
      const actual = [card, card.querySelector('h3')].map(element => getComputedStyle(element).color);
      const gray = getComputedStyle(reference).color;
      reference.remove();
      return { actual, gray, dark: document.body.classList.contains('dark') };
    });
    assert.ok(themeColors.dark && themeColors.actual.every(color => color === themeColors.gray),
      'card and heading use readable gray on the default dark theme');
    assert.equal((await context.cookies()).find(cookie => cookie.name === 'lang').value, 'en-US');
    console.log('Subscription page: visible account-wide traffic, quota, status, three protocols, config downloads, locale and mobile rendering passed.');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exit(1); });
