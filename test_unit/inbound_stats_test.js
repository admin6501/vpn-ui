// Execute the shipped page's aggregate function with shared-account fixtures.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const page = fs.readFileSync('web/html/inbounds.html', 'utf8');
const start = page.lastIndexOf('      total() {');
const end = page.indexOf('\n      }\n    },', start);
assert.ok(start > 0 && end > start);
const aggregate = vm.runInNewContext('(function () {' + page.slice(start + '      total() {'.length, end) + '\n})');
const inbound = (id, emails) => ({ id, up: 10, down: 20, allTime: 30, toInbound: () => ({ clients: emails.map(email => ({ email })) }) });
const counts = { clients: 2, deactive: ['second'], depleted: ['Shared'], expiring: ['second'] };
const state = { dbInbounds: [inbound(1, ['Shared', 'second']), inbound(2, [' shared ', 'second'])], clientCount: { 1: counts, 2: { ...counts, depleted: ['shared'] } } };
const total = aggregate.call(state);
assert.equal(total.clients, 2, 'shared memberships must not inflate total accounts');
assert.equal(total.depleted.length, 1, 'shared expiry/quota counts once');
assert.equal(total.expiring.length, 1);
assert.equal(total.deactive.length, 1);
assert.equal(total.up, 20, 'inbound traffic still adds across different listeners');
assert.equal(total.down, 40);
const publishStart=page.indexOf('      setInbounds(dbInbounds) {');
const publishEnd=page.indexOf('\n      },',publishStart);
const publish=vm.runInNewContext('(function(dbInbounds){'+page.slice(publishStart+'      setInbounds(dbInbounds) {'.length,publishEnd)+'\n})',{
 DBInbound:class {constructor(row){Object.assign(this,row);this.isSS=row.protocol==='shadowsocks';this.isSSMultiUser=true}toInbound(){return {isSSMultiUser:true,clients:[{email:this.protocol+'-client',enable:true}]}}}
});
const protocols=['vless','vmess','trojan','shadowsocks','wireguard','wg-c','awg','hysteria','tuic','anytls','naive','ssh','openvpn','ikev2','l2tp','pptp','sstp','openconnect','mtproto','gre'];
const rows=protocols.map((protocol,i)=>({id:1000+i,protocol,enable:true}));
const view={refreshAccountSummary(){},getClientCounts(record,config){return {clients:config.clients.length,online:[config.clients[0].email]}},loadingStates:{fetched:false},enableFilter:false,searchKey:'',searchInbounds(){}};
publish.call(view,rows);
for(const row of rows){assert.equal(view.clientCount[row.id].clients,1,'all account-bearing protocols contribute counters: '+row.protocol);assert.equal(view.clientCount[row.id].online[0],row.protocol+'-client')}
publish.call(view,[rows[0]]);
assert.equal(Object.keys(view.clientCount).length,1,'replacing inbound list retires removed counters');
assert.equal(view.clientCount[rows[0].id].online.length,1,'edit must republish live count');
assert.match(page, /\/panel\/api\/server\/userStats/, 'the page shares the Overview account summary');
assert.match(page, /accountSummary\.expired/, 'time expiry is displayed separately from quota depletion');
console.log('Inbound unique-account counters passed.');
