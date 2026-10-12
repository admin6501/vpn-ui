const assert = require('node:assert/strict');
const fs = require('node:fs');
global.window = { location: { hostname: 'example.com' }, crypto: require('node:crypto').webcrypto };
global.document = { addEventListener() {} };
(0, eval)(fs.readFileSync('web/assets/js/util/index.js', 'utf8') + '\n' + fs.readFileSync('web/assets/js/model/inbound.js', 'utf8') + '\nglobalThis.TestInbound = Inbound; globalThis.TestStream = StreamSettings;');
const state = { settings: { version: 2 }, stream: { tls: { settings: { allowInsecure: true }, alpn: ['h3'], sni: 'vpn.example' }, finalmask: { udp: [{ type: 'salamander', settings: { password: 'obfs@?#%' } }] } } };
for (const address of ['vpn.example', '2001:db8::1', '[2001:db8::1]']) {
  const token = 'test@auth:/?#%';
  const link = TestInbound.prototype.genHysteriaLink.call(state, address, 8443, 'test user', token);
  const parsed = new URL(link);
  assert.equal(decodeURIComponent(parsed.username), token);
  assert.equal(parsed.password, '');
  assert.equal(parsed.port, '8443');
  assert.equal(parsed.searchParams.get('obfs-password'), 'obfs@?#%');
  assert.equal(parsed.searchParams.get('insecure'), '1');
}
console.log('Hysteria panel links preserve credentials, IPv6 and Salamander');

const stream = new TestStream();
stream.hysteriaObfsPassword = 'exact @?#% secret';
assert.equal(stream.finalmask.udp.length, 1);
assert.equal(stream.finalmask.udp[0].settings.password, 'exact @?#% secret');
stream.hysteriaObfsPassword = 'changed';
assert.equal(stream.finalmask.udp.length, 1, 'editing must not create a second mask');
const restored = TestStream.fromJson(stream.toJson());
assert.equal(restored.hysteriaObfsPassword, 'changed');
restored.addUdpMask('header-dns');
restored.hysteriaObfsPassword = '';
assert.equal(restored.hysteriaObfsPassword, '');
assert.equal(restored.finalmask.udp.length, 1, 'clearing obfs preserves unrelated masks');
assert.equal(restored.finalmask.udp[0].type, 'header-dns');
console.log('Hysteria obfs form model round trips and disables only Salamander');
