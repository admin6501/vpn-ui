import test from 'node:test';
import assert from 'node:assert/strict';
import { DatabaseSync } from 'node:sqlite';
import { readFileSync } from 'node:fs';
import { migratePlanModel } from '../plan-migration.mjs';
function db(){const d=new DatabaseSync(':memory:');d.exec(readFileSync(new URL('../schema.sql',import.meta.url),'utf8'));return d}
test('legacy packages and pending orders retain original duration and group exactly once',()=>{
 const d=db();d.exec("INSERT INTO plans VALUES('a',10,100,1); INSERT INTO settings VALUES('panel_group_ids','7,9'),('panel_inbound_id','31'),('duration_days','45'),('panel_identity_v1','oldpanel'); INSERT INTO orders(user_id,status,days) VALUES(2,'pending',45),(2,'approved',45)");
 migratePlanModel(d);migratePlanModel(d);
 assert.deepEqual({...d.prepare('SELECT group_ids,panel_identity FROM bot_categories').get()},{group_ids:'7,9',panel_identity:'oldpanel'});
 assert.equal(d.prepare('SELECT days FROM bot_plan_specs WHERE plan_key=?').get('a').days,45);
 assert.deepEqual({...d.prepare('SELECT order_id,group_ids,anchor_id,panel_identity FROM bot_order_routes').get()},{order_id:1,group_ids:'7,9',anchor_id:31,panel_identity:'oldpanel'});
 assert.equal(d.prepare('SELECT COUNT(*) n FROM bot_categories').get().n,1);d.close();
});
test('ambiguous legacy settings never invent a route or group',()=>{
 const d=db();d.exec("INSERT INTO plans VALUES('a',10,100,1); INSERT INTO orders(user_id,status) VALUES(2,'pending'); INSERT INTO settings VALUES('panel_group_ids','7'),('panel_inbound_id','')");migratePlanModel(d);
 for(const table of ['bot_plan_specs','bot_order_routes','bot_categories'])assert.equal(d.prepare(`SELECT COUNT(*) n FROM ${table}`).get().n,0);assert.ok(d.prepare("SELECT value FROM settings WHERE key='__plan_model_v1'").get());d.close();
});
