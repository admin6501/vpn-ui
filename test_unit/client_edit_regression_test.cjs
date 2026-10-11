const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync('web/html/modals/client_membership_modal.html', 'utf8');
function method(name) {
  const start = source.indexOf('    ' + name);
  assert.ok(start >= 0);
  const end = source.indexOf('\n    },', start);
  return vm.runInNewContext('({' + source.slice(start, end) + '\n}})');
}
const quota = method('get quotaBytes()');
const getter = Object.getOwnPropertyDescriptor(quota, 'quotaBytes').get;
const unit = method('setQuotaUnit(unit)');
const state = { totalGb: 100, quotaUnit: 'MB' };
Object.defineProperty(state, 'quotaBytes', {get: getter});
assert.equal(state.quotaBytes, 104857600);
unit.setQuotaUnit.call(state, 'GB');
assert.equal(state.quotaBytes, 104857600);
unit.setQuotaUnit.call(state, 'MB');
assert.equal(state.totalGb, 100);
state.totalGb = 0.5;
assert.equal(state.quotaBytes, 524288);
assert.ok(!source.includes('Math.round(row.totalGB / 1073741824)'));
assert.ok(!/client.email"\s*:disabled/.test(source));
const info = fs.readFileSync('web/html/modals/account_info_modal.html', 'utf8');
const start = info.indexOf('      onlineProtocols() {');
const end = info.indexOf('\n      },', start);
const online = vm.runInNewContext('({' + info.slice(start, end) + '\n}})');
const protocols = online.onlineProtocols.call({row:{},app:{membershipOnline:(r)=>r.id===2},memberships:[{record:{id:1,protocol:'vless'},client:{email:'a'}},{record:{id:2,protocol:'trojan'},client:{email:'a'}}]});
assert.equal(JSON.stringify(protocols), '["trojan"]');
console.log('Client quota round trips, editable identity, and online protocol evidence passed');

const speed = {limitDown:'12345',limitDownUnit:'KB/s', ...method('speedFactor(unit)'), ...method('setSpeedUnit(field, unit)'), ...method('postedSpeed(field)')};
for (const unit of ['Mbps','MB/s','KB/s']) {
  speed.setSpeedUnit('limitDown',unit);
  assert.equal(speed.postedSpeed('limitDown'),12345);
}
speed.limitDown='10'; speed.limitDownUnit='Mbps';
assert.equal(speed.postedSpeed('limitDown'),1221);
speed.limitDown='0'; assert.equal(speed.postedSpeed('limitDown'),0);
speed.limitDown=''; assert.equal(speed.postedSpeed('limitDown'),null);
speed.limitDown='0.0001'; assert.equal(speed.postedSpeed('limitDown'),1);
speed.limitDown='-0.0001'; assert.equal(speed.postedSpeed('limitDown'),-1);
speed.limitDown='invalid'; assert.equal(speed.postedSpeed('limitDown'),'invalid');
console.log('Speed units preserve limits, inheritance and unlimited state');
