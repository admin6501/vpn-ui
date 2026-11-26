(function () {
  // Vue app for Subscription page
  const el = document.getElementById('subscription-data');
  if (!el) return;
  const textarea = document.getElementById('subscription-links');
  const rawLinks = (textarea?.value || '').split('\n').filter(Boolean);

  const data = {
    sId: el.getAttribute('data-sid') || '',
    subUrl: el.getAttribute('data-sub-url') || '',
    subJsonUrl: el.getAttribute('data-subjson-url') || '',
    subClashUrl: el.getAttribute('data-subclash-url') || '',
    download: el.getAttribute('data-download') || '',
    upload: el.getAttribute('data-upload') || '',
    used: el.getAttribute('data-used') || '',
    total: el.getAttribute('data-total') || '',
    remained: el.getAttribute('data-remained') || '',
    expireMs: (parseInt(el.getAttribute('data-expire') || '0', 10) || 0) * 1000,
    lastOnlineMs: (parseInt(el.getAttribute('data-lastonline') || '0', 10) || 0),
    downloadByte: parseInt(el.getAttribute('data-downloadbyte') || '0', 10) || 0,
    uploadByte: parseInt(el.getAttribute('data-uploadbyte') || '0', 10) || 0,
    totalByte: parseInt(el.getAttribute('data-totalbyte') || '0', 10) || 0,
    enabled: el.getAttribute('data-enabled') !== 'false',
    datepicker: el.getAttribute('data-datepicker') || 'gregorian',
  };

  // Normalize lastOnline to milliseconds if it looks like seconds
  if (data.lastOnlineMs && data.lastOnlineMs < 10_000_000_000) {
    data.lastOnlineMs *= 1000;
  }

  function renderLink(item) {
    return (
      Vue.h('a-list-item', {}, [
        Vue.h('a-space', { props: { size: 'small' } }, [
          Vue.h('a-button', { props: { size: 'small' }, on: { click: () => copy(item) } }, [Vue.h('a-icon', { props: { type: 'copy' } })]),
          Vue.h('span', { class: 'break-all' }, item)
        ])
      ])
    );
  }

  function copy(text) {
    ClipboardManager.copyText(text).then(ok => {
      const messageType = ok ? 'success' : 'error';
      Vue.prototype.$message[messageType](ok ? 'Copied' : 'Copy failed');
    });
  }

  function open(url) {
    window.location.href = url;
  }

  function drawQR(value) {
    try {
      new QRious({ element: document.getElementById('qrcode'), value, size: 220 });
    } catch (e) {
      console.warn(e);
    }
  }

  // Try to extract a human label (email/ps) from different link types
  function linkName(link, idx) {
    try {
      const fragment = link.indexOf('#');
      if (fragment >= 0 && fragment < link.length - 1) return decodeURIComponent(link.slice(fragment + 1));
      if (link.startsWith('vmess://')) {
        const encoded = atob(link.slice('vmess://'.length));
        const json = JSON.parse(new TextDecoder().decode(Uint8Array.from(encoded, char => char.charCodeAt(0))));
        if (json.ps) return json.ps;
        if (json.add && json.id) return json.add; // fallback host
      } else if (link.startsWith('vless://') || link.startsWith('trojan://')) {
        const qIdx = link.indexOf('?');
        if (qIdx !== -1) {
          const qs = new URL('http://x/?' + link.substring(qIdx + 1, fragment !== -1 ? fragment : undefined)).searchParams;
          if (qs.get('remark')) return qs.get('remark');
          if (qs.get('email')) return qs.get('email');
        }
        const at = link.indexOf('@');
        const protSep = link.indexOf('://');
        if (at !== -1 && protSep !== -1) return link.substring(protSep + 3, at);
      } else if (link.startsWith('tg://')) {
        // tg://proxy?server=HOST&port=..&secret=.. -> label with the server host.
        const qs = new URL('http://x/?' + link.substring(link.indexOf('?') + 1)).searchParams;
        const host = qs.get('server');
        return host ? 'MTProto (' + host + ')' : 'MTProto';
      } else if (link.startsWith('ssh://')) {
        // ssh://base64(user:pass@host:port)[#label] -> prefer the #label, else the host.
        const decoded = atob(link.substring('ssh://'.length));
        const at = decoded.lastIndexOf('@');
        return 'SSH' + (at !== -1 ? ' (' + decoded.substring(at + 1) + ')' : '');
      } else if (link.startsWith('wireguard://')) {
        // wg-c/awg: wireguard://<privkey>@host:port?..#remark -> the remark names the
        // device; fall back to the endpoint so a link without one still reads.
        const at = link.indexOf('@');
        const qIdx = link.indexOf('?');
        return at !== -1 ? 'WireGuard (' + link.substring(at + 1, qIdx === -1 ? undefined : qIdx) + ')'
                         : 'WireGuard';
      }
    } catch (e) { /* ignore and fallback */ }
    return 'Link ' + (idx + 1);
  }

  const app = new Vue({
    delimiters: ['[[', ']]'],
    el: '#app',
    data: {
      themeSwitcher,
      app: data,
      links: rawLinks,
      lang: document.documentElement.lang,
      viewportWidth: (typeof window !== 'undefined' ? window.innerWidth : 1024),
    },
    async mounted() {
      const tpl = document.getElementById('subscription-data');
      const sj = tpl ? tpl.getAttribute('data-subjson-url') : '';
      const sc = tpl ? tpl.getAttribute('data-subclash-url') : '';
      if (sj) this.app.subJsonUrl = sj;
      if (sc) this.app.subClashUrl = sc;
      drawQR(this.app.subUrl);
      try {
        const elJson = document.getElementById('qrcode-subjson');
        if (elJson && this.app.subJsonUrl) {
          new QRious({ element: elJson, value: this.app.subJsonUrl, size: 220 });
        }
        const elClash = document.getElementById('qrcode-subclash');
        if (elClash && this.app.subClashUrl) {
          new QRious({ element: elClash, value: this.app.subClashUrl, size: 220 });
        }
      } catch (e) { /* ignore */ }
      this._onResize = () => { this.viewportWidth = window.innerWidth; };
      window.addEventListener('resize', this._onResize);
    },
    beforeDestroy() {
      if (this._onResize) window.removeEventListener('resize', this._onResize);
    },
    computed: {
      isMobile() {
        return this.viewportWidth < 576;
      },
      isActive() {
        const now = Date.now();
        const expiryOk = !this.app.expireMs || this.app.expireMs > now;
        const trafficOk = !this.app.totalByte || (this.app.uploadByte + this.app.downloadByte) < this.app.totalByte;
        return this.app.enabled && expiryOk && trafficOk;
      },
    },
    methods: {
      setSubscriptionLanguage(lang) {
        const page = new URL(window.location.href);
        page.searchParams.set('lang', lang);
        window.location.assign(page.href);
      },
      renderLink,
      copy,
      open,
      linkName,
      i18nLabel(key) {
        return '{{ i18n "' + key + '" }}';
      },
    },
  });
})();
