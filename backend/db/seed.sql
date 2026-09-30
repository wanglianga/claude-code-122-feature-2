-- 种子数据（演示环境）
BEGIN;

-- ---------- 账号（密码均为 demo123） ----------
INSERT INTO app_users (id, username, display_name, role, org_id, password_sha) VALUES
 (1,'admin','平台管理员','admin',NULL,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791'),
 (2,'brewery','周厂长(酒厂仓库)','brewery',NULL,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791'),
 (3,'qc','程品控','qc',NULL,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791'),
 (4,'finance','钱会计(财务)','finance',NULL,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791'),
 (10,'bar1','阿May(泡沫实验室店长)','bar_manager',4,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791'),
 (11,'bar2','老陈(醉猫酒馆店长)','bar_manager',5,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791'),
 (12,'bar3','祥子(胡同里店长)','bar_manager',6,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791'),
 (13,'bar4','小唐(春熙小馆店长)','bar_manager',7,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791'),
 (20,'driver1','王师傅(上海司机)','driver',1,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791'),
 (21,'driver2','李师傅(北京司机)','driver',2,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791'),
 (30,'wash1','上海净桶中心','washplant',1,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791'),
 (40,'dealer1','华东精酿经销·林经理','dealer',1,'d3ad9315b7be5dd53b31a273b3b3aba5defe700808305aa16a3062b76658a791');
SELECT setval(pg_get_serial_sequence('app_users','id'), 100);

-- ---------- 城市 / 经销商 / 酒吧 / 司机 / 清洗厂 ----------
INSERT INTO cities (id, name) VALUES (1,'上海'),(2,'北京'),(3,'成都');

INSERT INTO dealers (id, city_id, name, contact) VALUES
 (1,1,'华东精酿经销','林经理 13800000001'),
 (2,2,'京味酒水经销','赵经理 13800000002');

INSERT INTO bars (id, city_id, dealer_id, name, address, manager, phone) VALUES
 (4,1,1,'泡沫实验室 Taproom','上海市静安区愚园路123号','阿May','13911110001'),
 (5,1,1,'醉猫酒馆','上海市徐汇区永康路45号','老陈','13911110002'),
 (6,2,2,'胡同里 Beer Bar','北京市东城区方家胡同8号','祥子','13911110003'),
 (7,3,NULL,'春熙小馆','成都市锦江区春熙路66号','小唐','13911110004');

INSERT INTO drivers (id, city_id, name, phone) VALUES
 (1,1,'王师傅','13722220001'),
 (2,2,'李师傅','13722220002');

INSERT INTO washplants (id, city_id, name) VALUES
 (1,1,'上海净桶中心'),(2,2,'北京洗桶站');

-- ---------- 酒款 / 批次 ----------
INSERT INTO beers (id, name, style, abv) VALUES
 (1,'雾都 IPA','IPA',6.2),
 (2,'黑海世涛','世涛',8.1),
 (3,'云朵小麦','小麦',5.0),
 (4,'莓果酸啤','酸啤',4.3);

INSERT INTO batches (id, beer_id, batch_no, brewed_date, filled_date, best_before_days, total_kegs, status) VALUES
 (101,1,'BT2026-IPA-01','2026-08-15','2026-08-20',30,40,'NORMAL'),
 (102,2,'BT2026-ST-01', '2026-08-18','2026-08-25',45,30,'NORMAL'),
 (103,3,'BT2026-WT-01', '2026-08-28','2026-09-01',25,30,'NORMAL'),
 (104,4,'BT2026-SOUR-01','2026-08-05','2026-08-10',20,24,'SUSPENDED');
UPDATE batches SET suspend_reason='多店反馈酸感异常、疑似污染，暂停销售待品控复检', suspended_at=now() WHERE id=104;
SELECT setval(pg_get_serial_sequence('beers','id'), 10);
SELECT setval(pg_get_serial_sequence('batches','id'), 200);

-- ---------- 酒桶（30 只） ----------
INSERT INTO kegs (id, keg_code, capacity_liters, asset_status)
SELECT g, 'KEG-' || lpad(g::text,4,'0'),
       CASE WHEN g <= 12 THEN 30 ELSE 20 END,
       'IN_STOCK'
FROM generate_series(1,30) g;
SELECT setval(pg_get_serial_sequence('kegs','id'), (SELECT max(id) FROM kegs));

-- ---------- 订单 ----------
INSERT INTO orders (id, order_no, bar_id, dealer_id, driver_id, city_id, is_festival, deposit_term_days, status, created_by, delivered_at) VALUES
 (501,'ORD-0901',4,1,1,1,FALSE,30,'DELIVERED',2,'2026-09-05 10:00+08'),
 (502,'ORD-0902',5,1,1,1,FALSE,7, 'DELIVERED',2,'2026-09-08 11:00+08'),
 (503,'ORD-0903',6,2,2,2,FALSE,30,'DELIVERED',2,'2026-09-10 09:30+08'),
 (504,'ORD-0820',6,2,2,2,FALSE,30,'DELIVERED',2,'2026-08-20 15:00+08'),
 (505,'ORD-0905',4,1,1,1,FALSE,30,'DELIVERED',2,'2026-09-01 10:30+08'),
 (506,'ORD-0910',7,NULL,NULL,3,TRUE, 15,'PLANNED',2,NULL),
 (507,'ORD-0911',4,1,NULL,1,FALSE,30,'PLANNED',2,NULL);
UPDATE orders SET festival_note='中秋国庆双节临时铺货，节后统一回收' WHERE id=506;
SELECT setval(pg_get_serial_sequence('orders','id'), 600);

-- ---------- 桶流转周期 ----------
-- 501 泡沫实验室：IPA 未开/已开/打空待回收；世涛已开口味异常
INSERT INTO keg_cycles (id,keg_id,batch_id,order_id,bar_id,driver_id,city_id,deposit_amount,deposit_due_date,stage,bar_receiver,tapped_at) VALUES
 (901 ,1,101,501,4,1,1,700,'2026-10-05','DELIVERED','阿May',NULL),
 (902 ,2,101,501,4,1,1,700,'2026-10-05','TAPPED','阿May','2026-09-12 20:00+08'),
 (903 ,3,101,501,4,1,1,700,'2026-10-05','EMPTY','阿May','2026-09-11 19:00+08'),
 (904 ,4,102,501,4,1,1,700,'2026-10-05','TAPPED','阿May','2026-09-13 21:00+08'),
 (912 ,12,102,501,4,1,1,700,'2026-10-05','WASHED','阿May','2026-09-06 18:00+08');
UPDATE keg_cycles SET emptied_at='2026-09-20 23:00+08', return_reserved_at='2026-09-24 12:00+08', stage='RESERVED' WHERE id=903;
UPDATE keg_cycles SET picked_at='2026-09-22 14:00+08', deposit_status='RELEASED', released_amount=700, settled_by=4, settled_at='2026-09-23 10:00+08' WHERE id=912;

-- 502 醉猫酒馆：账期 7 天 → 已逾期；世涛第二家投诉；酸啤(停售批次)未开桶
INSERT INTO keg_cycles (id,keg_id,batch_id,order_id,bar_id,driver_id,city_id,deposit_amount,deposit_due_date,stage,bar_receiver,tapped_at) VALUES
 (905 ,5, 102,502,5,1,1,700,'2026-09-15','TAPPED','老陈','2026-09-14 20:30+08'),
 (906 ,6, 101,502,5,1,1,700,'2026-09-15','TAPPED','老陈','2026-09-12 19:30+08'),
 (907 ,7, 101,502,5,1,1,700,'2026-09-15','EMPTY','老陈','2026-09-16 22:00+08'),
 (908 ,8, 104,502,5,1,1,700,'2026-09-15','DELIVERED','老陈',NULL);

-- 503 胡同里：小麦已开；回收在途；世涛清洗不合格
INSERT INTO keg_cycles (id,keg_id,batch_id,order_id,bar_id,driver_id,city_id,deposit_amount,deposit_due_date,stage,bar_receiver,tapped_at) VALUES
 (909 ,9, 103,503,6,2,2,700,'2026-10-10','TAPPED','祥子','2026-09-15 20:00+08'),
 (910 ,10,103,503,6,2,2,700,'2026-10-10','RETURNED','祥子','2026-09-14 19:00+08'),
 (911 ,11,102,503,6,2,2,700,'2026-10-10','FAILED','祥子','2026-09-12 18:00+08');
UPDATE keg_cycles SET emptied_at='2026-09-19 23:00+08', picked_at='2026-09-20 10:00+08' WHERE id=910;
UPDATE keg_cycles SET emptied_at='2026-09-18 23:00+08', picked_at='2026-09-19 10:00+08',
                      deposit_status='PARTIAL', released_amount=550, deducted_amount=150,
                      deduct_reason='桶体变形+明显残液+封盖损坏，清洗厂检测不合格',
                      settled_by=4, settled_at='2026-09-22 16:00+08', deformed=TRUE WHERE id=911;

-- 504 老订单：世涛桶丢失，押金全扣，逾期
INSERT INTO keg_cycles (id,keg_id,batch_id,order_id,bar_id,driver_id,city_id,deposit_amount,deposit_due_date,stage,bar_receiver,tapped_at) VALUES
 (913 ,13,102,504,6,2,2,500,'2026-09-19','EMPTY','祥子','2026-08-25 20:00+08');
UPDATE keg_cycles SET emptied_at='2026-09-01 23:00+08', lost=TRUE, late_return=TRUE,
                      deposit_status='DEDUCTED', released_amount=0, deducted_amount=500,
                      deduct_reason='超账期未归还且酒桶确认丢失', settled_by=4, settled_at='2026-09-24 11:00+08'
WHERE id=913;

-- 505 泡沫实验室：小麦桶正常回收清洗合格已退款；另一只桶司机漏扫
INSERT INTO keg_cycles (id,keg_id,batch_id,order_id,bar_id,driver_id,city_id,deposit_amount,deposit_due_date,stage,bar_receiver,tapped_at) VALUES
 (914 ,14,103,505,4,1,1,500,'2026-10-01','WASHED','阿May','2026-09-08 18:00+08'),
 (915 ,15,101,505,4,1,1,500,'2026-10-01','RETURNED','阿May','2026-09-09 18:00+08');
UPDATE keg_cycles SET emptied_at='2026-09-20 23:50+08', picked_at='2026-09-22 13:00+08',
                      deposit_status='RELEASED', released_amount=500, settled_by=4, settled_at='2026-09-23 10:00+08' WHERE id=914;
UPDATE keg_cycles SET emptied_at='2026-09-21 23:00+08', picked_at='2026-09-22 13:00+08', missing_scan=TRUE WHERE id=915;

-- 502 醉猫酒馆逾期冻结场景（账期 7 天，2026-09-15 到期）：
-- 916 KEG-0016：早打空但一直未回收，平台已按桶冻结押金并催店长预约，店长已上传"已打空"现场照片
INSERT INTO keg_cycles (id,keg_id,batch_id,order_id,bar_id,driver_id,city_id,deposit_amount,deposit_due_date,stage,bar_receiver,tapped_at) VALUES
 (916 ,16,101,502,5,1,1,700,'2026-09-15','EMPTY','老陈','2026-09-14 20:00+08'),
 (917 ,17,101,502,5,1,1,700,'2026-09-15','RESERVED','老陈','2026-09-13 21:00+08');
UPDATE keg_cycles SET emptied_at='2026-09-18 22:00+08', late_return=TRUE,
                      deposit_status='FROZEN', deposit_frozen_at='2026-09-22 10:00+08'
WHERE id=916;
UPDATE keg_cycles SET emptied_at='2026-09-17 23:00+08', return_reserved_at='2026-09-21 12:00+08', late_return=TRUE,
                      deposit_status='FROZEN', deposit_frozen_at='2026-09-22 10:00+08'
WHERE id=917;

SELECT setval(pg_get_serial_sequence('keg_cycles','id'), 1000);

-- 订单明细（含待配送订单：停售批次酸啤仍挂在节日铺货订单上）
INSERT INTO order_items (order_id, batch_id, keg_id, qty) VALUES
 (501,101,1,1),(501,101,2,1),(501,101,3,1),(501,102,4,1),(501,102,12,1),
 (502,102,5,1),(502,101,6,1),(502,101,7,1),(502,104,8,1),(502,101,16,1),(502,101,17,1),
 (503,103,9,1),(503,103,10,1),(503,102,11,1),
 (504,102,13,1),
 (505,103,14,1),(505,101,15,1),
 (506,104,NULL,2),(506,101,NULL,2),
 (507,103,NULL,2);

-- ---------- 酒桶资产状态同步 ----------
UPDATE kegs k SET asset_status='AT_BAR', current_bar_id=c.bar_id, current_cycle_id=c.id
FROM keg_cycles c WHERE c.keg_id=k.id AND c.stage IN ('DELIVERED','TAPPED','EMPTY','RESERVED');
UPDATE kegs k SET asset_status='PICKED', current_bar_id=c.bar_id, current_cycle_id=c.id
FROM keg_cycles c WHERE c.keg_id=k.id AND c.id IN (910,915);
UPDATE kegs k SET asset_status='IN_STOCK', current_bar_id=NULL, current_cycle_id=c.id
FROM keg_cycles c WHERE c.keg_id=k.id AND c.id=914;
UPDATE kegs k SET asset_status='IN_STOCK', current_bar_id=NULL, current_cycle_id=c.id, refillable=FALSE
FROM keg_cycles c WHERE c.keg_id=k.id AND c.id=912;
UPDATE kegs k SET asset_status='WASH_FAIL', current_bar_id=NULL, current_cycle_id=c.id, refillable=FALSE
FROM keg_cycles c WHERE c.keg_id=k.id AND c.id=911;
UPDATE kegs k SET asset_status='LOST', current_bar_id=c.bar_id, current_cycle_id=c.id
FROM keg_cycles c WHERE c.keg_id=k.id AND c.id=913;
-- 916/917 已打空，资产为空桶待回收（覆盖上方按 stage 的批量 AT_BAR 更新）
UPDATE kegs SET asset_status='EMPTY_RESERVED' WHERE id IN (16,17);

-- ---------- 开桶反馈 ----------
INSERT INTO tap_feedback (cycle_id,bar_id,reported_by,pour_speed,pressure,taste_off,taste_note,complaint) VALUES
 (902,4,'阿May','偏慢','气压过低',FALSE,NULL,'客人等酒时间长'),
 (904,4,'阿May','正常','正常',TRUE,'入口发酸，后段有纸板味','两桌客人反映世涛不像平时的味道'),
 (905,5,'老陈','偏慢','波动',TRUE,'明显酸味，酒体寡淡','熟客投诉这批世涛口感异常，要求退酒'),
 (906,5,'老陈','正常','正常',FALSE,NULL,NULL),
 (909,6,'祥子','正常','正常',FALSE,NULL,NULL);

-- ---------- 回收预约 ----------
INSERT INTO return_reservations (id,cycle_id,bar_id,driver_id,pickup_date,empty_count,note,status) VALUES
 (801,903,4,NULL,'2026-09-27',1,'傍晚到店，巷口不好停车','WAITING'),
 (802,914,4,1,'2026-09-22',2,'两只小麦/IPA 一起收（其中一只司机漏扫）','PICKED'),
 (803,910,6,2,'2026-09-20',1,NULL,'PICKED'),
 (804,911,6,2,'2026-09-19',1,'桶身有凹陷','PICKED'),
 -- 司机按约到店，但酒吧未把空桶备好（KEG-0005 仍在售卖）→ 到场记录影响逾期责任，倾向酒吧责任
 (805,905,5,1,'2026-09-20',1,'世涛桶还没打空，让司机白跑一趟','BAR_NOT_READY'),
 -- 酒吧已预约且备好空桶，司机当天未到店 → 到场失败影响逾期责任，倾向司机责任
 (806,917,5,1,'2026-09-23',1,'空桶已放门口，等司机上门','DRIVER_FAILED');
UPDATE return_reservations SET visit_note='到店后店长称 KEG-0005 世涛还在卖、空桶未整理，请改日再来',
                               visited_at='2026-09-20 16:10+08' WHERE id=805;
UPDATE return_reservations SET visit_note='店长反馈全天未见司机到场，电话未接',
                               visited_at='2026-09-23 19:30+08' WHERE id=806;
SELECT setval(pg_get_serial_sequence('return_reservations','id'), 900);

INSERT INTO reservation_cycles (reservation_id, cycle_id) VALUES
 (801,903),(802,914),(802,915),(803,910),(804,911),(805,905),(806,917);

-- ---------- 司机到场记录（逾期责任判定依据） ----------
INSERT INTO driver_visits (reservation_id,cycle_id,bar_id,driver_id,result,note,visited_at,created_by) VALUES
 (805,905,5,1,'BAR_NOT_READY','王师傅 16:10 到店，KEG-0005 仍在售卖、空桶未备好','2026-09-20 16:10+08',20),
 (806,917,5,1,'DRIVER_FAILED','预约 9/23 取桶，司机未到店且联系不上','2026-09-23 19:30+08',11);

-- ---------- 逾期冻结后店长上传的现场照片 ----------
INSERT INTO empty_proofs (cycle_id,bar_id,photo_data,note,created_by,created_at) VALUES
 (916,5,
  'data:image/svg+xml;base64,PHN2ZyB4bWxucz0naHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmcnIHdpZHRoPSczMjAnIGhlaWdodD0nMjQwJz48cmVjdCB3aWR0aD0nMTAwJScgaGVpZ2h0PScxMDAlJyBmaWxsPScjZjRlZmU0Jy8+PHJlY3QgeD0nMTA1JyB5PSc2MCcgd2lkdGg9JzExMCcgaGVpZ2h0PScxMzAnIHJ4PScxNCcgZmlsbD0nbm9uZScgc3Ryb2tlPScjOGE3YTVhJyBzdHJva2Utd2lkdGg9JzQnLz48ZWxsaXBzZSBjeD0nMTYwJyBjeT0nNjAnIHJ4PSc1NScgcnk9JzEyJyBmaWxsPSdub25lJyBzdHJva2U9JyM4YTdhNWEnIHN0cm9rZS13aWR0aD0nNCcvPjxsaW5lIHgxPScxMjAnIHkxPScxMDUnIHgyPScyMDAnIHkyPScxMDUnIHN0cm9rZT0nI2M5Yjk4Zicgc3Ryb2tlLXdpZHRoPSczJy8+PGxpbmUgeDE9JzEyMCcgeTE9JzE0NScgeDI9JzIwMCcgeTI9JzE0NScgc3Ryb2tlPScjYzliOThmJyBzdHJva2Utd2lkdGg9JzMnLz48dGV4dCB4PScxNjAnIHk9JzIyMicgZm9udC1zaXplPScxNScgdGV4dC1hbmNob3I9J21pZGRsZScgZmlsbD0nIzVhNGEyYSc+S0VHLTAwMTYg5bey5omT56m6IMK3IOmGieeMq+WQjuWOqCAyMDI2LTA5LTE5PC90ZXh0Pjwvc3ZnPg==',
  'KEG-0016 9/18 就打空了，一直放后厨等回收，麻烦尽快安排司机',11,'2026-09-22 15:20+08'),
 (917,5,
  'data:image/svg+xml;base64,PHN2ZyB4bWxucz0naHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmcnIHdpZHRoPSczMjAnIGhlaWdodD0nMjQwJz48cmVjdCB3aWR0aD0nMTAwJScgaGVpZ2h0PScxMDAlJyBmaWxsPScjZWVmMmVlJy8+PHJlY3QgeD0nNzAnIHk9JzkwJyB3aWR0aD0nODAnIGhlaWdodD0nOTUnIHJ4PScxMCcgZmlsbD0nbm9uZScgc3Ryb2tlPScjNmE3YTZhJyBzdHJva2Utd2lkdGg9JzQnLz48cmVjdCB4PScxNzUnIHk9JzcwJyB3aWR0aD0nODAnIGhlaWdodD0nMTE1JyByeD0nMTAnIGZpbGw9J25vbmUnIHN0cm9rZT0nIzZhN2E2YScgc3Ryb2tlLXdpZHRoPScjNScvPjxjaXJjbGUgY3g9JzExMCcgY3k9JzkwJyByPSc5JyBmaWxsPScjNmE3YTZhJy8+PGNpcmNsZSBjeD0nMjE1JyBjeT0nNzAnIHI9JzknIGZpbGw9JyM2YTdhNmEnLz48dGV4dCB4PScxNjAnIHk9JzIyMCcgZm9udC1zaXplPScxNCcgdGV4dC1hbmNob3I9J21pZGRsZScgZmlsbD0nIzNhNGEzYSc+7Lik5Y+q56m65qGm5bey5pW055CG5b6F5pi2IMK3IDIwMjYtMDktMjQ8L3RleHQ+PC9zdmc+',
  '9/23 约了取桶没等到司机，空桶至今在店门口',11,'2026-09-24 09:40+08');

-- ---------- 司机扫码 ----------
INSERT INTO pickup_scans (cycle_id,reservation_id,driver_id,scanned_code,appearance,residue_level,cap_status,scanned_at) VALUES
 (914,802,1,'KEG-0014','完好','无','已封盖','2026-09-22 13:05+08'),
 -- 同次预约应收 2 只，KEG-0015 无扫码记录 → 漏扫
 (910,803,2,'KEG-0010','完好','少量','已封盖','2026-09-20 10:10+08'),
 (911,804,2,'KEG-0011','变形','明显残液','封盖损坏','2026-09-19 10:20+08'),
 (912,NULL,1,'KEG-0012','完好','无','已封盖','2026-09-22 14:00+08');

-- ---------- 清洗厂检测 ----------
INSERT INTO wash_checks (id,cycle_id,washplant_id,inspector,passed,pressure_ok,seal_ok,cleanliness_ok,defect_note,checked_at) VALUES
 (701,912,1,'刘师傅',TRUE,TRUE,TRUE,TRUE,NULL,'2026-09-23 09:00+08'),
 (702,914,1,'刘师傅',TRUE,TRUE,TRUE,TRUE,NULL,'2026-09-23 09:10+08'),
 (703,911,2,'马师傅',FALSE,TRUE,FALSE,FALSE,'桶身变形、密封圈损坏、内壁残留明显，不得再灌装','2026-09-21 15:00+08');
UPDATE keg_cycles SET wash_id=w.id FROM wash_checks w WHERE w.cycle_id=keg_cycles.id;
SELECT setval(pg_get_serial_sequence('wash_checks','id'), 800);

-- ---------- 停售通知（酸啤 BT2026-SOUR-01） ----------
INSERT INTO notifications (batch_id,audience,target_id,title,body) VALUES
 (104,'BAR',5,'批次停售通知：BT2026-SOUR-01 莓果酸啤','该批次暂停销售，贵店 KEG-0008 请勿继续开桶售卖，等待回收安排。'),
 (104,'PENDING_ORDER',506,'待配送订单冻结：ORD-0910 含停售批次 2 桶','节日铺货订单 ORD-0910 中 BT2026-SOUR-01 ×2 暂停配送，请联系酒厂换批次。'),
 (104,'DEALER_STOCK',1,'经销商库存排查：华东精酿经销','请盘点渠道内 BT2026-SOUR-01 库存并停止向下游酒吧发货。'),
 (104,'ALL',NULL,'全渠道通告：BT2026-SOUR-01 暂停销售','品控发起批次暂停，已收货酒吧、待配送订单与经销商库存全部冻结，等待复检结论。');

-- ---------- 桶级催还/责任提醒（逾期冻结，区别于批次停售通知） ----------
INSERT INTO bar_alerts (cycle_id,bar_id,kind,title,body,created_at) VALUES
 (916,5,'OVERDUE_FREEZE',
  '押金冻结提醒：KEG-0016 已超 7 天账期未归还',
  '平台已按桶编号冻结该桶押金 ¥700。您上传的"已打空"照片已收到，请尽快预约回收；逾期责任确认后押金释放时间将重新计算。',
  '2026-09-22 10:00+08'),
 (917,5,'OVERDUE_FREEZE',
  '押金冻结提醒：KEG-0017 已超 7 天账期未归还',
  '平台已按桶编号冻结该桶押金 ¥700。系统记录显示 9/23 预约取桶司机未到店，请确认回收时间并留意财务的逾期责任判定。',
  '2026-09-22 10:00+08');

-- ---------- 桶事件时间线（由周期数据回填） ----------
INSERT INTO keg_events (keg_id,cycle_id,event_type,detail,actor)
SELECT keg_id,id,'DELIVERED',jsonb_build_object('order_id',order_id,'bar_id',bar_id,'receiver',bar_receiver),'系统'
FROM keg_cycles;
INSERT INTO keg_events (keg_id,cycle_id,event_type,actor)
SELECT keg_id,id,'TAPPED','酒吧' FROM keg_cycles WHERE tapped_at IS NOT NULL;
INSERT INTO keg_events (keg_id,cycle_id,event_type,actor)
SELECT keg_id,id,'EMPTIED','酒吧' FROM keg_cycles WHERE emptied_at IS NOT NULL;
INSERT INTO keg_events (keg_id,cycle_id,event_type,actor)
SELECT keg_id,id,'PICKED','司机' FROM keg_cycles WHERE picked_at IS NOT NULL;
INSERT INTO keg_events (keg_id,cycle_id,event_type,detail,actor)
SELECT c.keg_id,c.id,'WASHED',jsonb_build_object('passed',TRUE),'清洗厂'
FROM keg_cycles c JOIN wash_checks w ON w.cycle_id=c.id WHERE w.passed;
INSERT INTO keg_events (keg_id,cycle_id,event_type,detail,actor)
SELECT c.keg_id,c.id,'WASH_FAILED',jsonb_build_object('note',w.defect_note),'清洗厂'
FROM keg_cycles c JOIN wash_checks w ON w.cycle_id=c.id WHERE NOT w.passed;
INSERT INTO keg_events (keg_id,cycle_id,event_type,detail,actor)
SELECT keg_id,id,'DEPOSIT_SETTLED',jsonb_build_object('status',deposit_status,'released',released_amount,'deducted',deducted_amount),'财务'
FROM keg_cycles WHERE deposit_status IN ('RELEASED','DEDUCTED','PARTIAL');
-- 逾期冻结/到场/照片/预约事件回填
INSERT INTO keg_events (keg_id,cycle_id,event_type,detail,actor)
SELECT keg_id,id,'DEPOSIT_FROZEN',jsonb_build_object('deposit',deposit_amount,'due',deposit_due_date),'平台'
FROM keg_cycles WHERE deposit_status='FROZEN';
INSERT INTO keg_events (keg_id,cycle_id,event_type,detail,actor) VALUES
 (17,917,'RETURN_RESERVED',jsonb_build_object('reservation_id',806,'pickup_date','2026-09-23'),'酒吧');
INSERT INTO keg_events (keg_id,cycle_id,event_type,detail,actor) VALUES
 (5,905,'VISIT_BAR_NOT_READY',jsonb_build_object('reservation_id',805,'driver','王师傅','note','到店时空桶未备好'),'司机'),
 (17,917,'VISIT_DRIVER_FAILED',jsonb_build_object('reservation_id',806,'driver','王师傅','note','预约日司机未到店'),'酒吧'),
 (16,916,'EMPTY_PROOF_UPLOADED',jsonb_build_object('note','KEG-0016 已打空放后厨待回收'),'酒吧'),
 (17,917,'EMPTY_PROOF_UPLOADED',jsonb_build_object('note','司机失约，空桶在店门口'),'酒吧');

COMMIT;
