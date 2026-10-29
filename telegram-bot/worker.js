import { qrPng } from './qr.js';
import { parseMoney, walletBalance, reserveWalletPayment, completeWalletPayment, adjustWallet, recoverWalletPayments } from './wallet.js';
import { encryptPanel, decryptPanel, panelURL, panelAccountKey, probePanel } from './panel-config.js';
import { applyCoupon, expireCouponReservations, cancelDraftOrder, normalizeCoupon, validCouponCode } from './coupons.js';
import { createMessageJob, confirmMessageJob, processMessageJobs, encodeMessageReference, queuedMessagePayload } from './messages.js';

// ربات فروش کانفیگ | NovaRay VPN — Cloudflare Workers + D1 + اتصال به پنل vpn-ui
// ساخت/تمدید/حذف سرویس مستقیم از طریق API پنل (حساب نماینده) انجام می‌شود.

const NAME_RE = /^[A-Za-z]+$/;
function randomServiceName() {
  const letters="abcdefghijklmnopqrstuvwxyz", bytes=crypto.getRandomValues(new Uint8Array(12));
  return "Client"+Array.from(bytes,b=>letters[b%letters.length]).join("");
}
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
const appKeyOK = key => /^[A-Za-z0-9_-]{1,30}$/.test(key);
function parseAppLinks(value) {
  if(value.length>2500)throw Error('مجموع لینک‌ها بیش از حد طولانی است.');
  const lines=value.split('\n').map(x=>x.trim()).filter(Boolean);
  if(!lines.length || lines.length>10)throw Error('از ۱ تا ۱۰ لینک وارد کنید.');
  return lines.map(line=>{const i=line.indexOf('|');if(i<1)throw Error('هر خط باید عنوان | لینک باشد.');const label=line.slice(0,i).trim(),raw=line.slice(i+1).trim();let url;try{url=new URL(raw)}catch{throw Error('لینک معتبر نیست.')}if(!label || label.length>60 || raw.length>512 || !['https:','http:'].includes(url.protocol) || url.username || url.password)throw Error('عنوان یا آدرس لینک معتبر نیست.');return [label,url.toString()];});
}
async function storeApp(env,key,app) {
  await run(env,"INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",`bot_app:${key}`,JSON.stringify(app));
}

// ---------------------------------------------------------- تنظیمات قابل تغییر با /set
// مقدار پیش‌فرض از اینجا (و wrangler.toml) می‌آید؛ هر مقداری که با /set عوض شود در دیتابیس می‌ماند.
const META = {
  referral_discount: ["int", "🎁 مبلغ هدیه دعوت (تومان)"],
  test_enabled: ["bool", "سرویس تست: 1 = فعال، 0 = غیرفعال"],
  test_mb: ["int", "حجم سرویس تست (مگابایت)"],
  test_days: ["int", "مدت سرویس تست (روز)"],
  wallet_topup_min: ["int", "💰 حداقل شارژ کیف پول (تومان)"],
  wallet_topup_max: ["int", "💰 حداکثر شارژ کیف پول (تومان)"],
  alerts_enabled: ["bool", "🔔 هشدار باقی‌مانده اشتراک"],
  alerts_days: ["int", "⏳ هشدار چند روز مانده"],
  alerts_gb: ["number", "📦 هشدار چند گیگ مانده"],
  auto_delete_enabled: ["bool", "🗑 حذف خودکار اشتراک منقضی"],
  auto_delete_days: ["int", "📅 حذف چند روز پس از انقضا"],
  card_number: ["text", "شماره کارت"],
  card_holder: ["text", "نام صاحب کارت"],
  support_id: ["text", "آیدی پشتیبانی (با @)"],
  channel_username: ["text", "یوزرنیم کانال عضویت اجباری (با @)"],
  channel_link: ["text", "لینک کانال"],
  main_menu_image_url: ["text", "لینک عکس منوی اصلی (برای خالی کردن: -)"],
  start_text: ["text", "متن پیام شروع"],
  sub_base_url: ["text", "آدرس پایه لینک ساب، مثلا https://example.com:8000/sub/"],
};

const defaults = (env) => ({
  referral_discount: "50000",
  name_mode: "custom",
  duration_days: "30",
  test_enabled: "1",
  test_mb: "500",
  test_days: "1",
  wallet_topup_min: "1",
  wallet_topup_max: "0",
  alerts_enabled: "0",
  alerts_days: "1",
  alerts_gb: "1",
  auto_delete_enabled: "0",
  auto_delete_days: "7",
  test_category_id: "",
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
function topupLimitError(env,key,value) {
  if(!["wallet_topup_min","wallet_topup_max"].includes(key))return "";
  const n=parseMoney(value,key === "wallet_topup_max");
  if(n===null)return key === "wallet_topup_min" ? "حداقل باید یک عدد مثبت معتبر به تومان باشد." : "حداکثر باید عدد معتبر به تومان باشد؛ ۰ یعنی بدون سقف تنظیم‌شده.";
  const min=key === "wallet_topup_min" ? n : Number(env.cfg.wallet_topup_min);
  const max=key === "wallet_topup_max" ? n : Number(env.cfg.wallet_topup_max);
  return max>0 && min>max ? "حداقل شارژ نمی‌تواند از حداکثر بیشتر باشد." : "";
}
function policySettingError(key,value) {
  if(key==='alerts_gb' && (!/^\d+(?:\.\d{1,3})?$/.test(value) || Number(value)>1000000))return 'حجم را با عدد انگلیسی از ۰ تا ۱۰۰۰۰۰۰ گیگ وارد کنید؛ ۰ هشدار حجم را خاموش می‌کند.';
  if(['alerts_days','auto_delete_days'].includes(key) && (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value)) || Number(value)>36500))return 'تعداد روز باید عددی بین ۰ تا ۳۶۵۰۰ باشد.';
  return '';
}
function topupLimitsText(env) {
  const min=num(env,"wallet_topup_min"),max=num(env,"wallet_topup_max");
  return `حداقل: ${toman(min)}\nحداکثر: ${max>0 ? toman(max) : "بدون سقف تنظیم‌شده"}`;
}
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
    ...(env.policySignal ? {signal:AbortSignal.any([env.policySignal,AbortSignal.timeout(15000)])} : {}),
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

const PLAN_SELECT = "SELECT p.*,s.category_id,s.days,c.name AS category_name,c.group_ids,c.panel_identity,c.active AS category_active FROM plans p LEFT JOIN bot_plan_specs s ON s.plan_key=p.key LEFT JOIN bot_categories c ON c.id=s.category_id";
const getPlans = (env) => all(env, `${PLAN_SELECT} ORDER BY p.sort,p.price`);
const getPlan = (env, key) => first(env, `${PLAN_SELECT} WHERE p.key=?`, key);
const getCategory = (env,id) => first(env,"SELECT * FROM bot_categories WHERE id=?",id);
async function currentPanelIdentity(env) {
  if (env.panelStoredIdentity) return env.panelStoredIdentity;
  if (!env.PANEL_URL || !env.PANEL_USER) throw new Error("ابتدا اتصال پنل را تنظیم کنید");
  return panelAccountKey(env.PANEL_URL,env.PANEL_USER);
}
async function assertCategory(env,category) {
  if (!category || Number(category.active ?? category.category_active)!==1 || !parseIds(category.group_ids).length) throw new Error("دسته‌بندی بسته تنظیم نشده یا غیرفعال است");
  const identity=await currentPanelIdentity(env);
  if(category.panel_identity && category.panel_identity!==identity) throw new Error("دسته‌بندی متعلق به اتصال قبلی پنل است؛ دوباره تنظیم کنید");
  return identity;
}
const getOrder = (env, id) => first(env, "SELECT * FROM orders WHERE id=?", id);
const setOrderStatus = (env, id, s) => run(env, "UPDATE orders SET status=? WHERE id=?", s, id);
const claimOrder = async (env, id, from, to) =>
  (await run(env, "UPDATE orders SET status=? WHERE id=? AND status=? AND (? NOT IN ('draft','needs_fix') OR NOT EXISTS(SELECT 1 FROM bot_coupon_uses u WHERE u.order_id=orders.id AND u.state='reserved' AND u.expires_ms<=?))", to, id, from, from,Date.now())).meta.changes > 0;
const getService = (env, id) => first(env, "SELECT * FROM services WHERE id=?", id);
const getUserServices = (env, uid) => all(env, "SELECT * FROM services WHERE user_id=? ORDER BY id DESC", uid);
const latestNeedsFix = (env, uid) =>
  first(env, "SELECT * FROM orders WHERE user_id=? AND status='needs_fix' ORDER BY id DESC LIMIT 1", uid);

async function reserveReferral(env,orderId,uid) {
  return run(env,"INSERT INTO bot_order_referrals(order_id,discount_id,user_id,amount,state) SELECT o.id,d.id,d.user_id,MIN(d.amount,o.final_price),'reserved' FROM discounts d JOIN orders o ON o.id=? AND o.user_id=d.user_id WHERE d.user_id=? AND d.used=0 AND d.amount>0 AND o.status='draft' AND o.final_price>0 ORDER BY d.id LIMIT 1",orderId,uid);
}

async function registerReferral(env, referrer, referred) {
  if (await first(env, "SELECT id FROM referrals WHERE referred_id=?", referred)) return false;
  const amount = num(env, "referral_discount");
  if (!Number.isSafeInteger(amount) || amount <= 0) return false;
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
    ...(env.policySignal ? {signal:AbortSignal.any([env.policySignal,AbortSignal.timeout(15000)])} : {}),
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
    const r = await fetch(panelBase(env) + path, { method, headers, body, redirect:"error",...(env.policySignal ? {signal:AbortSignal.any([env.policySignal,AbortSignal.timeout(15000)])} : {}) });
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
}

// Read actual permitted groups/inbounds before constructing credentials.
async function groupCredentials(env, groupIds=env.cfg.panel_group_ids, anchorId=env.cfg.panel_inbound_id) {
  const [groups, inbounds] = await Promise.all([
    panelCall(env, "GET", "/panel/api/clients/groups"),
    panelCall(env, "GET", "/panel/api/clients/assignable"),
  ]);
  if (!groups.success || !inbounds.success || !Array.isArray(groups.obj) || !Array.isArray(inbounds.obj))
    throw new Error("دریافت گروه‌ها یا ورودی‌های مجاز از پنل ناموفق بود");
  const ids = parseIds(groupIds);
  if(!ids.length) throw new Error("گروه بسته تنظیم نشده است");
  const selected = ids.map(id => groups.obj.find(g => Number(g.id) === id));
  if (selected.some(g => !g || !Array.isArray(g.inboundIds))) throw new Error("گروه انتخاب‌شده در دسترس نیست");
  const inboundIds = [...new Set(selected.flatMap(g => g.inboundIds.map(Number)))];
  if (!inboundIds.length || inboundIds.some(id=>!Number.isSafeInteger(id)||id<=0)) throw new Error("ورودی‌های گروه معتبر نیستند");
  const anchor = Number(anchorId) || inboundIds[0];
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
    ...(anchorProtocol === "vmess" ? { security: "auto" } : {}), anchorProtocol, anchorId:anchor };

}

async function panelCreate(env, { name, bytes, days, attemptKey, onDispatch, groupIds=env.cfg.panel_group_ids, anchorId=env.cfg.panel_inbound_id }) {
  assertPanelReady(env);
  if (!Number.isSafeInteger(bytes) || bytes <= 0 || !Number.isSafeInteger(days) || days <= 0 || !Number.isSafeInteger(Date.now() + days * DAY_MS)) throw new Error("حجم یا مدت بسته معتبر نیست");
  const credentials = await groupCredentials(env,groupIds,anchorId);
  const subId = b64url(18);
  const email = name;
  if(await panelAccount(env,email)) throw new Error("این نام قبلاً در پنل ثبت شده است؛ نام دیگری انتخاب کنید");
  if (["wg-c", "awg", "wireguard", "gre", "mtproto"].includes(credentials.anchorProtocol)) credentials.id = email;
  delete credentials.anchorProtocol;
  const inboundId = credentials.anchorId;
  delete credentials.anchorId;
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
  for (const g of parseIds(groupIds)) form.append("groupIds", String(g));
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

async function panelDelete(env, svc, opts = {}) {
  let account = await panelServiceAccount(env, svc);
  if (!account || !Array.isArray(account.memberships) || !account.memberships.length) throw new Error("عضویت‌های سرویس مشخص نیست؛ حذف دستی لازم است");
  if(opts.expectedExpiry !== undefined && Number(account.expiryTime)!==opts.expectedExpiry)throw new Error('تاریخ انقضای سرویس تغییر کرده است');
  const ids = [...new Set(account.memberships.map(m => Number(m.inboundId)))];
  if (ids.some(id => !Number.isSafeInteger(id) || id <= 0) || ids.length !== account.memberships.length) throw new Error("فهرست ورودی‌های سرویس نامعتبر است");
  if (ids.length > 1) {
    const capability = await panelCall(env, "GET", "/panel/api/inbounds/multiDeleteCapability");
    if (!capability.success || capability.obj?.preservesSharedTraffic !== true) throw new Error("پنل باید برای حذف امن چندورودی به‌روزرسانی شود");
  }
  for (let n = 0; n < ids.length; n++) {
    const id = ids[n];
    if(opts.beforeDelete)await opts.beforeDelete();
    const result = await panelCall(env, "POST", `/panel/api/inbounds/${id}/delClientByEmail/${encodeURIComponent(svc.panel_email)}`);
    if (!result.success) throw new Error(`حذف ورودی ${id} رد شد؛ وضعیت سایر ورودی‌ها را بررسی کنید`);
    account = await panelAccount(env, svc.panel_email);
    if (n < ids.length - 1) {
      if(opts.expectedExpiry !== undefined && Number(account?.expiryTime)!==opts.expectedExpiry)throw new Error('تاریخ انقضای سرویس تغییر کرده است');
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

async function plansKb(env,prefix,plans) {
  const rows=plans.map(p=>[btn(`📦 ${planLabel(p)} — ${p.days} روز — ${toman(p.price)}`,`${prefix}_${p.key}`)]);
  rows.push([btn("🔙 بازگشت","main_menu")]);return kb(rows);
}
async function showCustomerCategories(env,cq,mode,page=0) {
  const suffix=mode === "test" ? "testcat" : "buycat";
  const where=mode === "test" ? "" : "AND EXISTS(SELECT 1 FROM bot_plan_specs s JOIN plans p ON p.key=s.plan_key WHERE s.category_id=c.id)";
  const cats=await all(env,`SELECT c.* FROM bot_categories c WHERE c.active=1 ${where} ORDER BY c.id DESC LIMIT 11 OFFSET ?`,page*10);
  const rows=cats.slice(0,10).map(c=>[btn(String(c.name).slice(0,45),`${suffix}:${c.id}:0`)]);
  const nav=[];if(page>0)nav.push(btn("◀️ قبلی",`${mode}cats:${page-1}`));if(cats.length>10)nav.push(btn("بعدی ▶️",`${mode}cats:${page+1}`));if(nav.length)rows.push(nav);rows.push([btn("🔙 بازگشت","main_menu")]);
  return render(env,cq,cats.length ? "🗂 دسته‌بندی را انتخاب کنید:" : "دسته‌بندی قابل استفاده‌ای تنظیم نشده است.",kb(rows));
}
async function categoryMatchesAccount(env,category,account,knownChoices) {
  await assertCategory(env,category);
  const choices=knownChoices || await panelChoices(env);if(!choices || !Array.isArray(account.memberships))return false;
  const ids=parseIds(category.group_ids), groups=ids.map(id=>choices.groups.find(g=>Number(g.id)===id));
  if(groups.some(g=>!g || !Array.isArray(g.inboundIds)))return false;
  const wanted=new Set(groups.flatMap(g=>g.inboundIds.map(Number))), actual=new Set(account.memberships.map(m=>Number(m.inboundId)));
  return wanted.size>0 && wanted.size===actual.size && [...wanted].every(id=>actual.has(id));
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
  L.push("\nروش پرداخت را انتخاب کنید:");
  return L.join("\n");
}
const invoiceKb=(id,hasCoupon=false)=>kb([
  ...(!hasCoupon ? [[btn("🎟 کد تخفیف",`coupon:enter:${id}`)]] : []),
  [btn("💳 کارت به کارت",`card:pay:${id}`)],[btn("💰 پرداخت از کیف پول",`wallet:pay:${id}`)],[btn("🔙 انصراف و منوی اصلی","main_menu")],
]);

async function createInvoice(c, user, chat_id, name, plan, renewId, cq) {
  const { env, st } = c;
  const identity=await assertCategory(env,plan);
  if(!renewId) {assertPanelReady(env);await groupCredentials(env,plan.group_ids,0)}
  const days = Number(plan.days);
  if (!Number.isSafeInteger(days) || days <= 0 || days>36500 || !Number.isSafeInteger(Date.now()+days*DAY_MS)) throw new Error("مدت سرویس مشخص نیست؛ با پشتیبانی تماس بگیرید");
  const r = await run(env,
    "INSERT INTO orders(user_id,username,service_name,volume,price,discount,final_price,duration,renew_service_id,gb,days,created_at,status) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)",
    user.id, user.username || "-", name, planLabel(plan), plan.price, 0, plan.price, durationText(days),
    renewId || null, plan.gb, days, now(), "draft");
  const orderId = r.meta.last_row_id;
  await run(env,"INSERT INTO bot_order_routes(order_id,group_ids,anchor_id,panel_identity) VALUES(?,?,0,?)",orderId,plan.group_ids,identity);
  await reserveReferral(env,orderId,user.id);
  st.step = "payment_choice"; st.order_id = orderId; delete st.renew_service_id;
  const order = await getOrder(env, orderId);
  if (cq) { try { await tg(env, "deleteMessage", { chat_id, message_id: cq.message.message_id }); } catch {} }
  await sendMsg(env, chat_id, invoiceText(env, order), invoiceKb(orderId));
}
async function claimPurchaseChoice(env,st,uid) {
  if(!st.purchase_choice)return false;
  const result=await run(env,"UPDATE settings SET value=? WHERE key=? AND value=?",`used:${st.purchase_choice}`,`purchase_choice:${uid}`,st.purchase_choice);
  return !!result.meta.changes;
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
    const route=await first(env,"SELECT group_ids,panel_identity FROM bot_order_routes WHERE order_id=?",order.id);
    if(!route || !(await categoryMatchesAccount(env,{...route,active:1},account))) throw new Error("گروه سفارش با سرویس فعلی مطابقت ندارد؛ بررسی ادمین لازم است");
    const lock = await run(env, "INSERT OR IGNORE INTO service_operations(service_id,kind,created_at) VALUES(?,?,?)",svc.id,"renew",now());
    if (!lock.meta.changes) throw new Error("عملیات دیگری روی سرویس در حال انجام است");
    try {
      const warn = await panelRenew(env, svc, bytes, days, onDispatch);
      await run(env, "UPDATE services SET volume=?, duration=?, volume_bytes=?, created_at=? WHERE id=?", order.volume, order.duration, bytes, now(), svc.id);
      return { renewed: true, name: svc.name, link: currentServiceLink(env,svc,account), warn };
    } finally { await run(env, "DELETE FROM service_operations WHERE service_id=?",svc.id); }
  }
  const route=await first(env,"SELECT group_ids,anchor_id,panel_identity FROM bot_order_routes WHERE order_id=?",order.id);
  if(!route || !parseIds(route.group_ids).length) throw new Error("گروه سفارش مشخص نیست؛ بررسی ادمین لازم است");
  const identity=await currentPanelIdentity(env);
  if(route.panel_identity && route.panel_identity!==identity) throw new Error("سفارش متعلق به اتصال قبلی پنل است؛ بررسی ادمین لازم است");
  const p = await panelCreate(env, { name: order.service_name, bytes, days, groupIds:route.group_ids,anchorId:route.anchor_id,attemptKey: `order:${order.id}:${hex(8)}`, onDispatch });
  await insertService(env, { ...p, user_id: order.user_id, name: order.service_name, volume: order.volume, duration: order.duration });
  return { renewed: false, name: order.service_name, link: p.link, warn: "" };
}

const walletReason = reason => ({topup:"شارژ با فیش",purchase:"خرید یا تمدید",refund:"بازگشت وجه",admin_adjustment:"تغییر توسط ادمین"}[reason] || "تراکنش");
async function showAdminWallet(env, chat, userId, owner) {
  const balance=await walletBalance(env,userId);
  const history=await all(env,"SELECT delta,reason FROM wallet_entries WHERE user_id=? ORDER BY id DESC LIMIT 10",userId);
  const lines=history.length ? history.map(x=>`${Number(x.delta)>0?"➕":"➖"} ${toman(Math.abs(Number(x.delta)))} — ${walletReason(x.reason)}`).join("\n") : "بدون تراکنش";
  const rows=[];if(owner) rows.push([btn("➕ افزایش موجودی",`admin:wallet_add:${userId}`),btn("➖ کاهش موجودی",`admin:wallet_sub:${userId}`)]);
  rows.push([btn("👤 پنل کاربر", `admin:user:${userId}`),btn("🔙 مدیریت", "admin:root")]);
  return sendMsg(env,chat,`💰 <b>کیف پول کاربر</b> <code>${userId}</code>\nموجودی: <b>${toman(balance)}</b>\n\n${lines}`,kb(rows));
}
async function handleWalletAmount(c,msg) {
  const {env,st}=c, amount=parseMoney(msg.text);
  if(amount===null) return sendMsg(env,msg.chat.id,"مبلغ معتبر را به تومان و با عدد انگلیسی بفرستید.",backKb("wallet:home"));
  const min=num(env,"wallet_topup_min"),max=num(env,"wallet_topup_max");
  if(amount<min || (max>0 && amount>max))return sendMsg(env,msg.chat.id,`مبلغ خارج از محدودهٔ مجاز است. دوباره وارد کنید:\n${topupLimitsText(env)}`,backKb("wallet:home"));
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
  if(c.st.order_id)await cancelDraftOrder(env,Number(c.st.order_id),msg.from.id);
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
  await expireCouponReservations(env);
  let order = st.order_id ? await getOrder(env, st.order_id) : null;
  if (!order) {
    await sendMsg(env, msg.chat.id, "❌ سفارش یافت نشد. دوباره از منو اقدام کنید.");
    c.st = {};
    return;
  }
  const u = msg.from;
  const expiredReceipt=async()=>{
    const use=await first(env,"SELECT id,amount FROM bot_coupon_uses WHERE order_id=? AND user_id=? AND state='released' ORDER BY id DESC LIMIT 1",order.id,u.id);
    if(!use)return false;
    const current=await getOrder(env,order.id),originalFinal=Math.max(0,Number(current?.final_price || 0)-Number(use.amount || 0));
    let delivered=0;
    for(const adminId of env.admins){try{const copy=await tg(env,"copyMessage",{chat_id:adminId,from_chat_id:msg.chat.id,message_id:msg.message_id});if(copy.ok){const note=await sendMsg(env,adminId,`⚠️ فیش سفارش <code>${order.id}</code> پس از پایان مهلت کد تخفیف رسید. مبلغ فاکتور قبلی: ${toman(originalFinal)}. سفارش اجرا نشده و سهمیه تخفیف آزاد شده؛ مبلغ و وضعیت را دستی بررسی کنید.`);if(note.ok)delivered++}}catch{}}
    delete st.step;delete st.order_id;
    await sendMsg(env,msg.chat.id,delivered ? "⚠️ مهلت کد تمام شده و سفارش اجرا نشده است. فیش برای بررسی به مدیریت ارسال شد؛ دوباره پرداخت نکنید." : "⚠️ مهلت کد تمام شده و سفارش اجرا نشده است. ارسال فیش به مدیریت ناموفق بود؛ دوباره پرداخت نکنید و با پشتیبانی تماس بگیرید.",backKb());
    return true;
  };
  if(Number(order.user_id) === Number(u.id) && order.status==='cancelled' && await expiredReceipt())return;
  if (Number(order.user_id) !== Number(u.id) || !["draft", "needs_fix"].includes(order.status) || !(await claimOrder(env, order.id, order.status, "pending"))) {
    await expireCouponReservations(env);
    const current=await getOrder(env,order.id);
    if(Number(order.user_id) === Number(u.id) && current?.status==='cancelled' && await expiredReceipt())return;
    delete st.step; delete st.order_id; return sendMsg(env, msg.chat.id, "این سفارش قبلاً ثبت یا بررسی شده است.");
  }
  order=await getOrder(env,order.id);
  if(!order || order.status!=="pending")return sendMsg(env,msg.chat.id,"وضعیت سفارش تغییر کرده است؛ با پشتیبانی تماس بگیرید.",backKb());
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
  // Only acknowledge a receipt when at least one admin receives both the
  // receipt and actionable order details. A failed Telegram call is not success.
  let delivered = 0;
  for (const adminId of env.admins) {
    try {
      const copy = await tg(env, "copyMessage", { chat_id: adminId, from_chat_id: msg.chat.id, message_id: msg.message_id });
      if (!copy.ok) continue;
      const notification = await sendMsg(env, adminId, caption,
        kb([[btn("✅ تایید", `approve_${order.id}`), btn("⚠️ ایراد در فیش", `issue_${order.id}`)],[btn("❌ رد سفارش",`reject_order:${order.id}`)]]));
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
  "<b>بسته‌ها</b>\n/plans — لیست\n/setprice KEY PRICE — تغییر قیمت\n/setplan KEY PRICE GB — ویرایش بسته موجود\n/delplan KEY — حذف بسته\nبرای ساخت بسته و تنظیم دسته و مدت از پنل دکمه‌ای استفاده کنید.\n\n" +
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
    if (type === "int" && (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value)))) return reply("❌ مقدار باید عدد صحیح معتبر باشد.");
    const limitError=topupLimitError(env,key,value);if(limitError)return reply(`❌ ${limitError}`);
    const policyError=policySettingError(key,value);if(policyError)return reply(`❌ ${policyError}`);
    if (type === "bool" && !/^[01]$/.test(value)) return reply("❌ مقدار باید 0 یا 1 باشد.");
    if (type === "ids" && !/^\d+(,\d+)*$/.test(value)) return reply("❌ مقدار باید عدد یا چند عدد با کاما باشد (مثلا 1,2).");
    if (type === "name_mode" && !["custom","random"].includes(value)) return reply("❌ نوع نام باید custom یا random باشد.");
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
    const existing=await getPlan(env,a[0]);
    if (!existing?.category_id || !existing?.days) return reply("برای ایجاد بسته و انتخاب مدت و دسته‌بندی، از پنل مدیریت استفاده کنید.");
    const sort = (await first(env, "SELECT COALESCE(MAX(sort),0)+1 AS n FROM plans")).n;
    await run(env,
      "INSERT INTO plans(key,gb,price,sort) VALUES(?,?,?,?) ON CONFLICT(key) DO UPDATE SET gb=excluded.gb, price=excluded.price",
      a[0], Number(a[2]), Number(a[1]), sort);
    return reply("✅ بسته ذخیره شد.");
  }
  if (cmd === "/delplan" && a.length === 1) {
    await env.DB.batch([env.DB.prepare("DELETE FROM bot_plan_specs WHERE plan_key=?").bind(a[0]),env.DB.prepare("DELETE FROM plans WHERE key=?").bind(a[0])]);
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
const BOT_USERS_QUERY = `WITH ids AS (SELECT user_id FROM bot_users UNION SELECT user_id FROM states UNION SELECT user_id FROM services UNION SELECT user_id FROM orders UNION SELECT user_id FROM wallet_entries UNION SELECT user_id FROM wallet_topups UNION SELECT user_id FROM wallet_payments UNION SELECT user_id FROM discounts UNION SELECT user_id FROM test_claims UNION SELECT referrer_id AS user_id FROM referrals UNION SELECT referred_id AS user_id FROM referrals)`;
async function botUserExists(env,id) { return !!(await first(env,`${BOT_USERS_QUERY} SELECT user_id FROM ids WHERE user_id=? LIMIT 1`,id)); }
async function botUserIdentity(env,id,cached) {
  let user=cached || await first(env,"SELECT first_name,username,blocked FROM bot_users WHERE user_id=?",id) || {};
  if (!user.first_name || !user.username) {
    try {
      const response=await fetch(`https://api.telegram.org/bot${env.BOT_TOKEN}/getChat`,{
        method:"POST",headers:{"content-type":"application/json"},body:JSON.stringify({chat_id:id}),signal:AbortSignal.timeout(4000),
      });
      const result=await response.json(), chat=result?.result;
      if (result?.ok && Number(chat?.id)===id && chat.type === "private" && typeof chat.first_name === "string" && chat.first_name) {
        await run(env,"INSERT INTO bot_users(user_id,first_name,username) VALUES(?,?,?) ON CONFLICT(user_id) DO UPDATE SET first_name=CASE WHEN bot_users.first_name='' THEN excluded.first_name ELSE bot_users.first_name END,username=CASE WHEN bot_users.username='' THEN excluded.username ELSE bot_users.username END",id,chat.first_name.slice(0,128),String(chat.username || "").slice(0,64));
        user=await first(env,"SELECT first_name,username,blocked FROM bot_users WHERE user_id=?",id) || user;
      }
    } catch { /* Telegram may not have a reachable private chat for older users. */ }
  }
  if (!user.username) {
    const old=await first(env,"SELECT username FROM orders WHERE user_id=? AND username IS NOT NULL AND username!='' AND username!='-' ORDER BY id DESC LIMIT 1",id);
    if(old?.username) user={...user,username:String(old.username).slice(0,64),usernameFromOrder:true};
  }
  return user;
}
const adminTime = value => /^\d{12,13}$/.test(String(value || "")) ? new Date(Number(value)).toLocaleString("fa-IR",{timeZone:"Asia/Tehran"}) : String(value || "—");
const adminOrderStatus = status => ({draft:"پیش‌نویس",pending:"منتظر تأیید",approved:"تأییدشده",needs_fix:"نیازمند اصلاح",needs_review:"نیازمند بررسی",needs_review_refunded:"نیازمند بررسی؛ وجه برگشت داده شد",wallet_processing:"در حال پرداخت",wallet_failed_refunded:"ناموفق؛ وجه برگشت داده شد",wallet_review_resolved:"بررسی‌شده"}[status] || status || "—");
const messageJobStatus = status => ({draft:"پیش‌نویس",building:"در حال آماده‌سازی",queued:"در صف ارسال",done:"پایان‌یافته",cancelled:"متوقف‌شده"}[status] || "نامشخص");
const adminBack = (target = "admin:root") => kb([[btn("🔙 بازگشت", target)]]);
const adminHomeKb = () => kb([
  [btn("⚙️ تنظیمات ربات", "admin:settings"), btn("📦 مدیریت بسته‌ها", "admin:plans")],
  [btn("🗂 مدیریت دسته‌بندی‌ها", "admin:categories")],
  [btn("👮 مدیریت ادمین‌ها", "admin:admins"), btn("📊 آمار فروش", "admin:stats")],
  [btn("👥 لیست کاربران", "admin:users:0"),btn("🔎 جستجوی سرویس", "admin:service_search")],
  [btn("📣 پیام همگانی", "admin:broadcast"),btn("📨 وضعیت پیام‌ها", "admin:messages:0")],
  [btn("💰 مدیریت کیف پول", "admin:wallet"),btn("⚠️ سفارش‌های قابل بررسی", "admin:wallet_reviews")],
  [btn("🎁 تنظیم هدیه دعوت", "admin:setting:referral_discount"),btn("🎟 مدیریت کد تخفیف", "admin:coupons:0")],
  [btn("📲 مدیریت اپلیکیشن‌ها", "admin:apps")],
  [btn("🎁 مجوز دوباره تست", "admin:resettest"), btn("🔌 بررسی اتصال پنل", "admin:panelcheck")],
  [btn("♻️ ریست تست همه", "admin:reset_all_tests")],
  [btn("🏠 منوی مشتری", "main_menu")],
]);
const showAdminHome = (env, chat) => sendMsg(env, chat, "🛠 <b>پنل مدیریت ربات</b>\n\nبخش موردنظر را انتخاب کنید:", adminHomeKb());
const planKeyOK = key => /^[A-Za-z0-9_-]{1,32}$/.test(key);
const positiveId = value => /^\d+$/.test(value) && Number.isSafeInteger(Number(value)) && Number(value) > 0;

async function showAdminServiceSearch(env,chat,query,page) {
  const rows=await all(env,"SELECT id,user_id,name,panel_email FROM services WHERE instr(lower(COALESCE(name,'')),lower(?))>0 OR instr(lower(COALESCE(panel_email,'')),lower(?))>0 ORDER BY id DESC LIMIT 11 OFFSET ?",query,query,page*10);
  const buttons=rows.slice(0,10).map(x=>[btn(`${String(x.name || x.panel_email || "سرویس").slice(0,35)} — ${x.user_id}`,`admin:svc:${x.id}`)]);
  const nav=[];if(page>0)nav.push(btn("◀️ قبلی",`admin:searchpage:${page-1}`));if(rows.length>10)nav.push(btn("بعدی ▶️",`admin:searchpage:${page+1}`));if(nav.length)buttons.push(nav);buttons.push([btn("🔎 جستجوی جدید","admin:service_search"),btn("🔙 مدیریت","admin:root")]);
  return sendMsg(env,chat,`🔎 <b>نتیجه جستجوی سرویس</b>\nعبارت: <code>${esc(query)}</code>\nصفحه ${page+1}\n${rows.length ? "سرویس موردنظر را انتخاب کنید." : "سرویسی یافت نشد."}`,kb(buttons));
}

async function handleAdminUIInput(c, msg) {
  const { env, st } = c, step = st.admin_ui_step, value = (msg.text || "").trim();
  const send = text => sendMsg(env, msg.chat.id, text, adminBack());
  let command;
  if (step === 'app_new_title') {
    if(!value || value.length>60 || /[\r\n]/.test(value))return send('نام باید در یک خط و حداکثر ۶۰ نویسه باشد.');
    st.app_draft={title:value};st.admin_ui_step='app_new_text';return sendMsg(env,msg.chat.id,'توضیحات برنامه را بفرستید؛ برای بدون توضیح، - بفرستید.',adminBack('admin:apps'));
  }
  if (step === 'app_new_text') {
    if(!st.app_draft || !value || value.length>1000)return send('توضیحات را حداکثر در ۱۰۰۰ نویسه بفرستید.');
    st.app_draft.text=value==='-'?'':value;st.admin_ui_step='app_new_links';return sendMsg(env,msg.chat.id,'برای هر لینک یک خط به شکل عنوان | آدرس بفرستید.\nمثال: اندروید | https://example.com/app',adminBack('admin:apps'));
  }
  if (step === 'app_new_links') {
    if(!st.app_draft)return send('درخواست منقضی شده؛ دوباره شروع کنید.');let links;try{links=parseAppLinks(value)}catch(e){return send(e.message)}
    if(Object.keys(env.apps).length>=50)return send('حداکثر ۵۰ اپلیکیشن قابل ثبت است.');
    const key='a'+hex(8);await storeApp(env,key,{...st.app_draft,links,enabled:true});delete st.app_draft;delete st.admin_ui_step;delete st.admin_ui_key;
    return sendMsg(env,msg.chat.id,'✅ اپلیکیشن اضافه شد.',adminBack('admin:apps'));
  }
  if (['app_edit_title','app_edit_text','app_edit_links'].includes(step)) {
    const key=st.admin_ui_key,app=env.apps[key];if(!app || !appKeyOK(key))return send('اپلیکیشن پیدا نشد.');
    let updated={...app};
    if(step==='app_edit_title'){if(!value||value.length>60||/[\r\n]/.test(value))return send('نام باید در یک خط و حداکثر ۶۰ نویسه باشد.');updated.title=value;}
    if(step==='app_edit_text'){if(!value||value.length>1000)return send('توضیحات را حداکثر در ۱۰۰۰ نویسه بفرستید؛ برای پاک کردن - بفرستید.');updated.text=value==='-'?'':value;}
    if(step==='app_edit_links'){try{updated.links=parseAppLinks(value)}catch(e){return send(e.message)}}
    await storeApp(env,key,updated);delete st.admin_ui_step;delete st.admin_ui_key;
    return sendMsg(env,msg.chat.id,'✅ تغییر ذخیره شد.',adminBack(`admin:app:${key}`));
  }
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
      if(changingAccount) {
        for(const key of ["panel_group_ids","panel_inbound_id","sub_base_url"]) queries.push(env.DB.prepare("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value").bind(key,""));
        queries.push(env.DB.prepare("UPDATE bot_categories SET active=0"),env.DB.prepare("UPDATE bot_order_routes SET panel_identity='__blocked__' WHERE panel_identity=''"));
      }
      await env.DB.batch(queries); panelCookie=null;panelCookieKey="";
      st.admin_ui_step="panel_sub"; delete st.admin_ui_key;
      return sendMsg(env,msg.chat.id,"✅ اتصال پنل ذخیره شد. آدرس پایه HTTPS اشتراک را بفرستید؛ بعداً برای هر دسته‌بندی گروه پنل را انتخاب می‌کنید.",adminBack("admin:settings"));
    }
    delete st.admin_ui_step; return send("درخواست نامعتبر است؛ دوباره شروع کنید.");
  }
  if (step === "message_broadcast" || step === "message_private") {
    if(step === "message_broadcast" && (msg.photo || msg.voice || msg.audio || msg.forward_origin || msg.forward_date)) {
      if(msg.media_group_id)return send("آلبوم چندرسانه‌ای پشتیبانی نمی‌شود؛ هر پیام را جداگانه بفرستید.");
      const forward=!!(msg.forward_origin || msg.forward_date);
      const body=encodeMessageReference(msg.chat.id,msg.message_id,forward);
      let preview;try {preview=await tg(env,forward?"forwardMessage":"copyMessage",{chat_id:msg.chat.id,from_chat_id:msg.chat.id,message_id:msg.message_id})}catch{return send("پیش‌نمایش پیام ناموفق بود؛ دوباره تلاش کنید.")}
      if(!preview.ok)return send("این پیام قابل ارسال مجدد نیست؛ محدودیت فوروارد یا محتوای محافظت‌شده را بررسی کنید.");
      let id;try{id=await createMessageJob(env,msg.from.id,"broadcast",null,body)}catch{return send("ساخت پیش‌نویس پیام ناموفق بود.")}
      delete st.admin_ui_step;delete st.admin_ui_key;
      return sendMsg(env,msg.chat.id,"📣 پیش‌نمایش پیام بالا نمایش داده شد. کاربران مسدودشده پیام را دریافت نمی‌کنند. پیام اصلی را تا پایان ارسال پاک نکنید. آیا ارسال شود؟",kb([[btn("✅ ارسال",`admin:message_send:${id}`)],[btn("❌ انصراف",`admin:message_cancel:${id}`)]]));
    }
    if(!value || value.length>3500)return send("متن پیام باید حداکثر ۳۵۰۰ نویسه باشد.");
    const privateMessage=step === "message_private",target=privateMessage ? Number(st.admin_ui_key) : null;
    if(privateMessage && (!positiveId(String(target)) || !(await botUserExists(env,target))))return send("کاربر پیدا نشد.");
    let id;try{id=await createMessageJob(env,msg.from.id,privateMessage ? "private" : "broadcast",target,value)}catch{return send("ساخت پیش‌نویس پیام ناموفق بود.")}
    delete st.admin_ui_step;delete st.admin_ui_key;
    return sendMsg(env,msg.chat.id,`📝 <b>پیش‌نمایش پیام ${privateMessage ? `به کاربر <code>${target}</code>` : "همگانی"}</b>\n\n${esc(value)}\n\n${privateMessage ? "" : "کاربران مسدودشده از دریافت همگانی مستثنا هستند.\n"}آیا ارسال شود؟`,kb([[btn("✅ ارسال",`admin:message_send:${id}`)],[btn("❌ انصراف",`admin:message_cancel:${id}`)]]));
  }
  if (step === "coupon_create") {
    const parts=value.split(/\s+/),kind=st.admin_ui_key,code=normalizeCoupon(parts[0]);
    if(parts.length!==5 || !validCouponCode(code) || !["percent","fixed"].includes(kind) || parts.slice(1).some(x=>!/^\d+$/.test(x)||!Number.isSafeInteger(Number(x))) ) return send("فرمت: کد، مقدار، تعداد کل، تعداد هر کاربر، اعتبار روز. برای محدودیت نامحدود عدد 0 بفرستید. مثال: SALE25 25 100 1 30");
    const [amount,total,perUser,days]=parts.slice(1).map(Number);
    if(amount<=0 || amount>9000000000000000 || (kind==="percent" && amount>100) || total>1000000000 || perUser>1000000000 || days>36500) return send("مقادیر نامعتبرند؛ درصد باید ۱ تا ۱۰۰ و سایر محدودیت‌ها در بازهٔ معتبر باشند.");
    const now=Date.now(),expiry=days ? now+days*DAY_MS : null;
    try{await run(env,"INSERT INTO bot_coupons(code,kind,value,created_ms,expires_ms,total_limit,user_limit) VALUES(?,?,?,?,?,?,?)",code,kind,amount,now,expiry,total || null,perUser || null)}catch{return send("ثبت نشد؛ ممکن است این کد قبلاً وجود داشته باشد.")}
    delete st.admin_ui_step;delete st.admin_ui_key;
    return sendMsg(env,msg.chat.id,`✅ کد <code>${esc(code)}</code> ساخته شد.`,adminBack("admin:coupons:0"));
  }
  if (step === "service_lookup") {
    if (!value || value.length>80 || /[\r\n]/.test(value)) return send("نام سرویس یا نام حساب پنل را تا ۸۰ نویسه در یک خط بفرستید.");
    st.admin_service_search=value;delete st.admin_ui_step;delete st.admin_ui_key;
    return showAdminServiceSearch(env,msg.chat.id,value,0);
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
    if (meta[0] === "int" && (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value)))) return send("مقدار باید عدد صحیح معتبر باشد.");
    const limitError=topupLimitError(env,st.admin_ui_key,value);if(limitError)return send(limitError);
    const policyError=policySettingError(st.admin_ui_key,value);if(policyError)return send(policyError);
    if (meta[0] === "bool" && !/^[01]$/.test(value)) return send("مقدار باید ۰ یا ۱ (به انگلیسی) باشد.");
    if (meta[0] === "ids" && !/^\d+(,\d+)*$/.test(value)) return send("شناسه‌ها را با اعداد انگلیسی و کاما وارد کنید.");
    command = `/set ${st.admin_ui_key} ${value}`;
  } else if (step === "price") {
    if (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value))) return send("قیمت را به تومان و با عدد صحیح انگلیسی وارد کنید.");
    if (!(await getPlan(env, st.admin_ui_key))) return send("بسته دیگر وجود ندارد؛ از منو انتخاب کنید.");
    command = `/setprice ${st.admin_ui_key} ${value}`;
  } else if (step === "plan_edit" || step === "plan_new") {
    const parts=value.split(/\s+/), editing=step === "plan_edit";
    const key=editing ? st.admin_ui_key : parts.shift();
    if(!planKeyOK(key) || parts.length!==3 || !/^\d+$/.test(parts[0]) || !Number.isSafeInteger(Number(parts[0])) || !/^\d+(\.\d+)?$/.test(parts[1]) || !(Number(parts[1])>0) || !Number.isSafeInteger(Math.round(Number(parts[1])*GB)) || Math.round(Number(parts[1])*GB)<1 || !positiveId(parts[2]) || Number(parts[2])>36500) return send("قیمت، حجم گیگ و مدت روز را با اعداد انگلیسی وارد کنید؛ مثال: 70000 10 30.");
    const old=await getPlan(env,key);
    if(editing && !old) return send("بسته پیدا نشد.");
    if(!editing && old) return send("این کلید قبلاً ثبت شده است.");
    st.admin_plan_draft={key,price:Number(parts[0]),gb:Number(parts[1]),days:Number(parts[2]),editing,expires:Date.now()+30*60*1000};
    delete st.admin_ui_step;delete st.admin_ui_key;
    return showPlanCategoryChoices(env,msg.chat.id,0);
  } else if (step === "category_name") {
    if(!value || value.length>50 || /[\r\n]/.test(value)) return send("نام دسته‌بندی باید یک خط و حداکثر ۵۰ نویسه باشد.");
    st.admin_category_draft={name:value,expires:Date.now()+30*60*1000};delete st.admin_ui_step;delete st.admin_ui_key;
    return showCategoryGroupChoices(env,msg.chat.id,0);
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

async function showPlanCategoryChoices(env,chat,page) {
  const cats=await all(env,"SELECT id,name FROM bot_categories WHERE active=1 ORDER BY id DESC LIMIT 11 OFFSET ?",page*10);
  const rows=cats.slice(0,10).map(x=>[btn(String(x.name).slice(0,45),`admin:planpick:${x.id}`)]);
  const nav=[];if(page>0)nav.push(btn("◀️ قبلی",`admin:planchoices:${page-1}`));if(cats.length>10)nav.push(btn("بعدی ▶️",`admin:planchoices:${page+1}`));if(nav.length)rows.push(nav);rows.push([btn("🔙 مدیریت بسته‌ها","admin:plans")]);
  return sendMsg(env,chat,cats.length ? "دسته‌بندی بسته را انتخاب کنید:" : "دسته‌بندی فعالی نیست؛ ابتدا دسته‌بندی بسازید.",kb(rows));
}
async function showTestCategoryChoices(env,cq,page) {
  const cats=await all(env,"SELECT id,name FROM bot_categories WHERE active=1 ORDER BY id DESC LIMIT 11 OFFSET ?",page*10);
  const rows=cats.slice(0,10).map(x=>[btn(`${Number(env.cfg.test_category_id)===x.id ? '✅ ' : ''}${String(x.name).slice(0,40)}`,`admin:testcategory:${x.id}`)]);
  const nav=[];if(page>0)nav.push(btn("◀️ قبلی",`admin:testcategories:${page-1}`));if(cats.length>10)nav.push(btn("بعدی ▶️",`admin:testcategories:${page+1}`));if(nav.length)rows.push(nav);
  rows.push([btn("🔙 تنظیمات", "admin:settings")]);
  return render(env,cq,cats.length ? "🎁 دسته‌بندی سرویس تست را انتخاب کنید:" : "دسته‌بندی فعالی وجود ندارد؛ ابتدا یک دسته‌بندی بسازید.",kb(rows));
}
async function showCategoryGroupChoices(env,chat,page) {
  const choices=await panelChoices(env);if(!choices)return sendMsg(env,chat,"خواندن گروه‌ها از پنل ناموفق بود.",adminBack("admin:categories"));
  const groups=choices.groups.filter(g=>positiveId(String(g.id)) && Array.isArray(g.inboundIds) && g.inboundIds.length>0 && g.inboundIds.every(id=>choices.inbounds.some(i=>Number(i.inboundId)===Number(id))));
  const rows=groups.slice(page*10,(page+1)*10).map(g=>[btn(String(g.name || g.remark || `گروه ${g.id}`).slice(0,45),`admin:categorygroup:${g.id}`)]);
  const nav=[];if(page>0)nav.push(btn("◀️ قبلی",`admin:categorygroups:${page-1}`));if((page+1)*10<groups.length)nav.push(btn("بعدی ▶️",`admin:categorygroups:${page+1}`));if(nav.length)rows.push(nav);rows.push([btn("🔙 دسته‌بندی‌ها","admin:categories")]);
  return sendMsg(env,chat,groups.length ? "گروه پنل این دسته‌بندی را انتخاب کنید:" : "گروه دارای ورودی مجاز پیدا نشد.",kb(rows));
}

async function handleAdminUICallback(c, cq, answer) {
  const { env, st } = c, uid = cq.from.id, chat = cq.message.chat.id, d = cq.data;
  const owner = env.owners.includes(uid);
  if (d !== "admin:reset_confirm" && d !== "admin:reset_all_confirm") { delete st.admin_ui_step; delete st.admin_ui_key; }
  if (d !== "admin:wallet_confirm") delete st.admin_wallet_change;
  if (!d.startsWith("admin:planpick:") && !d.startsWith("admin:planchoices:")) delete st.admin_plan_draft;
  if (!d.startsWith("admin:categorygroup:") && !d.startsWith("admin:categorygroups:")) delete st.admin_category_draft;
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
  if (d === 'admin:apps') {
    const rows=Object.entries(env.apps).map(([key,app])=>[btn(`${app.enabled===false?'⛔':'✅'} ${String(app.title).slice(0,45)}`,`admin:app:${key}`)]);
    rows.push([btn('➕ افزودن اپلیکیشن','admin:appadd')],[btn('🔙 پنل مدیریت','admin:root')]);
    return renderAdmin('📲 <b>مدیریت اپلیکیشن‌ها</b>\nبرنامه را برای ویرایش انتخاب کنید.',kb(rows));
  }
  if (d === 'admin:appadd') {delete st.app_draft;return prompt('app_new_title','','نام اپلیکیشن جدید را بفرستید.','admin:apps');}
  if (/^admin:app:[A-Za-z0-9_-]{1,30}$/.test(d)) {
    const key=d.slice('admin:app:'.length),app=env.apps[key];if(!app)return answer('اپلیکیشن پیدا نشد.',true);
    const links=app.links.map(([label,url])=>`${esc(label)} | <code>${esc(url)}</code>`).join('\n');
    return renderAdmin(`📱 <b>${esc(app.title)}</b>\nوضعیت: ${app.enabled===false?'غیرفعال':'فعال'}\n\n${esc(app.text)}\n\n${links}`,kb([[btn('✏️ نام',`admin:apptitle:${key}`),btn('📝 توضیحات',`admin:apptext:${key}`)],[btn('🔗 تغییر لینک‌ها',`admin:applinks:${key}`)],[btn(app.enabled===false?'✅ فعال‌سازی':'⛔ غیرفعال‌سازی',`admin:apptoggle:${key}`)],[btn('🗑 حذف',`admin:appdelete:${key}`)],[btn('🔙 اپلیکیشن‌ها','admin:apps')]]));
  }
  if (/^admin:app(title|text|links|toggle|delete|confirm):[A-Za-z0-9_-]{1,30}$/.test(d)) {
    const [,action,key]=d.split(':'),app=env.apps[key];if(!app)return answer('اپلیکیشن پیدا نشد.',true);
    if(action==='apptitle')return prompt('app_edit_title',key,'نام جدید اپلیکیشن را بفرستید.',`admin:app:${key}`);
    if(action==='apptext')return prompt('app_edit_text',key,'توضیحات جدید را بفرستید؛ برای پاک کردن - بفرستید.',`admin:app:${key}`);
    if(action==='applinks')return prompt('app_edit_links',key,'تمام لینک‌های جدید را وارد کنید؛ هر خط: عنوان | آدرس\nمثال: اندروید | https://example.com/app',`admin:app:${key}`);
    if(action==='apptoggle'){await storeApp(env,key,{...app,enabled:app.enabled===false});return renderAdmin('✅ وضعیت تغییر کرد.',adminBack(`admin:app:${key}`));}
    if(action==='appdelete')return renderAdmin(`اپلیکیشن «${esc(app.title)}» حذف شود؟`,kb([[btn('✅ تأیید حذف',`admin:appconfirm:${key}`)],[btn('❌ انصراف',`admin:app:${key}`)]]));
    if(action==='appconfirm'){await storeApp(env,key,{deleted:true});return renderAdmin('✅ اپلیکیشن حذف شد.',adminBack('admin:apps'));}
  }
  if (/^admin:coupons:\d+$/.test(d)) {
    const page=Number(d.split(":")[2]);if(!Number.isSafeInteger(page)||page>1000000)return answer("صفحه نامعتبر است.",true);
    await expireCouponReservations(env);
    const codes=await all(env,"SELECT id,code,enabled,expires_ms FROM bot_coupons WHERE deleted=0 ORDER BY id DESC LIMIT 11 OFFSET ?",page*10);
    const rows=codes.slice(0,10).map(x=>[btn(`${!x.enabled ? "⛔" : x.expires_ms && Number(x.expires_ms)<=Date.now() ? "⌛" : "✅"} ${x.code}`,`admin:coupon:${x.id}`)]),nav=[];
    if(page>0)nav.push(btn("◀️ قبلی",`admin:coupons:${page-1}`));if(codes.length>10)nav.push(btn("بعدی ▶️",`admin:coupons:${page+1}`));if(nav.length)rows.push(nav);rows.push([btn("➕ ساخت کد", "admin:coupon_new")],[btn("🔙 مدیریت", "admin:root")]);
    return renderAdmin("🎟 <b>کدهای تخفیف</b>",kb(rows));
  }
  if(d === "admin:coupon_new")return renderAdmin("نوع تخفیف را انتخاب کنید:",kb([[btn("٪ درصدی","admin:coupon_type:percent"),btn("💵 مبلغی","admin:coupon_type:fixed")],[btn("🔙 بازگشت","admin:coupons:0")]]));
  if (/^admin:coupon_type:(percent|fixed)$/.test(d)) {
    const kind=d.split(":")[2];return prompt("coupon_create",kind,`🎟 <b>ساخت کد ${kind === "percent" ? "درصدی" : "مبلغی"}</b>\nپنج مقدار با فاصله و عدد انگلیسی بفرستید:\nکد، ${kind === "percent" ? "درصد (۱ تا ۱۰۰)" : "مبلغ تومان"}، تعداد کل، تعداد هر کاربر، اعتبار به روز.\nبرای هریک از سه محدودیت، 0 یعنی نامحدود.\nمثال: <code>SALE25 25 100 1 30</code>`,"admin:coupons:0");
  }
  if (/^admin:coupon:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]);if(!positiveId(String(id)))return answer("کد نامعتبر است.",true);
    await expireCouponReservations(env);
    const c=await first(env,"SELECT * FROM bot_coupons WHERE id=? AND deleted=0",id);if(!c)return answer("کد پیدا نشد.",true);
    const usage=await first(env,"SELECT COUNT(CASE WHEN state='consumed' THEN 1 END) AS used,COUNT(CASE WHEN state='reserved' THEN 1 END) AS reserved FROM bot_coupon_uses WHERE coupon_id=?",id);
    const exp=c.expires_ms ? new Date(Number(c.expires_ms)).toLocaleString("fa-IR",{timeZone:"Asia/Tehran"}) : "نامحدود";
    return renderAdmin(`🎟 <b>${esc(c.code)}</b>\nتخفیف: ${c.kind === "percent" ? `${c.value}٪` : toman(c.value)}\nوضعیت: ${!c.enabled ? "غیرفعال" : c.expires_ms && Number(c.expires_ms)<=Date.now() ? "منقضی" : "فعال"}\nاعتبار تا: ${esc(exp)}\nسقف کل: ${c.total_limit ?? "نامحدود"}\nسقف هر کاربر: ${c.user_limit ?? "نامحدود"}\nمصرف موفق: ${usage.used}؛ رزروشده: ${usage.reserved}`,kb([[btn(c.enabled ? "⛔ غیرفعال‌سازی" : "✅ فعال‌سازی",`admin:coupon_toggle:${id}`)],[btn("🗑 حذف کد",`admin:coupon_delete:${id}`)],[btn("🔙 کدها","admin:coupons:0")]]));
  }
  if (/^admin:coupon_toggle:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]);if(!positiveId(String(id)))return answer("کد نامعتبر است.",true);
    const r=await run(env,"UPDATE bot_coupons SET enabled=1-enabled WHERE id=? AND deleted=0",id);if(!r.meta.changes)return answer("کد پیدا نشد.",true);
    return renderAdmin("✅ وضعیت کد تغییر کرد. رزروهای قبلی تا پایان مهلت یا تعیین تکلیف سفارش معتبرند.",adminBack(`admin:coupon:${id}`));
  }
  if (/^admin:coupon_delete:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]),c=await first(env,"SELECT code FROM bot_coupons WHERE id=? AND deleted=0",id);if(!c)return answer("کد پیدا نشد.",true);
    return renderAdmin(`کد <code>${esc(c.code)}</code> حذف شود؟ رزروها و تاریخچه سفارش‌های قبلی حفظ می‌شوند.`,kb([[btn("✅ تأیید",`admin:coupon_delete_yes:${id}`)],[btn("❌ انصراف",`admin:coupon:${id}`)]]));
  }
  if (/^admin:coupon_delete_yes:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]);if(!positiveId(String(id)))return answer("کد نامعتبر است.",true);
    const r=await run(env,"UPDATE bot_coupons SET deleted=1,enabled=0 WHERE id=? AND deleted=0",id);
    return renderAdmin(r.meta.changes ? "✅ کد حذف شد؛ تاریخچه حفظ شد." : "کد پیدا نشد.",adminBack("admin:coupons:0"));
  }
  if(d === "admin:broadcast")return prompt("message_broadcast","","📣 متن، عکس، پیام صوتی یا پیام فورواردشده را بفرستید. پیش از ارسال پیش‌نمایش و تأیید نمایش داده می‌شود.");
  if (/^admin:message_user:\d+$/.test(d)) {
    const id=d.split(":")[2];if(!positiveId(id)||!(await botUserExists(env,Number(id))))return answer("کاربر پیدا نشد.",true);
    return prompt("message_private",id,`✉️ متن پیام خصوصی برای کاربر <code>${id}</code> را بفرستید. پیش از ارسال تأیید می‌گیرید.`,`admin:user:${id}`);
  }
  if (/^admin:messages:\d+$/.test(d)) {
    const page=Number(d.split(":")[2]);if(!Number.isSafeInteger(page)||page>1000000)return answer("صفحه نامعتبر است.",true);
    const jobs=await all(env,"SELECT id,audience,status FROM bot_message_jobs ORDER BY id DESC LIMIT 11 OFFSET ?",page*10);
    const rows=jobs.slice(0,10).map(j=>[btn(`#${j.id} ${j.audience==='broadcast'?'همگانی':'خصوصی'} — ${messageJobStatus(j.status)}`,`admin:message_job:${j.id}`)]),nav=[];
    if(page>0)nav.push(btn("◀️ قبلی",`admin:messages:${page-1}`));if(jobs.length>10)nav.push(btn("بعدی ▶️",`admin:messages:${page+1}`));if(nav.length)rows.push(nav);rows.push([btn("🔙 مدیریت","admin:root")]);
    return renderAdmin("📨 <b>وضعیت پیام‌ها</b>",kb(rows));
  }
  if (/^admin:message_job:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]);if(!positiveId(String(id)))return answer("شناسه نامعتبر است.",true);
    const job=await first(env,"SELECT * FROM bot_message_jobs WHERE id=?",id);if(!job)return answer("پیام پیدا نشد.",true);
    const counts=await first(env,"SELECT COUNT(CASE WHEN status='sent' THEN 1 END) sent,COUNT(CASE WHEN status='failed' THEN 1 END) failed,COUNT(CASE WHEN status='uncertain' THEN 1 END) uncertain,COUNT(CASE WHEN status IN ('pending','sending') THEN 1 END) waiting FROM bot_message_targets WHERE job_id=?",id);
    const rows=[];if(["draft","queued"].includes(job.status))rows.push([btn("⛔ توقف ارسال",`admin:message_cancel:${id}`)]);rows.push([btn("🔙 پیام‌ها","admin:messages:0")]);
    return renderAdmin(`📨 پیام <code>${id}</code> — ${job.audience==='broadcast'?'همگانی':'خصوصی'}\nوضعیت: ${messageJobStatus(job.status)}\nموفق: ${counts.sent}، ناموفق: ${counts.failed}، نامشخص: ${counts.uncertain}، در انتظار: ${counts.waiting}`,kb(rows));
  }
  if (/^admin:message_send:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]);if(!positiveId(String(id)))return answer("شناسه نامعتبر است.",true);
    const ok=await confirmMessageJob(env,id,uid);if(!ok)return answer("پیام قبلاً تأیید شده یا متعلق به شما نیست.",true);
    return renderAdmin("✅ پیام در صف ارسال قرار گرفت. وضعیت موفق، ناموفق و نامشخص را در «وضعیت پیام‌ها» ببینید.",adminBack(`admin:message_job:${id}`));
  }
  if (/^admin:message_cancel:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]);if(!positiveId(String(id)))return answer("شناسه نامعتبر است.",true);
    const r=await run(env,"UPDATE bot_message_jobs SET status='cancelled',finished_ms=? WHERE id=? AND status IN ('draft','queued')",Date.now(),id);
    return renderAdmin(r.meta.changes ? "ارسال متوقف شد؛ پیامی که همان لحظه در حال ارسال بوده ممکن است رسیده باشد." : "ارسال قبلاً تمام یا متوقف شده است.",adminBack("admin:messages:0"));
  }
  if (d === "admin:service_search") return prompt("service_lookup","","🔎 نام سرویس یا نام حساب کانفیگ در پنل را بفرستید.");
  if (/^admin:searchpage:\d+$/.test(d)) {
    const page=Number(d.split(":")[2]);if(!st.admin_service_search || !Number.isSafeInteger(page)||page>1000000)return answer("جستجو منقضی شده است.",true);
    return showAdminServiceSearch(env,chat,st.admin_service_search,page);
  }
  if (/^admin:svc:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]);if(!Number.isSafeInteger(id)||id<=0)return answer("سرویس نامعتبر است.",true);
    const svc=await getService(env,id);if(!svc)return answer("سرویس یافت نشد.",true);
    let details;try { const account=await panelServiceAccount(env,svc); details=account ? liveServiceDetails(account) : "⚠️ سرویس در پنل متصل فعلی یافت نشد."; } catch { details="⚠️ وضعیت سرویس نامشخص است؛ ارتباط با پنل برقرار نشد."; }
    const operation=await first(env,'SELECT kind FROM service_operations WHERE service_id=?',id);const review=operation?.kind==='auto_delete_review';
    const buttons=[[btn("🔘 وضعیت کانفیگ‌ها",`admin:sc:${svc.id}:0`)],[btn("🗑 حذف سرویس",`admin:svcdelete:${svc.id}`)]];
    if(review)buttons.push([btn('⚠️ بررسی توقف حذف خودکار',`admin:autodelreview:${id}`)]);
    buttons.push([btn("👤 پنل کاربر",`admin:user:${svc.user_id}`)],[btn("🔙 نتایج جستجو","admin:searchpage:0"),btn("🔎 جستجوی جدید","admin:service_search")]);
    return renderAdmin(`📦 <b>سرویس ${esc(String(svc.name || "—").slice(0,50))}</b>\nشناسه ربات: <code>${svc.id}</code>\nکاربر: <code>${svc.user_id}</code>\nحساب پنل: <code>${esc(String(svc.panel_email || "—").slice(0,100))}</code>\n\n${details}${review?'\n⚠️ حذف خودکار نیاز به بررسی دارد.':''}`,kb(buttons));
  }
  if (/^admin:autodelreview:\d+$/.test(d)) {
    const id=Number(d.split(':')[2]),lock=await first(env,"SELECT kind FROM service_operations WHERE service_id=? AND kind='auto_delete_review'",id);if(!lock)return answer('عملیات نیازمند بررسی پیدا نشد.',true);
    return renderAdmin('⚠️ ممکن است فقط بخشی از سرویس در پنل حذف شده باشد. پس از بررسی و اصلاح دستی پنل و سفارش‌ها، می‌توانید توقف را بردارید. در صورت فعال بودن حذف خودکار ممکن است دوباره تلاش شود.',kb([[btn('✅ بررسی کردم؛ رفع توقف',`admin:autodelrelease:${id}`)],[btn('🔙 بازگشت',`admin:svc:${id}`)]]));
  }
  if (/^admin:autodelrelease:\d+$/.test(d)) {
    const id=Number(d.split(':')[2]);const result=await run(env,"DELETE FROM service_operations WHERE service_id=? AND kind='auto_delete_review'",id);
    return renderAdmin(result.meta.changes?'✅ توقف بررسی برداشته شد.':'عملیات پیدا نشد.',adminBack(`admin:svc:${id}`));
  }
  if (/^admin:svcdelete:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]),svc=await getService(env,id);if(!svc)return answer("سرویس یافت نشد.",true);
    return renderAdmin(`🗑 سرویس «${esc(svc.name)}» از پنل و ربات حذف شود؟ سفارش‌های مرتبط از فهرست پنهان می‌شوند و سوابق مالی برای محاسبه تخفیف و کیف پول نگهداری می‌شوند.`,kb([[btn("✅ تأیید حذف",`admin:svcdeleteconfirm:${id}`)],[btn("❌ انصراف",`admin:svc:${id}`)]]));
  }
  if (/^admin:svcdeleteconfirm:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]),svc=await getService(env,id);if(!svc)return answer("سرویس یافت نشد.",true);
    const lock=await run(env,"INSERT OR IGNORE INTO service_operations(service_id,kind,created_at) VALUES(?,?,?)",id,"delete",now());
    if(!lock.meta.changes)return answer("عملیات دیگری روی سرویس در جریان است.",true);
    try {
      const orders=await all(env,"SELECT o.id,o.status FROM orders o WHERE o.user_id=? AND (o.renew_service_id=? OR EXISTS(SELECT 1 FROM provision_attempts p WHERE p.sub_id=? AND p.email=? AND p.attempt_key LIKE 'order:'||o.id||':%'))",svc.user_id,id,svc.sub_id,svc.panel_email);
      if(orders.some(o=>!["approved","rejected","cancelled"].includes(o.status)))throw new Error("سفارش مرتبط در حال بررسی است؛ ابتدا وضعیت آن را مشخص کنید.");
      await panelDelete(env,svc);
      const statements=orders.map(o=>env.DB.prepare("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value").bind(`hidden_order:${o.id}`,"1"));
      statements.push(env.DB.prepare("DELETE FROM services WHERE id=? AND user_id=?").bind(id,svc.user_id));
      await env.DB.batch(statements);
      return renderAdmin("✅ حذف سرویس از پنل تأیید شد و سرویس از ربات حذف شد. سفارش‌های قابل شناسایی از فهرست پنهان شدند؛ سوابق مالی حفظ شده است.",adminBack("admin:service_search"));
    } catch(e) {return renderAdmin(`⚠️ حذف کامل تأیید نشد: ${esc(String(e.message).slice(0,200))}؛ پیش از تکرار، پنل را بررسی کنید.`,adminBack(`admin:svc:${id}`));}
    finally {await run(env,"DELETE FROM service_operations WHERE service_id=?",id);}
  }
  if (/^admin:sc:\d+:\d+$/.test(d)) {
    const [, ,id,pageText]=d.split(":"), serviceId=Number(id), page=Number(pageText);
    if(!Number.isSafeInteger(serviceId)||serviceId<=0||!Number.isSafeInteger(page)||page>1000000)return answer("درخواست نامعتبر است.",true);
    const svc=await getService(env,serviceId);if(!svc)return answer("سرویس پیدا نشد.",true);
    let state;try{state=await panelMemberships(env,svc)}catch{return renderAdmin("⚠️ دریافت وضعیت سرویس از پنل ناموفق بود یا سرویس پیدا نشد.",adminBack(`admin:svc:${serviceId}`))}
    if(page>Math.max(0,Math.ceil(state.list.length/10)-1))return answer("صفحه پیدا نشد.",true);
    const rows=[];
    for(const m of state.list.slice(page*10,(page+1)*10)) {
      if(!Number.isSafeInteger(m.id)||m.id<=0)continue;
      if(m.enabled===null || !state.accountEnabled)rows.push([btn(`⏸ ${m.name.slice(0,35)} — وضعیت قابل تغییر نیست`,`admin:scinfo:${serviceId}`)]);
      else rows.push([btn(`${m.enabled ? "⛔ غیرفعال‌سازی" : "✅ فعال‌سازی"} ${m.name.slice(0,35)}`,`admin:ss:${serviceId}:${m.id}:${m.enabled?0:1}`)]);
    }
    const nav=[];if(page>0)nav.push(btn("◀️ قبلی",`admin:sc:${serviceId}:${page-1}`));if((page+1)*10<state.list.length)nav.push(btn("بعدی ▶️",`admin:sc:${serviceId}:${page+1}`));if(nav.length)rows.push(nav);rows.push([btn("🔙 سرویس",`admin:svc:${serviceId}`)]);
    return renderAdmin(state.accountEnabled ? "🔘 وضعیت هر کانفیگ را تغییر دهید. هر ورودی فقط همین سرویس را تغییر می‌دهد." : "⚠️ حساب در پنل خاموش است. برای فعال‌کردن حساب از پنل استفاده کنید.",kb(rows));
  }
  if (/^admin:scinfo:\d+$/.test(d)) return answer("وضعیت نامشخص است یا حساب در پنل خاموش است.",true);
  if (/^admin:ss:\d+:\d+:[01]$/.test(d)) {
    const [, ,id,inboundIdText,desired]=d.split(":"),serviceId=Number(id),inboundId=Number(inboundIdText);
    if(!Number.isSafeInteger(serviceId)||serviceId<=0||!Number.isSafeInteger(inboundId)||inboundId<=0)return answer("درخواست نامعتبر است.",true);
    const svc=await getService(env,serviceId);if(!svc)return answer("سرویس پیدا نشد.",true);
    const lock=await run(env,"INSERT OR IGNORE INTO service_operations(service_id,kind,created_at) VALUES(?,?,?)",serviceId,"membership",now());
    if(!lock.meta.changes)return answer("عملیات دیگری در جریان است.",true);
    try{
      const before=await panelMemberships(env,svc),member=before.list.find(m=>m.id===inboundId),wanted=desired==="1";
      if(!before.accountEnabled||!member||member.enabled===null)return renderAdmin("⚠️ حساب یا وضعیت کانفیگ قابل تغییر نیست.",adminBack(`admin:sc:${serviceId}:0`));
      if(member.enabled===wanted)return renderAdmin("این وضعیت قبلاً تغییر کرده است.",adminBack(`admin:sc:${serviceId}:0`));
      const response=await panelCall(env,"POST",`/panel/api/inbounds/${inboundId}/setMembershipEnable/${encodeURIComponent(svc.panel_email)}`,new URLSearchParams({enable:String(wanted)}));
      if(!response.success)throw new Error("panel rejected");
      const after=await panelMemberships(env,svc),changed=after.list.find(m=>m.id===inboundId);
      if(!changed || changed.enabled!==wanted)return renderAdmin("⚠️ تغییر در پنل تأیید نشد؛ وضعیت را بررسی کنید.",adminBack(`admin:sc:${serviceId}:0`));
      return renderAdmin(`✅ کانفیگ «${esc(changed.name)}» ${wanted ? "فعال" : "غیرفعال"} شد.${wanted && !changed.inboundEnabled ? "\nورودی اصلی خاموش است." : ""}`,adminBack(`admin:sc:${serviceId}:0`));
    }catch{return renderAdmin("⚠️ نتیجهٔ تغییر وضعیت مشخص نیست؛ پنل را بررسی کنید.",adminBack(`admin:sc:${serviceId}:0`))}
    finally{await run(env,"DELETE FROM service_operations WHERE service_id=?",serviceId)}
  }
  if (d === "admin:reset_all_tests") {
    st.admin_ui_step="reset_all_confirm";
    return renderAdmin("♻️ مجوز تست همهٔ کاربران ریست شود؟ درخواست‌های در حال پردازش یا نیازمند بررسی برای جلوگیری از ساخت تکراری حفظ می‌شوند. سرویس‌های قبلی حذف نمی‌شوند.",kb([[btn("✅ تأیید ریست", "admin:reset_all_confirm")],[btn("❌ انصراف", "admin:root")]]));
  }
  if (d === "admin:reset_all_confirm") {
    if(st.admin_ui_step !== "reset_all_confirm") return answer("درخواست منقضی شده است.",true);
    delete st.admin_ui_step;
    await run(env,"INSERT OR IGNORE INTO bot_users(user_id) SELECT user_id FROM test_claims WHERE user_id > 0");
    const r=await run(env,"DELETE FROM test_claims WHERE COALESCE(created_at,'') NOT LIKE 'processing|%' AND COALESCE(created_at,'') NOT LIKE 'review|%'");
    const remaining=await first(env,"SELECT COUNT(*) AS n FROM test_claims");
    return renderAdmin(`✅ مجوز تست ${Number(r.meta.changes)} کاربر ریست شد. ${Number(remaining.n)} درخواست در حال پردازش یا نیازمند بررسی حفظ شد.`,adminBack());
  }
  if (/^admin:users:\d+$/.test(d)) {
    const page=Number(d.split(":")[2]); if(!Number.isSafeInteger(page)||page<0||page>1000000)return answer("صفحه نامعتبر است.",true);
    const users=await all(env,`${BOT_USERS_QUERY} SELECT ids.user_id, u.first_name, u.username, u.blocked FROM ids LEFT JOIN bot_users u ON u.user_id=ids.user_id WHERE ids.user_id>0 ORDER BY ids.user_id DESC LIMIT 11 OFFSET ?`,page*10);
    const shown=await Promise.all(users.slice(0,10).map(async u=>({...u,...await botUserIdentity(env,Number(u.user_id),u)})));
    const rows=shown.map(u=>[btn(`${u.blocked ? "⛔ " : "👤 "}${String(u.first_name || "نام نامشخص").slice(0,20)}${u.username ? ` (@${String(u.username).replace(/^@/,"").slice(0,20)})` : ""} — ${u.user_id}`,`admin:user:${u.user_id}`)]);
    const nav=[];if(page>0)nav.push(btn("◀️ قبلی",`admin:users:${page-1}`));if(users.length>10)nav.push(btn("بعدی ▶️",`admin:users:${page+1}`));if(nav.length)rows.push(nav);rows.push([btn("🔙 بازگشت","admin:root")]);
    return renderAdmin(`👥 <b>کاربران ربات — صفحه ${page+1}</b>\nکاربران قدیمی بر اساس رکوردهای موجود نمایش داده می‌شوند.`,kb(rows));
  }
  if (/^admin:user:\d+$/.test(d)) {
    const target=Number(d.split(":")[2]);if(!positiveId(String(target))||!await botUserExists(env,target))return answer("کاربر پیدا نشد.",true);
    const u=await botUserIdentity(env,target);
    const balance=await walletBalance(env,target); const name=esc(u?.first_name || "نام در دسترس نیست"), username=esc(u?.username ? "@"+String(u.username).replace(/^@/,"") : "ثبت نشده یا در دسترس نیست");
    const rows=[[btn("📦 سرویس‌ها و وضعیت",`admin:us:${target}:0`),btn("🧾 همه سفارش‌ها",`admin:uo:${target}:0`)],[btn("💰 کیف پول",`admin:wallet_view:${target}`),btn("📜 تراکنش‌ها",`admin:ut:${target}:0`)],[btn("✉️ پیام خصوصی",`admin:message_user:${target}`)]];
    if(!env.admins.has(target)) rows.push([btn(u?.blocked ? "✅ رفع مسدودی" : "⛔ مسدود کردن",`admin:ub:${target}:${u?.blocked ? 0 : 1}`)]);
    rows.push([btn("🔙 کاربران","admin:users:0")]);
    return renderAdmin(`👤 <b>${name}</b>\nشناسه: <code>${target}</code>\nنام کاربری: ${username}${u.usernameFromOrder ? " (از سفارش قدیمی؛ ممکن است تغییر کرده باشد)" : ""}\nدسترسی ربات: ${u?.blocked ? "مسدود" : "باز"}\nموجودی: ${toman(balance)}`,kb(rows));
  }
  if (/^admin:ub:\d+:[01]$/.test(d)) {
    const [, ,id,blocked]=d.split(":"),target=Number(id);
    if(!positiveId(id)||env.admins.has(target)||!await botUserExists(env,target))return answer("کاربر قابل تغییر نیست.",true);
    await run(env,"INSERT INTO bot_users(user_id,blocked) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET blocked=excluded.blocked",target,Number(blocked));
    return renderAdmin(blocked === "1" ? "⛔ دسترسی کاربر به ربات مسدود شد. سرویس‌های VPN او تغییری نکردند." : "✅ دسترسی کاربر به ربات باز شد.",adminBack(`admin:user:${target}`));
  }
  if (/^admin:u[ost]:\d+:\d+$/.test(d)) {
    const [,kind,id,pageText]=d.split(":"),target=Number(id),page=Number(pageText);
    if(!positiveId(id)||!Number.isSafeInteger(page)||page>1000000||!await botUserExists(env,target))return answer("درخواست نامعتبر است.",true);
    let rows, lines, title;
    if(kind === "uo") {
      rows=await all(env,"SELECT id,service_name,final_price,status,created_at FROM orders WHERE user_id=? AND NOT EXISTS(SELECT 1 FROM settings WHERE key='hidden_order:'||orders.id) ORDER BY id DESC LIMIT 11 OFFSET ?",target,page*10);
      lines=rows.slice(0,10).map(x=>`#${x.id} — ${esc(String(x.service_name || "—").slice(0,50))} — ${toman(x.final_price || 0)} — ${esc(adminOrderStatus(x.status))} — ${esc(adminTime(x.created_at))}`); title="همه سفارش‌ها";
    } else if(kind === "ut") {
      rows=await all(env,"SELECT delta,reason,created_at FROM wallet_entries WHERE user_id=? ORDER BY id DESC LIMIT 11 OFFSET ?",target,page*10);
      lines=rows.slice(0,10).map(x=>`${Number(x.delta)>0 ? "+" : "−"} ${toman(Math.abs(Number(x.delta)))} — ${walletReason(x.reason)} — ${esc(adminTime(x.created_at))}`); title="تراکنش‌های کیف پول";
    } else {
      rows=await all(env,"SELECT * FROM services WHERE user_id=? ORDER BY id DESC LIMIT 11 OFFSET ?",target,page*10);
      lines=await Promise.all(rows.slice(0,10).map(async x=>{let state;try{const a=await panelServiceAccount(env,x);if(!a)state="در پنل یافت نشد";else {const used=Math.max(0,Number(a.up)||0)+Math.max(0,Number(a.down)||0),quota=Number(a.totalGB)||0,expiry=Number(a.expiryTime)||0;state=a.enable===true&&(quota<=0||used<quota)&&(expiry<=0||Date.now()<expiry)?"فعال":"غیرفعال یا منقضی"}}catch{state="وضعیت نامشخص؛ پنل در دسترس نیست"}return `#${x.id} — ${esc(String(x.name || "—").slice(0,50))} — ${state}`;}));title="سرویس‌ها و وضعیت لحظه‌ای";
    }
    const nav=[];if(page>0)nav.push(btn("◀️ قبلی",`admin:${kind}:${target}:${page-1}`));if(rows.length>10)nav.push(btn("بعدی ▶️",`admin:${kind}:${target}:${page+1}`));const buttons=[];if(kind === "us")for(const service of rows.slice(0,10))buttons.push([btn(`مدیریت ${String(service.name || service.id).slice(0,35)}`,`admin:svc:${service.id}`)]);if(nav.length)buttons.push(nav);buttons.push([btn("🔙 بازگشت",`admin:user:${target}`)]);
    return renderAdmin(`<b>${title}</b> — کاربر <code>${target}</code>\nصفحه ${page+1}\n\n${lines.join("\n") || "موردی ثبت نشده است."}`,kb(buttons));
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
    let notified=false;
    try {const balance=await walletBalance(env,change.userId), result=await sendMsg(env,change.userId,`💰 موجودی کیف پول شما ${toman(Math.abs(change.delta))} ${change.delta>0 ? "افزایش" : "کاهش"} یافت.\nموجودی فعلی: <b>${toman(balance)}</b>`,backKb("wallet:home"));notified=!!result?.ok;}catch{}
    if(!notified)await sendMsg(env,chat,"⚠️ تغییر موجودی ثبت شد، اما ارسال اعلان به کاربر ناموفق بود.",adminBack());
    return showAdminWallet(env,chat,change.userId,true);
  }
  if (d === "admin:categories" || /^admin:categories:\d+$/.test(d)) {
    const page=Number(d.split(":")[2] || 0);if(!Number.isSafeInteger(page)||page>1000000)return answer("صفحه نامعتبر است.",true);
    const cats=await all(env,"SELECT id,name,active FROM bot_categories ORDER BY id DESC LIMIT 11 OFFSET ?",page*10);
    const rows=cats.slice(0,10).map(x=>[btn(`${Number(x.active) ? "🗂" : "⚠️"} ${String(x.name).slice(0,40)}`,`admin:category:${x.id}`)]);
    const nav=[];if(page>0)nav.push(btn("◀️ قبلی",`admin:categories:${page-1}`));if(cats.length>10)nav.push(btn("بعدی ▶️",`admin:categories:${page+1}`));if(nav.length)rows.push(nav);rows.push([btn("➕ افزودن دسته‌بندی","admin:category_add")],[btn("🔙 بازگشت","admin:root")]);
    return renderAdmin("🗂 دسته‌بندی‌ها و گروه‌های متصل به پنل:",kb(rows));
  }
  if(d === "admin:category_add")return prompt("category_name","","نام دسته‌بندی را بفرستید؛ سپس گروه پنل را انتخاب کنید.","admin:categories");
  if (/^admin:category:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]),cat=await getCategory(env,id);if(!cat)return answer("دسته‌بندی پیدا نشد.",true);
    return renderAdmin(`🗂 <b>${esc(cat.name)}</b>\nگروه پنل: <code>${esc(cat.group_ids)}</code>\nوضعیت: ${cat.active ? "فعال" : "نیازمند انتخاب دوباره گروه"}`,kb([[btn("✏️ تغییر گروه پنل",`admin:category_rebind:${id}`)],[btn("🗑 حذف دسته‌بندی",`admin:category_delete:${id}`)],[btn("🔙 بازگشت","admin:categories")]]));
  }
  if (/^admin:category_rebind:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]),cat=await getCategory(env,id);if(!cat)return answer("دسته‌بندی پیدا نشد.",true);
    st.admin_category_draft={id,name:cat.name,expires:Date.now()+30*60*1000};return showCategoryGroupChoices(env,chat,0);
  }
  if (/^admin:categorygroups:\d+$/.test(d)) {
    const page=Number(d.split(":")[2]);if(!st.admin_category_draft || Date.now()>st.admin_category_draft.expires || !Number.isSafeInteger(page)||page>1000000)return answer("درخواست منقضی شده است.",true);
    return showCategoryGroupChoices(env,chat,page);
  }
  if (/^admin:categorygroup:\d+$/.test(d)) {
    const draft=st.admin_category_draft,id=d.split(":")[2];if(!draft||Date.now()>draft.expires||!positiveId(id))return answer("درخواست منقضی است.",true);
    let identity;try{identity=await currentPanelIdentity(env);await groupCredentials(env,id,0)}catch(e){return answer(`گروه معتبر یا قابل استفاده نیست: ${String(e.message).slice(0,120)}`,true)}
    if(draft.id){const updated=await run(env,"UPDATE bot_categories SET group_ids=?,panel_identity=?,active=1 WHERE id=?",id,identity,draft.id);if(!updated.meta.changes)return answer("دسته‌بندی پیدا نشد.",true)}
    else await run(env,"INSERT INTO bot_categories(name,group_ids,panel_identity) VALUES(?,?,?)",draft.name,id,identity);
    delete st.admin_category_draft;return renderAdmin("✅ دسته‌بندی و گروه پنل ذخیره شدند.",adminBack("admin:categories"));
  }
  if (/^admin:category_delete:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]),cat=await getCategory(env,id);if(!cat)return answer("دسته‌بندی پیدا نشد.",true);
    const use=await first(env,"SELECT COUNT(*) AS n FROM bot_plan_specs WHERE category_id=?",id);if(use.n)return answer("ابتدا بسته‌های این دسته را جابه‌جا یا حذف کنید.",true);
    return renderAdmin(`دسته‌بندی «${esc(cat.name)}» حذف شود؟`,kb([[btn("✅ تأیید",`admin:category_confirm:${id}`)],[btn("❌ انصراف","admin:categories")]]));
  }
  if (/^admin:category_confirm:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]);const r=await run(env,"DELETE FROM bot_categories WHERE id=? AND NOT EXISTS(SELECT 1 FROM bot_plan_specs WHERE category_id=?)",id,id);
    return renderAdmin(r.meta.changes ? "✅ دسته‌بندی حذف شد." : "دسته‌بندی پیدا نشد یا دارای بسته است.",adminBack("admin:categories"));
  }
  if (/^admin:planchoices:\d+$/.test(d)) {
    const page=Number(d.split(":")[2]);if(!st.admin_plan_draft||Date.now()>st.admin_plan_draft.expires||!Number.isSafeInteger(page)||page>1000000)return answer("درخواست منقضی شده است.",true);
    return showPlanCategoryChoices(env,chat,page);
  }
  if (/^admin:planpick:\d+$/.test(d)) {
    const draft=st.admin_plan_draft,id=Number(d.split(":")[2]);if(!draft||Date.now()>draft.expires)return answer("درخواست منقضی شده است.",true);
    const cat=await getCategory(env,id);try{await assertCategory(env,cat);await groupCredentials(env,cat.group_ids,0)}catch(e){return answer(`دسته‌بندی معتبر نیست: ${String(e.message).slice(0,120)}`,true)}
    if(draft.categoryOnly) {
      const changed=await run(env,"UPDATE bot_plan_specs SET category_id=? WHERE plan_key=? AND EXISTS(SELECT 1 FROM plans WHERE key=?) AND EXISTS(SELECT 1 FROM bot_categories WHERE id=? AND active=1 AND group_ids=? AND panel_identity=?)",id,draft.key,draft.key,id,cat.group_ids,cat.panel_identity);
      if(!changed.meta.changes)return answer("بسته یا دسته‌بندی تغییر کرده است؛ دوباره تلاش کنید.",true);
      delete st.admin_plan_draft;return renderAdmin("✅ دسته‌بندی بسته تغییر کرد؛ قیمت، حجم و مدت آن حفظ شد.",adminBack(`admin:plan:${draft.key}`));
    }
    try {
      const old=await getPlan(env,draft.key);
      if(draft.editing && !old)return answer("بسته پیدا نشد.",true);
      if(!draft.editing && old)return answer("کلید بسته قبلاً ثبت شده است.",true);
      const sort=(await first(env,"SELECT COALESCE(MAX(sort),0)+1 AS n FROM plans")).n;
      const change=draft.editing ? env.DB.prepare("UPDATE plans SET gb=?,price=? WHERE key=?").bind(draft.gb,draft.price,draft.key) : env.DB.prepare("INSERT INTO plans(key,gb,price,sort) VALUES(?,?,?,?)").bind(draft.key,draft.gb,draft.price,sort);
      await env.DB.batch([change,env.DB.prepare("INSERT INTO bot_plan_specs(plan_key,category_id,days) VALUES(?,?,?) ON CONFLICT(plan_key) DO UPDATE SET category_id=excluded.category_id,days=excluded.days").bind(draft.key,id,draft.days)]);
    }catch{return answer("ذخیره بسته ناموفق بود؛ وضعیت را دوباره بررسی کنید.",true)}
    delete st.admin_plan_draft;return renderAdmin("✅ بسته با مدت و دسته‌بندی ذخیره شد.",adminBack("admin:plans"));
  }
  if (d === "admin:settings") {
    delete st.admin_ui_step; delete st.admin_ui_key;
    const rows = [];
    if (owner) rows.push([btn("➕ افزودن یا تنظیم اتصال پنل", "admin:panelsetup")]);
    rows.push([btn("🎁 دسته‌بندی سرویس تست", "admin:testcategories:0")]);
    rows.push(...Object.entries(META).map(([key, [, label]]) => [btn(label, `admin:setting:${key}`)]));
    rows.push([btn("🔙 بازگشت", "admin:root")]);
    return renderAdmin("⚙️ <b>تنظیمات</b>\nبرای دیدن و تغییر هر گزینه آن را انتخاب کنید:", kb(rows));
  }
  if (/^admin:testcategories:\d+$/.test(d)) {
    const page=Number(d.split(":")[2]);if(!Number.isSafeInteger(page)||page>1000000)return answer("صفحه نامعتبر است.",true);
    return showTestCategoryChoices(env,cq,page);
  }
  if (/^admin:testcategory:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]),cat=await getCategory(env,id);
    try { await assertCategory(env,cat);await groupCredentials(env,cat.group_ids,0); } catch(e) { return answer(`دسته‌بندی قابل استفاده نیست: ${String(e.message).slice(0,120)}`,true); }
    await run(env,"INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value","test_category_id",String(id));
    return renderAdmin(`✅ دسته‌بندی «${esc(cat.name)}» برای سرویس تست انتخاب شد.`,adminBack("admin:settings"));
  }
  if (d === "admin:panelsetup") {
    if (!owner) return answer("فقط ادمین اصلی اجازه تنظیم اتصال پنل را دارد.", true);
    await run(env, "DELETE FROM settings WHERE key=?", `panel_draft:${uid}`);
    return prompt("panel_url", "", "➕ آدرس کامل HTTPS پنل شامل پورت و مسیر پنل را بفرستید.\nمثال: <code>https://panel.example.com:2053/SECRET_PATH</code>", "admin:settings");
  }
  if (d.startsWith("admin:setting:")) {
    const key = d.slice("admin:setting:".length), meta = Object.hasOwn(META, key) ? META[key] : null;
    if (!meta) return answer("تنظیم یافت نشد.", true);
    if (key === "start_text") return prompt("setting",key,`✏️ <b>متن پیام شروع</b>\nمتن فعلی: <code>${esc(env.cfg.start_text || DEFAULT_START_TEXT)}</code>\n\nمتن جدید را بفرستید؛ برای نمایش نام کاربر از <code>{name}</code> استفاده کنید. حداکثر ${MAX_START_TEXT} نویسه؛ برای بازگشت به متن پیش‌فرض، - بفرستید.`,"admin:settings");
    if (key === "referral_discount") return prompt("setting",key,`🎁 <b>هدیه دعوت</b>\nمقدار فعلی: ${toman(env.cfg.referral_discount)}\nمبلغ جدید را به تومان بفرستید؛ عدد 0 هدیه را غیرفعال می‌کند. این هدیه تخفیف خرید بعدی معرف است و به کیف پول اضافه نمی‌شود.`,"admin:settings");
    if (key === "channel_username" || key === "channel_link") return prompt("setting",key,`📢 <b>عضویت اجباری</b>\nبرای بررسی عضویت کاربران، ابتدا ربات را به کانال اضافه و ادمین کانال کنید. یوزرنیم یا شناسهٔ کانال و لینک عضویت را در تنظیمات جداگانه وارد کنید.\n\n${key === "channel_username" ? "یوزرنیم کانال همراه @ یا شناسهٔ عددی کانال را بفرستید." : "لینک عضویت کانال را بفرستید."}\nمقدار فعلی: <code>${esc(env.cfg[key] || "خالی")}</code>\nبرای پاک‌کردن مقدار، - بفرستید.`,"admin:settings");
    if (key === "test_enabled") return renderAdmin(`🎁 سرویس تست اکنون ${env.cfg.test_enabled === "1" ? "فعال" : "غیرفعال"} است.`, kb([[btn("✅ فعال", "admin:test:1"),btn("⛔ غیرفعال", "admin:test:0")],[btn("🔙 بازگشت", "admin:settings")]]));
    if (key === 'alerts_enabled' || key === 'auto_delete_enabled') {
      const deletion=key==='auto_delete_enabled';
      return renderAdmin(`${deletion?'🗑 حذف خودکار اشتراک‌های منقضی':'🔔 هشدار اشتراک'} اکنون ${env.cfg[key]==='1'?'فعال':'خاموش'} است.\n${deletion?'پس از تعداد روز تنظیم‌شده از تاریخ انقضا، سرویس واجد شرایط از پنل و ربات حذف می‌شود؛ سفارش در حال بررسی از حذف جلوگیری می‌کند.':'به مشتری نزدیک انقضا یا پایان حجم پیام ارسال می‌شود.'}`,kb([[btn('✅ فعال',`admin:policy:${key}:1`),btn('⛔ خاموش',`admin:policy:${key}:0`)],[btn('🔙 بازگشت','admin:settings')]]));
    }
    if (['alerts_days','alerts_gb','auto_delete_days'].includes(key))return prompt('setting',key,`⚙️ <b>${esc(meta[1])}</b>\nمقدار فعلی: <code>${esc(env.cfg[key])}</code>\nمقدار جدید را با عدد انگلیسی بفرستید.${key==='alerts_days'||key==='alerts_gb'?' عدد ۰ این نوع هشدار را خاموش می‌کند.':''}${key==='auto_delete_days'?' عدد ۰ یعنی پس از پایان تاریخ بدون مهلت اضافه.':''}`,'admin:settings');
    if (["wallet_topup_min","wallet_topup_max"].includes(key))return prompt("setting",key,`💰 <b>${esc(meta[1])}</b>\nمقدار فعلی: ${key === "wallet_topup_max" && num(env,key)===0 ? "بدون سقف تنظیم‌شده" : toman(env.cfg[key])}\n\n${topupLimitsText(env)}\n\nمبلغ جدید را به تومان و با عدد انگلیسی بفرستید.${key === "wallet_topup_max" ? " برای برداشتن سقف، 0 بفرستید." : ""}`,"admin:settings");
    return prompt("setting", key, `⚙️ <b>${esc(meta[1])}</b>\nمقدار فعلی: <code>${esc(env.cfg[key] || "خالی")}</code>\n\nمقدار جدید را بفرستید.${meta[0] === "text" ? " برای پاک‌کردن مقدار، - بفرستید." : ""}`, "admin:settings");
  }
  if (/^admin:test:[01]$/.test(d)) {
    const value = d.slice(-1); await handleAdminCommand(c, {chat:{id:chat},from:cq.from,text:`/set test_enabled ${value}`});
    return showAdminHome(env, chat);
  }
  if (/^admin:policy:(alerts_enabled|auto_delete_enabled):[01]$/.test(d)) {
    const [, ,key,value]=d.split(':');
    await run(env,'INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value',key,value);
    return renderAdmin(value==='1'?'✅ فعال شد.':'⛔ خاموش شد.',adminBack('admin:settings'));
  }
  if (/^admin:namemode:(custom|random)$/.test(d)) {
    return answer("انتخاب نام اکنون هنگام خرید در اختیار مشتری است.",true);
  }
  if (d === "admin:plans") {
    delete st.admin_ui_step; delete st.admin_ui_key;
    const plans = await getPlans(env), safe = plans.filter(p => planKeyOK(p.key));
    const rows = safe.map(p => [btn(`${planLabel(p)} — ${p.days || "?"} روز — ${p.category_name || "بدون دسته"} — ${toman(p.price)}`, `admin:plan:${p.key}`)]);
    rows.push([btn("➕ افزودن بسته", "admin:plan_add")],[btn("🔙 بازگشت", "admin:root")]);
    return renderAdmin(`📦 <b>مدیریت بسته‌ها</b>${plans.length !== safe.length ? "\nبعضی کلیدهای قدیمی نامعتبرند؛ با دستورات قبلی اصلاحشان کنید." : ""}`, kb(rows));
  }
  if (d === "admin:plan_add") return prompt("plan_new", "", "➕ کلید انگلیسی، قیمت تومان، حجم گیگ و مدت روز را با فاصله بفرستید.\nمثال: <code>p10 70000 10 30</code>", "admin:plans");
  for (const [prefix, kind] of [["admin:plan:","show"],["admin:price:","price"],["admin:editplan:","edit"],["admin:planmove:","move"],["admin:delplan:","delete"],["admin:confirmplan:","confirm"]]) {
    if (!d.startsWith(prefix)) continue;
    const key = d.slice(prefix.length); if (!planKeyOK(key)) return answer("کلید بسته نامعتبر است.", true);
    const plan = await getPlan(env, key); if (!plan) return answer("بسته پیدا نشد.", true);
    if (kind === "show") return renderAdmin(`📦 <b>${esc(planLabel(plan))}</b>\nکلید: <code>${esc(key)}</code>\nقیمت: ${toman(plan.price)}\nمدت: ${plan.days || "تنظیم نشده"} روز\nدسته‌بندی: ${esc(plan.category_name || "تنظیم نشده")}`, kb([[btn("💰 تغییر قیمت", `admin:price:${key}`),btn("✏️ تغییر حجم و قیمت", `admin:editplan:${key}`)],[btn("🗂 تغییر دسته‌بندی", `admin:planmove:${key}`)],[btn("🗑 حذف بسته", `admin:delplan:${key}`)],[btn("🔙 بازگشت", "admin:plans")]]));
    if (kind === "price") return prompt("price", key, `قیمت جدید «${esc(key)}» را به تومان بفرستید.`, "admin:plans");
    if (kind === "edit") return prompt("plan_edit", key, `قیمت، حجم گیگ و مدت روز «${esc(key)}» را بفرستید. مثال: <code>70000 10 30</code>`, "admin:plans");
    if (kind === "move") {
      if (!Number.isSafeInteger(Number(plan.days)) || Number(plan.days)<=0) return answer("مدت بسته تنظیم نشده؛ ابتدا از ویرایش بسته مدت را وارد کنید.",true);
      st.admin_plan_draft={key,categoryOnly:true,expires:Date.now()+30*60*1000};return showPlanCategoryChoices(env,chat,0);
    }
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

  if (isAdmin && (c.st.admin_ui_step === "message_broadcast" || (text && ["panel_pass","message_private"].includes(c.st.admin_ui_step)))) return handleAdminUIInput(c, msg);
  if (text && text.startsWith("/start")) return handleStart(c, msg);
  if (isAdmin && text && ADMIN_CMD_RE.test(text)) return handleAdminCommand(c, msg);
  if (text && isAdmin && c.st.admin_step === "issue_message") return handleAdminText(c, msg);
  if (text && isAdmin && c.st.admin_ui_step) return handleAdminUIInput(c, msg);
  if (!isAdmin && !(await isMember(env, uid))) return sendMsg(env, msg.chat.id, "برای استفاده ابتدا در کانال عضو شوید و /start را بزنید.");

  const st = c.st;
  if (text) {
    if (st.step === "coupon_code") {
      const id=Number(st.order_id);if(!Number.isSafeInteger(id)||id<=0){delete st.step;return sendMsg(env,msg.chat.id,"سفارش پیدا نشد.",backKb())}
      try {
        const applied=await applyCoupon(env,id,uid,text);
        st.step="payment_choice";const order=await getOrder(env,id);
        return sendMsg(env,msg.chat.id,`${invoiceText(env,order)}\n\n🎟 کد ${esc(applied.code)} اعمال شد. این فاکتور تا ۳۰ دقیقه یا پایان اعتبار کد، هرکدام زودتر، رزرو است؛ پیش از پایان مهلت پرداخت و فیش را ارسال کنید.`,invoiceKb(id,true));
      } catch(e) {return sendMsg(env,msg.chat.id,`❌ ${esc(e.message)}\nدوباره کد بفرستید یا با دکمهٔ زیر انصراف دهید.`,backKb())}
    }
    if (st.step === "wallet_amount") return handleWalletAmount(c,msg);
    if (st.step === "wallet_receipt") return handleWalletReceipt(c,msg);
    if (st.step === "test_name") {
      const name=text.trim();
      if(!NAME_RE.test(name) || name.length>30) return sendMsg(env,msg.chat.id,"نام باید حداکثر ۳۰ حرف انگلیسی بدون فاصله و عدد باشد.");
      try {if(await panelAccount(env,name))return sendMsg(env,msg.chat.id,"این نام قبلاً ثبت شده است؛ نام دیگری وارد کنید.");} catch {return sendMsg(env,msg.chat.id,"بررسی نام در پنل ناموفق بود؛ دوباره تلاش کنید.");}
      st.test_name=name;st.step="test_name_ready";
      return sendMsg(env,msg.chat.id,`نام سرویس تست: <b>${esc(name)}</b>\nبرای ساخت، دکمه زیر را بزنید.`,kb([[btn("✅ ساخت سرویس تست","testname:make")],[btn("🔙 بازگشت","main_menu")]]));
    }
    if (st.step === "service_name" || st.step === "purchase_name_choice") {
      const name = text.trim();
      if (!NAME_RE.test(name)) {
        return sendMsg(env, msg.chat.id,
          "❌ نام نامعتبر است.\n\n⚠️ فقط حروف انگلیسی (A-Z) مجاز است، بدون فاصله و بدون هیچ علامتی.\n<i>مثال درست: Amir  یا  Mobile</i>");
      }
      if (name.length > 30) return sendMsg(env, msg.chat.id, "❌ نام سرویس خیلی طولانی است. کوتاه‌تر بنویسید.");
      try {if(await panelAccount(env,name))return sendMsg(env,msg.chat.id,"این نام قبلاً ثبت شده است؛ نام دیگری وارد کنید.");} catch {return sendMsg(env,msg.chat.id,"بررسی نام در پنل ناموفق بود؛ دوباره تلاش کنید.");}
      const plan = await getPlan(env, st.plan_key);
      if (!plan) { c.st = {}; return sendMsg(env, msg.chat.id, "❌ بسته یافت نشد، دوباره از منو اقدام کنید."); }
      try { await assertCategory(env,plan);await groupCredentials(env,plan.group_ids,0);if(!(await claimPurchaseChoice(env,st,uid)))return sendMsg(env,msg.chat.id,"این درخواست قبلاً ثبت شده یا منقضی است؛ بسته را دوباره انتخاب کنید.",backKb("buy"));return await createInvoice(c, msg.from, msg.chat.id, name, plan, null, null); }
      catch(e) { return sendMsg(env,msg.chat.id,`❌ بسته در دسترس نیست: ${esc(String(e.message).slice(0,160))}`,backKb("buy")); }
    }
    if (st.step === "receipt") return sendMsg(env,msg.chat.id,"لطفاً عکس رسید واریز را ارسال کنید.");
    if (st.step === "payment_choice") return sendMsg(env,msg.chat.id,"ابتدا روش پرداخت را با دکمهٔ فاکتور انتخاب کنید.");
    if (st.step === "invite_code_input") {
      if (num(env, "referral_discount") <= 0) { delete st.step; return sendMsg(env,msg.chat.id,"هدیه دعوت فعلاً غیرفعال است.",backKb()); }
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
    if (st.step === "receipt") { if(!msg.photo)return sendMsg(env,msg.chat.id,"لطفاً عکس رسید واریز را ارسال کنید.");return handleReceipt(c, msg); }
    if (st.step === "payment_choice") return sendMsg(env,msg.chat.id,"ابتدا روش پرداخت را با دکمهٔ فاکتور انتخاب کنید.");
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

  if (/^reject_order:\d+$/.test(d) || /^reject_order_yes:\d+$/.test(d)) {
    if(!isAdmin)return answer("دسترسی ندارید.",true);
    const [action,idText]=d.split(":"),id=Number(idText);if(!positiveId(idText))return answer("شناسه نامعتبر است.",true);
    const order=await getOrder(env,id);if(!order||order.status!=='pending')return answer("سفارش در انتظار تأیید نیست.",true);
    if(action==='reject_order')return render(env,cq,`سفارش <code>${id}</code> رد شود؟ اگر مبلغی واریز شده، بازگشت آن باید جداگانه بررسی شود.`,kb([[btn("✅ تأیید رد",`reject_order_yes:${id}`)],[btn("❌ انصراف","admin:root")]]));
    const results=await env.DB.batch([env.DB.prepare("UPDATE orders SET status='rejected' WHERE id=? AND status='pending'").bind(id),env.DB.prepare("UPDATE bot_coupon_uses SET state='released' WHERE order_id=? AND state='reserved' AND EXISTS(SELECT 1 FROM orders WHERE id=? AND status='rejected')").bind(id,id)]);
    if(!results[0]?.meta?.changes)return answer("سفارش قبلاً بررسی شده است.",true);
    try{await sendMsg(env,order.user_id,"❌ سفارش شما رد شد. اگر وجهی پرداخت کرده‌اید، برای پیگیری با پشتیبانی تماس بگیرید.",backKb())}catch{}
    return render(env,cq,"✅ سفارش رد و سهمیهٔ کد تخفیف آزاد شد. وضعیت وجه را جداگانه بررسی کنید.",adminBack());
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
      const committed=await env.DB.batch([env.DB.prepare("UPDATE orders SET status='approved' WHERE id=? AND status='processing'").bind(orderId),env.DB.prepare("UPDATE bot_coupon_uses SET state='consumed' WHERE order_id=? AND state='reserved' AND EXISTS(SELECT 1 FROM orders WHERE id=? AND status='approved')").bind(orderId,orderId)]);
      if(committed[0]?.meta && !committed[0].meta.changes)throw Error("ثبت وضعیت سفارش تأیید نشد");
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

  if (/^coupon:enter:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]);if(!positiveId(String(id)))return answer("سفارش نامعتبر است.",true);
    await expireCouponReservations(env);
    const order=await getOrder(env,id);if(!order || Number(order.user_id)!==uid || order.status!=='draft')return answer("سفارش منقضی شده یا قابل ویرایش نیست.",true);
    const applied=await first(env,"SELECT id FROM bot_coupon_uses WHERE order_id=? AND state IN ('reserved','consumed')",id);
    if(applied)return answer("برای این سفارش قبلاً کد تخفیف ثبت شده است.",true);
    st.order_id=id;st.step="coupon_code";
    return render(env,cq,"🎟 کد تخفیف را بفرستید. کد روی مبلغ باقی‌مانده پس از هدیهٔ دعوت حساب می‌شود و رزرو آن حداکثر ۳۰ دقیقه معتبر است.",backKb());
  }

  if (/^card:pay:\d+$/.test(d)) {
    const id=Number(d.split(":")[2]);if(!positiveId(String(id)))return answer("سفارش نامعتبر است.",true);
    await expireCouponReservations(env);
    const order=await getOrder(env,id);if(!order || Number(order.user_id)!==uid || order.status!=="draft")return answer("سفارش قابل پرداخت نیست.",true);
    if(!env.cfg.card_number || !env.cfg.card_holder)return answer("اطلاعات کارت هنوز تنظیم نشده است؛ با پشتیبانی تماس بگیرید.",true);
    st.order_id=id;st.step="receipt";
    return render(env,cq,`💳 <b>پرداخت کارت به کارت</b>\n\nشماره کارت: <code>${esc(env.cfg.card_number)}</code>\nبه نام: <b>${esc(env.cfg.card_holder)}</b>\nمبلغ: <b>${toman(order.final_price)}</b>\n\nپس از واریز، عکس رسید را همین‌جا بفرستید تا ادمین بررسی کند.`,backKb("main_menu"));
  }

  if (d === "wallet:home") {
    if(st.order_id)await cancelDraftOrder(env,Number(st.order_id),uid);
    delete st.step; delete st.order_id; delete st.wallet_topup_id;
    const balance=await walletBalance(env,uid);
    return render(env,cq,`💰 <b>کیف پول</b>\nموجودی: <b>${toman(balance)}</b>`,kb([[btn("➕ شارژ کیف پول", "wallet:topup")],[btn("📋 گردش کیف پول", "wallet:history")],[btn("🔙 بازگشت", "main_menu")]]));
  }
  if (d === "wallet:history") {
    const rows=await all(env,"SELECT delta,reason,created_at FROM wallet_entries WHERE user_id=? ORDER BY id DESC LIMIT 10",uid);
    const text=rows.length ? rows.map(x=>`${Number(x.delta)>0?"➕":"➖"} ${toman(Math.abs(Number(x.delta)))} — ${esc(walletReason(x.reason))}`).join("\n") : "تراکنشی ثبت نشده است.";
    return render(env,cq,`📋 <b>۱۰ تراکنش اخیر</b>\n\n${text}`,backKb("wallet:home"));
  }
  if (d === "wallet:topup") { st.step="wallet_amount"; return render(env,cq,`مبلغ شارژ را به تومان و با عدد انگلیسی بفرستید.\n\n${topupLimitsText(env)}`,backKb("wallet:home")); }
  if (d.startsWith("wallet:pay:")) {
    const id=Number(d.slice("wallet:pay:".length)); if(!Number.isSafeInteger(id)||id<=0) return answer("سفارش نامعتبر است.",true);
    await expireCouponReservations(env);
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

  if (d === "main_menu") { if(st.order_id)await cancelDraftOrder(env,Number(st.order_id),uid); delete st.order_id;delete st.step; delete st.plan_key;delete st.purchase_choice; delete st.test_category; delete st.test_name; delete st.admin_ui_step; delete st.admin_ui_key; delete st.admin_step; delete st.admin_order; delete st.admin_wallet_change; delete st.wallet_topup_id; await del(); return sendMainMenu(env, chat_id); }

  // --- سرویس تست (هر کاربر فقط یک‌بار)
  if (d === "test_service") {
    if (env.cfg.test_enabled !== "1") return answer("سرویس تست فعلا غیرفعال است.", true);
    const existing=await first(env,"SELECT created_at FROM test_claims WHERE user_id=?",uid);
    if(existing) return answer(String(existing.created_at || "").startsWith("processing|") || String(existing.created_at || "").startsWith("review|") ? "درخواست تست قبلی در حال پردازش یا بررسی است." : "شما قبلاً سرویس تست دریافت کرده‌اید.",true);
    delete st.test_name;delete st.step;delete st.test_category;
    const id=Number(env.cfg.test_category_id),cat=await getCategory(env,id);
    try {if(!Number.isSafeInteger(id)||id<=0)throw Error();await assertCategory(env,cat);await groupCredentials(env,cat.group_ids,0)} catch {return render(env,cq,"سرویس تست فعلاً آماده نیست؛ با پشتیبانی تماس بگیرید.",backKb());}
    return render(env,cq,"🎁 دسته‌بندی سرویس تست را انتخاب کنید:",kb([[btn(String(cat.name).slice(0,45),`testcat:${id}:0`)],[btn("🔙 بازگشت","main_menu")]]));
  }
  if (/^testcat:\d+:\d+$/.test(d)) {
    if(env.cfg.test_enabled!=="1")return answer("سرویس تست فعلا غیرفعال است.",true);
    const id=Number(d.split(":")[1]),cat=await getCategory(env,id);
    if(!id || id!==Number(env.cfg.test_category_id))return answer("دسته‌بندی تست تغییر کرده است؛ دوباره تلاش کنید.",true);
    try {await assertCategory(env,cat);await groupCredentials(env,cat.group_ids,0)} catch {return answer("این دسته‌بندی فعلاً در دسترس نیست.",true)}
    st.test_category=id;delete st.test_name;delete st.step;
    return render(env,cq,`🎁 دسته‌بندی «${esc(cat.name)}» انتخاب شد. نام سرویس تست را چگونه انتخاب می‌کنید؟`,kb([[btn("✏️ نام دلخواه", "testname:custom")],[btn("🎲 نام تصادفی", "testname:random")],[btn("🔙 دسته‌بندی‌ها", "test_service")]]));
  }
  if (d === "testname:custom") {
    if (env.cfg.test_enabled !== "1") return answer("سرویس تست فعلا غیرفعال است.",true);
    st.step="test_name";delete st.test_name;
    return render(env,cq,"✏️ نام سرویس تست را با حروف انگلیسی، بدون فاصله و عدد بفرستید؛ حداکثر ۳۰ حرف.",backKb("test_service"));
  }
  if (d === "testname:random" || d === "testname:make") {
    if (env.cfg.test_enabled !== "1") return answer("سرویس تست فعلا غیرفعال است.", true);
    if (d === "testname:make" && (st.step !== "test_name_ready" || !NAME_RE.test(st.test_name || "") || st.test_name.length>30)) return answer("ابتدا نام سرویس را انتخاب کنید.",true);
    const cat=await getCategory(env,Number(st.test_category));
    try{if(!Number(st.test_category)||Number(st.test_category)!==Number(env.cfg.test_category_id))throw Error();await assertCategory(env,cat)}catch{return answer("دسته‌بندی تست را دوباره انتخاب کنید.",true)}
    const testName=d === "testname:make" ? st.test_name : randomServiceName();
    const token = `processing|${hex(12)}|${now()}`;
    const claimed = await run(env, "INSERT OR IGNORE INTO test_claims(user_id,created_at) VALUES(?,?)", uid, token);
    if (!claimed.meta.changes) {
      const existing = await first(env, "SELECT created_at FROM test_claims WHERE user_id=?", uid);
      const state = String(existing?.created_at || "");
      if (state.startsWith("processing|")) return answer("درخواست تست شما در حال بررسی است؛ برای نتیجه با پشتیبانی تماس بگیرید.", true);
      if (state.startsWith("review|")) return answer("نتیجهٔ درخواست قبلی نیاز به بررسی دارد؛ با پشتیبانی تماس بگیرید.", true);
      return answer("شما قبلا سرویس تست دریافت کرده‌اید.", true);
    }
    delete st.step;delete st.test_name;
    let dispatched = false, p;
    const mb = num(env, "test_mb"), days = num(env, "test_days"), label = `${mb} مگابایت`;
    try {
      p = await panelCreate(env, { name: testName, bytes: mb * MB, days, groupIds:cat.group_ids,anchorId:0,attemptKey: `test:${uid}:${hex(8)}`, onDispatch: () => { dispatched = true; } });
      await insertService(env, { ...p, user_id: uid, name: testName, volume: label, duration: `${days} روزه` });
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
      const delivered = await sendServiceCard(env, chat_id, testName, label, `${days} روزه`, p.link, null, mainMenuKb(env), "🎁 سرویس تست شما ساخته شد!");
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

  if (d.startsWith("cfgmanage:") || d.startsWith("cfgset:") || d.startsWith("cfgnoop:")) return answer("مدیریت وضعیت کانفیگ فقط برای ادمین است.",true);

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

  if (d === "buy") { delete st.renew_service_id;if(['purchase_name_choice','service_name'].includes(st.step)){delete st.step;delete st.plan_key;delete st.purchase_choice;}return showCustomerCategories(env,cq,"buy"); }
  if (/^(buy|test)cats:\d+$/.test(d)) {
    const [type,pageText]=d.split(":"),page=Number(pageText);if(!Number.isSafeInteger(page)||page>1000000)return answer("صفحه نامعتبر است.",true);
    if(type==="testcats")return answer("برای دریافت تست دوباره وارد منوی سرویس تست شوید.",true);
    return showCustomerCategories(env,cq,type==="testcats"?"test":"buy",page);
  }
  if (/^buycat:\d+:\d+$/.test(d)) {
    const [,id,pageText]=d.split(":"),page=Number(pageText),cat=await getCategory(env,Number(id));
    if(!Number.isSafeInteger(page)||page>1000000)return answer("صفحه نامعتبر است.",true);
    try {await assertCategory(env,cat)}catch{return answer("دسته‌بندی در دسترس نیست.",true)}
    const plans=await all(env,`${PLAN_SELECT} WHERE s.category_id=? AND c.active=1 ORDER BY p.sort,p.price LIMIT 11 OFFSET ?`,Number(id),page*10);
    const rows=plans.slice(0,10).map(p=>[btn(`${planLabel(p)} — ${p.days} روز — ${toman(p.price)}`,`plan_${p.key}`)]);
    const nav=[];if(page>0)nav.push(btn("◀️ قبلی",`buycat:${id}:${page-1}`));if(plans.length>10)nav.push(btn("بعدی ▶️",`buycat:${id}:${page+1}`));if(nav.length)rows.push(nav);rows.push([btn("🔙 دسته‌بندی‌ها","buy")]);
    return render(env,cq,`📦 <b>${esc(cat.name)}</b>\n${plans.length ? "بسته را انتخاب کنید:" : "بسته‌ای در این دسته ثبت نشده است."}`,kb(rows));
  }
  if (d.startsWith("plan_")) {
    const plan = await getPlan(env, d.slice(5));
    if (!plan) return;
    try {await assertCategory(env,plan);await groupCredentials(env,plan.group_ids,0)} catch(e) { return answer(`بسته در دسترس نیست: ${String(e.message).slice(0,120)}`,true); }
    if (["receipt","payment_choice"].includes(st.step) && st.order_id) return answer("سفارش فعلی را تکمیل کنید یا به منوی اصلی برگردید.",true);
    st.plan_key = plan.key; st.step = "purchase_name_choice";st.purchase_choice=hex(12);
    await run(env,"INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",`purchase_choice:${uid}`,st.purchase_choice);
    return render(env,cq,`📦 بسته انتخابی: <b>${esc(planLabel(plan))}</b> — ${toman(plan.price)}\n\nنام سرویس را چگونه انتخاب می‌کنید؟`,kb([[btn("✏️ نام دلخواه","buyname:custom")],[btn("🎲 نام تصادفی","buyname:random")],[btn("🔙 بازگشت","buy")]]));
  }
  if (d === 'buyname:custom' || d === 'buyname:random') {
    if(st.step !== 'purchase_name_choice' || !st.plan_key)return answer('ابتدا بسته را دوباره انتخاب کنید.',true);
    const plan=await getPlan(env,st.plan_key);if(!plan)return answer('بسته پیدا نشد؛ دوباره انتخاب کنید.',true);
    try {await assertCategory(env,plan);await groupCredentials(env,plan.group_ids,0)}catch{return answer('بسته در دسترس نیست؛ دوباره انتخاب کنید.',true)}
    if(d === 'buyname:random'){if(!(await claimPurchaseChoice(env,st,uid)))return answer('این درخواست قبلاً ثبت شده یا منقضی است.',true);return createInvoice(c,cq.from,chat_id,randomServiceName(),plan,null,cq);}
    st.step='service_name';return render(env,cq,'✏️ نام دلخواه را با حروف انگلیسی، بدون فاصله، عدد و علامت بفرستید؛ حداکثر ۳۰ حرف.\nمثال: Amir',backKb('buy'));
  }

  if (d.startsWith("renew_")) {
    const id=Number(d.split("_")[1]),svc=await getService(env,id);if(!svc || svc.user_id!==uid)return answer("سرویس یافت نشد.",true);
    let account;try{account=await panelServiceAccount(env,svc)}catch{return sendMsg(env,chat_id,"⚠️ بررسی سرویس در پنل ناموفق بود.",backKb("my_services"))}
    if(!account)return answer("سرویس در پنل یافت نشد.",true);
    const candidates=(await getPlans(env)).filter(p=>Number(p.category_active)===1 && Number(p.days)===durationDays(svc.duration) && Math.round(Number(p.gb)*GB)===Number(svc.volume_bytes));
    const choices=await panelChoices(env);if(!choices)return answer("دریافت گروه‌ها از پنل ناموفق بود.",true);
    const valid=[];for(const p of candidates){try{if(await categoryMatchesAccount(env,p,account,choices))valid.push(p)}catch{}}
    if(!valid.length)return render(env,cq,"بسته‌ای با حجم، مدت و گروه فعلی این سرویس برای تمدید موجود نیست؛ با پشتیبانی تماس بگیرید.",backKb("my_services"));
    st.renew_service_id=id;return render(env,cq,`🔄 <b>تمدید سرویس «${esc(svc.name)}»</b>\nبسته مطابق سرویس را انتخاب کنید:`,await plansKb(env,"rplan",valid));
  }
  if (d.startsWith("rplan_")) {
    const plan=await getPlan(env,d.slice(6)),svc=st.renew_service_id ? await getService(env,st.renew_service_id) : null;
    if(!plan||!svc||svc.user_id!==uid)return answer("خطا در پردازش، دوباره تلاش کنید.",true);
    try {
      const account=await panelServiceAccount(env,svc);
      if(!account || !(await categoryMatchesAccount(env,plan,account)) || Number(plan.days)!==durationDays(svc.duration) || Math.round(Number(plan.gb)*GB)!==Number(svc.volume_bytes))return answer("این بسته با گروه، حجم یا مدت سرویس مطابقت ندارد.",true);
      return await createInvoice(c,cq.from,chat_id,svc.name,plan,svc.id,cq);
    } catch {return answer("بررسی پنل یا دسته‌بندی ناموفق بود.",true)}
  }

  if (d === "invite_code") {
    if (num(env, "referral_discount") <= 0) return render(env,cq,"هدیه دعوت فعلاً غیرفعال است.",backKb());
    return render(env, cq,
      `🎁 <b>کد دعوت شما</b>\n\n<code>${uid}</code>\n\nاین کد (آیدی عددی شما) را برای دوستانتان بفرستید.\n` +
      `هر کسی این کد را در ربات وارد و ثبت کند، <b>${toman(num(env, "referral_discount"))} تخفیف</b> برای یک خرید به حساب شما اضافه می‌شود.\n\n💡 روی کد بزنید تا کپی شود.`,
      backKb());
  }

  if (d === "enter_invite") {
    if (num(env, "referral_discount") <= 0) return render(env,cq,"هدیه دعوت فعلاً غیرفعال است.",backKb());
    st.step = "invite_code_input";
    return render(env, cq, "➕ <b>ثبت کد دعوت</b>\n\nآیدی عددی (کد دعوت) شخصی که معرفتان کرده را بفرستید:", backKb());
  }

  if (d === "support") {
    const sid = env.cfg.support_id;
    return render(env, cq, `🆘 <b>پشتیبانی</b>\n\nبرای ارتباط با پشتیبانی به آیدی زیر پیام دهید:\n${esc(sid)}`,
      kb([[urlBtn("💬 ارتباط با پشتیبانی", `https://t.me/${sid.replace(/^@/, "")}`)], [btn("🔙 بازگشت", "main_menu")]]));
  }

  if (d === "apps_menu") {
    const rows = Object.entries(env.apps).filter(([,i])=>i.enabled!==false).map(([k, i]) => [btn(`📱 ${i.title}`, `app_${k}`)]);
    rows.push([btn("🔙 بازگشت", "main_menu")]);
    return render(env, cq, "📲 <b>دانلود اپلیکیشن‌ها</b>\n\nاپلیکیشن مناسب دستگاه خود را انتخاب کنید:", kb(rows));
  }

  if (d.startsWith("app_")) {
    const info = env.apps[d.slice(4)];
    if (!info || info.enabled===false) return answer('اپلیکیشن در دسترس نیست.',true);
    const rows = info.links.map(([l, u]) => [urlBtn(l, u)]);
    rows.push([btn("🔙 بازگشت", "apps_menu")]);
    return render(env, cq, `📱 <b>${esc(info.title)}</b>\n\n${esc(info.text)}`, kb(rows));
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
  env.apps=Object.fromEntries(Object.entries(APPS).map(([key,app])=>[key,{...app,text:app.text.replace(/<\/?b>/g,''),enabled:true}]));
  for(const r of setR.results)if(r.key.startsWith('bot_app:')){const key=r.key.slice(8);if(!appKeyOK(key))continue;try{const app=JSON.parse(r.value);if(app.deleted){delete env.apps[key];continue}if(typeof app.title==='string' && typeof app.text==='string' && Array.isArray(app.links) && app.links.length && app.links.every(x=>Array.isArray(x)&&x.length===2))env.apps[key]=app}catch{}}
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

  const chat=update.message?.chat || update.callback_query?.message?.chat;
  if(chat?.type === "private" && chat.id === from.id && positiveId(String(from.id))) {
    await run(baseEnv,"INSERT INTO bot_users(user_id,first_name,username,last_seen) VALUES(?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET first_name=CASE WHEN excluded.first_name='' THEN bot_users.first_name ELSE excluded.first_name END,username=excluded.username,last_seen=excluded.last_seen",from.id,String(from.first_name || "").slice(0,128),String(from.username || "").slice(0,64),String(Date.now()));
    const access=await first(baseEnv,"SELECT blocked FROM bot_users WHERE user_id=?",from.id);
    if(access?.blocked && !env.admins.has(from.id)) {
      if(update.callback_query) { await tg(baseEnv,"answerCallbackQuery",{callback_query_id:update.callback_query.id,text:"دسترسی شما به ربات مسدود است.",show_alert:true}); update.callback_query._answered=true; }
      else await sendMsg(baseEnv,chat.id,"دسترسی شما به ربات مسدود است. با پشتیبانی تماس بگیرید.");
      return;
    }
  }
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

async function backgroundEnv(base) {
  const rows=await all(base,'SELECT key,value FROM settings');const env=Object.create(base);env.cfg=defaults(base);
  for(const r of rows)if(Object.hasOwn(env.cfg,r.key))env.cfg[r.key]=String(r.value ?? '');
  env.panelStoredIdentity=rows.find(r=>r.key==='panel_identity_v1')?.value || '';
  const saved=rows.find(r=>r.key==='panel_credentials_v1');
  if(saved){try{const p=await decryptPanel(base.BOT_TOKEN,saved.value);env.PANEL_URL=panelURL(p.url);env.PANEL_USER=p.user;env.PANEL_PASS=p.pass;if(!p.user||!p.pass)throw Error()}catch{env.PANEL_URL='';env.PANEL_USER='';env.PANEL_PASS='';}}
  env.admins=new Set([...parseIds(base.ADMIN_ID),...(await all(base,'SELECT user_id FROM admins')).map(r=>Number(r.user_id))]);
  return env;
}
async function automatedDelete(env,svc) {
  const lock=await run(env,"INSERT OR IGNORE INTO service_operations(service_id,kind,created_at) VALUES(?,?,?)",svc.id,'auto_delete_running',String(Date.now()));
  if(!lock.meta.changes)return;
  let dispatched=false,finished=false;
  try {
    const enabled=await first(env,"SELECT value FROM settings WHERE key='auto_delete_enabled'");if(enabled?.value!=='1')return;
    const grace=await first(env,"SELECT value FROM settings WHERE key='auto_delete_days'");const days=Number(grace?.value ?? defaults(env).auto_delete_days);
    const fresh=await panelServiceAccount(env,svc);const expiry=Number(fresh?.expiryTime);
    if(!fresh || !svc.sub_id || String(fresh.subId || '')!==String(svc.sub_id) || !Number.isSafeInteger(expiry) || expiry<=0 || !Number.isSafeInteger(days) || days<0 || expiry+days*DAY_MS>Date.now())return;
    const matches=await first(env,"SELECT COUNT(*) AS n FROM services WHERE lower(trim(panel_email))=lower(trim(?)) AND sub_id=?",svc.panel_email,svc.sub_id);if(Number(matches.n)!==1)return;
    const orders=await all(env,"SELECT o.id,o.status FROM orders o WHERE o.user_id=? AND (o.renew_service_id=? OR EXISTS(SELECT 1 FROM provision_attempts p WHERE p.sub_id=? AND p.email=? AND p.attempt_key LIKE 'order:'||o.id||':%'))",svc.user_id,svc.id,svc.sub_id,svc.panel_email);
    if(orders.some(o=>!['approved','rejected','cancelled'].includes(o.status)))return;
    if(env.policySignal?.aborted)return;
    dispatched=true;await panelDelete(env,svc,{expectedExpiry:expiry,beforeDelete:async()=>{
      if(env.policySignal?.aborted)throw Error('متوقف شد');
      const active=await first(env,"SELECT value FROM settings WHERE key='auto_delete_enabled'");if(active?.value!=='1')throw Error('حذف خودکار خاموش شده است');
      const currentGrace=await first(env,"SELECT value FROM settings WHERE key='auto_delete_days'");const liveDays=Number(currentGrace?.value ?? defaults(env).auto_delete_days);
      if(!Number.isSafeInteger(liveDays)||liveDays<0||expiry+liveDays*DAY_MS>Date.now())throw Error('مهلت حذف تغییر کرده است');
      const pending=await first(env,"SELECT COUNT(*) n FROM orders o WHERE o.user_id=? AND (o.renew_service_id=? OR EXISTS(SELECT 1 FROM provision_attempts p WHERE p.sub_id=? AND p.email=? AND p.attempt_key LIKE 'order:'||o.id||':%')) AND o.status NOT IN ('approved','rejected','cancelled')",svc.user_id,svc.id,svc.sub_id,svc.panel_email);
      if(Number(pending.n))throw Error('سفارش مرتبط در حال بررسی است');
    }});
    const changes=orders.map(o=>env.DB.prepare("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value").bind(`hidden_order:${o.id}`,'1'));
    changes.push(env.DB.prepare('DELETE FROM services WHERE id=? AND user_id=?').bind(svc.id,svc.user_id));
    await env.DB.batch(changes);finished=true;
  } catch(e) {
    if(dispatched){await run(env,"UPDATE service_operations SET kind='auto_delete_review' WHERE service_id=? AND kind='auto_delete_running'",svc.id);for(const id of env.admins){try{await sendMsg(env,id,`⚠️ حذف خودکار سرویس <code>${svc.id}</code> کامل تأیید نشد؛ وضعیت آن را در پنل و ربات بررسی کنید. تا بررسی، عملیات مجدد روی سرویس متوقف است.`)}catch{}}}
    console.log('automatic deletion needs review',svc.id,String(e.message).slice(0,100));
  } finally {if(!dispatched || finished)await run(env,'DELETE FROM service_operations WHERE service_id=? AND kind=?',svc.id,'auto_delete_running');}
}
async function notifyService(env,svc,account) {
  if(env.policySignal?.aborted)return;
  const expiry=Number(account.expiryTime),quota=Number(account.totalGB),used=Number(account.up)+Number(account.down),time=Date.now();
  if(account.enable!==true || !Number.isFinite(used) || used<0 || !Number.isFinite(quota) || quota<0 || !Number.isFinite(expiry))return;
  const key=`alert_state:${svc.id}`,row=await first(env,'SELECT value FROM settings WHERE key=?',key);let state={};try{state=JSON.parse(row?.value || '{}')}catch{}
  if(state.subId!==svc.sub_id)state={};
  if(state.expiry!==expiry)state.timeSent=false;
  if(Number.isFinite(state.lastUsed) && used<state.lastUsed && (state.lastUsed-used>=MB || used===0))state.volumeSent=false;
  state.subId=svc.sub_id;state.expiry=expiry;state.lastUsed=used;
  const days=num(env,'alerts_days'),gb=num(env,'alerts_gb');
  const timeDue=days>0 && expiry>time && expiry-time<=days*DAY_MS && !state.timeSent;
  const volumeDue=gb>0 && quota>0 && used<quota && quota-used<=gb*GB && !state.volumeSent;
  if(!timeDue && !volumeDue){await run(env,'INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value',key,JSON.stringify(state));return;}
  const enabled=await first(env,"SELECT value FROM settings WHERE key='alerts_enabled'");if(enabled?.value!=='1')return;
  const previous={...state};if(timeDue)state.timeSent=true;if(volumeDue)state.volumeSent=true;
  await run(env,'INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value',key,JSON.stringify(state));
  const parts=[`🔔 <b>هشدار سرویس ${esc(svc.name || '')}</b>`];
  if(timeDue)parts.push(`⏳ زمان باقی‌مانده: حدود ${Math.max(1,Math.ceil((expiry-time)/3600000))} ساعت`);
  if(volumeDue)parts.push(`📦 حجم باقی‌مانده: ${formatBytes(quota-used)}`);
  try{const result=await sendMsg(env,svc.user_id,parts.join('\n'),kb([[btn('🗂 مشاهده سرویس',`svc_${svc.id}`)]]));if(!result.ok)await run(env,'UPDATE settings SET value=? WHERE key=? AND value=?',JSON.stringify(previous),key,JSON.stringify(state));}catch{console.log('subscription alert delivery uncertain',svc.id)}
  if(!env.policySignal?.aborted)await new Promise(resolve=>setTimeout(resolve,45));
}
async function runSubscriptionPolicies(base,{signal}={}) {
  const env=await backgroundEnv(base);env.policySignal=signal;if(signal?.aborted)return;
  await run(env,"UPDATE service_operations SET kind='auto_delete_review' WHERE kind='auto_delete_running' AND CAST(created_at AS INTEGER)<? AND NOT EXISTS(SELECT 1 FROM settings WHERE key='subscription_policy_lease' AND CAST(value AS INTEGER)>?)",Date.now()-21600000,Date.now());
  if(env.cfg.alerts_enabled!=='1' && env.cfg.auto_delete_enabled!=='1')return;
  if(!env.PANEL_URL || !env.PANEL_USER || !env.PANEL_PASS)return;
  let lease=`${Date.now()+21600000}|${hex(8)}`;
  const acquired=await run(env,"INSERT INTO settings(key,value) VALUES('subscription_policy_lease',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value WHERE CAST(settings.value AS INTEGER)<?",lease,Date.now());
  if(!acquired.meta.changes)return;
  try {
    const cursor=await first(env,"SELECT value FROM settings WHERE key='subscription_policy_page'");let page=Number(cursor?.value || 1);if(!Number.isSafeInteger(page)||page<1||page>100000)page=1;
    const response=await panelCall(env,'GET',`/panel/api/clients/list?page=${page}&size=500`);
    if(!response.success || !Array.isArray(response.obj?.rows))throw Error('reading panel accounts failed');
    for(const account of response.obj.rows){
      if(signal?.aborted)break;
      const renewed=`${Date.now()+21600000}|${hex(8)}`;
      const heartbeat=await run(env,"UPDATE settings SET value=? WHERE key='subscription_policy_lease' AND value=?",renewed,lease);
      if(!heartbeat.meta.changes)throw Error('subscription policy lease expired');lease=renewed;
      if(!account.email || !account.subId)continue;
      const services=await all(env,"SELECT s.*,COALESCE(u.blocked,0) AS bot_blocked FROM services s LEFT JOIN bot_users u ON u.user_id=s.user_id WHERE lower(trim(s.panel_email))=lower(trim(?)) AND s.sub_id=?",account.email,account.subId);
      for(const svc of services){
        if(signal?.aborted)break;
        if(env.cfg.auto_delete_enabled==='1' && Number(account.expiryTime)>0 && Number(account.expiryTime)+num(env,'auto_delete_days')*DAY_MS<=Date.now()){await automatedDelete(env,svc);continue;}
        if(env.cfg.alerts_enabled==='1' && !svc.bot_blocked)await notifyService(env,svc,account);
      }
    }
    const next=response.obj.rows.length<500 || page*500>=Number(response.obj.total || 0) ? 1 : page+1;
    await run(env,"INSERT INTO settings(key,value) VALUES('subscription_policy_page',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",String(next));
  } finally {await run(env,"DELETE FROM settings WHERE key='subscription_policy_lease' AND value=?",lease);}
}
export { groupCredentials, panelCreate, panelRenew, panelDelete, panelAccount, panelServiceAccount, panelMemberships, liveServiceDetails, panelCall, handleUpdate, subscriptionLinks, configName, linkFingerprint, runSubscriptionPolicies };

export default {
  async scheduled(event, env, ctx) {
    ctx.waitUntil((async()=>{
      await expireCouponReservations(env);
      const recovered=await recoverWalletPayments(env);
      for(const item of recovered) {
        try { await sendMsg(env,item.userId,`⚠️ پرداخت سفارش ${item.orderId} به کیف پول برگشت و برای بررسی ارسال شد.`,backKb()); } catch {}
        for(const admin of parseIds(env.ADMIN_ID)) { try { await sendMsg(env,admin,`⚠️ پرداخت سفارش <code>${item.orderId}</code> پس از قطع عملیات برگشت خورد. احتمال انجام عملیات پنل را بررسی کنید.`); } catch {} }
      }
      if(!env.SERVER_MODE)await processMessageJobs(env,(id,body)=>{const p=queuedMessagePayload(id,body);return tg(env,p.method,p.params)});
      if(!env.SERVER_MODE)await runSubscriptionPolicies(env);
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
