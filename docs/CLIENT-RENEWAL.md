# Manual client renewal — v1.0.0

Use **Users → the client's three-dot menu → Renew**. Renewal clears current
upload/download usage on the account and all its inbound memberships, starts the
original package duration from now, and enables the account. It preserves the
traffic quota, subscription ID, all credentials, devices, peer addresses, group
assignments, speed/IP/device limits, and individual inbound access switches.
An account without a time limit stays unlimited in time; one without a traffic
limit keeps unlimited traffic. Clients without an assigned inbound can also be
renewed by the main administrator.

New packages created in the Users form record their full duration separately
from their expiry timestamp. Editing the duration updates that record; ordinary
edits, first use, freezing and renewal retain it. Older packages whose original
duration is unknown ask for their days on the first renewal and remember that
answer. The automatic-renewal setting is independent and is not enabled by this
action. Renewal starts a first-use package immediately.

Lifetime accounting and cumulative core/node cursors are retained so renewing
does not erase a representative's consumption, manufacture a refund, or bill old
node traffic twice. The visible current-period upload and download start at zero.
Representative blocking and deliberately disabled inbound memberships still
apply. Renewal does not grant access to additional inbounds.

Representatives need both user-update and usage-reset grants; the narrower scope
applies. Consumption-based representatives retain their measured usage and role
limits. Native prepaid traffic renewal uses existing reset pricing (the cleared
bytes); a days-per-GB package purchases the full package to restore its duration
and cannot exceed the representative's days-per-GB allowance. Administrators do
not incur a representative balance charge. All database changes, including
billing and membership usage, commit or roll back together.

API: `POST /panel/api/inbounds/renewClient/:email` with the usual authenticated
panel session. The optional `renewalDuration` field is an integer number of
milliseconds. Supply it for a legacy timed account with an unknown period. An
existing recorded period must match any supplied value; no-expiry accounts accept
only zero. The maximum supported period is 36,500 days. Renewing preserves the
account's subscription URL and connection configuration.


For the separate sales bot, renewal is additive and edits the existing client without resetting its consumption. The customer can select another plan in the service category. See [sales bot](TELEGRAM-SALES-BOT.md), [migration](BOT-LOCATION-MIGRATION.fa.md) and [integration API](REPRESENTATIVE-BOT-API.fa.md). The panel `renewClient` operation described above has different reset semantics.
