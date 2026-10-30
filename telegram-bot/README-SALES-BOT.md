# Telegram sales bot: installation and administration

This guide covers the **standalone VPN-UI sales bot**, not the panel's built-in management bot. The sales bot's current interface is Persian. [راهنمای فارسی](README-SALES-BOT-FA.md) · [API integration](../docs/REPRESENTATIVE-BOT-API.fa.md) · [Panel migration](../docs/BOT-LOCATION-MIGRATION.fa.md)

## 1. Requirements

Use Ubuntu or Debian with systemd, root or sudo, x86-64 or ARM64, and network access to Telegram and the panel. The installer downloads prerequisites and a dedicated Node.js runtime. Obtain a BotFather token and the **numeric Telegram ID** of the main bot administrator, not an `@username`. That administrator must open the bot in a private chat and send `/start`.

Have a VPN-UI administrator or representative account with access to the necessary groups, the full panel URL including port and base path, and each panel's subscription base URL. Importing traffic during migration requires a main administrator account on the destination panel. The current sales bot cannot automatically log in to an account with two-factor authentication enabled. HTTP and HTTPS are supported; HTTPS protects credentials in transit.

Fresh installation creates an empty local database; it does not import existing Cloudflare services, orders or wallets. If a previous database or configuration exists, fresh installation stops rather than replacing it. Protect the bot token, panel credentials, subscription links and backups.

## 2. Install and set up your first sale

Run on the server:

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/scripts/install-telegram-bot.sh -o install-telegram-bot.sh && sudo bash install-telegram-bot.sh
```

1. Choose **1, fresh install**, and enter the BotFather token and numeric administrator ID. The installer creates and starts `vpn-ui-telegram-bot`.
2. Send `/start` to the bot in Telegram and open the administration panel, or send `/admin`.
3. Under **bot settings**, choose the panel connection option and enter the complete panel URL, username, password and subscription base URL. Use the separate add-panel option for additional panels.
4. Check connectivity and groups using the panel connection check. Under **category management**, create a category bound to one group on one panel. A group may contain multiple inbounds.
5. Under **plan management**, create a plan with category, traffic allowance, duration and price. Configure card and support details. Verify purchase and renewal with a test account and payment before selling.

Only one bot instance should receive updates for a token. If another instance uses a webhook, server menu option 7 removes the webhook after `YES` confirmation and disconnects that instance; pending updates are retained.

## 3. Server management menu

After installation open the menu from any directory:

```bash
sudo vpn-ui-bot
```

| Option | Purpose |
| --- | --- |
| 1 | Fresh install; stops if previous configuration or database exists. |
| 2 | Service status. |
| 3, 4, 5 | Start, stop and restart the bot. |
| 6 | Show the last 100 log lines. |
| 7 | Move from webhook to server polling, with confirmation. |
| 8 | Create a local bot database backup and show its path. |
| 9 | Download the current main-branch bot, back up and update it. Existing data is preserved; an unrecognized database schema change stops the update. |
| 10 | Uninstall the program. Permanently deleting configuration, database and backups requires a separate `DELETE` confirmation. |
| 11 | Counts in the **bot** database, not the total number of panel users. |
| 12 | Restore a bot database backup from its full path after `RESTORE` confirmation. |
| 13 | Enable or change the automatic Telegram backup interval, disable it, inspect its status or send a test backup; intervals use minutes or hours. |
| 0 | Exit the menu. |

Local snapshots are stored in `/var/lib/vpn-ui-bot/backups`. These are bot database backups, not panel backups. Restore validates the file and backs up the current bot database before replacing it; reconcile pending orders and services with the panel afterwards. Current configuration and token remain; a changed token may require panel credentials to be configured again. Automatic backups go only to the main administrator IDs recorded at installation; each must first send `/start`.

## 4. Customer menu

- **My services:** View each service's live status and subscription link, list individual configurations and request their text or QR, open the subscription page and renew. Configuration availability depends on the subscription output and panel. Customers may confirm deletion of a service, which also deletes it from the panel. Only bot administrators can toggle individual configurations.
- **Buy:** Choose a category, plan and custom or random service name, then pay by wallet or card transfer. A duplicate custom name is rejected and another is required.
- **Wallet:** View balance and history and request a top-up within configured limits. A card-receipt top-up is credited after administrator approval.
- **Trial:** When enabled, only the category selected by the administrator is offered; the customer chooses a custom or random name.
- **My referral code / enter referral code:** Obtain or register an invitation code according to the administrator's settings. The inviter's reward is a discount on the next purchase, not a wallet deposit.
- **Download apps / support:** View configured applications and download links or contact support.
- **Change location:** When enabled by the administrator, the customer can choose a destination for their service. The new subscription link appears in that service after migration.

## 5. Telegram administrator menu

Open `/admin` or the administrator button:

| Option | Purpose |
| --- | --- |
| Bot settings | Panel connections, trial, card, support, channel, start text, menu image, wallet limits, alerts, automatic deletion and location changes; see the next section. |
| Plan management | Add, change price, traffic, duration and category, or delete a plan. Customers choose category before plan. |
| Category management | Create, rebind or delete categories and select a group on a panel. Identically named groups on different panels are distinct. |
| Administrators | List administrators; only the main administrator can add or remove additional administrators. |
| Sales statistics | View statistics recorded by the bot. |
| User list | Ten users per page; view services, all orders and wallet history, adjust balance, send a private message and block/unblock bot access. Blocking the bot does not disable VPN service. |
| Service search | Search bot-registered service or panel account names, view status, manage configurations and confirm deletion from both bot and panel. The bot ID is the service's internal database number. |
| Broadcast / message status | Preview and confirm text, photo, voice/audio or forwarded broadcasts; inspect sent, failed and uncertain results. Blocked users are excluded and uncertain delivery is not automatically repeated. Private messages from the user page are text. |
| Wallet management / orders needing review | Review and adjust balances with a ledger; the user receives an adjustment notification. Refunded payments with uncertain outcomes remain available for review. |
| Referral reward | Set the inviter's next-purchase discount in toman; zero disables it. |
| Discount codes | Create percentage or fixed codes with total, per-user and day limits; zero for a limit means unlimited. Disable or delete codes as needed. |
| App management | Add, edit, enable, disable or remove an application and its download links. |
| Allow trial again / reset all trials | Re-enable a single user's trial or, after confirmation, reset completed trial eligibility for everyone. Existing services are not deleted and uncertain requests remain protected. |
| Check panel connection | Test connectivity and show accessible groups/inbounds. |
| Customer menu | Return to the customer view. |

Commands are also available: `/settings`, `/set KEY VALUE`, `/plans`, `/setprice KEY PRICE`, `/setplan KEY PRICE GB`, `/delplan KEY`, `/stats`, `/admins`, `/addadmin ID`, `/deladmin ID`, `/resettest ID` and `/panelcheck`. Use the button menu to create a plan and set its category and duration.

## 6. Settings and payments

- **Connected panels:** The main bot administrator can view status/groups and edit address, username, password and subscription base. A panel with a registered service or pending order cannot be removed. Changing a category's group or panel URL alone does not transfer existing services.
- **Trial:** Enable/disable it and set its traffic in MB, duration in days and one trial category.
- **Card and support:** Set the card number, cardholder and support ID. After choosing card transfer, the customer sees amount and card details and sends a photo receipt for administrator review. Wallet is a separate payment method.
- **Minimum and maximum wallet top-up:** Set in toman; a maximum of zero means no configured maximum.
- **Required channel membership:** First add the bot to the channel as an administrator, then set the channel username or ID and invite link.
- **Start text and menu image:** `{name}` inserts the user's name. Send `-` to restore the default start text or clear the image URL.
- **Subscription alerts:** Enable/disable and configure days and GB remaining. Zero disables the respective threshold; no alert is sent for an unlimited dimension. The server checks approximately every five minutes, up to 500 panel accounts per pass.
- **Automatic expired-service deletion:** Disabled by default. Set days after expiry; zero means no additional grace period. Only registered services with a real expiry are considered; exhausted traffic alone is insufficient. A pending review or uncertain identity blocks deletion.
- **Customer location change:** Enable or disable customer-initiated migration.

Referral discount is applied first, then a coupon applies to the remaining price. Coupon capacity is reserved during checkout and released on cancellation or incomplete payment according to its deadline; a receipt awaiting review can hold it until resolved. Rejecting a receipt does not automatically refund a bank transfer.

## 7. Renewal and migration

Bot renewal offers plans in the service's category and **edits the existing client**. Existing usage remains; the purchased traffic and time are added to the remaining allowance. An expired deadline starts from renewal time; unlimited quota or time stays unlimited. The panel's own renewal button has different reset semantics. Representative renewals and identities containing `/` require a panel with the guarded edit capability; representative credit and duration policies still apply. Coordinate simultaneous edits made directly in the panel and bot.

For migration connect both panels and create a destination category bound to its group. From **connected panels → bulk service migration**, the administrator can choose one, several or all services of the source panel. When enabled, a customer can migrate their own service. Only services registered in the bot are moved. The destination requires the traffic-import API and a main administrator account; after destination verification, the source service is deleted. Source usage not yet recorded can be missed. Rebinding a category alone does not move services.

When a create, renewal, delete or migration response is uncertain, reconcile the order and service in the bot and panel before retrying; blind retries can apply an operation twice. When a wallet payment has an uncertain panel result, the amount returns to the wallet and the order is held for review. Check both panels before releasing a migration review lock.

## 8. Troubleshooting and files

```bash
sudo systemctl status vpn-ui-telegram-bot --no-pager -l
sudo journalctl -u vpn-ui-telegram-bot -n 100 --no-pager
```

If the bot does not respond, check service status, Telegram connectivity and any webhook used by another instance. If panel connection fails, verify the full URL/base path, credentials, group permissions and the 2FA limitation. “Service not found” can mean the saved identity was not found in the connected panel; also check connectivity. For uncertain payments or operations, reconcile the order, wallet and both sides before retrying.

Application files are under `/opt/vpn-ui-telegram-bot`, configuration under `/etc/vpn-ui-bot`, and the database and backups under `/var/lib/vpn-ui-bot`. Changing the bot token can require encrypted panel connections to be configured again.
