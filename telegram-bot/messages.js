// Admin messages use durable claims. A send with an unknown result is not retried blindly.
const run=(env,sql,...a)=>env.DB.prepare(sql).bind(...a).run();
const first=(env,sql,...a)=>env.DB.prepare(sql).bind(...a).first();
const all=async(env,sql,...a)=>(await env.DB.prepare(sql).bind(...a).all()).results;
const USER_IDS=`WITH ids AS (SELECT user_id FROM bot_users UNION SELECT user_id FROM states UNION SELECT user_id FROM services UNION SELECT user_id FROM orders UNION SELECT user_id FROM wallet_entries UNION SELECT user_id FROM wallet_topups UNION SELECT user_id FROM wallet_payments UNION SELECT user_id FROM discounts UNION SELECT user_id FROM test_claims UNION SELECT referrer_id AS user_id FROM referrals UNION SELECT referred_id AS user_id FROM referrals)`;
export async function createMessageJob(env,creator,audience,target,body){
 if(!Number.isSafeInteger(creator)||creator<=0||!['broadcast','private'].includes(audience)||typeof body!=='string'||!body.trim()||body.length>3500||(audience==='private'&&(!Number.isSafeInteger(target)||target<=0)))throw Error('پیام یا گیرنده معتبر نیست.');
 const result=await run(env,"INSERT INTO bot_message_jobs(creator_id,audience,target_id,body,status,created_ms) VALUES(?,?,?,?,'draft',?)",creator,audience,target||null,body,Date.now());return Number(result.meta.last_row_id);
}
export async function confirmMessageJob(env,id,creator){
 const job=await first(env,"SELECT id,audience,target_id FROM bot_message_jobs WHERE id=? AND creator_id=? AND status='draft'",id,creator);if(!job)return false;
 const populate=job.audience==='private'
  ? env.DB.prepare("INSERT OR IGNORE INTO bot_message_targets(job_id,user_id,status) SELECT ?,?,'pending' WHERE EXISTS(SELECT 1 FROM bot_message_jobs WHERE id=? AND creator_id=? AND status='building')").bind(id,job.target_id,id,creator)
  : env.DB.prepare(`${USER_IDS} INSERT OR IGNORE INTO bot_message_targets(job_id,user_id,status) SELECT ?,ids.user_id,'pending' FROM ids LEFT JOIN bot_users u ON u.user_id=ids.user_id WHERE ids.user_id>0 AND COALESCE(u.blocked,0)=0 AND EXISTS(SELECT 1 FROM bot_message_jobs WHERE id=? AND creator_id=? AND status='building')`).bind(id,id,creator);
 const r=await env.DB.batch([
  env.DB.prepare("UPDATE bot_message_jobs SET status='building' WHERE id=? AND creator_id=? AND status='draft'").bind(id,creator),populate,
  env.DB.prepare("UPDATE bot_message_jobs SET status=CASE WHEN EXISTS(SELECT 1 FROM bot_message_targets WHERE job_id=?) THEN 'queued' ELSE 'done' END,finished_ms=CASE WHEN EXISTS(SELECT 1 FROM bot_message_targets WHERE job_id=?) THEN NULL ELSE ? END WHERE id=? AND creator_id=? AND status='building'").bind(id,id,Date.now(),id,creator),
 ]);
 return Number(r[0]?.meta?.changes)>0;
}
export async function processMessageJobs(env,send,{signal,now=()=>Date.now(),limit=20}={}){
 await run(env,"UPDATE bot_message_targets SET status='uncertain' WHERE status='sending' AND claimed_ms<?",now()-120000);
 const job=await first(env,"SELECT * FROM bot_message_jobs WHERE status='queued' ORDER BY id LIMIT 1");if(!job)return;
 if(job.audience==='broadcast')await run(env,"UPDATE bot_message_targets SET status='failed' WHERE job_id=? AND status='pending' AND EXISTS(SELECT 1 FROM bot_users u WHERE u.user_id=bot_message_targets.user_id AND u.blocked=1)",job.id);
 const recipients=await all(env,"SELECT user_id FROM bot_message_targets WHERE job_id=? AND status='pending' ORDER BY user_id LIMIT ?",job.id,limit);
 for(const r of recipients){
  if(signal?.aborted)break;
  const claimed=await run(env,"UPDATE bot_message_targets SET status='sending',claimed_ms=? WHERE job_id=? AND user_id=? AND status='pending' AND EXISTS(SELECT 1 FROM bot_message_jobs WHERE id=? AND status='queued')",now(),job.id,r.user_id,job.id);
  if(!claimed.meta.changes)continue;
  let state='uncertain';try{const result=await send(r.user_id,job.body,signal);state=result?.ok?'sent':'failed'}catch{}
  await run(env,"UPDATE bot_message_targets SET status=? WHERE job_id=? AND user_id=? AND status='sending'",state,job.id,r.user_id);
  if(!signal?.aborted)await new Promise(resolve=>setTimeout(resolve,55));
 }
 const done=await run(env,"UPDATE bot_message_jobs SET status='done',finished_ms=? WHERE id=? AND status='queued' AND NOT EXISTS(SELECT 1 FROM bot_message_targets WHERE job_id=? AND status IN ('pending','sending'))",now(),job.id,job.id);
 if(done.meta.changes){const stats=await first(env,"SELECT COUNT(CASE WHEN status='sent' THEN 1 END) sent,COUNT(CASE WHEN status='failed' THEN 1 END) failed,COUNT(CASE WHEN status='uncertain' THEN 1 END) uncertain FROM bot_message_targets WHERE job_id=?",job.id);try{await send(job.creator_id,`ارسال پیام ${job.id} پایان یافت. موفق: ${stats.sent}، ناموفق: ${stats.failed}، نامشخص: ${stats.uncertain}.`,signal)}catch{}}
}
