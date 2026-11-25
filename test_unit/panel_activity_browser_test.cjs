const assert = require('node:assert/strict');
const { chromium } = require('playwright');

(async () => {
    const browser = await chromium.launch({ executablePath: process.env.NODE_UI_CHROME || '/usr/bin/chromium', headless: true, args: ['--no-sandbox'] });
    try {
        const base = process.env.PANEL_ACTIVITY_BASE;
        const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
        const errors = [];
        page.on('pageerror', error => errors.push(error.message));
        let expiryUnread = true;
        let expiryScoped = false;
        let appChanges;
        const at = Date.now();
        const login = { id: 2, kind: 'login', at, username: 'panel-owner', os: 'Windows', ip: '203.0.113.19' };
        const renewal = { id: 3, kind: 'autoRenewed', at, email: 'test-renewed', owner: 'test-representative' };
        await page.route('**/test/panel/api/**', async route => {
            const url = new URL(route.request().url());
            const method = route.request().method();
            const path = url.pathname.split('/api/')[1];
            let obj = [];
            if (path === 'activity/notices') {
                obj = {
                    expired: expiryUnread ? { count: 1, through: 7, names: ['test-expired'] } : {},
                    renewals: {},
                };
            } else if (path === 'activity/logins') {
                obj = url.searchParams.get('before') === '2'
                    ? { rows: [{ ...login, id: 1, kind: 'logout', os: 'Android' }], more: false }
                    : { rows: [login], more: true };
            } else if (path === 'activity/renewals') {
                obj = { rows: [renewal], more: false };
            } else if (path === 'activity/changes/settings') {
                obj = { enabled: true, months: ['2026-11'] };
            } else if (path === 'activity/changes') {
                obj = { rows: [{ ...renewal, kind: 'updated', changes: 'quota' }], owners: ['test-representative'] };
            } else if (path === 'activity/apps' && method === 'POST') {
                assert.match(route.request().headers()['content-type'], /^application\/json/);
                appChanges = route.request().postDataJSON().apps;
            } else if (path === 'activity/notices/dismiss') {
                const payload = route.request().postDataJSON();
                if (payload.kind === 'expired' && payload.through === 7) expiryUnread = false;
            } else if (path === 'clients/list') {
                expiryScoped = url.searchParams.get('scope') === 'expired-own';
                obj = { rows: [{
                    id: 1, email: 'test-expired', groupIds: [], enable: false, expiryTime: at - 86400000,
                    up: 0, down: 0, totalGB: 100, memberships: [{ inboundId: 1, port: 443, protocol: 'vless', remark: 'Test inbound' }],
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
        await page.getByRole('button', { name: 'Activity' }).click();
        await page.getByRole('link', { name: 'Automatic renewals' }).click();
        await page.getByRole('cell', { name: 'test-renewed' }).waitFor();
        await page.getByRole('button', { name: 'Activity' }).click();
        await page.getByRole('link', { name: 'Client change history' }).click();
        await page.getByRole('cell', { name: 'test-representative' }).waitFor();
        await page.getByRole('button', { name: 'Activity' }).click();
        await page.getByRole('link', { name: 'Subscription app downloads' }).click();
        await page.getByRole('button', { name: 'Create' }).click();
        await page.getByRole('textbox', { name: 'App name' }).fill('Test App');
        await page.getByRole('textbox', { name: 'Download URL' }).fill('https://example.com/download');
        const saved = page.waitForResponse(response => response.url().endsWith('/activity/apps') && response.request().method() === 'POST');
        await page.getByRole('button', { name: 'Save' }).click();
        await saved;
        assert.deepEqual(appChanges, [{ platform: 'android', name: 'Test App', url: 'https://example.com/download' }]);
        assert.deepEqual(errors, []);
    } finally {
        await browser.close();
    }
})().catch(error => { console.error(error); process.exit(1); });
