package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

// 集成测试：需要本地 PostgreSQL（默认 5433），测试库 kegplatform_test
// 运行：先 createdb kegplatform_test，再 go test ./...

var testServer *httptest.Server

func TestMain(m *testing.M) {
	dsn := os.Getenv("KEG_TEST_DSN")
	if dsn == "" {
		dsn = "host=/tmp port=5433 user=node dbname=kegplatform_test sslmode=disable"
	}
	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		os.Stderr.WriteString("跳过集成测试（无法连接测试库）: " + err.Error() + "\n")
		os.Exit(0)
	}
	if err := db.Ping(); err != nil {
		os.Stderr.WriteString("跳过集成测试（无法连接测试库）: " + err.Error() + "\n")
		os.Exit(0)
	}
	if err := resetTestDB(); err != nil {
		os.Stderr.WriteString("测试库初始化失败: " + err.Error() + "\n")
		os.Exit(1)
	}
	mux := http.NewServeMux()
	registerRoutes(mux)
	testServer = httptest.NewServer(mux)
	code := m.Run()
	testServer.Close()
	os.Exit(code)
}

// ---------- 测试辅助 ----------

func resetTestDB() error {
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		return err
	}
	for _, name := range []string{"db/schema.sql", "db/seed.sql"} {
		b, err := sqlFS.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(b)); err != nil {
			return err
		}
	}
	return nil
}

func doJSON(t *testing.T, method, path, token string, body interface{}) (int, M) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, testServer.URL+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败 %s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out M
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return res.StatusCode, out
}

func login(t *testing.T, username string) string {
	t.Helper()
	code, d := doJSON(t, "POST", "/api/login", "", M{"username": username, "password": "demo123"})
	if code != 200 {
		t.Fatalf("登录失败 %s: %v", username, d)
	}
	return d["token"].(string)
}

func mustOK(t *testing.T, code int, d M, ctx string) {
	t.Helper()
	if code != 200 {
		t.Fatalf("%s: 期望 200 实际 %d: %v", ctx, code, d["error"])
	}
}

// ---------- 端到端主流程 ----------

func TestEndToEnd(t *testing.T) {
	admin := login(t, "admin")
	bar1 := login(t, "bar1")
	bar2 := login(t, "bar2")
	driver1 := login(t, "driver1")
	wash1 := login(t, "wash1")
	qc := login(t, "qc")
	finance := login(t, "finance")
	dealer := login(t, "dealer1")

	t.Run("登录与鉴权", func(t *testing.T) {
		// 错误密码
		code, _ := doJSON(t, "POST", "/api/login", "", M{"username": "admin", "password": "wrong"})
		if code != 401 {
			t.Fatalf("错误密码应 401, 实际 %d", code)
		}
		// 无 token
		code, _ = doJSON(t, "GET", "/api/overview", "", nil)
		if code != 401 {
			t.Fatalf("无 token 应 401, 实际 %d", code)
		}
		// 越权：司机不得访问财务
		code, d := doJSON(t, "GET", "/api/finance/deposits", driver1, nil)
		if code != 403 {
			t.Fatalf("司机访问财务应 403, 实际 %d (%v)", code, d)
		}
		// 越权：酒吧1 不能操作酒吧2 的桶
		code, d = doJSON(t, "POST", "/api/cycles/feedback/905", bar1, M{"pour_speed": "正常"})
		if code != 400 || !strings.Contains(d["error"].(string), "本店") {
			t.Fatalf("跨店操作应被拒, 实际 %d %v", code, d)
		}
	})

	t.Run("酒吧开桶反馈-打空-预约", func(t *testing.T) {
		// 901 初始 DELIVERED，首次反馈自动开桶
		code, d := doJSON(t, "POST", "/api/cycles/feedback/901", bar1,
			M{"pour_speed": "偏慢", "pressure": "气压过低", "taste_off": false})
		mustOK(t, code, d, "首次反馈")
		if d["first_tap"] != true {
			t.Fatalf("首次反馈应标记开桶: %v", d)
		}
		code, d = doJSON(t, "POST", "/api/cycles/empty/901", bar1, nil)
		mustOK(t, code, d, "标记打空")
		// 未打空的桶不能预约
		code, d = doJSON(t, "POST", "/api/cycles/reserve", bar1,
			M{"cycle_ids": []int64{902}, "pickup_date": "2026-09-28"})
		if code != 400 {
			t.Fatalf("未打空桶预约应 400, 实际 %d %v", code, d)
		}
		code, d = doJSON(t, "POST", "/api/cycles/reserve", bar1,
			M{"cycle_ids": []int64{901}, "pickup_date": "2026-09-28", "note": "测试"})
		mustOK(t, code, d, "预约回收")
	})

	t.Run("司机扫码回收", func(t *testing.T) {
		code, d := doJSON(t, "POST", "/api/pickups/scan", driver1,
			M{"keg_code": "KEG-0001", "appearance": "凹陷", "residue_level": "少量", "cap_status": "已封盖"})
		mustOK(t, code, d, "扫码 KEG-0001")
		// 重复扫码拒绝
		code, d = doJSON(t, "POST", "/api/pickups/scan", driver1, M{"keg_code": "KEG-0001"})
		if code != 400 || !strings.Contains(d["error"].(string), "已回收") {
			t.Fatalf("重复扫码应拒绝, 实际 %d %v", code, d)
		}
		// 不存在的桶
		code, _ = doJSON(t, "POST", "/api/pickups/scan", driver1, M{"keg_code": "KEG-9999"})
		if code != 400 {
			t.Fatalf("不存在桶应 400, 实际 %d", code)
		}
	})

	t.Run("回收审计-漏扫与逾期", func(t *testing.T) {
		code, d := doJSON(t, "POST", "/api/driver/audit", admin, M{})
		mustOK(t, code, d, "审计")
		found := d["missing_scans"].([]interface{})
		var has915 bool
		for _, x := range found {
			m := x.(map[string]interface{})
			if m["cycle_id"].(float64) == 915 {
				has915 = true
			}
		}
		if !has915 {
			t.Fatalf("审计应发现预约 802 的漏扫桶 KEG-0015(cycle 915): %v", found)
		}
	})

	t.Run("清洗检测合格与不合格", func(t *testing.T) {
		// 901 刚回收 → 合格
		code, d := doJSON(t, "POST", "/api/wash/checks", wash1,
			M{"cycle_id": 901, "inspector": "刘师傅", "passed": true,
				"pressure_ok": true, "seal_ok": true, "cleanliness_ok": true})
		mustOK(t, code, d, "清洗合格")
		if d["refillable"] != true || d["asset_status"] != "IN_STOCK" {
			t.Fatalf("合格桶应回库可再灌装: %v", d)
		}
		// 911 种子已是 FAILED；改为对 910 做不合格检测
		code, d = doJSON(t, "POST", "/api/wash/checks", wash1,
			M{"cycle_id": 910, "inspector": "马师傅", "passed": true,
				"pressure_ok": true, "seal_ok": false, "cleanliness_ok": false,
				"defect_note": "密封失效+内壁残留"})
		mustOK(t, code, d, "清洗不合格")
		if d["refillable"] != false || d["asset_status"] != "WASH_FAIL" {
			t.Fatalf("不合格桶应冻结: %v", d)
		}
		// 已检测的桶不能重复检测
		code, _ = doJSON(t, "POST", "/api/wash/checks", wash1, M{"cycle_id": 901, "passed": true})
		if code != 400 {
			t.Fatalf("重复检测应 400, 实际 %d", code)
		}
	})

	t.Run("财务押金结算", func(t *testing.T) {
		// 903 已被预约 801 但尚未扫码；先扫码再洗合格，然后释放
		code, d := doJSON(t, "POST", "/api/pickups/scan", driver1,
			M{"reservation_id": 801, "keg_code": "KEG-0003"})
		mustOK(t, code, d, "扫 KEG-0003")
		code, d = doJSON(t, "POST", "/api/wash/checks", wash1, M{"cycle_id": 903, "passed": true})
		mustOK(t, code, d, "洗 903")
		code, d = doJSON(t, "POST", "/api/finance/settle/903", finance, M{"action": "release"})
		mustOK(t, code, d, "释放 903")
		if d["released_amount"].(float64) != 700 {
			t.Fatalf("应释放 700: %v", d)
		}
		// 重复结算拒绝
		code, d = doJSON(t, "POST", "/api/finance/settle/903", finance, M{"action": "deduct"})
		if code != 400 || !strings.Contains(d["error"].(string), "已结算") {
			t.Fatalf("重复结算应拒绝: %d %v", code, d)
		}
		// 部分扣款
		code, d = doJSON(t, "POST", "/api/finance/settle/905", finance,
			M{"action": "partial", "deduct_amount": 200, "reason": "逾期+桶身划痕"})
		mustOK(t, code, d, "部分扣款 905")
		if d["released_amount"].(float64) != 500 || d["deducted_amount"].(float64) != 200 {
			t.Fatalf("部分扣款金额错误: %v", d)
		}
		// 非法扣款金额
		code, _ = doJSON(t, "POST", "/api/finance/settle/906", finance,
			M{"action": "partial", "deduct_amount": 9999})
		if code != 400 {
			t.Fatalf("超额扣款应 400, 实际 %d", code)
		}
	})

	t.Run("批次停售通知与库存分层", func(t *testing.T) {
		// 批次 104 种子已 SUSPENDED，先解除，再通过接口暂停以验证通知生成
		code, d := doJSON(t, "POST", "/api/batches/clear/104", qc, nil)
		mustOK(t, code, d, "解除 104")
		code, d = doJSON(t, "POST", "/api/batches/suspend/104", qc,
			M{"reason": "集成测试暂停"})
		mustOK(t, code, d, "暂停 104")
		n := d["notifications"].(map[string]interface{})
		// 已收货酒吧：醉猫(KEG-0008) → 1
		if n["bars"].(float64) != 1 {
			t.Fatalf("应通知 1 家在店酒吧: %v", n)
		}
		// 待配送订单：506 含酸啤 2 只 → 1
		if n["pending_orders"].(float64) != 1 {
			t.Fatalf("应通知 1 个待配送订单: %v", n)
		}
		// 经销商：502/506 关联华东、506 无经销商(成都直营) → 华东 1 家
		if n["dealers"].(float64) != 1 {
			t.Fatalf("应通知 1 家经销商: %v", n)
		}
		// 酒吧2 应看到 BAR 通知，酒吧1 不应看到
		_, d2 := doJSON(t, "GET", "/api/notifications", bar2, nil)
		saw := false
		for _, x := range d2["notifications"].([]interface{}) {
			m := x.(map[string]interface{})
			if m["batch_no"] == "BT2026-SOUR-01" && m["audience"] == "BAR" {
				saw = true
			}
		}
		if !saw {
			t.Fatalf("醉猫酒馆应收到该批次的 BAR 通知")
		}
		_, d1 := doJSON(t, "GET", "/api/notifications", bar1, nil)
		for _, x := range d1["notifications"].([]interface{}) {
			m := x.(map[string]interface{})
			if m["batch_no"] == "BT2026-SOUR-01" && m["audience"] == "BAR" {
				t.Fatalf("泡沫实验室不应收到该批次 BAR 定向通知")
			}
		}
		// 经销商可见 DEALER_STOCK 通知
		_, dd := doJSON(t, "GET", "/api/notifications", dealer, nil)
		sawDealer := false
		for _, x := range dd["notifications"].([]interface{}) {
			if x.(map[string]interface{})["audience"] == "DEALER_STOCK" {
				sawDealer = true
			}
		}
		if !sawDealer {
			t.Fatalf("经销商应收到库存排查通知")
		}
		// 召回库存分层
		code, detail := doJSON(t, "GET", "/api/batches/104", admin, nil)
		mustOK(t, code, detail, "批次明细")
		if detail["pending_orders"].(map[string]interface{})["kegs"].(float64) != 2 {
			t.Fatalf("待配送应为 2 只: %v", detail["pending_orders"])
		}
	})

	t.Run("停售批次发货拦截", func(t *testing.T) {
		code, d := doJSON(t, "POST", "/api/orders/dispatch/506", admin,
			M{"driver_id": 1, "bar_receiver": "小唐"})
		if code != 400 || !strings.Contains(d["error"].(string), "暂停销售") {
			t.Fatalf("含停售批次的订单应被拦截: %d %v", code, d)
		}
	})

	t.Run("建单-发货正向闭环", func(t *testing.T) {
		// 新建订单给醉猫酒馆：批次 103 两只
		code, d := doJSON(t, "POST", "/api/orders", admin, M{
			"bar_id": 5, "driver_id": 1, "deposit_term_days": 20,
			"items": []M{{"batch_id": 103, "qty": 2}},
		})
		mustOK(t, code, d, "建单")
		oid := int64(d["order_id"].(float64))
		code, d = doJSON(t, "POST", "/api/orders/dispatch/"+itoa(oid), admin,
			M{"driver_id": 1, "bar_receiver": "老陈"})
		mustOK(t, code, d, "发货")
		if len(d["dispatched"].([]interface{})) != 2 {
			t.Fatalf("应发出 2 只: %v", d)
		}
	})

	t.Run("按桶追溯时间线", func(t *testing.T) {
		code, d := doJSON(t, "GET", "/api/kegs/trace/KEG-0003", admin, nil)
		mustOK(t, code, d, "追溯 KEG-0003")
		types := map[string]bool{}
		for _, e := range d["events"].([]interface{}) {
			types[e.(map[string]interface{})["event_type"].(string)] = true
		}
		for _, want := range []string{"DELIVERED", "EMPTIED", "PICKED", "WASHED", "DEPOSIT_SETTLED"} {
			if !types[want] {
				t.Fatalf("时间线缺少事件 %s: %v", want, types)
			}
		}
	})

	t.Run("多店口味投诉触发品控停售建议", func(t *testing.T) {
		code, d := doJSON(t, "GET", "/api/qc/overview", qc, nil)
		mustOK(t, code, d, "品控总览")
		var hit bool
		for _, x := range d["batch_alerts"].([]interface{}) {
			m := x.(map[string]interface{})
			if m["batch_no"] == "BT2026-ST-01" && m["suggest_suspend"] == true {
				hit = true
			}
		}
		if !hit {
			t.Fatalf("世涛批次被 2 家酒吧投诉口味异常，应建议停售")
		}
	})
}

func TestRegisterKegsNumbering(t *testing.T) {
	if err := resetTestDB(); err != nil {
		t.Fatal(err)
	}
	admin := login(t, "admin")
	code, d := doJSON(t, "POST", "/api/kegs/register", admin,
		M{"count": 3, "capacity_liters": 20})
	mustOK(t, code, d, "登记新桶")
	created := d["created"].([]interface{})
	want := []string{"KEG-0031", "KEG-0032", "KEG-0033"}
	for i, w := range want {
		if created[i].(string) != w {
			t.Fatalf("新桶编号错误: 得到 %v 期望 %v", created, want)
		}
	}
}

// 空桶逾期押金冻结全流程：冻结 → 提醒 → 现场照片 → 到店记录 → 店长确认回收时间
// → 逾期责任确认 → 押金释放时间重新计算
func TestOverdueFreezeFlow(t *testing.T) {
	if err := resetTestDB(); err != nil {
		t.Fatal(err)
	}
	admin := login(t, "admin")
	finance := login(t, "finance")
	bar1 := login(t, "bar1")
	bar2 := login(t, "bar2")
	driver1 := login(t, "driver1")

	// 1) 平台逾期扫描：种子已冻结 905/906/907；KEG-0008(908) 同样逾期，本次被冻结
	code, d := doJSON(t, "POST", "/api/overdue/scan", finance, M{})
	mustOK(t, code, d, "逾期扫描")
	var f908 bool
	for _, x := range d["frozen"].([]interface{}) {
		m := x.(map[string]interface{})
		if m["cycle_id"].(float64) == 908 {
			f908 = true
		}
	}
	if !f908 {
		t.Fatalf("KEG-0008(cycle 908) 账期 2026-09-15 已逾期，应被冻结: %v", d["frozen"])
	}
	if d["frozen_count"].(float64) < 1 {
		t.Fatalf("本次扫描应至少冻结 1 只: %v", d)
	}
	// 重复扫描不会重复冻结
	code, d = doJSON(t, "POST", "/api/overdue/scan", finance, M{"cycle_ids": []int64{908}})
	mustOK(t, code, d, "重复扫描")
	if d["frozen_count"].(float64) != 0 {
		t.Fatalf("已冻结桶不应重复冻结: %v", d)
	}

	// 2) 店长收到冻结提醒；跨店不可见
	code, d = doJSON(t, "GET", "/api/bar/alerts", bar2, nil)
	mustOK(t, code, d, "店长提醒")
	if d["unacked_count"].(float64) == 0 {
		t.Fatalf("醉猫酒馆应有未确认逾期提醒")
	}
	var saw908Alert bool
	for _, x := range d["alerts"].([]interface{}) {
		m := x.(map[string]interface{})
		if m["cycle_id"].(float64) == 908 && m["kind"] == "OVERDUE_FREEZE" {
			saw908Alert = true
		}
	}
	if !saw908Alert {
		t.Fatalf("KEG-0008 冻结后应生成店长提醒")
	}
	_, d1 := doJSON(t, "GET", "/api/bar/alerts", bar1, nil)
	for _, x := range d1["alerts"].([]interface{}) {
		if x.(map[string]interface{})["cycle_id"].(float64) == 908 {
			t.Fatalf("泡沫实验室不应看到醉猫酒馆的冻结提醒")
		}
	}

	// 3) 冻结桶 908 必须走"确认回收时间"：先打空
	code, _ = doJSON(t, "POST", "/api/cycles/feedback/908", bar2, M{"pour_speed": "正常"})
	if code != 200 {
		t.Fatalf("908 首次反馈应自动开桶, 实际 %d", code)
	}
	code, d = doJSON(t, "POST", "/api/cycles/empty/908", bar2, nil)
	mustOK(t, code, d, "908 标记打空")
	// 普通预约对冻结桶关闭
	code, d = doJSON(t, "POST", "/api/cycles/reserve", bar2,
		M{"cycle_ids": []int64{908}, "pickup_date": "2026-10-02"})
	if code != 400 {
		t.Fatalf("冻结桶普通预约应被引导到确认回收时间, 实际 %d %v", code, d)
	}
	// 店长确认回收时间
	code, d = doJSON(t, "POST", "/api/cycles/confirm-return", bar2,
		M{"cycle_ids": []int64{908}, "pickup_date": "2026-10-02", "note": "节后回收"})
	mustOK(t, code, d, "确认回收时间")
	resID := int64(d["reservation_id"].(float64))

	// 4) 司机到店但酒吧未备好空桶 → 计入酒吧责任
	code, d = doJSON(t, "POST", "/api/driver/visit", driver1,
		M{"reservation_id": resID, "result": "BAR_NOT_READY", "note": "到店时桶还在酒头上"})
	mustOK(t, code, d, "到店未备货登记")
	if d["count"].(float64) != 1 {
		t.Fatalf("应登记 1 只桶的到场记录: %v", d)
	}
	// 越权：店长不能登记到店
	code, _ = doJSON(t, "POST", "/api/driver/visit", bar2,
		M{"reservation_id": resID, "result": "BAR_NOT_READY"})
	if code != 403 {
		t.Fatalf("店长登记到店记录应 403, 实际 %d", code)
	}
	// 司机任务中可看到到场统计
	_, dtasks := doJSON(t, "GET", "/api/driver/tasks", driver1, nil)
	var sawVisit bool
	for _, x := range dtasks["tasks"].([]interface{}) {
		tk := x.(map[string]interface{})
		if tk["reservation_id"].(float64) == float64(resID) &&
			tk["visit_not_ready"].(float64) == 1 {
			sawVisit = true
		}
	}
	if !sawVisit {
		t.Fatalf("司机任务应带出到店未备货统计")
	}

	// 5) 店长逾期冻结后上传"已打空但未回收"现场照片
	const dataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	code, d = doJSON(t, "POST", "/api/cycles/photo/905", bar2,
		M{"photo_data": dataURL, "note": "桶已打空放角落待回收"})
	mustOK(t, code, d, "上传现场照片")
	// 非冻结桶不能上传
	code, _ = doJSON(t, "POST", "/api/cycles/photo/901", bar2, M{"photo_data": dataURL})
	if code != 400 {
		t.Fatalf("非冻结桶上传举证照片应 400, 实际 %d", code)
	}
	// 跨店上传被拒
	code, _ = doJSON(t, "POST", "/api/cycles/photo/905", bar1, M{"photo_data": dataURL})
	if code != 400 {
		t.Fatalf("跨店上传照片应 400, 实际 %d", code)
	}

	// 6) 财务照片审核队列：种子 907 一张待核实 + 905 一张
	code, q := doJSON(t, "GET", "/api/finance/photo-queue", finance, nil)
	mustOK(t, code, q, "照片队列")
	var p907 int64
	pendingN := 0
	for _, x := range q["photos"].([]interface{}) {
		p := x.(map[string]interface{})
		if p["status"] == "PENDING" {
			pendingN++
		}
		if p["cycle_id"].(float64) == 907 && p["status"] == "PENDING" {
			p907 = int64(p["id"].(float64))
		}
	}
	if pendingN < 2 || p907 == 0 {
		t.Fatalf("待核实照片应包含种子 907 与新上传 905: %v", q["photos"])
	}
	// 采信 907 的照片
	code, d = doJSON(t, "POST", "/api/finance/photos/"+itoa(p907), finance,
		M{"action": "approve", "note": "照片清晰可采信"})
	mustOK(t, code, d, "采信照片")
	// 重复审核拒绝
	code, _ = doJSON(t, "POST", "/api/finance/photos/"+itoa(p907), finance, M{"action": "approve"})
	if code != 400 {
		t.Fatalf("照片重复审核应 400, 实际 %d", code)
	}

	// 7) 907：司机到店失败 + 照片采信 → 自动判定酒吧无责，押金即时可释放
	code, d = doJSON(t, "POST", "/api/finance/liability/907", finance, M{})
	mustOK(t, code, d, "907 责任自动判定")
	if d["party"] != "NONE" {
		t.Fatalf("907 有司机到店失败且无未备货记录、照片已采信，应判酒吧无责: %v", d)
	}
	if d["release_due_date"].(string) != "2026-09-30" {
		t.Fatalf("酒吧无责时释放日应为当天(2026-09-30): %v", d["release_due_date"])
	}
	code, d = doJSON(t, "POST", "/api/finance/settle/907", finance, M{"action": "release"})
	mustOK(t, code, d, "907 当日释放押金")

	// 8) 908：到店未备货 → 酒吧责任，释放日 = 确认取桶日(10-02)+7 天
	code, d = doJSON(t, "POST", "/api/finance/liability/908", finance, M{})
	mustOK(t, code, d, "908 责任自动判定")
	if d["party"] != "BAR" {
		t.Fatalf("908 有到店未备货记录，应判酒吧责任: %v", d)
	}
	if d["release_due_date"].(string) != "2026-10-09" {
		t.Fatalf("酒吧责任时释放日应为取桶日+7 天 2026-10-09: %v", d["release_due_date"])
	}
	// 未到新释放日不能全额释放
	code, d = doJSON(t, "POST", "/api/finance/settle/908", finance, M{"action": "release"})
	if code != 400 {
		t.Fatalf("未到重算后的释放日应拒绝释放, 实际 %d %v", code, d)
	}
	// 部分扣款不受释放日限制
	code, d = doJSON(t, "POST", "/api/finance/settle/908", finance,
		M{"action": "partial", "deduct_amount": 100, "reason": "逾期未备货"})
	mustOK(t, code, d, "908 部分扣款")

	// 9) 重复责任确认被拒；非法责任方被拒
	code, _ = doJSON(t, "POST", "/api/finance/liability/907", finance, M{})
	if code != 400 {
		t.Fatalf("重复责任确认应 400, 实际 %d", code)
	}
	code, _ = doJSON(t, "POST", "/api/finance/liability/906", finance, M{"party": "WHO"})
	if code != 400 {
		t.Fatalf("非法责任方应 400, 实际 %d", code)
	}
	// 906：种子里到店未备货 1 次 → 酒吧责任
	code, d = doJSON(t, "POST", "/api/finance/liability/906", finance, M{})
	mustOK(t, code, d, "906 责任判定")
	if d["party"] != "BAR" {
		t.Fatalf("906 司机到店未备货，应判酒吧责任: %v", d)
	}

	// 10) 店长收到责任结果通知
	_, d = doJSON(t, "GET", "/api/bar/alerts", bar2, nil)
	var liabN int
	var ackID int64
	for _, x := range d["alerts"].([]interface{}) {
		m := x.(map[string]interface{})
		if m["kind"] == "LIABILITY_RESULT" {
			liabN++
			ackID = int64(m["id"].(float64))
		}
	}
	if liabN < 2 {
		t.Fatalf("907/908 责任确认后应至少生成 2 条责任结果提醒, 实际 %d", liabN)
	}
	// 跨店不能确认提醒
	code, _ = doJSON(t, "POST", "/api/bar/alerts/ack/"+itoa(ackID), bar1, nil)
	if code != 400 {
		t.Fatalf("跨店确认提醒应 400, 实际 %d", code)
	}
	code, d = doJSON(t, "POST", "/api/bar/alerts/ack/"+itoa(ackID), bar2, nil)
	mustOK(t, code, d, "本店确认提醒")

	// 11) 财务列表冻结筛选与汇总
	code, d = doJSON(t, "GET", "/api/finance/deposits?frozen=1", admin, nil)
	mustOK(t, code, d, "冻结筛选")
	if d["summary"].(map[string]interface{})["frozen_count"].(float64) < 1 {
		t.Fatalf("仍应有未解除的冻结桶（如 906）: %v", d["summary"])
	}
}
