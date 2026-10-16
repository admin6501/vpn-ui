const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const timers = new Map();
let nextID = 0;
const context = vm.createContext({console, document:{hidden:false},
  setTimeout:(fn,ms)=>{const id=++nextID;timers.set(id,{fn,ms});return id;},
  clearTimeout:id=>timers.delete(id)});
vm.runInContext(fs.readFileSync('web/assets/js/util/index.js','utf8')+'\nthis.RefreshQueue=RefreshQueue;this.RefreshLoop=RefreshLoop;',context);
const flush=()=>new Promise(resolve=>setImmediate(resolve));
(async()=>{
 const queue=new context.RefreshQueue();
 let release,calls=0,active=0,maxActive=0,value='before',display;
 const task=async()=>{calls++;active++;maxActive=Math.max(maxActive,active);const snapshot=value;if(calls===1)await new Promise(r=>release=r);display=snapshot;active--;};
 const first=queue.request(task);await flush();
 value='saved';
 const followers=Array.from({length:20},()=>queue.request(task));
 release();await Promise.all([first,...followers]);
 assert.equal(calls,2,'overlapping invalidations should produce one follow-up');
 assert.equal(maxActive,1);assert.equal(display,'saved','post-save callers received pre-save data');
 await assert.rejects(queue.request(async()=>{throw Error('offline')}));
 await queue.request(task);assert.equal(display,'saved');
 let ticks=0,interval=5000;
 const loop=new context.RefreshLoop(async()=>{ticks++},()=>interval);
 for(let i=0;i<12;i++)loop.start();
 assert.equal(timers.size,1,'reconnects created duplicate polling chains');
 let [id,timer]=[...timers][0];timers.delete(id);await timer.fn();
 assert.equal(ticks,1);assert.equal(timers.size,1);
 interval=2000;[id,timer]=[...timers][0];timers.delete(id);await timer.fn();
 assert.equal([...timers.values()][0].ms,2000);
 loop.stop();assert.equal(timers.size,0);
 await timer.fn();assert.equal(ticks,2,'stopped timer refreshed the page');
 loop.start(true);context.document.hidden=true;
 [id,timer]=[...timers][0];timers.delete(id);await timer.fn();assert.equal(ticks,2,'hidden page kept polling');
 loop.stop();context.document.hidden=false;loop.start();assert.equal(timers.size,1);loop.stop();
 console.log('Refresh queue: single request, post-save follow-up, failure recovery; polling: reconnects, toggles, interval, hidden page and cleanup passed');
})().catch(error=>{console.error(error);process.exit(1)});
