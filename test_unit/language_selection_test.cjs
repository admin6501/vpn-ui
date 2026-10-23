'use strict';
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const source = fs.readFileSync('web/assets/js/util/index.js', 'utf8');
let cookie = '', reloads = 0;
const document = {documentElement: {lang: ''}};
const window = {navigator:{languages:['ja-JP'],language:'ja-JP'},location:{reload(){reloads++;}}};
const manager = vm.runInNewContext(source.slice(source.indexOf('class LanguageManager {'), source.indexOf('const MediaQueryMixin'))+'\nLanguageManager', {
 document,window,CookieManager:{getCookie(){return cookie;},setCookie(key,value){cookie=value;}}
});
const expected = fs.readdirSync('web/translation').filter(x=>x.endsWith('.toml')).map(x=>x.slice(10,-5).replace('_','-')).sort();
assert.deepEqual(Array.from(manager.supportedLanguages,x=>x.value).sort(),expected);
for (const [input,output] of [['fa_IR','fa-IR'],['pt-PT','pt-BR'],['ZH_hk','zh-TW'],['zh-Hant','zh-TW'],['zh-MO','zh-TW'],['zh-SG','zh-CN'],['es-MX','es-ES'],['en-GB','en-US'],['de-DE','']]) {
 assert.equal(manager.normalizeLanguage(input),output,input);
}
cookie='garbage'; document.documentElement.lang='fa-IR';
assert.equal(manager.getLanguage(),'fa-IR','server resolved header wins over invalid cookie');
assert.equal(cookie,'fa-IR'); assert.equal(reloads,0,'initial selection must not reload away a valid server language');
cookie=''; document.documentElement.lang='';
assert.equal(manager.getLanguage(),'ja-JP','browser fallback if server has no language');
cookie='pt-PT'; document.documentElement.lang='en-US';
assert.equal(manager.getLanguage(),'pt-BR');
manager.setLanguage('zh-TW'); assert.equal(cookie,'zh-TW'); assert.equal(reloads,1);
manager.setLanguage('invalid'); assert.equal(cookie,'en-US'); assert.equal(reloads,2);
console.log('All 13 languages, regional aliases, cookie/header fallback and reload behavior passed');
