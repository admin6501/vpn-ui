const assert = require('node:assert/strict');
const { chromium } = require('playwright');

(async () => {
    const browser = await chromium.launch({ executablePath: process.env.NODE_UI_CHROME || '/usr/bin/chromium', headless: true, args: ['--no-sandbox'] });
    try {
        const base = process.env.PANEL_ACTIVITY_BASE;
        const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
        page.setDefaultTimeout(10000);
        await page.routeWebSocket('**/ws', () => {});
        const errors = [];
        page.on('pageerror', error => errors.push(error.message));
        page.on('console', entry => { if (entry.type() === 'error') errors.push(entry.text()); });
        let expiryUnread = true;
        let expiryScoped = false;
        let appChanges;
        let retention;
        let paymentReminders = false;
        let pendingPayment;
        const at = Date.now();
        const login = { id: 2, kind: 'login', at, username: 'panel-owner', os: 'Windows', ip: '203.0.113.19' };
        const renewal = { id: 3, kind: 'autoRenewed', at, email: 'test-renewed', owner: 'test-representative' };
        pendingPayment = { id: 15, kind: 'created', at, email: 'payment-client' };
        await page.route('**/test/panel/api/**', async route => {
            const url = new URL(route.request().url());
            const method = route.request().method();
            const path = url.pathname.split('/api/')[1];
            let obj = [];
            if (path === 'activity/notices') {
                obj = {
                    expired: expiryUnread ? { count: 1, through: 7, names: ['test-expired'] } : {},
                    renewals: {},
                    payments: paymentReminders && pendingPayment ? { count: 1, names: [pendingPayment.email] } : {},
                };
            } else if (path === 'activity/payments') {
                obj = { enabled: paymentReminders, rows: paymentReminders && pendingPayment ? [pendingPayment] : [], more: false };
            } else if (path === 'activity/payments/settings') {
                paymentReminders = route.request().postDataJSON().enabled;
            } else if (path === 'activity/payments/confirm') {
                assert.ok(paymentReminders);
                assert.equal(route.request().postDataJSON().id, pendingPayment.id);
                pendingPayment = null;
            } else if (path === 'activity/logins') {
                obj = url.searchParams.get('before') === '2'
                    ? { rows: [{ ...login, id: 1, kind: 'logout', os: 'Android' }], more: false }
                    : { rows: [login], more: true };
            } else if (path === 'activity/renewals') {
                obj = { rows: [renewal], more: false };
            } else if (path === 'activity/changes/settings') {
                obj = { enabled: true, months: ['2026-11'] };
            } else if (path === 'activity/changes/retention') {
                retention = route.request().postDataJSON().days;
            } else if (path === 'clients/history/test-expired') {
                obj = { next: 0, events: [{ id: 1, kind: 'created', at,
                    details: { created: { quota: 5 * 2 ** 30, expiry: at + 864000000, period: 864000000 } }
                }] };
            } else if (path === 'activity/changes') {
                obj = { rows: [{
                    ...renewal, kind: 'renewed', changes: 'quota,expiry',
                    details: { quota: { before: 2 ** 30, after: 2 * 2 ** 30 },
                        expiry: { before: at, after: at + 864000000 }, renewalPeriod: { before: 86400000, after: 864000000 },
                        usageReset: true },
                }], owners: ['test-representative'] };
            } else if (path === 'activity/apps' && method === 'POST') {
                assert.match(route.request().headers()['content-type'], /^application\/json/);
                appChanges = route.request().postDataJSON().apps;
            } else if (path === 'activity/notices/dismiss') {
                const payload = route.request().postDataJSON();
                if (payload.kind === 'expired' && payload.through === 7) expiryUnread = false;
            } else if (path === 'inbounds/onlines') {
                obj = ['test-expired'];
            } else if (path === 'clients/list') {
                expiryScoped = url.searchParams.get('scope') === 'expired-own';
                obj = { rows: [{
                    id: 1, email: 'test-expired', groupIds: [], enable: false, expiryTime: at - 86400000,
                    up: 0, down: 0, totalGB: 100, liveRate: { at, down: 137027403, up: 923 }, memberships: [{ inboundId: 1, port: 443, protocol: 'vless', remark: 'Test inbound' }],
                }], total: 1 };
            }
            await route.fulfill({ json: { success: true, obj } });
        });
        await page.goto(base + 'panel/activity/logins');
        const badge = page.getByRole('link', { name: 'Expired clients 1' });
        assert.equal(await badge.getAttribute('title'), 'test-expired');
        await page.getByRole('cell', { name: '203.0.113.19' }).waitFor();
        await page.getByRole('cell', { name: 'Windows' }).waitFor();
        await page.getByRole('button', { name: 'Next' }).click();
        await page.getByRole('cell', { name: 'Android' }).waitFor();
        await badge.click();
        await page.waitForURL('**/panel/activity/expired');
        await page.getByRole('heading', { name: 'Expired clients' }).waitFor();
        await badge.waitFor({ state: 'hidden' });
        assert.ok(expiryScoped, 'the expired page must load only the logged-in owner’s expired clients');
        assert.equal(expiryUnread, false, 'viewing the expiry list marks the badge as read');
        await page.locator('.bo-info-button').last().waitFor();
        await page.locator('.bo-lg-rates').waitFor();
        for (const width of [390, 1440]) {
            await page.setViewportSize({ width, height: 844 });
            for (const zoom of [0.9, 1, 1.25, 1.5]) {
                await page.evaluate(zoom => { document.documentElement.style.zoom = zoom; }, zoom);
                const presence = await page.locator('.bo-lg-presence').evaluate(el => {
                    const status = el.querySelector('.bo-lg-seen').getBoundingClientRect();
                    const rate = el.querySelector('.bo-lg-rates').getBoundingClientRect();
                    const cell = el.closest('td').getBoundingClientRect();
                    const protocol = el.closest('tr').querySelector('.bo-lg-serving').closest('td').getBoundingClientRect();
                    const labels = [status, ...Array.from(el.querySelectorAll('bdi'), item => item.getBoundingClientRect())];
                    return { stacked: status.bottom <= rate.top + 1, contained: rate.bottom <= cell.bottom + 1 &&
                        labels.every(item => item.left >= cell.left - 1 && item.right <= cell.right + 1),
                        separated: labels.every(item => item.right <= protocol.left + 1 || item.left >= protocol.right - 1) };
                });
                assert.ok(presence.stacked && presence.contained && presence.separated,
                    `presence has its own cell at width ${width}, zoom ${zoom}: ${JSON.stringify(presence)}`);
            }
        }
        await page.setViewportSize({ width: 390, height: 844 });
        for (const zoom of [1, 1.25]) {
            const positions = await page.evaluate(zoom => {
                document.documentElement.style.zoom = zoom;
                return [...document.querySelectorAll('.bo-info-button')].map(button => {
                    const outer = button.getBoundingClientRect();
                    const icon = button.querySelector('svg').getBoundingClientRect();
                    return { dx: icon.x + icon.width / 2 - outer.x - outer.width / 2,
                        dy: icon.y + icon.height / 2 - outer.y - outer.height / 2,
                        iconOnly: button.classList.contains('ant-btn-icon-only') };
                });
            }, zoom);
            assert.equal(positions.length, 2, 'both Account and Protocol Info buttons are present');
            for (const { dx, dy, iconOnly } of positions) {
                assert.ok(Math.abs(dy) <= 1.5, `info icon vertical alignment at zoom ${zoom}: ${dy}`);
                if (iconOnly) assert.ok(Math.abs(dx) <= 1.5, `info icon horizontal alignment at zoom ${zoom}: ${dx}`);
            }
        }
        await page.evaluate(() => { document.documentElement.style.zoom = 1; });
        await page.setViewportSize({ width: 1440, height: 900 });
        await page.getByRole('button', { name: 'Account Info', exact: true }).click();
        await page.getByRole('button', { name: /Logs/ }).click();
        const history = page.locator('.account-client-history');
        await history.locator('summary').waitFor();
        assert.match(await history.locator('summary').innerText(), /5(?:[.,]00)?\s*GB.*10 days/);
        await history.locator('summary').click();
        assert.match(await history.locator('.panel-activity-details').innerText(), /Initial validity.*10 days/);
        await page.locator('.ant-modal').filter({ has: history }).locator('.ant-modal-close').click();
        await history.waitFor({ state: 'hidden' });
        await page.locator('.ant-modal:visible').filter({ has: page.getByRole('button', { name: /Logs/ }) })
            .locator('.ant-modal-close').click();
        await page.getByRole('button', { name: 'Activity' }).click();
        const renewalLink = page.getByRole('link', { name: 'Automatic renewals' });
        await renewalLink.waitFor();
        await page.mouse.move(0, 0);
        const menuColor = await renewalLink.evaluate(link => {
            const reference = document.createElement('span');
            reference.style.color = 'var(--text-2)';
            document.body.append(reference);
            const result = { actual: getComputedStyle(link).color, gray: getComputedStyle(reference).color };
            reference.remove();
            return result;
        });
        assert.equal(menuColor.actual, menuColor.gray, 'Activity uses the same gray as other panel labels');
        await renewalLink.click();
        await page.getByRole('cell', { name: 'test-renewed' }).waitFor();
        await page.getByRole('button', { name: 'Activity' }).click();
        await page.getByRole('link', { name: 'Client change history' }).click();
        await page.getByRole('cell', { name: 'test-representative' }).waitFor();
        const detailBox = page.locator('.panel-activity-detail-box');
        assert.equal(await detailBox.getAttribute('open'), null, 'details start collapsed');
        await detailBox.locator('summary').click();
        const details = await page.locator('.panel-activity-details').innerText();
        assert.match(details, /1(?:[.,]00)?\s*GB.*2(?:[.,]00)?\s*GB/);
        assert.match(details, /Renewal period.*10 days/);
        assert.match(details, /Current usage reset to zero/);
        await page.getByRole('switch').last().click();
        await page.getByRole('spinbutton', { name: 'Keep logs for this many days' }).fill('60');
        const retentionSaved = page.waitForResponse(response => response.url().endsWith('/activity/changes/retention'));
        await page.getByRole('button', { name: 'Save' }).click();
        await retentionSaved;
        assert.equal(retention, 60);
        await page.getByRole('button', { name: 'Activity' }).click();
        await page.getByRole('link', { name: 'Subscription app downloads' }).click();
        await page.getByRole('button', { name: 'Create' }).click();
        await page.getByRole('textbox', { name: 'App name' }).fill('Test App');
        await page.getByRole('textbox', { name: 'Download URL' }).fill('https://example.com/download');
        const saved = page.waitForResponse(response => response.url().endsWith('/activity/apps') && response.request().method() === 'POST');
        await page.getByRole('button', { name: 'Save' }).click();
        await saved;
        assert.deepEqual(appChanges, [{ platform: 'android', name: 'Test App', url: 'https://example.com/download' }]);
        await page.getByRole('button', { name: 'Activity' }).click();
        await page.getByRole('link', { name: 'Pending payments', exact: true }).click();
        const payments = page.getByRole('link', { name: 'Pending payments 1' });
        const preference = page.getByRole('switch', { name: 'Remind me about client payments' });
        const customer = page.getByRole('cell', { name: 'payment-client' });
        await preference.waitFor();
        await payments.waitFor({ state: 'hidden' });
        await preference.click();
        await customer.waitFor();
        await payments.waitFor();
        assert.equal(await payments.getAttribute('title'), 'payment-client');
        await page.getByRole('link', { name: 'Close', exact: true }).click();
        await page.waitForURL('**/panel/clients');
        assert.equal(pendingPayment.id, 15, 'leaving the page never approves a payment');
        await page.goto(base + 'panel/activity/logins');
        await payments.click();
        await customer.waitFor();
        const confirm = page.getByRole('button', { name: 'Confirm payment', exact: true });
        await confirm.click();
        const prompt = page.locator('.ant-popover:visible').filter({ hasText: 'Has the client paid for the latest change?' });
        await prompt.getByRole('button', { name: 'Cancel' }).click();
        await prompt.waitFor({ state: 'hidden' });
        assert.equal(pendingPayment.id, 15);
        await confirm.click();
        await prompt.getByRole('button', { name: 'Confirm payment', exact: true }).click();
        await page.getByText('No payments awaiting confirmation').waitFor();
        await payments.waitFor({ state: 'hidden' });
        assert.equal(pendingPayment, null);
        const preferenceSaved = page.waitForResponse(response => response.url().endsWith('/activity/payments/settings'));
        await preference.click();
        await preferenceSaved;
        await page.getByRole('switch', { name: 'Remind me about client payments', checked: false }).waitFor();
        assert.equal(paymentReminders, false);
        assert.deepEqual(errors, []);
    } finally {
        await browser.close();
    }
})().catch(error => { console.error(error); process.exit(1); });
