const { chromium } = require('playwright');
const assert = require('node:assert/strict');

async function checkConfigLayout(page, context) {
  const manual = page.locator('.sub-native-grid');
  const openVPN = page.locator('.sub-openvpn-list .sub-native-card');
  const fileGroups = page.locator('.sub-file-downloads .sub-config-grid');
  const downloads = fileGroups.first();
  const gre = fileGroups.last();
  assert.match(await manual.innerText(), /IKEv2/);
  assert.match(await manual.innerText(), /PPTP/);
  assert.equal(await manual.locator('.sub-config-btn').count(), 0);
  assert.match(await openVPN.locator('h3').innerText(), /OpenVPN/);
  assert.deepEqual(await openVPN.locator('a').evaluateAll(links =>
    links.map(link => link.download.split('-').at(-1))), ['udp.ovpn', 'tcp.ovpn']);
  assert.deepEqual(await downloads.locator('bdi').allTextContents().then(labels =>
    labels.map(label => label.split(' · ')[0])), ['WireGuard', 'WireGuard', 'AmneziaWG']);
  assert.deepEqual(await gre.locator('bdi').allTextContents().then(labels =>
    labels.map(label => label.split(' · ')[0])), ['GRE MikroTik', 'GRE Linux']);
  for (const [index, filename] of ['mikrotik.txt', 'linux.txt'].entries()) {
    const response = await context.request.get(await gre.locator('a').nth(index).getAttribute('href'));
    assert.equal(response.status(), 200);
    assert.ok(response.headers()['content-disposition'].includes(filename));
  }

  for (const width of [390, 1280]) {
    await page.setViewportSize({width, height: 844});
    const [code, simple, ovpn, files, greFiles, apps] = await Promise.all([
      page.locator('.subscription-link-box').last(), manual, openVPN, downloads, gre, page.locator('.sub-apps'),
    ].map(item => item.boundingBox()));
    const bottom = rect => rect.y + rect.height;
    assert.ok(bottom(code) <= simple.y && bottom(simple) <= ovpn.y &&
      bottom(ovpn) <= files.y && bottom(files) <= greFiles.y && bottom(greFiles) <= apps.y,
    'subscription sections are shown in order');
    assert.ok(Math.abs(ovpn.width - files.width) < 2, 'OpenVPN spans the section width');
    for (const grid of [openVPN.locator('.sub-config-grid'), downloads, gre]) {
      const [first, second] = await Promise.all([grid.locator('a').first().boundingBox(),
        grid.locator('a').nth(1).boundingBox()]);
      if (width === 390) {
        assert.ok(bottom(first) <= second.y && Math.abs(first.x - second.x) < 2, 'file buttons stack on mobile');
      } else {
        assert.ok(Math.abs(first.y - second.y) < 2 &&
          Math.abs(first.x - second.x) >= first.width, 'file buttons share a desktop row');
      }
    }
  }
}

async function checkAppMenus(page, context, managedApps) {
  for (const [platform, name, href, managedHref] of [
    ['Android', 'v2rayNG', 'https://github.com/2dust/v2rayNG/releases', 'https://downloads.example.test/app.apk'],
    ['iOS', 'Shadowrocket', 'https://apps.apple.com/app/shadowrocket/id932747118', 'https://downloads.example.test/ios'],
  ]) {
    const menu = page.locator('.sub-app-menu').filter({has: page.locator('summary', {hasText: platform})});
    const summary = menu.locator('summary');
    const link = menu.getByText(name, {exact: true});
    assert.equal(await link.isVisible(), false, 'app list starts closed');
    assert.ok(await summary.evaluate(element => {
      const reference = document.createElement('span');
      reference.style.backgroundColor = 'var(--accent-strong)';
      element.append(reference);
      const green = getComputedStyle(reference).backgroundColor;
      reference.remove();
      return getComputedStyle(element).backgroundColor === green &&
        getComputedStyle(element, '::after').content !== 'none';
    }), 'the platform button uses the green accent and a disclosure arrow');
    await summary.press('Enter');
    await link.waitFor({state: 'visible'});
    assert.equal(await link.getAttribute('href'), href);
    if (managedApps) {
      await context.route(managedHref, route => route.fulfill({body: 'test download page'}));
      const [popup] = await Promise.all([page.waitForEvent('popup'),
        menu.getByRole('link', {name: 'Managed ' + platform}).click()]);
      await popup.waitForURL(managedHref);
      await popup.close();
    }
    await summary.click();
    await link.waitFor({state: 'hidden'});
  }
}

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
      assert.equal((text.match(/wireguard:\/\//g) || []).length, expected.checkNodeLabels ? 2 : 0,
        'the two WireGuard relays have links and AmneziaWG stays download-only');
      const awg = page.locator('a.sub-config-btn').filter({hasText: 'AmneziaWG'});
      if (expected.checkNodeLabels) {
        assert.match(await awg.innerText(), /browser-user\|Relay\|Browser server/);
        for (const proto of ['hysteria2', 'tuic', 'anytls', 'naive']) {
          const label = page.locator('.qr-box .ant-tag').filter({hasText: 'Node ' + proto});
          const text = await label.innerText();
          assert.ok(text.startsWith(`browser-user|Node ${proto}|`), text);
          assert.match(text, /7\.00GB📊.*(?:\d+D|\d+H)⏳/);
        }
        await checkConfigLayout(page, context);
      }
      const title = page.locator('.ant-divider bdi');
      if (expected.checkNodeLabels && expected.lang === 'fa-IR') {
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
      } else if (expected.checkNodeLabels && expected.lang === 'en-US') {
        assert.equal(await title.evaluate(element => getComputedStyle(element).direction), 'ltr');
      }
      assert.equal(await awg.count(), 1, 'AWG config file is offered');
      const response = await context.request.get(await awg.getAttribute('href'));
      assert.equal(response.status(), 200);
      const config = await response.text();
      assert.match(config, /\[Interface\]/);
      assert.match(config, /^Endpoint = awg-relay\.example\.test:55080$/m);
      assert.match(response.headers()['content-disposition'], /attachment/);
    } else {
      for (const scheme of ['vmess://', 'ss://', 'wireguard://']) assert.ok(!text.includes(scheme), 'disabled membership stays out of config exports');
      assert.equal(await page.locator('a.sub-config-btn').count(), 0, 'manually disabled profiles cannot be downloaded');
    }
    await checkAppMenus(page, context, expected.checkNodeLabels);
    if (expected.checkNodeLabels) {
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
    }
    assert.equal((await context.cookies()).find(cookie => cookie.name === 'lang').value, 'en-US');
    console.log('Subscription usage, configuration downloads, app menus and localized layout passed.');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exit(1); });
