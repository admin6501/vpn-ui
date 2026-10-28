import { qrPng } from './qr.js';
import { parseMoney, walletBalance, reserveWalletPayment, completeWalletPayment, adjustWallet, recoverWalletPayments } from './wallet.js';
import { encryptPanel, decryptPanel, panelURL, panelAccountKey, probePanel } from './panel-config.js';

// ربات فروش کانفیگ | NovaRay VPN — Cloudflare Workers + D1 + اتصال به پنل vpn-ui
// ساخت/تمدید/حذف سرویس مستقیم از طریق API پنل (حساب نماینده) انجام می‌شود.

const NAME_RE = /^[A-Za-z]+$/;
const GB = 1024 ** 3;
const MB = 1024 ** 2;
const DAY_MS = 86400000;

const APPS = {
  v2box: {
    title: "V2Box",
    text: "📱 <b>V2Box</b>\n\nکلاینت چندسکویی (iOS / Android / macOS) با پشتیبانی از VLESS، VMess، Trojan و Reality.\nبعد از نصب، لینک ساب یا کانفیگ خودتان را وارد یا اسکن کنید.",
    links: [
      ["🍏 دانلود برای iOS", "https://apps.apple.com/app/id6446814690"],
      ["🤖 دانلود برای اندروید", "https://play.google.com/store/search?q=V2Box&c=apps"],
    ],
  },
  hiddify: {
    title: "Hiddify",
    text: "📱 <b>Hiddify</b>\n\nکلاینت رایگان و متن‌باز، مناسب اندروید، iOS، ویندوز، مک و لینوکس.\nبعد از نصب، لینک ساب خود را وارد کنید تا سرویس‌ها خودکار اضافه شوند.",
    links: [["📥 دانلود (همه پلتفرم‌ها)", "https://github.com/hiddify/hiddify-app/releases/latest"]],
  },
  v2ray: {
    title: "v2rayNG",
    text: "📱 <b>v2rayNG (اندروید)</b>\n\nیکی از محبوب‌ترین کلاینت‌های v2ray/Xray برای اندروید.\nبعد از نصب، از علامت + کانفیگ یا لینک ساب را اضافه کنید.",
    links: [["📥 دانلود از گیت‌هاب", "https://github.com/2dust/v2rayNG/releases/latest"]],
  },
};

const MAIN_TEXT = "🏠 <b>منوی اصلی</b>\n\nیکی از گزینه‌های زیر را انتخاب کنید:";
const DEFAULT_START_TEXT = "سلام {name} 👋";
const MAX_START_TEXT = 700;

// ---------------------------------------------------------- تنظیمات قابل تغییر با /set
// مقدار پیش‌فرض از اینجا (و wrangler.toml) می‌آید؛ هر مقداری که با /set عوض شود در دیتابیس می‌ماند.
const META = {
  referral_discount: ["int", "مبلغ تخفیف کد دعوت (تومان)"],
  duration_days: ["int", "مدت همه بسته‌ها (روز)"],
  test_enabled: ["bool", "سرویس تست: 1 = فعال، 0 = غیرفعال"],
  test_mb: ["int", "حجم سرویس تست (مگابایت)"],
  test_days: ["int", "مدت سرویس تست (روز)"],
  card_number: ["text", "شماره کارت"],
  card_holder: ["text", "نام صاحب کارت"],
  support_id: ["text", "آیدی پشتیبانی (با @)"],
  channel_username: ["text", "یوزرنیم کانال عضویت اجباری (با @)"],
  channel_link: ["text", "لینک کانال"],
  main_menu_image_url: ["text", "لینک عکس منوی اصلی (برای خالی کردن: -)"],
  start_text: ["text", "متن پیام شروع"],
  panel_group_ids: ["ids", "آیدی گروه‌های پنل، با کاما (مثلا 1 یا 1,2)"],
  panel_inbound_id: ["int", "آیدی اینباند پنل"],
  sub_base_url: ["text", "آدرس پایه لینک ساب، مثلا https://example.com:8000/sub/"],
};

const defaults = (env) => ({
  referral_discount: "50000",
  duration_days: "30",
  test_enabled: "1",
  test_mb: "500",
  test_days: "1",
  card_number: env.CARD_NUMBER || "",
  card_holder: env.CARD_HOLDER || "",
  support_id: env.SUPPORT_ID || "",
  channel_username: env.CHANNEL_USERNAME || "",
  channel_link: env.CHANNEL_LINK || "",
  main_menu_image_url: env.MAIN_MENU_IMAGE_URL || "",
  start_text: "",
  panel_group_ids: "1",
  panel_inbound_id: "",
  sub_base_url: "",
});

// ---------------------------------------------------------- ابزارها
const esc = (s) => String(s ?? "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
const toman = (n) => `${Number(n).toLocaleString("en-US")} تومان`;
const now = () => new Date().toLocaleString("sv-SE", { timeZone: "Asia/Tehran" }).slice(0, 16);
const btn = (text, cb) => ({ text, callback_data: cb });
const urlBtn = (text, url) => ({ text, url });
const kb = (rows) => ({ inline_keyboard: rows });
const backKb = (t = "main_menu") => kb([[btn("🔙 بازگشت", t)]]);
const num = (env, k) => Number(env.cfg[k]);
const durationText = (days) => (Number(days) === 30 ? "۱ ماهه (۳۰ روزه)" : `${days} روزه`);
const planLabel = (p) => `${p.gb} گیگ`;
const durationDays = (s) => {
  const text = String(s || "").replace(/[۰-۹]/g, x => String("۰۱۲۳۴۵۶۷۸۹".indexOf(x))).replace(/[٠-٩]/g, x => String("٠١٢٣٤٥٦٧٨٩".indexOf(x)));
  return Number(text.match(/(\d+)\s*روزه/)?.[1] || 0);
};
const parseIds = (v) => String(v ?? "").split(/[,\s]+/).map(Number).filter((n) => Number.isFinite(n) && n > 0);
const b64url = (n) => {
  const b = crypto.getRandomValues(new Uint8Array(n));
  return btoa(String.fromCharCode(...b)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
};
const hex = (n) => Array.from(crypto.getRandomValues(new Uint8Array(n)), (b) => b.toString(16).padStart(2, "0")).join("");

async function tg(env, method, params = {}) {
  const r = await fetch(`https://api.telegram.org/bot${env.BOT_TOKEN}/${method}`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(params),
  });
  const j = await r.json();
  if (!j.ok) console.log("TG error", method, JSON.stringify(j));
  return j;
}

async function sendMsg(env, chat_id, text, reply_markup) {
  return tg(env, "sendMessage", {
    chat_id, text, reply_markup, parse_mode: "HTML", disable_web_page_preview: true,
  });
}

// ---------------------------------------------------------- دیتابیس
const run = (env, sql, ...a) => env.DB.prepare(sql).bind(...a).run();
const first = (env, sql, ...a) => env.DB.prepare(sql).bind(...a).first();
const all = async (env, sql, ...a) => (await env.DB.prepare(sql).bind(...a).all()).results;

async function saveState(env, uid, obj) {
  await run(env,
    "INSERT INTO states(user_id,data) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET data=excluded.data",
    uid, JSON.stringify(obj));
}

const getPlans = (env) => all(env, "SELECT * FROM plans ORDER BY sort, price");
const getPlan = (env, key) => first(env, "SELECT * FROM plans WHERE key=?", key);
const getOrder = (env, id) => first(env, "SELECT * FROM orders WHERE id=?", id);
const setOrderStatus = (env, id, s) => run(env, "UPDATE orders SET status=? WHERE id=?", s, id);
const claimOrder = async (env, id, from, to) =>
  (await run(env, "UPDATE orders SET status=? WHERE id=? AND status=?", to, id, from)).meta.changes > 0;
const getService = (env, id) => first(env, "SELECT * FROM services WHERE id=?", id);
const getUserServices = (env, uid) => all(env, "SELECT * FROM services WHERE user_id=? ORDER BY id DESC", uid);
const latestNeedsFix = (env, uid) =>
  first(env, "SELECT * FROM orders WHERE user_id=? AND status='needs_fix' ORDER BY id DESC LIMIT 1", uid);

async function takeDiscount(env, uid) {
  const row = await first(env, "SELECT * FROM discounts WHERE user_id=? AND used=0 ORDER BY id LIMIT 1", uid);
  if (!row) return 0;
  const r = await run(env, "UPDATE discounts SET used=1 WHERE id=? AND used=0", row.id);
  return r.meta.changes ? row.amount : 0;
}

async function registerReferral(env, referrer, referred) {
  if (await first(env, "SELECT id FROM referrals WHERE referred_id=?", referred)) return false;
  const amount = num(env, "referral_discount");
  await env.DB.batch([
    env.DB.prepare("INSERT INTO referrals(referrer_id,referred_id,created_at) VALUES(?,?,?)").bind(referrer, referred, now()),
    env.DB.prepare("INSERT INTO discounts(user_id,amount,created_at) VALUES(?,?,?)").bind(referrer, amount, now()),
  ]);
  return true;
}

// ---------------------------------------------------------- اتصال به پنل vpn-ui
let panelCookie = null; // نشست فقط برای همان آدرس و حساب استفاده می‌شود.
let panelCookieKey = "";

const panelBase = (env) => String(env.PANEL_URL || "").replace(/\/+$/, "");

async function panelIdentity(env) {
  const bytes = new TextEncoder().encode(JSON.stringify([panelBase(env),env.PANEL_USER || "",env.PANEL_PASS || ""]));
  const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", bytes));
  return Array.from(digest, x=>x.toString(16).padStart(2,"0")).join("");
}
async function panelLogin(env, identity) {
  if (!env.PANEL_URL || !env.PANEL_USER || !env.PANEL_PASS) {
    throw new Error("اطلاعات اتصال به پنل (PANEL_URL / PANEL_USER / PANEL_PASS) تنظیم نشده است");
  }
  const r = await fetch(`${panelBase(env)}/login`, {
    method: "POST", redirect: "error",
    headers: { "content-type": "application/x-www-form-urlencoded", "X-Requested-With": "XMLHttpRequest" },
    body: new URLSearchParams({ username: env.PANEL_USER, password: env.PANEL_PASS }).toString(),
  });
  const j = await r.json().catch(() => null);
  if (!j || !j.success) {
    if (j?.obj?.twoFactorRequired) throw new Error("حساب نماینده در پنل رمز دو مرحله‌ای دارد؛ برای ربات باید غیرفعال باشد");
    throw new Error("ورود به پنل ناموفق بود: " + (j?.msg || `HTTP ${r.status}`));
  }
  const raw = typeof r.headers.getSetCookie === "function" ? r.headers.getSetCookie() : [r.headers.get("set-cookie") || ""];
  const cookie = raw.map((c) => c.split(";")[0]).filter(Boolean).join("; ");
  if (!cookie) throw new Error("کوکی نشست از پنل دریافت نشد");
  panelCookie = cookie; panelCookieKey = identity;
  return cookie;
}

// اگر نشست منقضی شده باشد (404/401 یا HTML) یک بار دوباره لاگین می‌کند
async function panelCall(env, method, path, form, onDispatch) {
  for (let i = 0; i < 2; i++) {
    const identity = await panelIdentity(env);
    const cookie = panelCookie && panelCookieKey === identity ? panelCookie : await panelLogin(env, identity);
    const headers = { cookie, "X-Requested-With": "XMLHttpRequest" };
    let body;
    if (form) { headers["content-type"] = "application/x-www-form-urlencoded"; body = form.toString(); }
    if (onDispatch) onDispatch();
    const r = await fetch(panelBase(env) + path, { method, headers, body, redirect:"error" });
    const text = await r.text();
    if (r.status === 401 || r.status === 404 || text.trimStart().startsWith("<")) { panelCookie = null; if (method !== "GET") throw new Error("نشست نامعتبر یا نتیجه عملیات نامشخص است؛ پیش از تکرار دستی بررسی کنید"); continue; }
    if (r.status === 403) throw new Error("دسترسی حساب نماینده در پنل کافی نیست (نقش/گروه/اعتبار را بررسی کنید)");
    if (r.status === 429) throw new Error("پنل درخواست‌ها را محدود کرده (429)، کمی بعد تلاش کنید");
    if (!r.ok) throw new Error(`خطای پنل HTTP ${r.status}؛ نتیجه عملیات را پیش از تکرار بررسی کنید`);
    try { return JSON.parse(text); } catch { throw new Error(`پاسخ نامعتبر از پنل (HTTP ${r.status})`); }
  }
  throw new Error("نشست پنل برقرار نشد؛ آدرس PANEL_URL و نام کاربری/رمز را بررسی کنید");
}

function subLink(env, subId) {
  return `${env.cfg.sub_base_url.replace(/\/+$/, "")}/${subId}`;
}

function assertPanelReady(env) {
  if (!/^https:\/\//i.test(env.PANEL_URL || "") || !/^https:\/\//i.test(env.cfg.sub_base_url || "")) throw new Error("آدرس پنل و ساب باید HTTPS باشند");
  if (!env.cfg.sub_base_url) throw new Error("تنظیم sub_base_url خالی است (/set sub_base_url ...)");
  if (!/^\d+$/.test(env.cfg.panel_inbound_id)) throw new Error("تنظیم panel_inbound_id خالی است (/set panel_inbound_id ...)");
  if (!parseIds(env.cfg.panel_group_ids).length) throw new Error("تنظیم panel_group_ids خالی است");
}

// Read actual permitted groups/inbounds before constructing credentials.
async function groupCredentials(env) {
  const [groups, inbounds] = await Promise.all([
    panelCall(env, "GET", "/panel/api/clients/groups"),
    panelCall(env, "GET", "/panel/api/clients/assignable"),
  ]);
  if (!groups.success || !inbounds.success || !Array.isArray(groups.obj) || !Array.isArray(inbounds.obj))
    throw new Error("دریافت گروه‌ها یا ورودی‌های مجاز از پنل ناموفق بود");
  const ids = parseIds(env.cfg.panel_group_ids);
  const selected = ids.map(id => groups.obj.find(g => Number(g.id) === id));
  if (selected.some(g => !g || !Array.isArray(g.inboundIds))) throw new Error("گروه انتخاب‌شده در دسترس نیست");
  const inboundIds = [...new Set(selected.flatMap(g => g.inboundIds.map(Number)))];
  const anchor = Number(env.cfg.panel_inbound_id);
  if (!inboundIds.includes(anchor)) throw new Error("ورودی انتخاب‌شده عضو گروه‌های انتخابی نیست");
  const members = inboundIds.map(id => inbounds.obj.find(i => Number(i.inboundId) === id));
  if (members.some(i => !i)) throw new Error("یکی از ورودی‌های گروه در دسترس نماینده نیست");
  const supported = new Set(["vless", "vmess", "shadowsocks", "trojan", "anytls", "tuic", "naive", "hysteria", "hysteria2", "l2tp", "pptp", "openvpn", "openconnect", "sstp", "ikev2", "ssh", "wg-c", "awg", "wireguard", "gre", "mtproto"]);
  for (const i of members) {
    if (!supported.has(String(i.protocol).toLowerCase())) throw new Error(`پروتکل ${i.protocol} در این نسخه ربات پشتیبانی نمی‌شود`);
    if (String(i.method || "").toLowerCase().startsWith("2022-") && !["2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm"].includes(String(i.method).toLowerCase())) throw new Error("روش Shadowsocks 2022 انتخاب‌شده پشتیبانی نمی‌شود");
  }
  const uuid = crypto.randomUUID();
  const anchorInbound = members.find(i => Number(i.inboundId) === anchor);
  const ss2022 = members.filter(i => String(i.protocol).toLowerCase() === "shadowsocks" && String(i.method || "").toLowerCase().startsWith("2022-"));
  const cipher = ss2022.find(i => Number(i.inboundId) === anchor) || ss2022[0];
  const bytes = cipher ? (String(cipher.method).toLowerCase().includes("aes-128") ? 16 : 32) : 24;
  const password = btoa(String.fromCharCode(...crypto.getRandomValues(new Uint8Array(bytes))));
  const anchorProtocol = String(anchorInbound.protocol).toLowerCase();
  const vpnUsername = `u${hex(6)}`;
  const auth = b64url(18);
  const secret = hex(16);
  const naiveUsername = `u${hex(6)}`;
  let id;
  if (["vless", "vmess", "tuic"].includes(anchorProtocol)) id = uuid;
  else if (["l2tp", "pptp", "openvpn", "openconnect", "sstp", "ikev2", "ssh"].includes(anchorProtocol)) id = vpnUsername;
  // WireGuard, GRE and MTProto use email as id, set after email is minted.
  return { id, uuid, password, vpnUsername, auth, secret, naiveUsername,
    ...(anchorProtocol === "naive" ? { username: naiveUsername } : {}),
    ...(anchorProtocol === "vmess" ? { security: "auto" } : {}), anchorProtocol };

}

async function panelCreate(env, { name, bytes, days, attemptKey, onDispatch }) {
  assertPanelReady(env);
  if (!Number.isSafeInteger(bytes) || bytes <= 0 || !Number.isSafeInteger(days) || days <= 0 || !Number.isSafeInteger(Date.now() + days * DAY_MS)) throw new Error("حجم یا مدت بسته معتبر نیست");
  const credentials = await groupCredentials(env);
  const subId = b64url(18);
  const email = `${name}_${hex(8)}`;
  if (["wg-c", "awg", "wireguard", "gre", "mtproto"].includes(credentials.anchorProtocol)) credentials.id = email;
  delete credentials.anchorProtocol;
  const inboundId = Number(env.cfg.panel_inbound_id);
  if (attemptKey) await run(env, "INSERT INTO provision_attempts(attempt_key,email,sub_id,inbound_id,created_at) VALUES(?,?,?,?,?)", attemptKey, email, subId, inboundId, now());
  const settings = JSON.stringify({
    clients: [{
      ...credentials, email, enable: true, subId,
      totalGB: bytes, expiryTime: Date.now() + days * DAY_MS,
      renewalDuration: days * DAY_MS, limitIp: 0, reset: 0,
    }],
  });
  const form = new URLSearchParams();
  form.set("id", String(inboundId));
  for (const g of parseIds(env.cfg.panel_group_ids)) form.append("groupIds", String(g));
  form.set("settings", settings);
  const j = await panelCall(env, "POST", "/panel/api/inbounds/addClient", form, onDispatch);
  if (!j.success) throw new Error("پنل رد کرد: " + (j.msg || JSON.stringify(j)));
  return { email, inboundId, subId, link: subLink(env, subId), bytes };
}

// تمدید: مصرف صفر می‌شود و مدت از الان شروع می‌شود (مدت ثبت‌شده هنگام ساخت)
async function panelRenew(env, svc, newBytes, days, onDispatch) {
  // Renewal restarts the original package; it cannot safely change quota/duration.
  if (!svc.volume_bytes || Number(svc.volume_bytes) !== Number(newBytes) || svc.duration !== durationText(days))
    throw new Error("تمدید خودکار فقط با همان حجم و مدت قبلی ممکن است؛ تغییر بسته را دستی انجام دهید");
  const j = await panelCall(env, "POST", `/panel/api/inbounds/renewClient/${encodeURIComponent(svc.panel_email)}`, undefined, onDispatch);
  if (!j.success) throw new Error("پنل تمدید را رد کرد: " + (j.msg || "خطای نامشخص"));
  return "";
}

// Exact account lookup; list search is substring-based and may span pages.
async function panelAccount(env, email) {
  if (!email) throw new Error("این سرویس قدیمی به پنل متصل نیست");
  for (let page = 1; page <= 100; page++) {
    const response = await panelCall(env, "GET", `/panel/api/clients/list?page=${page}&size=500&search=${encodeURIComponent(email)}`);
    if (!response.success || !Array.isArray(response.obj?.rows)) throw new Error("خواندن سرویس از پنل ناموفق بود");
    const rows = response.obj.rows;
    const found = rows.find(x => String(x.email).trim().toLowerCase() === String(email).trim().toLowerCase());
    if (found) return found;
    if (!rows.length || page * 500 >= Number(response.obj.total ?? rows.length)) return null;
  }
  throw new Error("نتیجه جست‌وجوی پنل بیش از حد بزرگ است");
}

// The account list's membership.enable describes the inbound itself, not the
// customer's switch. Read the projected client entry, like the panel UI does.
// The email and (when known) subscription id identify the actual service on
// the currently configured panel. A similarly named account is not sufficient.
async function panelServiceAccount(env, svc) {
  if (!svc.panel_email) return null;
  const account=await panelAccount(env,svc.panel_email);
  if (!account) return null;
  if (svc.sub_id && String(account.subId || "") !== String(svc.sub_id)) return null;
  return account;
}

function currentServiceLink(env, svc, account) {
  if (svc.sub_id && account?.subId) return env.cfg.sub_base_url ? subLink(env,account.subId) : "";
  return svc.sub_link || "";
}

async function panelMemberships(env, svc) {
  const account = await panelServiceAccount(env, svc);
  if (!account || !Array.isArray(account.memberships)) throw new Error("سرویس در پنل پیدا نشد.");
  const result = await panelCall(env, "GET", "/panel/api/clients/inbounds");
  if (!result.success || !Array.isArray(result.obj)) throw new Error("دریافت وضعیت کانفیگ‌ها ناموفق بود.");
  const list = account.memberships.map(m => {
    const inbound = result.obj.find(i => Number(i.id) === Number(m.inboundId));
    let settings; try { settings = typeof inbound?.settings === "string" ? JSON.parse(inbound.settings) : inbound?.settings; } catch {}
    const clients = Array.isArray(settings?.clients) ? settings.clients : [];
    const matches = clients.filter(c => String(c?.email || "").trim().toLowerCase() === String(svc.panel_email).trim().toLowerCase());
    const enabled = matches.length === 1 && typeof matches[0].enable === "boolean" ? matches[0].enable : null;
    return {id:Number(m.inboundId), name:String(m.remark || `ورودی ${m.inboundId}`), protocol:String(m.protocol || ""), inboundEnabled: m.enable === true, enabled};
  });
  return {accountEnabled:account.enable === true, list};
}

async function panelDelete(env, svc) {
  let account = await panelServiceAccount(env, svc);
  if (!account || !Array.isArray(account.memberships) || !account.memberships.length) throw new Error("عضویت‌های سرویس مشخص نیست؛ حذف دستی لازم است");
  const ids = [...new Set(account.memberships.map(m => Number(m.inboundId)))];
  if (ids.some(id => !Number.isSafeInteger(id) || id <= 0) || ids.length !== account.memberships.length) throw new Error("فهرست ورودی‌های سرویس نامعتبر است");
  if (ids.length > 1) {
    const capability = await panelCall(env, "GET", "/panel/api/inbounds/multiDeleteCapability");
    if (!capability.success || capability.obj?.preservesSharedTraffic !== true) throw new Error("پنل باید برای حذف امن چندورودی به‌روزرسانی شود");
  }
  for (let n = 0; n < ids.length; n++) {
    const id = ids[n];
    const result = await panelCall(env, "POST", `/panel/api/inbounds/${id}/delClientByEmail/${encodeURIComponent(svc.panel_email)}`);
    if (!result.success) throw new Error(`حذف ورودی ${id} رد شد؛ وضعیت سایر ورودی‌ها را بررسی کنید`);
    account = await panelAccount(env, svc.panel_email);
    if (n < ids.length - 1) {
      if (!account || !Array.isArray(account.memberships) || account.memberships.some(m => Number(m.inboundId) === id) || !ids.slice(n + 1).every(next => account.memberships.some(m => Number(m.inboundId) === next)))
        throw new Error("وضعیت حذف ناقص یا نامشخص است؛ بررسی دستی لازم است");
    } else if (account) throw new Error("پنل حذف کامل را تأیید نکرد؛ وضعیت را بررسی کنید");
  }
}

const formatBytes = n => {
  n = Math.max(0, Number(n) || 0);
  if (n < 1024) return `${n} B`;
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(2)} ${units[i]}`;
};
function liveServiceDetails(a) {
  const used = Math.max(0, Number(a.up) || 0) + Math.max(0, Number(a.down) || 0);
  const quota = Number(a.totalGB) || 0, expiry = Number(a.expiryTime) || 0;
  const enabled = a.enable === true;
  const active = enabled && (quota <= 0 || used < quota) && (expiry <= 0 || Date.now() < expiry);
  const expiration = expiry < 0 ? "پس از اولین اتصال" : expiry === 0 ? "بدون انقضا" : new Date(expiry).toLocaleString("fa-IR", {timeZone:"Asia/Tehran"});
  const list = Array.isArray(a.memberships) ? a.memberships : [];
  const members = list.map(m => `• ${esc(m.remark || `ورودی ${m.inboundId}`)} (${esc(m.protocol || "?")}) — ورودی ${m.enable ? "روشن" : "خاموش"}`).join("\n");
  return `📊 وضعیت حساب (سهمیه و تاریخ): ${active ? "فعال" : "غیرفعال یا منقضی"}\n📤 آپلود: ${formatBytes(a.up)}\n📥 دانلود: ${formatBytes(a.down)}\n📦 مصرف: ${formatBytes(used)} / ${quota > 0 ? formatBytes(quota) : "نامحدود"}\n📅 انقضا: ${expiration}\n\n🔌 ورودی‌ها (${list.length}):\n${members || "ورودی ثبت نشده است"}`;
}

async function insertService(env, o) {
  const r = await run(env,
    "INSERT INTO services(user_id,name,volume,duration,sub_link,config_link,created_at,panel_email,inbound_id,sub_id,volume_bytes) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
    o.user_id, o.name, o.volume, o.duration, o.link, null, now(), o.email, o.inboundId, o.subId, o.bytes);
  return r.meta.last_row_id;
}

// ---------------------------------------------------------- رندر
async function render(env, cq, text, keyboard) {
  try { await tg(env, "deleteMessage", { chat_id: cq.message.chat.id, message_id: cq.message.message_id }); } catch {}
  await sendMsg(env, cq.message.chat.id, text, keyboard);
}

const mainMenuKb = (env) => {
  const rows = [
    [btn("🗂 سرویس های من", "my_services")],
    [btn("🛍 خرید سرویس", "buy")],
    [btn("💰 کیف پول", "wallet:home")],
  ];
  if (env.cfg.test_enabled === "1") rows.push([btn(`🎁 سرویس تست (${env.cfg.test_mb} مگ)`, "test_service")]);
  rows.push(
    [btn("🎁 کد دعوت من", "invite_code"), btn("➕ ثبت کد دعوت", "enter_invite")],
    [btn("📲 دانلود اپلیکیشن‌ها", "apps_menu")],
    [btn("🆘 پشتیبانی", "support")],
  );
  if (env.admins?.has(env.actor)) rows.push([btn("🛠 پنل مدیریت", "admin:root")]);
  return kb(rows);
};

function startGreeting(env, name) {
  const template = env.cfg.start_text || DEFAULT_START_TEXT;
  return esc(template).replaceAll('{name}', () => esc(name || ''));
}
async function sendMainMenu(env, chat_id, greeting = "") {
  const text = greeting ? `${greeting}\n\n${MAIN_TEXT}` : MAIN_TEXT;
  if (env.cfg.main_menu_image_url) {
    try {
      const result=await tg(env, "sendPhoto", {chat_id, photo: env.cfg.main_menu_image_url, caption: text, parse_mode: "HTML", reply_markup: mainMenuKb(env)});
      if (result.ok) return;
    } catch {}
  }
  await sendMsg(env, chat_id, text, mainMenuKb(env));
}

async function isMember(env, uid) {
  if (!env.cfg.channel_username) return true;
  const r = await tg(env, "getChatMember", { chat_id: env.cfg.channel_username, user_id: uid });
  return !!r.ok && (["member", "administrator", "creator"].includes(r.result.status) || (r.result.status === "restricted" && r.result.is_member === true));
}

async function plansKb(env, prefix) {
  const plans = await getPlans(env);
  const rows = plans.map((p) => [btn(`📦 ${planLabel(p)} — ${toman(p.price)}`, `${prefix}_${p.key}`)]);
  rows.push([btn("🔙 بازگشت", "main_menu")]);
  return kb(rows);
}

// ---------------------------------------------------------- کارت سرویس
function serviceCaption(name, volume, dur, sub, cfg, header) {
  const L = [];
  if (header) L.push(header + "\n");
  L.push(
    `🔹 <b>${esc(name)}</b>\n`,
    `📦 حجم: ${esc(volume)}`,
    `⏳ مدت زمان: ${esc(dur)}`,
    "👥 تعداد کاربر: نامحدود\n",
    `🔗 <b>لینک ساب:</b>\n<code>${esc(sub || "-")}</code>\n`,
  );
  if (cfg) L.push(`⚙️ <b>کانفیگ:</b>\n<code>${esc(cfg)}</code>\n`);
  L.push("💡 روی لینک بزنید تا کپی شود و آن را در اپلیکیشن (بخش «دانلود اپلیکیشن‌ها») وارد کنید.");
  return L.join("\n");
}

const sendServiceCard = (env, chat_id, name, volume, dur, sub, cfg, keyboard, header) =>
  sendMsg(env, chat_id, serviceCaption(name, volume, dur, sub, cfg, header), keyboard);

const serviceKb = (id) => kb([
  [btn("🔗 کانفیگ‌ها و QR", `cfglist:${id}:0`)],
  [btn("🔘 فعال/غیرفعال کردن کانفیگ", `cfgmanage:${id}:0`)],
  [btn("🔄 تمدید سرویس", `renew_${id}`)],
  [btn("🗑 حذف سرویس", `delsvc_${id}`)],
  [btn("🔙 بازگشت", "my_services")],
]);

// The raw subscription is the panel's own enabled-only link export. Fetch it
// on demand; never log link contents or send them to a third-party QR service.
async function subscriptionLinks(svc) {
  if (!svc.sub_link) {
    const legacy = String(svc.config_link || "").trim().split(/\r?\n/).map(x=>x.trim()).filter(x=>/^[A-Za-z][A-Za-z0-9+.-]*:\/\//.test(x));
    if (legacy.length) return legacy;
    throw new Error("لینک اشتراک این سرویس ثبت نشده است.");
  }
  const url = new URL(svc.sub_link);
  if (url.protocol !== "https:" || url.username || url.password || url.hash) throw new Error("آدرس اشتراک معتبر و امن نیست.");
  url.pathname = url.pathname.replace(/\/+$/, "") + "/raw";
  const r = await fetch(url.toString(), { headers: {Accept:"text/plain", "User-Agent":"VPN-UI-Telegram-Bot/1.0"}, redirect:"error", signal:AbortSignal.timeout(15000) });
  if (!r.ok || !r.body) throw new Error(`دریافت اشتراک ناموفق بود (HTTP ${r.status}).`);
  const reader = r.body.getReader(); let size = 0; const chunks = [];
  try { for (;;) { const {done,value}=await reader.read(); if(done) break; size += value.length; if(size > 262144) throw new Error("پاسخ اشتراک بیش از حد بزرگ است."); chunks.push(value); } }
  finally { await reader.cancel().catch(()=>{}); }
  const bytes = new Uint8Array(size); let off=0; for(const chunk of chunks) {bytes.set(chunk,off);off+=chunk.length;}
  let raw = new TextDecoder('utf-8',{fatal:true}).decode(bytes).trim();
  if (!raw) return [];
  if (!/^[A-Za-z][A-Za-z0-9+.-]*:\/\//m.test(raw)) {
    if (!/^[A-Za-z0-9+/=\s]+$/.test(raw)) throw new Error("پاسخ اشتراک قابل خواندن نیست.");
    let decoded; try { decoded=atob(raw.replace(/\s/g,"")); } catch { throw new Error("پاسخ اشتراک نامعتبر است."); }
    raw=new TextDecoder('utf-8',{fatal:true}).decode(Uint8Array.from(decoded,c=>c.charCodeAt(0)));
  }
  return raw.split(/\r?\n/).map(x=>x.trim()).filter(x=>/^[A-Za-z][A-Za-z0-9+.-]*:\/\//.test(x));
}
function configName(link, index) {
  try {
    if (link.startsWith("vmess://")) {
      const encoded=link.slice(8).replace(/-/g,"+").replace(/_/g,"/");
      const data=JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(encoded),c=>c.charCodeAt(0))));
      if (data.ps) return String(data.ps).slice(0, 55);
    }
    const fragment = new URL(link).hash.slice(1);
    if (fragment) return decodeURIComponent(fragment).slice(0,55);
  } catch {}
  return `کانفیگ ${index + 1}`;
}
async function linkFingerprint(link) {
  const hash = new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(link)));
  return Array.from(hash.slice(0,6), x=>x.toString(16).padStart(2,'0')).join('');
}
async function sendConfigQR(env, chat, link, title, serviceId) {
  const qr = await qrPng(link);
  const body = new FormData(); body.set('chat_id', String(chat)); body.set('photo', qr, 'config.png');
  body.set('caption', `QR کانفیگ: ${title}`.slice(0, 200));
  body.set('reply_markup', JSON.stringify(backKb(`cfglist:${serviceId}:0`)));
  const res = await fetch(`https://api.telegram.org/bot${env.BOT_TOKEN}/sendPhoto`, {method:'POST', body});
  const data = await res.json(); if (!res.ok || !data.ok) throw new Error("ارسال QR در تلگرام ناموفق بود.");
}

// ---------------------------------------------------------- فاکتور
function invoiceText(env, o) {
  const L = [
    "💳 <b>فاکتور پرداخت</b>\n",
    `🏷 نام سرویس: <b>${esc(o.service_name)}</b>`,
    `📦 حجم بسته: <b>${esc(o.volume)}</b>`,
    `⏳ مدت زمان بسته: <b>${esc(o.duration)}</b>`,
    "👥 تعداد کاربر: <b>نامحدود</b>",
  ];
  if (o.renew_service_id) L.push("🔄 نوع: تمدید سرویس");
  L.push(`💰 مبلغ بسته: ${toman(o.price)}`);
  if (o.discount) L.push(`🎁 تخفیف: -${toman(o.discount)}`);
  L.push(`✅ مبلغ قابل پرداخت: <b>${toman(o.final_price)}</b>`);
  L.push(
    "\n➖➖➖➖➖➖➖➖➖",
    `💳 شماره کارت:\n<code>${esc(env.cfg.card_number)}</code>`,
    `👤 به نام: <b>${esc(env.cfg.card_holder)}</b>`,
    "➖➖➖➖➖➖➖➖➖\n",
    "📝 <b>توضیحات:</b>",
    "پس از واریز حتما فیش را همینجا ارسال کنید و منتظر تایید ادمین بمانید.",
    "به محض تایید، سرویس شما به‌صورت خودکار ساخته می‌شود ✅",
  );
  return L.join("\n");
}

async function createInvoice(c, user, chat_id, name, plan, renewId, cq) {
  const { env, st } = c;
  const days = renewId ? durationDays((await getService(env, renewId))?.duration) : num(env, "duration_days");
  if (!Number.isSafeInteger(days) || days <= 0) throw new Error("مدت سرویس مشخص نیست؛ با پشتیبانی تماس بگیرید");
  const discount = await takeDiscount(env, user.id);
  const final = Math.max(plan.price - discount, 0);
  const r = await run(env,
    "INSERT INTO orders(user_id,username,service_name,volume,price,discount,final_price,duration,renew_service_id,gb,days,created_at,status) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)",
    user.id, user.username || "-", name, planLabel(plan), plan.price, discount, final, durationText(days),
    renewId || null, plan.gb, days, now(), "draft");
  const orderId = r.meta.last_row_id;
  st.step = "receipt"; st.order_id = orderId; delete st.renew_service_id;
  const order = await getOrder(env, orderId);
  if (cq) { try { await tg(env, "deleteMessage", { chat_id, message_id: cq.message.message_id }); } catch {} }
  await sendMsg(env, chat_id, invoiceText(env, order), kb([[btn("💰 پرداخت از کیف پول", `wallet:pay:${orderId}`)],[btn("🔙 بازگشت", "main_menu")]]));
}

// ---------------------------------------------------------- تحویل سفارش (ساخت/تمدید در پنل)
async function fulfillOrder(env, order, onDispatch) {
  const bytes = Math.round((order.gb || 0) * GB);
  const days = order.days || num(env, "duration_days");
  if (!bytes) throw new Error("حجم سفارش نامعتبر است");
  if (order.renew_service_id) {
    const svc = await getService(env, order.renew_service_id);
    if (!svc || Number(svc.user_id) !== Number(order.user_id)) throw new Error("سرویس مربوط به این تمدید در دسترس نیست");
    if (!svc.panel_email) throw new Error("این سرویس به پنل متصل نیست (قدیمی است)؛ دستی در پنل تمدید کنید");
    const account = await panelServiceAccount(env, svc);
    if (!account) throw new Error("سرویس در پنل متصل فعلی یافت نشد");
    const lock = await run(env, "INSERT OR IGNORE INTO service_operations(service_id,kind,created_at) VALUES(?,?,?)",svc.id,"renew",now());
    if (!lock.meta.changes) throw new Error("عملیات دیگری روی سرویس در حال انجام است");
    try {
      const warn = await panelRenew(env, svc, bytes, days, onDispatch);
      await run(env, "UPDATE services SET volume=?, duration=?, volume_bytes=?, created_at=? WHERE id=?", order.volume, order.duration, bytes, now(), svc.id);
      return { renewed: true, name: svc.name, link: currentServiceLink(env,svc,account), warn };
    } finally { await run(env, "DELETE FROM service_operations WHERE service_id=?",svc.id); }
  }
  const p = await panelCreate(env, { name: order.service_name, bytes, days, attemptKey: `order:${order.id}:${hex(8)}`, onDispatch });
  await insertService(env, { ...p, user_id: order.user_id, name: order.service_name, volume: order.volume, duration: order.duration });
  return { renewed: false, name: order.service_name, link: p.link, warn: "" };
}

const walletReason = reason => ({topup:"شارژ با فیش",purchase:"خرید یا تمدید",refund:"بازگشت وجه",admin_adjustment:"تغییر توسط ادمین"}[reason] || "تراکنش");
async function showAdminWallet(env, chat, userId, owner) {
  const balance=await walletBalance(env,userId);
  const history=await all(env,"SELECT delta,reason FROM wallet_entries WHERE user_id=? ORDER BY id DESC LIMIT 10",userId);
  const lines=history.length ? history.map(x=>`${Number(x.delta)>0?"➕":"➖"} ${toman(Math.abs(Number(x.delta)))} — ${walletReason(x.reason)}`).join("\n") : "بدون تراکنش";
  const rows=[];if(owner) rows.push([btn("➕ افزایش موجودی",`admin:wallet_add:${userId}`),btn("➖ کاهش موجودی",`admin:wallet_sub:${userId}`)]);
  rows.push([btn("🔙 بازگشت", "admin:root")]);
  return sendMsg(env,chat,`💰 <b>کیف پول کاربر</b> <code>${userId}</code>\nموجودی: <b>${toman(balance)}</b>\n\n${lines}`,kb(rows));
}
async function handleWalletAmount(c,msg) {
  const {env,st}=c, amount=parseMoney(msg.text);
  if(amount===null) return sendMsg(env,msg.chat.id,"مبلغ معتبر را به تومان و با عدد انگلیسی بفرستید.",backKb("wallet:home"));
  if(!env.cfg.card_number || !env.cfg.card_holder) return sendMsg(env,msg.chat.id,"اطلاعات کارت شارژ تنظیم نشده است؛ با پشتیبانی تماس بگیرید.",backKb("wallet:home"));
  const row=await run(env,"INSERT INTO wallet_topups(user_id,amount,status,created_at) VALUES(?,?,'draft',?)",msg.from.id,amount,String(Date.now()));
  st.step="wallet_receipt";st.wallet_topup_id=row.meta.last_row_id;
  return sendMsg(env,msg.chat.id,`💳 <b>شارژ کیف پول</b>\nمبلغ: <b>${toman(amount)}</b>\nشماره کارت: <code>${esc(env.cfg.card_number)}</code>\nبه نام: ${esc(env.cfg.card_holder)}\n\nپس از واریز، فیش را همین‌جا بفرستید. شارژ پس از تأیید ادمین انجام می‌شود.`,backKb("wallet:home"));
}
async function handleWalletReceipt(c,msg) {
  const {env,st}=c, id=Number(st.wallet_topup_id);
  if(!Number.isSafeInteger(id)||id<=0){delete st.step;return sendMsg(env,msg.chat.id,"درخواست شارژ یافت نشد.",backKb("wallet:home"));}
  const topup=await first(env,"SELECT * FROM wallet_topups WHERE id=? AND user_id=?",id,msg.from.id);
  if(!topup || !["draft","needs_fix"].includes(topup.status)) {delete st.step;delete st.wallet_topup_id;return sendMsg(env,msg.chat.id,"درخواست شارژ قابل ارسال نیست.",backKb("wallet:home"));}
  const claimed=await run(env,"UPDATE wallet_topups SET status='pending' WHERE id=? AND user_id=? AND status=?",id,msg.from.id,topup.status);
  if(!claimed.meta.changes)return sendMsg(env,msg.chat.id,"درخواست قبلاً ارسال شده است.",backKb("wallet:home"));
  let delivered=0;
  for(const admin of env.admins) {
    try {
      const copy=await tg(env,"copyMessage",{chat_id:admin,from_chat_id:msg.chat.id,message_id:msg.message_id});if(!copy.ok)continue;
      const info=await sendMsg(env,admin,`💰 <b>فیش شارژ کیف پول</b>\nدرخواست: <code>${id}</code>\nکاربر: <code>${msg.from.id}</code>\nمبلغ: <b>${toman(topup.amount)}</b>`,kb([[btn("✅ تایید شارژ",`topup:approve:${id}`),btn("❌ رد فیش",`topup:reject:${id}`)]]));
      if(info.ok)delivered++;
    } catch(e) {console.log("wallet receipt delivery failed",e.message);}
  }
  if(!delivered) {
    const reverted=await run(env,"UPDATE wallet_topups SET status='needs_fix' WHERE id=? AND status='pending'",id);
    if(reverted.meta.changes)return sendMsg(env,msg.chat.id,"❌ ارسال فیش به ادمین ناموفق بود؛ دوباره فیش بفرستید یا با پشتیبانی تماس بگیرید.",backKb("wallet:home"));
    delete st.step;delete st.wallet_topup_id;return sendMsg(env,msg.chat.id,"وضعیت درخواست تغییر کرده؛ با پشتیبانی تماس بگیرید.",backKb("wallet:home"));
  }
  delete st.step;delete st.wallet_topup_id;
  return sendMsg(env,msg.chat.id,"✅ فیش دریافت شد؛ شارژ پس از تأیید ادمین ثبت می‌شود.",backKb("wallet:home"));
}

// ---------------------------------------------------------- هندلرهای کاربر
async function handleStart(c, msg) {
  const { env } = c;
  c.st = {};
  if (env.admins.has(msg.from.id) || await isMember(env, msg.from.id)) {
    await sendMainMenu(env, msg.chat.id, startGreeting(env,msg.from.first_name));
  } else {
    await sendMsg(env, msg.chat.id,
      "🔒 <b>برای استفاده از ربات ابتدا عضو کانال اصلی شوید</b>\n\nپس از عضویت، روی دکمه «✅ عضو شدم» بزنید.",
      kb([[urlBtn("📢 عضویت در کانال", env.cfg.channel_link)], [btn("✅ عضو شدم", "check_join")]]));
  }
}

async function handleReceipt(c, msg) {
  const { env, st } = c;
  const order = st.order_id ? await getOrder(env, st.order_id) : null;
  if (!order) {
    await sendMsg(env, msg.chat.id, "❌ سفارش یافت نشد. دوباره از منو اقدام کنید.");
    c.st = {};
    return;
  }
  const u = msg.from;
  const caption =
    "🧾 <b>فیش جدید</b>\n\n" +
    `🆔 سفارش: <code>${order.id}</code>\n` +
    `👤 کاربر: <a href="tg://user?id=${u.id}">${esc(u.first_name || "")}</a> (@${esc(order.username)})\n` +
    `🔢 آیدی عددی: <code>${u.id}</code>\n\n` +
    `🏷 نام سرویس: <code>${esc(order.service_name)}</code>\n` +
    `📦 حجم: ${esc(order.volume)}\n⏳ مدت: ${esc(order.duration)}\n👥 کاربر: نامحدود\n` +
    `💰 مبلغ بسته: ${toman(order.price)}\n` +
    (order.discount ? `🎁 تخفیف: -${toman(order.discount)}\n` : "") +
    `✅ مبلغ نهایی: <b>${toman(order.final_price)}</b>` +
    (order.renew_service_id ? "\n🔄 نوع: تمدید سرویس" : "");
  if (Number(order.user_id) !== Number(u.id) || !["draft", "needs_fix"].includes(order.status) || !(await claimOrder(env, order.id, order.status, "pending"))) {
    delete st.step; delete st.order_id; return sendMsg(env, msg.chat.id, "این سفارش قبلاً ثبت یا بررسی شده است.");
  }
  // Only acknowledge a receipt when at least one admin receives both the
  // receipt and actionable order details. A failed Telegram call is not success.
  let delivered = 0;
  for (const adminId of env.admins) {
    try {
      const copy = await tg(env, "copyMessage", { chat_id: adminId, from_chat_id: msg.chat.id, message_id: msg.message_id });
      if (!copy.ok) continue;
      const notification = await sendMsg(env, adminId, caption,
        kb([[btn("✅ تایید", `approve_${order.id}`), btn("⚠️ ایراد در فیش", `issue_${order.id}`)]]));
      if (notification.ok) delivered++;
    } catch (e) { console.log("receipt delivery failed", e.message); }
  }
  if (!delivered) {
    if (await claimOrder(env, order.id, "pending", "needs_fix")) {
      return sendMsg(env, msg.chat.id, "❌ ارسال فیش به ادمین ناموفق بود. لطفاً دوباره فیش را بفرستید یا با پشتیبانی تماس بگیرید.", backKb());
    }
    delete st.step; delete st.order_id;
    return sendMsg(env, msg.chat.id, "وضعیت سفارش تغییر کرده است؛ برای پیگیری با پشتیبانی تماس بگیرید.", backKb());
  }
  await sendMsg(env, msg.chat.id, "✅ فیش شما دریافت شد.\n⏳ لطفا تا بررسی و تایید ادمین منتظر بمانید.", backKb());
  delete st.step; delete st.order_id;
}

async function handleAdminText(c, msg) {
  const { env, st } = c;
  const order = await getOrder(env, st.admin_order);
  const text = (msg.text || "").trim();
  if (!order) { delete st.admin_step; delete st.admin_order; await sendMsg(env, msg.chat.id, "❌ سفارش یافت نشد."); return; }
  // اگر ادمین دیگری همین سفارش را قبلا بررسی کرده باشد، پیام ارسال نمی‌شود
  if (!(await claimOrder(env, order.id, "pending", "needs_fix"))) {
    delete st.admin_step; delete st.admin_order;
    return sendMsg(env, msg.chat.id, "ℹ️ این سفارش قبلا توسط ادمین دیگری بررسی شده است.");
  }
  const r = await tg(env, "sendMessage", {
    chat_id: order.user_id, parse_mode: "HTML",
    text: `⚠️ <b>مشکلی در فیش پرداختی شما وجود دارد:</b>\n\n${esc(text)}\n\nلطفا دوباره فیش پرداخت را ارسال کنید.`,
  });
  if (!r.ok) {
    await setOrderStatus(env, order.id, "pending");
    return sendMsg(env, msg.chat.id, `❌ ارسال پیام به کاربر ناموفق بود: ${esc(r.description)}`);
  }
  await sendMsg(env, msg.chat.id, "✅ پیام برای کاربر ارسال شد و منتظر فیش جدید می‌مانیم.");
  delete st.admin_step; delete st.admin_order;
}

// ---------------------------------------------------------- دستورات ادمین
const ADMIN_HELP =
  "🛠 <b>دستورات ادمین</b>\n\n" +
  "<b>تنظیمات</b>\n/settings — نمایش همه مقادیر\n/set KEY VALUE — تغییر یک مقدار\n\n" +
  "<b>بسته‌ها</b>\n/plans — لیست\n/setprice KEY PRICE — تغییر قیمت\n/setplan KEY PRICE GB — افزودن/ویرایش بسته\n/delplan KEY — حذف بسته\n\n" +
  "<b>ادمین‌ها</b>\n/admins — لیست\n/addadmin ID — افزودن ادمین (فقط ادمین اصلی)\n/deladmin ID — حذف ادمین (فقط ادمین اصلی)\n\n" +
  "<b>سایر</b>\n/stats — آمار فروش\n/resettest ID — اجازه تست دوباره به یک کاربر\n/panelcheck — تست اتصال به پنل و نمایش گروه‌ها/اینباندها";

async function handleAdminCommand(c, msg) {
  const { env } = c;
  const [cmdRaw, ...a] = msg.text.trim().split(/\s+/);
  const cmd = cmdRaw.split("@")[0];
  const reply = (t) => sendMsg(env, msg.chat.id, t);
  const isOwner = env.owners.includes(msg.from.id);

  if (cmd === "/admin") { delete c.st.admin_ui_step; delete c.st.admin_ui_key; delete c.st.admin_step; delete c.st.admin_order; return showAdminHome(env, msg.chat.id); }

  if (cmd === "/settings") {
    const lines = Object.entries(META).map(([k, [, desc]]) =>
      `<code>${k}</code> = <code>${esc(env.cfg[k] === "" ? "(خالی)" : env.cfg[k])}</code>\n   ↳ ${esc(desc)}`);
    return reply("⚙️ <b>تنظیمات فعلی</b>\n\n" + lines.join("\n\n") + "\n\nمثال: <code>/set test_mb 300</code>");
  }

  if (cmd === "/set") {
    if (a[0] === "start_text") {
      const match=msg.text.match(/^\/set(?:@\w+)?\s+start_text\s+([\s\S]+)$/);
      const value=match?.[1]?.trim() || "";
      if (!value || (value !== "-" && value.length > MAX_START_TEXT)) return reply(`❌ متن شروع باید حداکثر ${MAX_START_TEXT} نویسه باشد.`);
      await run(env,"INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value","start_text",value === "-" ? "" : value);
      return reply("✅ متن شروع ذخیره شد.");
    }
    const key = a[0];
    const raw = a.slice(1).join(" ").trim();
    const meta = META[key];
    if (!meta || !raw) return reply("❌ فرمت: <code>/set KEY VALUE</code>\nکلیدها را با /settings ببین.");
    const type = meta[0];
    const value = raw === "-" ? "" : raw;
    if (type === "int" && !/^\d+$/.test(value)) return reply("❌ مقدار باید عدد صحیح باشد.");
    if (type === "bool" && !/^[01]$/.test(value)) return reply("❌ مقدار باید 0 یا 1 باشد.");
    if (type === "ids" && !/^\d+(,\d+)*$/.test(value)) return reply("❌ مقدار باید عدد یا چند عدد با کاما باشد (مثلا 1,2).");
    await run(env, "INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value);
    return reply(`✅ <code>${key}</code> = <code>${esc(value === "" ? "(خالی)" : value)}</code>`);
  }

  if (cmd === "/plans") {
    const ps = await getPlans(env);
    return reply("📋 <b>بسته‌ها</b>\n\n" + ps.map((p) => `<code>${p.key}</code> | ${p.gb} گیگ | ${toman(p.price)}`).join("\n") +
      "\n\n/setprice KEY PRICE\n/setplan KEY PRICE GB\n/delplan KEY");
  }
  if (cmd === "/setprice" && a.length === 2 && /^\d+$/.test(a[1])) {
    const r = await run(env, "UPDATE plans SET price=? WHERE key=?", Number(a[1]), a[0]);
    return reply(r.meta.changes ? "✅ قیمت تغییر کرد." : "❌ کلید بسته پیدا نشد (/plans).");
  }
  if (cmd === "/setplan" && a.length === 3 && /^[A-Za-z0-9_-]{1,32}$/.test(a[0]) && /^\d+$/.test(a[1]) && /^\d+(\.\d+)?$/.test(a[2]) && Number(a[2]) > 0) {
    const sort = (await first(env, "SELECT COALESCE(MAX(sort),0)+1 AS n FROM plans")).n;
    await run(env,
      "INSERT INTO plans(key,gb,price,sort) VALUES(?,?,?,?) ON CONFLICT(key) DO UPDATE SET gb=excluded.gb, price=excluded.price",
      a[0], Number(a[2]), Number(a[1]), sort);
    return reply("✅ بسته ذخیره شد.");
  }
  if (cmd === "/delplan" && a.length === 1) {
    await run(env, "DELETE FROM plans WHERE key=?", a[0]);
    return reply("✅ حذف شد.");
  }

  if (cmd === "/admins") {
    const extra = (await all(env, "SELECT user_id FROM admins")).map((r) => r.user_id);
    return reply("👮 <b>ادمین‌ها</b>\n\nاصلی: " + env.owners.map((i) => `<code>${i}</code>`).join(", ") +
      "\nاضافه‌شده: " + (extra.length ? extra.map((i) => `<code>${i}</code>`).join(", ") : "—"));
  }
  if (cmd === "/addadmin" && a.length === 1 && /^\d+$/.test(a[0])) {
    if (!isOwner) return reply("❌ فقط ادمین اصلی می‌تواند ادمین اضافه کند.");
    await run(env, "INSERT OR IGNORE INTO admins(user_id,added_at) VALUES(?,?)", Number(a[0]), now());
    return reply("✅ ادمین اضافه شد. باید یک‌بار ربات را /start کند تا پیام‌ها به او برسد.");
  }
  if (cmd === "/deladmin" && a.length === 1 && /^\d+$/.test(a[0])) {
    if (!isOwner) return reply("❌ فقط ادمین اصلی می‌تواند ادمین حذف کند.");
    if (env.owners.includes(Number(a[0]))) return reply("❌ ادمین اصلی قابل حذف نیست.");
    await run(env, "DELETE FROM admins WHERE user_id=?", Number(a[0]));
    return reply("✅ حذف شد.");
  }

  if (cmd === "/resettest" && a.length === 1 && /^\d+$/.test(a[0])) {
    await run(env, "DELETE FROM test_claims WHERE user_id=?", Number(a[0]));
    return reply("✅ کاربر دوباره می‌تواند سرویس تست بگیرد.");
  }

  if (cmd === "/stats") {
    const u = await first(env, "SELECT COUNT(DISTINCT user_id) n FROM services");
    const s = await first(env, "SELECT COUNT(*) n FROM services");
    const o = await first(env, "SELECT COUNT(*) n, COALESCE(SUM(final_price),0) t FROM orders WHERE status='approved'");
    const t = await first(env, "SELECT COUNT(*) n FROM test_claims");
    return reply(`📊 مشتری: ${u.n}\nسرویس: ${s.n}\nسفارش تاییدشده: ${o.n}\nمجموع فروش: ${toman(o.t)}\nتست گرفته‌شده: ${t.n}`);
  }

  if (cmd === "/panelcheck") {
    try {
      panelCookie = null;
      const g = await panelCall(env, "GET", "/panel/api/clients/groups");
      const s = await panelCall(env, "GET", "/panel/api/clients/assignable");
      if (!g.success || !s.success) throw new Error(g.msg || s.msg || "دسترسی به گروه‌ها یا ورودی‌ها ممکن نیست");
      const cut = (x) => esc(JSON.stringify(x).slice(0, 1500));
      return reply("✅ اتصال به پنل برقرار است.\n\n<b>گروه‌ها:</b>\n<pre>" + cut(g) + "</pre>\n<b>اینباندهای قابل استفاده:</b>\n<pre>" + cut(s) + "</pre>\n" +
        "در پنل مدیریت، از بخش تنظیمات، گروه و ورودی را انتخاب کنید.");
    } catch (e) {
      return reply("❌ اتصال به پنل ناموفق:\n<code>" + esc(e.message) + "</code>");
    }
  }

  return reply("❌ دستور یا فرمت نادرست. /admin");
}

// دکمه‌های مدیریت فقط رابط دستورات موجود هستند؛ بررسی مجوز در هر درخواست انجام می‌شود.
const adminBack = (target = "admin:root") => kb([[btn("🔙 بازگشت", target)]]);
const adminHomeKb = () => kb([
  [btn("⚙️ تنظیمات ربات", "admin:settings"), btn("📦 مدیریت بسته‌ها", "admin:plans")],
  [btn("👮 مدیریت ادمین‌ها", "admin:admins"), btn("📊 آمار فروش", "admin:stats")],
  [btn("💰 مدیریت کیف پول", "admin:wallet"),btn("⚠️ سفارش‌های قابل بررسی", "admin:wallet_reviews")],
  [btn("🎁 مجوز دوباره تست", "admin:resettest"), btn("🔌 بررسی اتصال پنل", "admin:panelcheck")],
  [btn("📖 راهنما", "admin:help"), btn("🏠 منوی مشتری", "main_menu")],
]);
const showAdminHome = (env, chat) => sendMsg(env, chat, "🛠 <b>پنل مدیریت ربات</b>\n\nبخش موردنظر را انتخاب کنید:", adminHomeKb());
const planKeyOK = key => /^[A-Za-z0-9_-]{1,32}$/.test(key);
const positiveId = value => /^\d+$/.test(value) && Number.isSafeInteger(Number(value)) && Number(value) > 0;

async function handleAdminUIInput(c, msg) {
  const { env, st } = c, step = st.admin_ui_step, value = msg.text.trim();
  const send = text => sendMsg(env, msg.chat.id, text, adminBack());
  let command;
  if (step === "setting" && st.admin_ui_key === "start_text") {
    const text=msg.text.trim();
    if (!text || (text !== "-" && text.length > MAX_START_TEXT)) return send(`متن شروع باید حداکثر ${MAX_START_TEXT} نویسه باشد. برای بازگشت به متن پیش‌فرض، - بفرستید.`);
    await run(env,"INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value","start_text",text === "-" ? "" : text);
    delete st.admin_ui_step; delete st.admin_ui_key;
    await sendMsg(env,msg.chat.id,"✅ متن شروع ذخیره شد."); return showAdminHome(env,msg.chat.id);
  }
  if (step.startsWith("panel_")) {
    if (!env.owners.includes(msg.from.id)) { delete st.admin_ui_step; return send("فقط ادمین اصلی اجازه دارد."); }
    const draftKey=`panel_draft:${msg.from.id}`;
    // Try to remove the original message, but never claim Telegram erased it.
    try { await tg(env,"deleteMessage",{chat_id:msg.chat.id,message_id:msg.message_id}); } catch {}
    if (step === "panel_sub") {
      let url; try { url=panelURL(value); } catch { return send("آدرس ساب باید یک آدرس HTTPS معتبر باشد."); }
      await run(env,"INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value","sub_base_url",url+"/");
      delete st.admin_ui_step; delete st.admin_ui_key;
      await sendMsg(env,msg.chat.id,"✅ آدرس ساب ذخیره شد؛ تنظیم اتصال پنل کامل است."); return showAdminHome(env,msg.chat.id);
    }
    let draft;
    if (step === "panel_url") {
      let url; try { url=panelURL(value); } catch { return send("آدرس پنل باید HTTPS و بدون پارامتر باشد؛ دوباره بفرستید."); }
      draft={url,expires:Date.now()+30*60*1000};
      await run(env,"INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",draftKey,await encryptPanel(env.BOT_TOKEN,draft));
      st.admin_ui_step="panel_user"; return sendMsg(env,msg.chat.id,"نام کاربری حساب نمایندهٔ پنل را بفرستید.",adminBack("admin:settings"));
    }
    const row=await first(env,"SELECT value FROM settings WHERE key=?",draftKey);
    try { draft=row && await decryptPanel(env.BOT_TOKEN,row.value); } catch { draft=null; }
    if(!draft || !draft.url || Date.now()>Number(draft.expires)) { await run(env,"DELETE FROM settings WHERE key=?",draftKey); delete st.admin_ui_step; return send("درخواست منقضی شد؛ از تنظیمات دوباره شروع کنید."); }
    if(step === "panel_user") {
      if(!value || value.length>256 || /[\r\n]/.test(value))return send("نام کاربری معتبر نیست؛ دوباره بفرستید.");
      draft.user=value; await run(env,"INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",draftKey,await encryptPanel(env.BOT_TOKEN,draft));
      st.admin_ui_step="panel_pass"; return sendMsg(env,msg.chat.id,"رمز پنل را در همین گفت‌وگوی خصوصی بفرستید. ربات تلاش می‌کند پیام رمز را از گفت‌وگو حذف کند.",adminBack("admin:settings"));
    }
    if(step === "panel_pass") {
      const pass=msg.text; if(!pass || !pass.trim() || pass.length>1024 || /[\r\n]/.test(pass))return send("رمز معتبر نیست؛ دوباره بفرستید.");
      if(!draft.user) {delete st.admin_ui_step; return send("درخواست ناقص است؛ دوباره شروع کنید.");}
      const newIdentity=await panelAccountKey(draft.url,draft.user);
      const changing = draft.url !== panelBase(env);
      const changingAccount = env.panelStoredIdentity ? newIdentity !== env.panelStoredIdentity : (changing || !env.PANEL_USER || draft.user !== env.PANEL_USER);

      const config={url:draft.url,user:draft.user,pass};
      try { await probePanel(config); } catch(e) { return send(`❌ اتصال تأیید نشد: ${esc(e.message)}\nرمز را دوباره بفرستید یا از تنظیمات شروع کنید.`); }
      const queries=[env.DB.prepare("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value").bind("panel_credentials_v1",await encryptPanel(env.BOT_TOKEN,config)),env.DB.prepare("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value").bind("panel_identity_v1",newIdentity),env.DB.prepare("DELETE FROM settings WHERE key=?").bind(draftKey)];
      if(changingAccount) for(const key of ["panel_group_ids","panel_inbound_id","sub_base_url"]) queries.push(env.DB.prepare("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value").bind(key,""));
      await env.DB.batch(queries); panelCookie=null;panelCookieKey="";
      delete st.admin_ui_step; delete st.admin_ui_key;
      return sendMsg(env,msg.chat.id,"✅ اتصال پنل تأیید و ذخیره شد. حالا گروه و ورودی را انتخاب کنید.",kb([[btn("📂 انتخاب گروه و ورودی","admin:pgroups")],[btn("🔙 تنظیمات","admin:settings")]]));
    }
    delete st.admin_ui_step; return send("درخواست نامعتبر است؛ دوباره شروع کنید.");
  }
  if (step === "wallet_lookup") {
    if (!positiveId(value)) return send("آیدی عددی معتبر را با اعداد انگلیسی وارد کنید.");
    delete st.admin_ui_step; delete st.admin_ui_key;
    return showAdminWallet(env,msg.chat.id,Number(value),true);
  }
  if (step === "wallet_add" || step === "wallet_sub") {
    if (!env.admins.has(msg.from.id)) { delete st.admin_ui_step; return send("دسترسی ندارید."); }
    const amount=parseMoney(value); if(amount===null || !positiveId(String(st.admin_ui_key))) return send("مبلغ معتبر را به تومان و با عدد انگلیسی وارد کنید.");
    const userId=Number(st.admin_ui_key), delta=step === "wallet_add" ? amount : -amount;
    st.admin_wallet_change={userId,delta,nonce:crypto.randomUUID()}; delete st.admin_ui_step; delete st.admin_ui_key;
    return sendMsg(env,msg.chat.id,`تغییر موجودی کاربر <code>${userId}</code> به مبلغ ${toman(amount)} (${delta>0 ? "افزایش" : "کاهش"}) انجام شود؟`,kb([[btn("✅ تایید", "admin:wallet_confirm")],[btn("❌ انصراف", "admin:root")]]));
  }
  if (step === "setting") {
    const meta = META[st.admin_ui_key];
    if (!meta || !value) return send("مقدار نامعتبر است؛ دوباره وارد کنید.");
    if (meta[0] === "int" && !/^\d+$/.test(value)) return send("مقدار باید عدد صحیح باشد.");
    if (meta[0] === "bool" && !/^[01]$/.test(value)) return send("مقدار باید ۰ یا ۱ (به انگلیسی) باشد.");
    if (meta[0] === "ids" && !/^\d+(,\d+)*$/.test(value)) return send("شناسه‌ها را با اعداد انگلیسی و کاما وارد کنید.");
    command = `/set ${st.admin_ui_key} ${value}`;
  } else if (step === "price") {
    if (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value))) return send("قیمت را به تومان و با عدد صحیح انگلیسی وارد کنید.");
    if (!(await getPlan(env, st.admin_ui_key))) return send("بسته دیگر وجود ندارد؛ از منو انتخاب کنید.");
    command = `/setprice ${st.admin_ui_key} ${value}`;
  } else if (step === "plan_edit") {
    const parts = value.split(/\s+/);
    if (parts.length !== 2 || !/^\d+$/.test(parts[0]) || !/^\d+(\.\d+)?$/.test(parts[1]) || !Number.isSafeInteger(Number(parts[0])) || !(Number(parts[1]) > 0) || !Number.isFinite(Number(parts[1]))) return send("قیمت و حجم را به شکل «70000 10» با اعداد انگلیسی وارد کنید.");
    if (!(await getPlan(env, st.admin_ui_key))) return send("بسته دیگر وجود ندارد؛ از منو انتخاب کنید.");
    command = `/setplan ${st.admin_ui_key} ${parts.join(" ")}`;
  } else if (step === "plan_new") {
    const parts = value.split(/\s+/);
    if (parts.length !== 3 || !planKeyOK(parts[0]) || !/^\d+$/.test(parts[1]) || !Number.isSafeInteger(Number(parts[1])) || !/^\d+(\.\d+)?$/.test(parts[2]) || !(Number(parts[2]) > 0) || !Number.isFinite(Number(parts[2]))) return send("کلید، قیمت و حجم را به شکل «p10 70000 10» با حروف و اعداد انگلیسی وارد کنید.");
    if (await getPlan(env, parts[0])) return send("این کلید از قبل وجود دارد؛ برای تغییر آن از فهرست بسته‌ها استفاده کنید.");
    command = `/setplan ${parts.join(" ")}`;
  } else if (step === "add_admin") {
    if (!env.owners.includes(msg.from.id)) { delete st.admin_ui_step; return send("فقط ادمین اصلی اجازه دارد."); }
    if (!positiveId(value)) return send("آیدی عددی معتبر تلگرام را با اعداد انگلیسی وارد کنید.");
    command = `/addadmin ${value}`;
  } else if (step === "reset_test") {
    if (!positiveId(value)) return send("آیدی عددی معتبر تلگرام را با اعداد انگلیسی وارد کنید.");
    st.admin_ui_step = "reset_confirm"; st.admin_ui_key = value;
    return sendMsg(env, msg.chat.id, `اجازهٔ سرویس تست دوباره برای <code>${esc(value)}</code> ثبت شود؟ ابتدا مطمئن شوید سرویس قبلی بررسی شده است.`, kb([[btn("✅ تایید", "admin:reset_confirm")], [btn("❌ انصراف", "admin:root")]]));
  } else { delete st.admin_ui_step; delete st.admin_ui_key; return showAdminHome(env, msg.chat.id); }
  delete st.admin_ui_step; delete st.admin_ui_key;
  await handleAdminCommand(c, { ...msg, text: command });
  return showAdminHome(env, msg.chat.id);
}

async function panelChoices(env) {
  try {
    const [groups,inbounds]=await Promise.all([panelCall(env,"GET","/panel/api/clients/groups"),panelCall(env,"GET","/panel/api/clients/assignable")]);
    if(!groups.success || !inbounds.success || !Array.isArray(groups.obj) || !Array.isArray(inbounds.obj)) return null;
    return {groups:groups.obj,inbounds:inbounds.obj};
  } catch { return null; }
}

async function handleAdminUICallback(c, cq, answer) {
  const { env, st } = c, uid = cq.from.id, chat = cq.message.chat.id, d = cq.data;
  const owner = env.owners.includes(uid);
  if (d !== "admin:reset_confirm") { delete st.admin_ui_step; delete st.admin_ui_key; }
  if (d !== "admin:wallet_confirm") delete st.admin_wallet_change;
  delete st.admin_step; delete st.admin_order;
  const renderAdmin = (text, keyboard) => render(env, cq, text, keyboard);
  const prompt = async (step, key, text, back = "admin:root") => {
    st.admin_ui_step = step; st.admin_ui_key = key || "";
    return renderAdmin(text, adminBack(back));
  };
  if (d === "admin:root") {
    delete st.admin_ui_step; delete st.admin_ui_key;
    return renderAdmin("🛠 <b>پنل مدیریت ربات</b>\n\nبخش موردنظر را انتخاب کنید:", adminHomeKb());
  }
  if (d === "admin:wallet_reviews") {
    const rows=await all(env,"SELECT id,user_id,final_price FROM orders WHERE status='needs_review_refunded' ORDER BY id DESC LIMIT 20");
    const keys=rows.map(x=>[btn(`سفارش ${x.id} — ${x.user_id}`,`admin:wallet_review:${x.id}`)]);keys.push([btn("🔙 بازگشت","admin:root")]);
    return renderAdmin(rows.length ? "⚠️ <b>سفارش‌های برگشت‌وجه‌شده و نیازمند بررسی</b>" : "سفارش نیازمند بررسی وجود ندارد.",kb(keys));
  }
  if (d.startsWith("admin:wallet_review:")) {
    const id=Number(d.slice("admin:wallet_review:".length)); if(!Number.isSafeInteger(id)||id<=0)return answer("شناسه نامعتبر است.",true);
    const order=await first(env,"SELECT * FROM orders WHERE id=? AND status='needs_review_refunded'",id);
    if(!order)return answer("سفارش نیازمند بررسی پیدا نشد.",true);
    const attempt=await first(env,"SELECT email FROM provision_attempts WHERE attempt_key LIKE ? ORDER BY rowid DESC LIMIT 1",`order:${id}:%`);
    return renderAdmin(`⚠️ <b>سفارش ${id}</b>\nکاربر: <code>${order.user_id}</code>\nمبلغ برگشتی: ${toman(order.final_price)}\nحساب پنل: <code>${esc(attempt?.email || "نامشخص یا تمدید")}</code>\n\nموجودی بازگشته است. ممکن است عملیات پنل انجام شده باشد؛ پیش از هر اقدام، پنل و سرویس را بررسی کنید.`,kb([[btn("✅ بررسی شد",`admin:wallet_close:${id}`)],[btn("🔙 بازگشت","admin:wallet_reviews")]]));
  }
  if (d.startsWith("admin:wallet_close:")) {
    const id=Number(d.slice("admin:wallet_close:".length));if(!Number.isSafeInteger(id)||id<=0)return answer("شناسه نامعتبر است.",true);
    return renderAdmin(`بررسی سفارش <code>${id}</code> را تکمیل کرده‌اید؟ بستن این مورد هیچ تغییری در موجودی یا سرویس ایجاد نمی‌کند.`,kb([[btn("✅ تایید",`admin:wallet_close_yes:${id}`)],[btn("❌ انصراف","admin:wallet_reviews")]]));
  }
  if (d.startsWith("admin:wallet_close_yes:")) {
    const id=Number(d.slice("admin:wallet_close_yes:".length));if(!Number.isSafeInteger(id)||id<=0)return answer("شناسه نامعتبر است.",true);
    try {
      const res=await env.DB.batch([env.DB.prepare("INSERT INTO wallet_review_resolutions(order_id,actor_id,resolved_at) SELECT id,?,? FROM orders WHERE id=? AND status='needs_review_refunded'").bind(uid,String(Date.now()),id),env.DB.prepare("UPDATE orders SET status='wallet_review_resolved' WHERE id=? AND status='needs_review_refunded' AND EXISTS(SELECT 1 FROM wallet_review_resolutions WHERE order_id=?)").bind(id,id)]);
      if(!res[0]?.meta?.changes || !res[1]?.meta?.changes)return answer("این سفارش قبلاً بررسی شده است.",true);
    } catch {return answer("ثبت بررسی ناموفق بود؛ وضعیت را دوباره ببینید.",true);}
    return renderAdmin("✅ بررسی سفارش ثبت شد.",adminBack("admin:wallet_reviews"));
  }
  if (d === "admin:wallet") return prompt("wallet_lookup", "", "آیدی عددی کاربر را برای مدیریت کیف پول وارد کنید.");
  if (d.startsWith("admin:wallet_view:")) {
    const id=d.slice("admin:wallet_view:".length); if(!positiveId(id)) return answer("آیدی نامعتبر است.",true);
    return showAdminWallet(env,chat,Number(id),true);
  }
  if (d.startsWith("admin:wallet_add:") || d.startsWith("admin:wallet_sub:")) {
    const add=d.startsWith("admin:wallet_add:"), id=d.split(":").at(-1);
    if(!positiveId(id)) return answer("آیدی نامعتبر است.",true);
    return prompt(add ? "wallet_add" : "wallet_sub",id,`مبلغ ${add ? "افزایش" : "کاهش"} را به تومان و با عدد انگلیسی بفرستید.`);
  }
  if (d === "admin:wallet_confirm") {
    const change=st.admin_wallet_change; if(!change || !positiveId(String(change.userId)) || !Number.isSafeInteger(change.delta) || !/^[a-f0-9-]{36}$/.test(change.nonce)) return answer("درخواست منقضی شده است.",true);
    delete st.admin_wallet_change;
    try { await adjustWallet(env,change.userId,change.delta,uid,`admin:${change.nonce}`); }
    catch(e) { return sendMsg(env,chat, String(e.message).includes("WALLET_INSUFFICIENT") ? "❌ موجودی برای کسر کافی نیست." : "❌ تغییر موجودی انجام نشد یا قبلاً ثبت شده است؛ موجودی را بررسی کنید.",adminBack()); }
    return showAdminWallet(env,chat,change.userId,true);
  }
  if (d === "admin:settings") {
    delete st.admin_ui_step; delete st.admin_ui_key;
    const rows = [];
    if (owner) rows.push([btn("➕ افزودن یا تنظیم اتصال پنل", "admin:panelsetup")]);
    rows.push(...Object.entries(META).map(([key, [, label]]) => [btn(label, `admin:setting:${key}`)]));
    rows.push([btn("🔙 بازگشت", "admin:root")]);
    return renderAdmin("⚙️ <b>تنظیمات</b>\nبرای دیدن و تغییر هر گزینه آن را انتخاب کنید:", kb(rows));
  }
  if (d === "admin:panelsetup") {
    if (!owner) return answer("فقط ادمین اصلی اجازه تنظیم اتصال پنل را دارد.", true);
    await run(env, "DELETE FROM settings WHERE key=?", `panel_draft:${uid}`);
    return prompt("panel_url", "", "➕ آدرس کامل HTTPS پنل شامل پورت و مسیر پنل را بفرستید.\nمثال: <code>https://panel.example.com:2053/SECRET_PATH</code>", "admin:settings");
  }
  if (d === "admin:pgroups" || /^admin:pgroups:\d+$/.test(d)) {
    if (!owner) return answer("فقط ادمین اصلی اجازه دارد.", true);
    const choices=await panelChoices(env);
    if(!choices) return renderAdmin("خواندن گروه‌ها یا ورودی‌ها ناموفق بود. اتصال و دسترسی حساب نماینده را بررسی کنید.",adminBack("admin:settings"));
    const usable=choices.groups.filter(g=>positiveId(String(g.id)) && Array.isArray(g.inboundIds) && g.inboundIds.some(x=>choices.inbounds.some(i=>Number(i.inboundId)===Number(x))));
    const page=Number(d.split(":")[2] || 0); if(!Number.isSafeInteger(page) || page<0 || page>Math.max(0,Math.ceil(usable.length/15)-1)) return answer("صفحه یافت نشد.",true);
    const rows=usable.slice(page*15,(page+1)*15).map(g=>[btn(String(g.name || `گروه ${g.id}`).slice(0,50),`admin:pg:${g.id}`)]);
    const nav=[]; if(page>0)nav.push(btn("◀️ قبلی",`admin:pgroups:${page-1}`));if((page+1)*15<usable.length)nav.push(btn("بعدی ▶️",`admin:pgroups:${page+1}`));if(nav.length)rows.push(nav);rows.push([btn("🔙 بازگشت","admin:settings")]);
    return renderAdmin(usable.length ? "✅ پنل متصل است. گروه فروش را انتخاب کنید:" : "گروه قابل استفاده پیدا نشد؛ دسترسی نماینده را در پنل بررسی کنید.",kb(rows));
  }
  if (d.startsWith("admin:pg:")) {
    if (!owner) return answer("فقط ادمین اصلی اجازه دارد.", true);
    const [, ,groupId,pageText] = d.split(":"); const page=Number(pageText || 0);
    if(!positiveId(groupId) || !Number.isSafeInteger(page) || page<0) return answer("گروه نامعتبر است.",true);
    const choices=await panelChoices(env);
    if(!choices) return answer("دریافت اطلاعات پنل ناموفق بود.",true);
    const g=choices.groups.find(x=>Number(x.id)===Number(groupId)); if(!g || !Array.isArray(g.inboundIds)) return answer("گروه دیگر در دسترس نیست.",true);
    const allowed=choices.inbounds.filter(x=>positiveId(String(x.inboundId)) && g.inboundIds.some(id=>Number(id)===Number(x.inboundId)));
    if(page>Math.max(0,Math.ceil(allowed.length/15)-1))return answer("صفحه یافت نشد.",true);
    const rows=allowed.slice(page*15,(page+1)*15).map(i=>[btn(`${i.remark || `ورودی ${i.inboundId}`} (${i.protocol || "?"})`.slice(0,55),`admin:pi:${groupId}:${i.inboundId}`)]);
    const nav=[];if(page>0)nav.push(btn("◀️ قبلی",`admin:pg:${groupId}:${page-1}`));if((page+1)*15<allowed.length)nav.push(btn("بعدی ▶️",`admin:pg:${groupId}:${page+1}`));if(nav.length)rows.push(nav);rows.push([btn("🔙 بازگشت","admin:pgroups")]);
    return renderAdmin(allowed.length ? "ورودی اصلی این گروه را انتخاب کنید. همهٔ ورودی‌های گروه برای سرویس استفاده می‌شوند." : "ورودی قابل استفاده‌ای در این گروه نیست.",kb(rows));
  }
  if (d.startsWith("admin:pi:")) {
    if (!owner) return answer("فقط ادمین اصلی اجازه دارد.", true);
    const [, ,groupId,inboundId]=d.split(":"); if(!positiveId(groupId)||!positiveId(inboundId)) return answer("شناسه نامعتبر است.",true);
    const choices=await panelChoices(env);
    if(!choices)return answer("دریافت اطلاعات پنل ناموفق بود.",true);
    const g=choices.groups.find(x=>Number(x.id)===Number(groupId));
    if(!g || !Array.isArray(g.inboundIds) || !g.inboundIds.some(x=>Number(x)===Number(inboundId)) || !choices.inbounds.some(i=>Number(i.inboundId)===Number(inboundId))) return answer("گروه یا ورودی دیگر قابل استفاده نیست.",true);
    await env.DB.batch([env.DB.prepare("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value").bind("panel_group_ids",groupId),env.DB.prepare("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value").bind("panel_inbound_id",inboundId)]);
    return prompt("panel_sub", "", "✅ گروه و ورودی ذخیره شد. آدرس پایهٔ HTTPS اشتراک را بفرستید، مانند <code>https://sub.example.com:2096/sub/</code>.", "admin:settings");
  }
  if (d.startsWith("admin:setting:")) {
    const key = d.slice("admin:setting:".length), meta = Object.hasOwn(META, key) ? META[key] : null;
    if (!meta) return answer("تنظیم یافت نشد.", true);
    if (key === "start_text") return prompt("setting",key,`✏️ <b>متن پیام شروع</b>\nمتن فعلی: <code>${esc(env.cfg.start_text || DEFAULT_START_TEXT)}</code>\n\nمتن جدید را بفرستید؛ برای نمایش نام کاربر از <code>{name}</code> استفاده کنید. حداکثر ${MAX_START_TEXT} نویسه؛ برای بازگشت به متن پیش‌فرض، - بفرستید.`,"admin:settings");
    if (key === "test_enabled") return renderAdmin(`🎁 سرویس تست اکنون ${env.cfg.test_enabled === "1" ? "فعال" : "غیرفعال"} است.`, kb([[btn("✅ فعال", "admin:test:1"),btn("⛔ غیرفعال", "admin:test:0")],[btn("🔙 بازگشت", "admin:settings")]]));
    return prompt("setting", key, `⚙️ <b>${esc(meta[1])}</b>\nمقدار فعلی: <code>${esc(env.cfg[key] || "خالی")}</code>\n\nمقدار جدید را بفرستید.${meta[0] === "text" ? " برای پاک‌کردن مقدار، - بفرستید." : ""}`, "admin:settings");
  }
  if (/^admin:test:[01]$/.test(d)) {
    const value = d.slice(-1); await handleAdminCommand(c, {chat:{id:chat},from:cq.from,text:`/set test_enabled ${value}`});
    return showAdminHome(env, chat);
  }
  if (d === "admin:plans") {
    delete st.admin_ui_step; delete st.admin_ui_key;
    const plans = await getPlans(env), safe = plans.filter(p => planKeyOK(p.key));
    const rows = safe.map(p => [btn(`${planLabel(p)} — ${toman(p.price)}`, `admin:plan:${p.key}`)]);
    rows.push([btn("➕ افزودن بسته", "admin:plan_add")],[btn("🔙 بازگشت", "admin:root")]);
    return renderAdmin(`📦 <b>مدیریت بسته‌ها</b>${plans.length !== safe.length ? "\nبعضی کلیدهای قدیمی نامعتبرند؛ با دستورات قبلی اصلاحشان کنید." : ""}`, kb(rows));
  }
  if (d === "admin:plan_add") return prompt("plan_new", "", "➕ کلید انگلیسی، قیمت (تومان) و حجم (گیگ) را با فاصله بفرستید.\nمثال: <code>p10 70000 10</code>", "admin:plans");
  for (const [prefix, kind] of [["admin:plan:","show"],["admin:price:","price"],["admin:editplan:","edit"],["admin:delplan:","delete"],["admin:confirmplan:","confirm"]]) {
    if (!d.startsWith(prefix)) continue;
    const key = d.slice(prefix.length); if (!planKeyOK(key)) return answer("کلید بسته نامعتبر است.", true);
    const plan = await getPlan(env, key); if (!plan) return answer("بسته پیدا نشد.", true);
    if (kind === "show") return renderAdmin(`📦 <b>${esc(planLabel(plan))}</b>\nکلید: <code>${esc(key)}</code>\nقیمت: ${toman(plan.price)}`, kb([[btn("💰 تغییر قیمت", `admin:price:${key}`),btn("✏️ تغییر حجم و قیمت", `admin:editplan:${key}`)],[btn("🗑 حذف بسته", `admin:delplan:${key}`)],[btn("🔙 بازگشت", "admin:plans")]]));
    if (kind === "price") return prompt("price", key, `قیمت جدید «${esc(key)}» را به تومان بفرستید.`, "admin:plans");
    if (kind === "edit") return prompt("plan_edit", key, `قیمت و حجم جدید «${esc(key)}» را با فاصله بفرستید. مثال: <code>70000 10</code>`, "admin:plans");
    if (kind === "delete") return renderAdmin(`بستهٔ <code>${esc(key)}</code> حذف شود؟`, kb([[btn("✅ تایید حذف", `admin:confirmplan:${key}`)],[btn("❌ انصراف", "admin:plans")]]));
    await handleAdminCommand(c, {chat:{id:chat},from:cq.from,text:`/delplan ${key}`}); return showAdminHome(env, chat);
  }
  if (d === "admin:admins") {
    const extra = (await all(env, "SELECT user_id FROM admins")).map(x=>Number(x.user_id));
    const lines = `👮 <b>ادمین‌ها</b>\nاصلی: ${env.owners.map(x=>`<code>${x}</code>`).join(", ")}\nاضافه‌شده: ${extra.length ? extra.map(x=>`<code>${x}</code>`).join(", ") : "—"}`;
    const rows = owner ? extra.filter(x=>!env.owners.includes(x)).map(x=>[btn(`🗑 حذف ${x}`, `admin:rmadmin:${x}`)]) : [];
    if (owner) rows.push([btn("➕ افزودن ادمین", "admin:add_admin")]);
    rows.push([btn("🔙 بازگشت", "admin:root")]); return renderAdmin(lines, kb(rows));
  }
  if (d === "admin:add_admin") {
    if (!owner) return answer("فقط ادمین اصلی اجازه دارد.", true);
    return prompt("add_admin", "", "آیدی عددی ادمین جدید را بفرستید.", "admin:admins");
  }
  if (d.startsWith("admin:rmadmin:")) {
    if (!owner) return answer("فقط ادمین اصلی اجازه دارد.", true);
    const id=d.slice("admin:rmadmin:".length); if (!positiveId(id) || env.owners.includes(Number(id))) return answer("ادمین قابل حذف نیست.", true);
    return renderAdmin(`ادمین <code>${esc(id)}</code> حذف شود؟`, kb([[btn("✅ تایید حذف", `admin:rmconfirm:${id}`)],[btn("❌ انصراف", "admin:admins")]]));
  }
  if (d.startsWith("admin:rmconfirm:")) {
    if (!owner) return answer("فقط ادمین اصلی اجازه دارد.", true);
    const id=d.slice("admin:rmconfirm:".length); if (!positiveId(id) || env.owners.includes(Number(id))) return answer("ادمین قابل حذف نیست.", true);
    await handleAdminCommand(c, {chat:{id:chat},from:cq.from,text:`/deladmin ${id}`}); return showAdminHome(env, chat);
  }
  if (d === "admin:resettest") return prompt("reset_test", "", "آیدی عددی کاربری را که باید دوباره اجازه تست بگیرد بفرستید.");
  if (d === "admin:reset_confirm") {
    if (st.admin_ui_step !== "reset_confirm" || !positiveId(st.admin_ui_key)) return answer("درخواست منقضی است؛ دوباره اقدام کنید.", true);
    const id=st.admin_ui_key; delete st.admin_ui_step; delete st.admin_ui_key;
    await handleAdminCommand(c, {chat:{id:chat},from:cq.from,text:`/resettest ${id}`}); return showAdminHome(env, chat);
  }
  if (d === "admin:stats" || d === "admin:panelcheck") {
    await handleAdminCommand(c, {chat:{id:chat},from:cq.from,text:d === "admin:stats" ? "/stats" : "/panelcheck"}); return showAdminHome(env, chat);
  }
  if (d === "admin:help") return renderAdmin("📖 مدیریت تنظیمات، بسته‌ها، ادمین‌ها، آمار، تست و اتصال پنل از همین دکمه‌ها انجام می‌شود. فیش‌های پرداخت برای ادمین‌ها ارسال می‌شوند و دکمهٔ تایید یا اعلام ایراد دارند.", adminBack());
  return answer("گزینه پیدا نشد.", true);
}

const ADMIN_CMD_RE = /^\/(admin|settings|set|plans|setprice|setplan|delplan|stats|admins|addadmin|deladmin|resettest|panelcheck)(@\w+)?(\s|$)/;

async function onMessage(c, msg) {
  const { env } = c;
  if (msg.chat.type !== "private") return;
  const uid = msg.from.id;
  const isAdmin = env.admins.has(uid);
  const text = msg.text;

  if (text && isAdmin && c.st.admin_ui_step === "panel_pass") return handleAdminUIInput(c, msg);
  if (text && text.startsWith("/start")) return handleStart(c, msg);
  if (isAdmin && text && ADMIN_CMD_RE.test(text)) return handleAdminCommand(c, msg);
  if (text && isAdmin && c.st.admin_step === "issue_message") return handleAdminText(c, msg);
  if (text && isAdmin && c.st.admin_ui_step) return handleAdminUIInput(c, msg);
  if (!isAdmin && !(await isMember(env, uid))) return sendMsg(env, msg.chat.id, "برای استفاده ابتدا در کانال عضو شوید و /start را بزنید.");

  const st = c.st;
  if (text) {
    if (st.step === "wallet_amount") return handleWalletAmount(c,msg);
    if (st.step === "wallet_receipt") return handleWalletReceipt(c,msg);
    if (st.step === "service_name") {
      const name = text.trim();
      if (!NAME_RE.test(name)) {
        return sendMsg(env, msg.chat.id,
          "❌ نام نامعتبر است.\n\n⚠️ فقط حروف انگلیسی (A-Z) مجاز است، بدون فاصله و بدون هیچ علامتی.\n<i>مثال درست: Amir  یا  Mobile</i>");
      }
      if (name.length > 30) return sendMsg(env, msg.chat.id, "❌ نام سرویس خیلی طولانی است. کوتاه‌تر بنویسید.");
      const plan = await getPlan(env, st.plan_key);
      if (!plan) { c.st = {}; return sendMsg(env, msg.chat.id, "❌ بسته یافت نشد، دوباره از منو اقدام کنید."); }
      return createInvoice(c, msg.from, msg.chat.id, name, plan, null, null);
    }
    if (st.step === "receipt") return handleReceipt(c, msg);
    if (st.step === "invite_code_input") {
      const t = text.trim();
      if (!/^\d+$/.test(t)) return sendMsg(env, msg.chat.id, "❌ کد دعوت باید فقط عدد (آیدی عددی) باشد.");
      const ref = Number(t);
      if (ref === uid) return sendMsg(env, msg.chat.id, "❌ نمی‌توانید کد دعوت خودتان را وارد کنید.");
      const ok = await registerReferral(env, ref, uid);
      delete st.step;
      if (!ok) return sendMsg(env, msg.chat.id, "❌ شما قبلا یک کد دعوت ثبت کرده‌اید.", backKb());
      const amt = toman(num(env, "referral_discount"));
      await sendMsg(env, msg.chat.id, `✅ کد دعوت با موفقیت ثبت شد!\nتخفیف ${amt} برای دوست شما در خرید بعدی‌اش فعال شد.`, backKb());
      await sendMsg(env, ref, `🎉 یک نفر با کد دعوت شما ثبت‌نام کرد!\nتخفیف ${amt} برای خرید بعدی شما فعال شد.`);
      return;
    }
    return sendMainMenu(env, msg.chat.id);
  }

  // عکس / فایل
  if (msg.photo || msg.document) {
    if (st.step === "wallet_receipt") return handleWalletReceipt(c,msg);
    if (st.step === "receipt") return handleReceipt(c, msg);
    const fix = await latestNeedsFix(env, uid);
    if (fix) { st.step = "receipt"; st.order_id = fix.id; return handleReceipt(c, msg); }
    return sendMsg(env, msg.chat.id, "برای خرید از منوی زیر اقدام کنید 👇", mainMenuKb(env));
  }
}

async function onCallback(c, cq) {
  const { env, st } = c;
  const d = cq.data || "";
  if (!cq.message || cq.message.chat.type !== "private" || cq.message.chat.id !== cq.from.id) return;
  const uid = cq.from.id;
  const chat_id = cq.message.chat.id;
  const isAdmin = env.admins.has(uid);
  const answer = (text, alert = false) => { if (cq._answered) return; cq._answered = true; return tg(env, "answerCallbackQuery", { callback_query_id: cq.id, text, show_alert: alert }); };
  const del = () => tg(env, "deleteMessage", { chat_id, message_id: cq.message.message_id });

  if (d.startsWith("topup:")) {
    if(!isAdmin) return answer("دسترسی ندارید.",true);
    const [,action,idText]=d.split(":"), id=Number(idText);
    if(!Number.isSafeInteger(id)||id<=0||!["approve","reject"].includes(action)) return answer("درخواست نامعتبر است.",true);
    const next=action === "approve" ? "credited" : "rejected";
    let changed;
    try { changed=await run(env,"UPDATE wallet_topups SET status=?,reviewed_by=? WHERE id=? AND status='pending'",next,uid,id); }
    catch { return answer("ثبت شارژ انجام نشد؛ موجودی را بررسی کنید.",true); }
    if(!changed.meta.changes) return answer("این فیش قبلاً بررسی شده است.",true);
    const topup=await first(env,"SELECT * FROM wallet_topups WHERE id=?",id);
    try { await tg(env,"editMessageReplyMarkup",{chat_id,message_id:cq.message.message_id,reply_markup:{inline_keyboard:[]}}); } catch {}
    try { await sendMsg(env,topup.user_id,action === "approve" ? `✅ کیف پول شما ${toman(topup.amount)} شارژ شد.` : "❌ درخواست شارژ کیف پول تأیید نشد؛ با پشتیبانی تماس بگیرید.",backKb()); } catch {}
    return answer(action === "approve" ? "شارژ ثبت شد." : "درخواست رد شد.",true);
  }

  if (d.startsWith("admin:")) {
    if (!isAdmin) return answer("دسترسی ندارید.", true);
    return handleAdminUICallback(c, cq, answer);
  }

  if (d === "check_join") {
    if (await isMember(env, uid)) {
      await answer("عضویت شما تایید شد ✅");
      await del();
      return sendMainMenu(env, chat_id, startGreeting(env,cq.from.first_name));
    }
    return answer("❌ هنوز عضو کانال نشده‌اید! اگر همین الان عضو شدید، چند ثانیه صبر کنید و دوباره بزنید.", true);
  }

  // --- بخش ادمین: تایید یا ایراد فیش
  if (d.startsWith("approve_") || d.startsWith("issue_")) {
    if (!isAdmin) return answer("دسترسی ندارید!", true);
    const orderId = Number(d.split("_")[1]);
    const order = await getOrder(env, orderId);
    if (!order) return answer("سفارش یافت نشد.", true);
    if (order.status !== "pending") return answer("این سفارش قبلا بررسی شده است (وضعیت: " + order.status + ").", true);

    if (d.startsWith("issue_")) {
      await answer("");
      st.admin_order = orderId; st.admin_step = "issue_message";
      await tg(env, "editMessageReplyMarkup", { chat_id, message_id: cq.message.message_id, reply_markup: { inline_keyboard: [] } });
      return sendMsg(env, chat_id,
        `⚠️ سفارش <code>${orderId}</code>.\n\nپیامی که می‌خواهید برای کاربر ارسال شود را بنویسید (مثلا: مبلغ واریزی اشتباه است):`);
    }

    // تایید: سفارش را اتمیک می‌گیریم تا دو ادمین هم‌زمان دوبار نسازند
    if (!(await claimOrder(env, orderId, "pending", "processing"))) {
      return answer("این سفارش همین الان توسط ادمین دیگری بررسی شد.", true);
    }
    // Telegram delivery errors must not strand the claimed order or change a
    // successfully provisioned order back to retryable.
    try { await answer("⏳ در حال ساخت سرویس در پنل..."); } catch {}
    try { await tg(env, "editMessageReplyMarkup", { chat_id, message_id: cq.message.message_id, reply_markup: { inline_keyboard: [] } }); } catch {}
    let dispatched = false, res;
    try {
      res = await fulfillOrder(env, order, () => { dispatched = true; });
      await setOrderStatus(env, orderId, "approved");
    } catch (e) {
      // A failure before the write is sent is safe to retry. After dispatch,
      // even success:false can be a partial write: never replay automatically.
      await setOrderStatus(env, orderId, dispatched ? "needs_review" : "pending");
      console.log("fulfill error", e.message);
      if (dispatched) return sendMsg(env, chat_id, `❌ نتیجهٔ سفارش <code>${orderId}</code> نامشخص است: <code>${esc(e.message)}</code>\nقبل از هر اقدام مجدد، سفارش و مشتری را در پنل و دیتابیس بررسی کنید.`);
      return sendMsg(env, chat_id, `❌ درخواست سفارش <code>${orderId}</code> هنوز به پنل ارسال نشده است: <code>${esc(e.message)}</code>\nبعد از رفع مشکل می‌توانید دوباره تلاش کنید.`, kb([[btn("🔁 تلاش مجدد", `approve_${orderId}`)]]));
    }
    try {
      const header = res.renewed ? "✅ سرویس شما تمدید شد!" : "✅ سرویس شما ساخته شد!";
      const delivered = await sendServiceCard(env, order.user_id, res.name, order.volume, order.duration, res.link, null, mainMenuKb(env), header);
      if (!delivered.ok) throw new Error("Telegram delivery failed");
      await sendMsg(env, order.user_id, "این سرویس در بخش «🗂 سرویس های من» هم قابل مشاهده است.");
      await sendMsg(env, chat_id, `✅ سفارش <code>${orderId}</code> تایید و ${res.renewed ? "تمدید" : "ساخته"} شد و برای کاربر ارسال شد.` + (res.warn ? `\n\n${esc(res.warn)}` : ""));
      for (const a of env.admins) if (a !== uid) await sendMsg(env, a, `ℹ️ سفارش <code>${orderId}</code> توسط ${esc(cq.from.first_name || uid)} تایید شد.`);
    } catch (e) {
      console.log("order notification failed", e.message);
      try { await sendMsg(env, chat_id, `⚠️ سفارش <code>${orderId}</code> انجام شد، ولی ارسال پیام تأیید نشد. سرویس را در ربات/پنل بررسی و به مشتری اطلاع دهید؛ دوباره سفارش را اجرا نکنید.`); } catch {}
    }
    return;
  }

  if (!isAdmin && d !== "main_menu" && !(await isMember(env, uid))) return answer("ابتدا در کانال عضو شوید و /start را بزنید.", true);

  if (d === "wallet:home") {
    delete st.step; delete st.order_id; delete st.wallet_topup_id;
    const balance=await walletBalance(env,uid);
    return render(env,cq,`💰 <b>کیف پول</b>\nموجودی: <b>${toman(balance)}</b>`,kb([[btn("➕ شارژ کیف پول", "wallet:topup")],[btn("📋 گردش کیف پول", "wallet:history")],[btn("🔙 بازگشت", "main_menu")]]));
  }
  if (d === "wallet:history") {
    const rows=await all(env,"SELECT delta,reason,created_at FROM wallet_entries WHERE user_id=? ORDER BY id DESC LIMIT 10",uid);
    const text=rows.length ? rows.map(x=>`${Number(x.delta)>0?"➕":"➖"} ${toman(Math.abs(Number(x.delta)))} — ${esc(walletReason(x.reason))}`).join("\n") : "تراکنشی ثبت نشده است.";
    return render(env,cq,`📋 <b>۱۰ تراکنش اخیر</b>\n\n${text}`,backKb("wallet:home"));
  }
  if (d === "wallet:topup") { st.step="wallet_amount"; return render(env,cq,"مبلغ شارژ را به تومان و با عدد انگلیسی بفرستید.",backKb("wallet:home")); }
  if (d.startsWith("wallet:pay:")) {
    const id=Number(d.slice("wallet:pay:".length)); if(!Number.isSafeInteger(id)||id<=0) return answer("سفارش نامعتبر است.",true);
    const order=await getOrder(env,id); if(!order || Number(order.user_id)!==uid || order.status!=="draft") return answer("سفارش قابل پرداخت نیست.",true);
    if(parseMoney(order.final_price,true)===null) return answer("مبلغ سفارش معتبر نیست؛ با پشتیبانی تماس بگیرید.",true);
    try { if(!(await reserveWalletPayment(env,id,uid))) return answer("سفارش قابل پرداخت نیست یا یک پرداخت دیگر نیاز به بررسی دارد.",true); }
    catch(e) { return answer(String(e.message).includes("WALLET_INSUFFICIENT") ? "موجودی کیف پول کافی نیست." : "نتیجهٔ پرداخت نامشخص است؛ موجودی و سفارش را بررسی کنید یا با پشتیبانی تماس بگیرید.",true); }
    delete st.step; delete st.order_id;
    try { await answer("⏳ پرداخت ثبت شد؛ سرویس در حال پردازش است."); } catch {}
    let sent=false, result;
    try {
      result=await fulfillOrder(env,order,()=>{sent=true;});
      if(!(await completeWalletPayment(env,id,"settled"))) throw new Error("PAYMENT_SETTLEMENT_UNCONFIRMED");
    } catch(e) {
      let refunded=false;
      try { refunded=await completeWalletPayment(env,id,"refunded",!sent); } catch {}
      const state=await first(env,"SELECT state FROM wallet_payments WHERE order_id=?",id);
      if(state?.state === "settled") { try { await sendMsg(env,chat_id,"✅ پرداخت ثبت شده است؛ سرویس را در بخش سرویس‌های من بررسی کنید.",mainMenuKb(env)); } catch {} return; }
      for(const admin of env.admins) { try { await sendMsg(env,admin,`⚠️ سفارش کیف پول <code>${id}</code> نیاز به بررسی دارد. درخواست پنل ${sent ? "ارسال شده" : "ارسال نشده"}؛ پیش از اقدام دوباره وضعیت را بررسی کنید.`); } catch {} }
      return sendMsg(env,chat_id,refunded || state?.state === "refunded" ? (sent ? "⚠️ نتیجهٔ پنل نامشخص است و مبلغ به کیف پول برگشت. تا بررسی پشتیبانی پرداخت جدید با کیف پول موقتاً متوقف است." : "❌ درخواست به پنل ارسال نشد و مبلغ به کیف پول برگشت؛ می‌توانید سفارش جدید ثبت کنید.") : "⚠️ نتیجهٔ پرداخت نیاز به بررسی دارد؛ با پشتیبانی تماس بگیرید.",mainMenuKb(env));
    }
    try {
      const delivered=await sendServiceCard(env,uid,result.name,order.volume,order.duration,result.link,null,mainMenuKb(env),result.renewed?"✅ سرویس تمدید شد!":"✅ سرویس ساخته شد!");
      if(!delivered.ok) throw new Error('delivery failed');
    } catch { for(const admin of env.admins) {try {await sendMsg(env,admin,`⚠️ سفارش کیف پول <code>${id}</code> انجام شد ولی پیام تحویل ارسال نشد.`);}catch{}} }
    return;
  }

  if (d === "main_menu") { delete st.step; delete st.admin_ui_step; delete st.admin_ui_key; delete st.admin_step; delete st.admin_order; delete st.admin_wallet_change; delete st.wallet_topup_id; await del(); return sendMainMenu(env, chat_id); }

  // --- سرویس تست (هر کاربر فقط یک‌بار)
  if (d === "test_service") {
    if (env.cfg.test_enabled !== "1") return answer("سرویس تست فعلا غیرفعال است.", true);
    const token = `processing|${hex(12)}|${now()}`;
    const claimed = await run(env, "INSERT OR IGNORE INTO test_claims(user_id,created_at) VALUES(?,?)", uid, token);
    if (!claimed.meta.changes) {
      const existing = await first(env, "SELECT created_at FROM test_claims WHERE user_id=?", uid);
      const state = String(existing?.created_at || "");
      if (state.startsWith("processing|")) return answer("درخواست تست شما در حال بررسی است؛ برای نتیجه با پشتیبانی تماس بگیرید.", true);
      if (state.startsWith("review|")) return answer("نتیجهٔ درخواست قبلی نیاز به بررسی دارد؛ با پشتیبانی تماس بگیرید.", true);
      return answer("شما قبلا سرویس تست دریافت کرده‌اید.", true);
    }
    let dispatched = false, p;
    const mb = num(env, "test_mb"), days = num(env, "test_days"), label = `${mb} مگابایت`;
    try {
      p = await panelCreate(env, { name: "test", bytes: mb * MB, days, attemptKey: `test:${uid}:${hex(8)}`, onDispatch: () => { dispatched = true; } });
      await insertService(env, { ...p, user_id: uid, name: "Test", volume: label, duration: `${days} روزه` });
      await run(env, "UPDATE test_claims SET created_at=? WHERE user_id=? AND created_at=?", `done|${now()}`, uid, token);
    } catch (e) {
      if (dispatched) await run(env, "UPDATE test_claims SET created_at=? WHERE user_id=? AND created_at=?", `review|${now()}`, uid, token);
      else await run(env, "DELETE FROM test_claims WHERE user_id=? AND created_at=?", uid, token);
      console.log("test error", e.message);
      for (const a of env.admins) { try { await sendMsg(env, a, `⚠️ درخواست تست <code>${uid}</code> ${dispatched ? "نیازمند بررسی نتیجه در پنل" : "پیش از ارسال به پنل متوقف"} شد: <code>${esc(e.message)}</code>`); } catch {} }
      return sendMsg(env, chat_id, dispatched ? "⚠️ نتیجهٔ ساخت تست نامشخص است؛ با پشتیبانی تماس بگیرید." : "❌ درخواست هنوز به پنل ارسال نشد. پس از رفع مشکل می‌توانید دوباره تلاش کنید.", backKb());
    }
    // A delivery failure must not release the claim or create a second service.
    try {
      await del();
      const delivered = await sendServiceCard(env, chat_id, "Test", label, `${days} روزه`, p.link, null, mainMenuKb(env), "🎁 سرویس تست شما ساخته شد!");
      if (!delivered.ok) throw new Error("Telegram delivery failed");
    } catch (e) {
      console.log("test notification failed", e.message);
      for (const a of env.admins) { try { await sendMsg(env, a, `⚠️ سرویس تست <code>${uid}</code> ساخته شد ولی ارسال پیام تأیید نشد؛ سرویس را بررسی کنید.`); } catch {} }
    }
    return;
  }

  if (d === "my_services") {
    const list = await getUserServices(env, uid);
    if (!list.length) return render(env, cq, "🗂 <b>سرویس های من</b>\n\n❌ سرویسی ندارید !", backKb());
    const rows = list.map((s) => [btn(`🔹 ${s.name} (${s.volume})`, `svc_${s.id}`)]);
    rows.push([btn("🔙 بازگشت", "main_menu")]);
    return render(env, cq, "🗂 <b>سرویس های من</b>\n\nسرویس مورد نظر را انتخاب کنید:", kb(rows));
  }

  if (d.startsWith("cfgmanage:") || d.startsWith("cfgset:")) {
    const parts=d.split(":"), id=Number(parts[1]);
    if (!Number.isSafeInteger(id) || id <= 0) return answer("سرویس نامعتبر است.", true);
    const svc=await getService(env,id);
    if (!svc || Number(svc.user_id)!==uid) return answer("سرویس یافت نشد.", true);
    if (parts[0] === "cfgmanage") {
      const page=Number(parts[2]); if (!Number.isSafeInteger(page) || page < 0) return answer("صفحه نامعتبر است.",true);
      let state; try { state=await panelMemberships(env,svc); } catch(e) { return sendMsg(env,chat_id,e.message === "سرویس در پنل پیدا نشد." ? "❌ سرویس در پنل متصل فعلی یافت نشد." : "❌ دریافت وضعیت از پنل ناموفق بود. دوباره تلاش کنید.",backKb(`svc_${id}`)); }
      if (page > Math.max(0,Math.ceil(state.list.length/10)-1)) return answer("صفحه پیدا نشد.",true);
      const rows=[];
      for (const m of state.list.slice(page*10,(page+1)*10)) {
        if (!Number.isSafeInteger(m.id) || m.id<=0) continue;
        const name=m.name.slice(0,45);
        if (m.enabled===null) rows.push([btn(`⚠️ ${name} — نامشخص`,`cfgnoop:${id}`)]);
        else if (!state.accountEnabled) rows.push([btn(`⏸ ${name} — حساب خاموش است`,`cfgnoop:${id}`)]);
        else rows.push([btn(`${m.enabled ? "⛔ غیرفعال‌سازی" : "✅ فعال‌سازی"} ${name}`,`cfgset:${id}:${m.id}:${m.enabled ? 0 : 1}`)]);
      }
      const nav=[]; if(page>0) nav.push(btn("◀️ قبلی",`cfgmanage:${id}:${page-1}`)); if((page+1)*10<state.list.length) nav.push(btn("بعدی ▶️",`cfgmanage:${id}:${page+1}`)); if(nav.length) rows.push(nav);
      rows.push([btn("🔙 بازگشت",`svc_${id}`)]);
      const text = state.accountEnabled ? "🔘 <b>وضعیت کانفیگ‌ها</b>\nدکمهٔ هر ورودی فقط دسترسی همین سرویس روی همان ورودی را تغییر می‌دهد. اگر چند لینک به یک ورودی وابسته باشند، همگی با هم تغییر می‌کنند." : "⚠️ حساب در پنل غیرفعال است؛ برای بررسی انقضا یا تمدید با پشتیبانی تماس بگیرید.";
      return render(env,cq,text,kb(rows));
    }
    const inboundId=Number(parts[2]), desired=parts[3];
    if (!Number.isSafeInteger(inboundId) || inboundId<=0 || !["0","1"].includes(desired)) return answer("درخواست نامعتبر است.",true);
    const lock=await run(env,"INSERT OR IGNORE INTO service_operations(service_id,kind,created_at) VALUES(?,?,?)",id,"membership",now());
    if (!lock.meta.changes) return answer("عملیات دیگری روی این سرویس در جریان است؛ بعداً دوباره تلاش کنید.",true);
    try {
      const before=await panelMemberships(env,svc), member=before.list.find(m=>m.id===inboundId);
      if (!before.accountEnabled || !member || member.enabled===null) return sendMsg(env,chat_id,"⚠️ وضعیت قابل تغییر نیست؛ حساب و اشتراک را با پشتیبانی بررسی کنید.",backKb(`cfgmanage:${id}:0`));
      const wanted=desired==="1";
      if (member.enabled === wanted) return sendMsg(env,chat_id,"این وضعیت قبلاً تغییر کرده است؛ فهرست را دوباره ببینید.",backKb(`cfgmanage:${id}:0`));
      const response=await panelCall(env,"POST",`/panel/api/inbounds/${inboundId}/setMembershipEnable/${encodeURIComponent(svc.panel_email)}`,new URLSearchParams({enable:String(wanted)}));
      if (!response.success) throw new Error("پنل تغییر را رد کرد.");
      const after=await panelMemberships(env,svc), changed=after.list.find(m=>m.id===inboundId);
      if (!changed || changed.enabled!==wanted) return sendMsg(env,chat_id,"⚠️ تغییر در پنل تأیید نشد یا اشتراک/نماینده محدود است؛ با پشتیبانی تماس بگیرید.",backKb(`cfgmanage:${id}:0`));
      const note = wanted ? (changed.inboundEnabled ? "\nبرقراری اتصال به اعتبار، حجم و انقضای اشتراک هم بستگی دارد." : "\nورودی اصلی در پنل خاموش است؛ ممکن است اتصال برقرار نشود.") : "";
      return sendMsg(env,chat_id,`✅ سوییچ کانفیگ «${esc(changed.name)}» ${wanted ? "فعال" : "غیرفعال"} شد.${note}`,backKb(`cfgmanage:${id}:0`));
    } catch {
      return sendMsg(env,chat_id,"⚠️ نتیجهٔ تغییر وضعیت مشخص نیست؛ ابتدا وضعیت را دوباره بررسی کنید یا با پشتیبانی تماس بگیرید.",backKb(`cfgmanage:${id}:0`));
    } finally { await run(env,"DELETE FROM service_operations WHERE service_id=?",id); }
  }
  if (d.startsWith("cfgnoop:")) return answer("وضعیت دقیق مشخص نیست یا حساب غیرفعال است؛ با پشتیبانی تماس بگیرید.",true);

  if (d.startsWith("cfglist:") || d.startsWith("cfgtxt:") || d.startsWith("cfgqr:")) {
    const parts = d.split(":"), id = Number(parts[1]);
    if (!Number.isSafeInteger(id) || id <= 0) return answer("سرویس نامعتبر است.", true);
    const svc = await getService(env, id);
    if (!svc || Number(svc.user_id) !== uid) return answer("سرویس یافت نشد.", true);
    let links;
    let actualSvc=svc;
    try {
      if (svc.panel_email) {
        const account=await panelServiceAccount(env,svc);
        if (!account) return sendMsg(env, chat_id, "❌ سرویس در پنل متصل فعلی یافت نشد.", backKb("my_services"));
        actualSvc={...svc,sub_link:currentServiceLink(env,svc,account)};
      }
      links = await subscriptionLinks(actualSvc);
    }
    catch (e) { return sendMsg(env, chat_id, `❌ دریافت کانفیگ‌ها ناموفق بود: ${esc(e.message)}`, backKb(`svc_${id}`)); }
    if (parts[0] === "cfglist") {
      const page = Number(parts[2]);
      if (!Number.isSafeInteger(page) || page < 0 || page > Math.max(0, Math.ceil(links.length/10)-1)) return answer("صفحه یافت نشد.", true);
      const rows=[];
      for (let i=page*10; i<Math.min(links.length,(page+1)*10); i++) {
        const fingerprint=await linkFingerprint(links[i]), title=configName(links[i],i);
        rows.push([btn(`📋 ${title}`, `cfgtxt:${id}:${i}:${fingerprint}`),btn("📷 QR", `cfgqr:${id}:${i}:${fingerprint}`)]);
      }
      const nav=[]; if(page>0) nav.push(btn("◀️ قبلی",`cfglist:${id}:${page-1}`));
      if((page+1)*10<links.length) nav.push(btn("بعدی ▶️",`cfglist:${id}:${page+1}`)); if(nav.length) rows.push(nav);
      const subUrl = (()=>{try {const u=new URL(actualSvc.sub_link); return u.protocol === 'https:' ? u.toString() : null;} catch{return null;}})();
      if(subUrl) rows.push([urlBtn("🌐 صفحهٔ اشتراک و فایل‌ها",subUrl)]);
      rows.push([btn("🔙 بازگشت",`svc_${id}`)]);
      const message = links.length ? `🔗 <b>کانفیگ‌های قابل استفاده</b> (${links.length})\nبرای دریافت لینک یا QR هر مورد، دکمه‌اش را بزنید.` : "کانفیگ تکی فعالی در خروجی ساب موجود نیست. برای فایل‌های تنظیمات و جزئیات، صفحهٔ اشتراک را بررسی کنید.";
      return render(env,cq,message,kb(rows));
    }
    const index=Number(parts[2]), fingerprint=parts[3];
    if (!Number.isSafeInteger(index) || index < 0 || index >= links.length || fingerprint !== await linkFingerprint(links[index])) return answer("فهرست کانفیگ‌ها تغییر کرده؛ دوباره باز کنید.", true);
    const link=links[index], title=configName(link,index);
    if (parts[0] === "cfgtxt") {
      if (link.length > 3500) return sendMsg(env,chat_id,"کانفیگ طولانی است؛ آن را از صفحهٔ اشتراک کپی کنید.",backKb(`cfglist:${id}:0`));
      return sendMsg(env,chat_id,`🔗 <b>${esc(title)}</b>\n\n<code>${esc(link)}</code>`,backKb(`cfglist:${id}:0`));
    }
    try { await sendConfigQR(env,chat_id,link,title,id); }
    catch (e) { return sendMsg(env,chat_id,`❌ ${esc(e.message)}`,backKb(`cfglist:${id}:0`)); }
    return;
  }

  if (d.startsWith("svc_")) {
    const id = Number(d.split("_")[1]);
    const s = await getService(env, id);
    if (!s || s.user_id !== uid) return answer("سرویس یافت نشد!", true);
    let account;
    try { account = await panelServiceAccount(env, s); }
    catch { return sendMsg(env, chat_id, "⚠️ بررسی سرویس در پنل ناموفق بود؛ بعداً دوباره تلاش کنید.", backKb("my_services")); }
    if (!account) return render(env, cq, "❌ سرویس در پنل متصل فعلی یافت نشد.", backKb("my_services"));
    await del();
    return sendServiceCard(env, chat_id, s.name, s.volume, s.duration, currentServiceLink(env,s,account), s.config_link, serviceKb(id), liveServiceDetails(account));
  }

  if (d.startsWith("delsvc_")) {
    const id = Number(d.split("_")[1]);
    const s = await getService(env, id);
    if (!s || s.user_id !== uid) return answer("سرویس یافت نشد!", true);
    try { if (!(await panelServiceAccount(env,s))) return sendMsg(env,chat_id,"❌ سرویس در پنل متصل فعلی یافت نشد.",backKb("my_services")); }
    catch { return sendMsg(env,chat_id,"⚠️ بررسی سرویس در پنل ناموفق بود؛ دوباره تلاش کنید.",backKb("my_services")); }
    return render(env, cq, `⚠️ آیا مطمئنی می‌خوای سرویس «${esc(s.name)}» رو حذف کنی؟\n\nاین کار قابل بازگشت نیست و سرویس از پنل هم پاک می‌شود.`,
      kb([[btn("✅ بله، حذف کن", `delyes_${id}`)], [btn("❌ انصراف", `svc_${id}`)]]));
  }

  if (d.startsWith("delyes_")) {
    const id = Number(d.split("_")[1]);
    const s = await getService(env, id);
    if (!s || s.user_id !== uid) return answer("سرویس یافت نشد!", true);
    const lock = await run(env, "INSERT OR IGNORE INTO service_operations(service_id,kind,created_at) VALUES(?,?,?)", id, "delete", now());
    if (!lock.meta.changes) return answer("این سرویس در حال بررسی یا حذف است؛ با پشتیبانی تماس بگیرید.", true);
    try {
      await panelDelete(env, s);
      await run(env, "DELETE FROM services WHERE id=? AND user_id=?", id, uid);
    } catch (e) {
      for (const a of env.admins) await sendMsg(env, a, `⚠️ حذف سرویس ${id} نیاز به بررسی دارد: ${esc(e.message)}`);
      return sendMsg(env, chat_id, "حذف سرویس کامل نشد؛ لطفاً با پشتیبانی تماس بگیرید.", backKb());
    } finally { await run(env, "DELETE FROM service_operations WHERE service_id=?", id); }
    const list = await getUserServices(env, uid);
    if (!list.length) return render(env, cq, "✅ سرویس حذف شد.\n\n🗂 <b>سرویس های من</b>\n\n❌ سرویسی ندارید !", backKb());
    const rows = list.map((x) => [btn(`🔹 ${x.name} (${x.volume})`, `svc_${x.id}`)]);
    rows.push([btn("🔙 بازگشت", "main_menu")]);
    return render(env, cq, "✅ سرویس حذف شد.\n\n🗂 <b>سرویس های من</b>", kb(rows));
  }

  if (d === "buy") {
    delete st.renew_service_id;
    return render(env, cq,
      "🛍 <b>لطفا سرویسی که میخواهید خریداری کنید را انتخاب کنید!</b>\n\n" +
      `⏳ مدت زمان همه بسته‌ها: ${durationText(num(env, "duration_days"))}\n👥 تعداد کاربر: نامحدود\n\nبسته مورد نظر را انتخاب کنید:`,
      await plansKb(env, "plan"));
  }

  if (d.startsWith("plan_")) {
    const plan = await getPlan(env, d.slice(5));
    if (!plan) return;
    st.plan_key = plan.key; st.step = "service_name";
    return render(env, cq,
      `📦 بسته انتخابی: <b>${esc(planLabel(plan))}</b> — ${toman(plan.price)}\n\n` +
      "✏️ حالا یک <b>نام</b> برای سرویس خود بفرستید.\n\n⚠️ <b>قوانین نام‌گذاری:</b>\n" +
      "• فقط حروف انگلیسی (A-Z)\n• بدون فاصله\n• استفاده از عدد و علامت (!,@,_,- و ...) ممنوع\n\n" +
      "<i>مثال درست: Amir  یا  Mobile</i>", backKb("buy"));
  }

  if (d.startsWith("renew_")) {
    const id = Number(d.split("_")[1]);
    const s = await getService(env, id);
    if (!s || s.user_id !== uid) return answer("سرویس یافت نشد!", true);
    try { if (!(await panelServiceAccount(env,s))) return sendMsg(env,chat_id,"❌ سرویس در پنل متصل فعلی یافت نشد.",backKb("my_services")); }
    catch { return sendMsg(env,chat_id,"⚠️ بررسی سرویس در پنل ناموفق بود؛ دوباره تلاش کنید.",backKb("my_services")); }
    st.renew_service_id = id;
    return render(env, cq, `🔄 <b>تمدید سرویس «${esc(s.name)}»</b>\n\nبسته مورد نظر برای تمدید را انتخاب کنید:`,
      await plansKb(env, "rplan"));
  }

  if (d.startsWith("rplan_")) {
    const plan = await getPlan(env, d.slice(6));
    const s = st.renew_service_id ? await getService(env, st.renew_service_id) : null;
    if (!plan || !s || s.user_id !== uid) return answer("خطا در پردازش، دوباره تلاش کنید.", true);
    try { if (!(await panelServiceAccount(env,s))) return sendMsg(env,chat_id,"❌ سرویس در پنل متصل فعلی یافت نشد.",backKb("my_services")); }
    catch { return sendMsg(env,chat_id,"⚠️ بررسی سرویس در پنل ناموفق بود؛ دوباره تلاش کنید.",backKb("my_services")); }
    if (Math.round(plan.gb * GB) !== Number(s.volume_bytes)) return answer("تمدید فقط با همان حجم قبلی امکان‌پذیر است؛ برای تغییر بسته با پشتیبانی تماس بگیرید.", true);
    return createInvoice(c, cq.from, chat_id, s.name, plan, s.id, cq);
  }

  if (d === "invite_code") {
    return render(env, cq,
      `🎁 <b>کد دعوت شما</b>\n\n<code>${uid}</code>\n\nاین کد (آیدی عددی شما) را برای دوستانتان بفرستید.\n` +
      `هر کسی این کد را در ربات وارد و ثبت کند، <b>${toman(num(env, "referral_discount"))} تخفیف</b> برای یک خرید به حساب شما اضافه می‌شود.\n\n💡 روی کد بزنید تا کپی شود.`,
      backKb());
  }

  if (d === "enter_invite") {
    st.step = "invite_code_input";
    return render(env, cq, "➕ <b>ثبت کد دعوت</b>\n\nآیدی عددی (کد دعوت) شخصی که معرفتان کرده را بفرستید:", backKb());
  }

  if (d === "support") {
    const sid = env.cfg.support_id;
    return render(env, cq, `🆘 <b>پشتیبانی</b>\n\nبرای ارتباط با پشتیبانی به آیدی زیر پیام دهید:\n${esc(sid)}`,
      kb([[urlBtn("💬 ارتباط با پشتیبانی", `https://t.me/${sid.replace(/^@/, "")}`)], [btn("🔙 بازگشت", "main_menu")]]));
  }

  if (d === "apps_menu") {
    const rows = Object.entries(APPS).map(([k, i]) => [btn(`📱 ${i.title}`, `app_${k}`)]);
    rows.push([btn("🔙 بازگشت", "main_menu")]);
    return render(env, cq, "📲 <b>دانلود اپلیکیشن‌ها</b>\n\nاپلیکیشن مناسب دستگاه خود را انتخاب کنید:", kb(rows));
  }

  if (d.startsWith("app_")) {
    const info = APPS[d.slice(4)];
    if (!info) return;
    const rows = info.links.map(([l, u]) => [urlBtn(l, u)]);
    rows.push([btn("🔙 بازگشت", "apps_menu")]);
    return render(env, cq, info.text, kb(rows));
  }
}

// ---------------------------------------------------------- ورودی Worker
async function handleUpdate(baseEnv, update) {
  const from = update.message?.from || update.callback_query?.from;
  if (!from) return;

  // وضعیت کاربر + تنظیمات + ادمین‌ها با یک رفت‌وبرگشت به دیتابیس
  const [stR, setR, admR] = await baseEnv.DB.batch([
    baseEnv.DB.prepare("SELECT data FROM states WHERE user_id=?").bind(from.id),
    baseEnv.DB.prepare("SELECT key,value FROM settings"),
    baseEnv.DB.prepare("SELECT user_id FROM admins"),
  ]);
  const env = Object.create(baseEnv);
  env.cfg = defaults(baseEnv);
  for (const r of setR.results) if (Object.hasOwn(env.cfg, r.key)) env.cfg[r.key] = String(r.value ?? "");
  env.panelStoredIdentity = setR.results.find(r=>r.key === "panel_identity_v1")?.value || "";
  const savedPanel = setR.results.find(r=>r.key === "panel_credentials_v1");
  if (savedPanel) {
    try {
      const p = await decryptPanel(baseEnv.BOT_TOKEN, savedPanel.value);
      env.PANEL_URL = panelURL(p.url); env.PANEL_USER = p.user; env.PANEL_PASS = p.pass;
      if (typeof p.user !== "string" || !p.user || typeof p.pass !== "string" || !p.pass) throw new Error();
    } catch { env.PANEL_URL=""; env.PANEL_USER=""; env.PANEL_PASS=""; }
  }
  env.actor = from.id;
  env.owners = parseIds(baseEnv.ADMIN_ID);
  env.admins = new Set([...env.owners, ...admR.results.map((r) => Number(r.user_id))]);

  const st = stR.results[0] ? JSON.parse(stR.results[0].data) : {};
  const before = JSON.stringify(st);
  const c = { env, st };
  try {
    if (update.callback_query) await onCallback(c, update.callback_query);
    else if (update.message) await onMessage(c, update.message);
  } catch (e) {
    console.log("handler error", e.stack || e.message);
  } finally {
    if (update.callback_query && !update.callback_query._answered) await tg(env, "answerCallbackQuery", { callback_query_id: update.callback_query.id });
    if (JSON.stringify(c.st) !== before) await saveState(env, from.id, c.st);
  }
}

export { groupCredentials, panelCreate, panelRenew, panelDelete, panelAccount, panelServiceAccount, panelMemberships, liveServiceDetails, panelCall, handleUpdate, subscriptionLinks, configName, linkFingerprint };

export default {
  async scheduled(event, env, ctx) {
    ctx.waitUntil((async()=>{
      const recovered=await recoverWalletPayments(env);
      for(const item of recovered) {
        try { await sendMsg(env,item.userId,`⚠️ پرداخت سفارش ${item.orderId} به کیف پول برگشت و برای بررسی ارسال شد.`,backKb()); } catch {}
        for(const admin of parseIds(env.ADMIN_ID)) { try { await sendMsg(env,admin,`⚠️ پرداخت سفارش <code>${item.orderId}</code> پس از قطع عملیات برگشت خورد. احتمال انجام عملیات پنل را بررسی کنید.`); } catch {} }
      }
    })());
  },
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    if (url.pathname === "/webhook" && request.method === "POST") {
      if (!env.WEBHOOK_SECRET || request.headers.get("X-Telegram-Bot-Api-Secret-Token") !== env.WEBHOOK_SECRET) {
        return new Response("forbidden", { status: 403 });
      }
      const update = await request.json();
      if (!Number.isSafeInteger(update.update_id)) return new Response("bad update", { status: 400 });
      const claim = await run(env, "INSERT OR IGNORE INTO telegram_updates(update_id,created_at) VALUES(?,?)", update.update_id, now());
      if (claim.meta.changes) ctx.waitUntil(handleUpdate(env, update));
      return new Response("ok");
    }

    // یک‌بار بعد از دیپلوی باز کن:  https://<worker>/setup?key=WEBHOOK_SECRET
    if (url.pathname === "/setup" && request.method === "POST") {
      if (!env.WEBHOOK_SECRET || request.headers.get("X-Telegram-Bot-Api-Secret-Token") !== env.WEBHOOK_SECRET) return new Response("forbidden", { status: 403 });
      const r = await tg(env, "setWebhook", {
        url: `${url.origin}/webhook`,
        secret_token: env.WEBHOOK_SECRET,
        allowed_updates: ["message", "callback_query"],
        drop_pending_updates: false,
      });
      return new Response(JSON.stringify(r, null, 2), { headers: { "content-type": "application/json" } });
    }

    return new Response("Bot is running");
  },
};
