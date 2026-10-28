// Migrate existing bot-owned plans and in-flight orders without touching panel data.
export function migratePlanModel(db) {
 db.exec('BEGIN IMMEDIATE');
 try {
  const done=db.prepare("SELECT value FROM settings WHERE key='__plan_model_v1'").get();
  if(!done){
   const setting=key=>db.prepare('SELECT value FROM settings WHERE key=?').get(key)?.value || '';
   const groups=setting('panel_group_ids'),anchor=Number(setting('panel_inbound_id'));
   const days=Number(setting('duration_days') || '30');
   const validGroups=/^\d+(,\d+)*$/.test(groups)&&groups.split(',').every(n=>Number.isSafeInteger(Number(n))&&Number(n)>0);
   const validAnchor=Number.isSafeInteger(anchor)&&anchor>0;
   const identity=setting('panel_identity_v1');
   if(validGroups && validAnchor){
    db.prepare("INSERT OR IGNORE INTO bot_order_routes(order_id,group_ids,anchor_id,panel_identity) SELECT id,?,?,? FROM orders WHERE status NOT IN ('approved','wallet_review_resolved')").run(groups,anchor,identity);
    const count=db.prepare('SELECT COUNT(*) AS n FROM plans').get().n;
    if(count && Number.isSafeInteger(days)&&days>0&&days<=36500){
     const cat=db.prepare("INSERT INTO bot_categories(name,group_ids,panel_identity) VALUES('دسته قبلی',?,?)").run(groups,identity);
     db.prepare('INSERT INTO bot_plan_specs(plan_key,category_id,days) SELECT key,?,? FROM plans').run(Number(cat.lastInsertRowid),days);
    }
   }
   db.prepare("INSERT INTO settings(key,value) VALUES('__plan_model_v1','1')").run();
  }
  db.exec('COMMIT');
 }catch(e){db.exec('ROLLBACK');throw e}
}
