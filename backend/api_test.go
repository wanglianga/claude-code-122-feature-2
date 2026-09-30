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

// ---------- 空桶逾期押金冻结全链路 ----------

func TestOverdueFreezeFlow(t *testing.T) {
	if err := resetTestDB(); err != nil {
		t.Fatal(err)
	}
	admin := login(t, "admin")
	finance := login(t, "finance")
	bar2 := login(t, "bar2") // 醉猫酒馆 id=5
	bar1 := login(t, "bar1")
	driver1 := login(t, "driver1")

	const png1px = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	t.Run("批量冻结逾期桶并产生催还提醒", func(t *testing.T) {
		// 冻结前 905/906 均为 HELD 且账期 2026-09-15 已过
		code, d := doJSON(t, "POST", "/api/finance/freeze-overdue", finance, M{})
		mustOK(t, code, d, "冻结全部逾期桶")
		frozen := d["frozen"].([]interface{})
		codes := map[string]bool{}
		for _, x := range frozen {
			codes[x.(map[string]interface{})["keg_code"].(string)] = true
		}
		if !codes["KEG-0005"] || !codes["KEG-0006"] {
			t.Fatalf("905/906 逾期桶应被冻结: %v", codes)
		}
		if codes["KEG-0001"] {
			t.Fatalf("未逾期的 KEG-0001 不应被冻结")
		}
		// 幂等：再次执行不应重复冻结/重复提醒
		code, d2 := doJSON(t, "POST", "/api/finance/freeze-overdue", finance, M{})
		mustOK(t, code, d2, "重复冻结")
		if d2["count"].(float64) != 0 {
			t.Fatalf("重复执行不应新增冻结: %v", d2)
		}

		// 店长收到桶级催还提醒，别的酒吧看不到
		_, da := doJSON(t, "GET", "/api/bar/alerts", bar2, nil)
		saw := false
		for _, x := range da["alerts"].([]interface{}) {
			m := x.(map[string]interface{})
			if m["kind"] == "OVERDUE_FREEZE" && m["keg_code"] == "KEG-0005" {
				saw = true
			}
		}
		if !saw {
			t.Fatalf("醉猫酒馆应收到 KEG-0005 冻结催还提醒")
		}
		_, d1 := doJSON(t, "GET", "/api/bar/alerts", bar1, nil)
		for _, x := range d1["alerts"].([]interface{}) {
			if x.(map[string]interface{})["keg_code"] == "KEG-0005" {
				t.Fatalf("泡沫实验室不应看到醉猫的桶级提醒")
			}
		}
	})

	t.Run("店长上传已打空照片与权限校验", func(t *testing.T) {
		// 跨店上传被拒
		code, d := doJSON(t, "POST", "/api/cycles/proofs/905", bar1,
			M{"photo_data": png1px, "note": "别家店"})
		if code != 400 {
			t.Fatalf("跨店上传照片应 400, 实际 %d %v", code, d)
		}
		// 非图片数据被拒
		code, _ = doJSON(t, "POST", "/api/cycles/proofs/905", bar2,
			M{"photo_data": "not-an-image"})
		if code != 400 {
			t.Fatalf("非图片照片应 400, 实际 %d", code)
		}
		code, d = doJSON(t, "POST", "/api/cycles/proofs/905", bar2,
			M{"photo_data": png1px, "note": "KEG-0005 实际已打空，司机 9/20 到店时桶在角落没找到"})
		mustOK(t, code, d, "上传现场照片")
		// 财务可查看凭证（照片 + 805 到场记录）
		code, d = doJSON(t, "GET", "/api/cycles/proofs/905", finance, nil)
		mustOK(t, code, d, "查看凭证")
		if len(d["proofs"].([]interface{})) != 1 {
			t.Fatalf("905 应有 1 张照片: %v", d["proofs"])
		}
		visits := d["visits"].([]interface{})
		if len(visits) != 1 || visits[0].(map[string]interface{})["result"] != "BAR_NOT_READY" {
			t.Fatalf("905 应带 1 条酒吧未备货到场记录: %v", visits)
		}
	})

	t.Run("未判定责任前拒绝结算冻结押金", func(t *testing.T) {
		code, d := doJSON(t, "POST", "/api/finance/settle/905", finance, M{"action": "release"})
		if code != 400 || !strings.Contains(d["error"].(string), "责任判定") {
			t.Fatalf("冻结未定则结算应拒绝: %d %v", code, d)
		}
	})

	t.Run("酒吧未备货责任-维持冻结可扣款", func(t *testing.T) {
		code, d := doJSON(t, "POST", "/api/finance/responsibility/905", finance,
			M{"responsibility": "BAR", "note": "司机已到店，空桶未备好，责任在酒吧"})
		mustOK(t, code, d, "判定酒吧责任")
		if d["deposit_status"] != "FROZEN" || d["overdue_responsibility"] != "BAR" {
			t.Fatalf("酒吧责任应维持冻结: %v", d)
		}
		// 定则后可结算：部分扣款
		code, d = doJSON(t, "POST", "/api/finance/settle/905", finance,
			M{"action": "partial", "deduct_amount": 100, "reason": "逾期责任酒吧承担，扣款 100"})
		mustOK(t, code, d, "酒吧责任后部分扣款")
		if d["deducted_amount"].(float64) != 100 || d["released_amount"].(float64) != 600 {
			t.Fatalf("扣款金额错误: %v", d)
		}
	})

	t.Run("司机到店失败责任-解冻并重算释放时间", func(t *testing.T) {
		// 种子 806/917 已存在 DRIVER_FAILED 到场记录
		code, d := doJSON(t, "GET", "/api/cycles/proofs/917", finance, nil)
		mustOK(t, code, d, "917 凭证")
		visits := d["visits"].([]interface{})
		if len(visits) != 1 || visits[0].(map[string]interface{})["result"] != "DRIVER_FAILED" {
			t.Fatalf("917 应有司机到店失败记录: %v", visits)
		}
		// 店长确认回收时间（改约 806 为未来日期）
		code, d = doJSON(t, "POST", "/api/cycles/confirm-pickup", bar2,
			M{"cycle_id": 917, "reservation_id": 806, "pickup_date": "2026-10-02", "note": "空桶已备好"})
		mustOK(t, code, d, "店长确认回收时间")
		if d["reservation_id"].(float64) != 806 {
			t.Fatalf("应改约 806: %v", d)
		}
		// 非本店店长不能确认
		code, _ = doJSON(t, "POST", "/api/cycles/confirm-pickup", bar1,
			M{"cycle_id": 917, "pickup_date": "2026-10-02"})
		if code != 400 {
			t.Fatalf("跨店确认回收时间应 400, 实际 %d", code)
		}
		// 判定司机责任 → 解冻，释放日 = 确认回收日 + 7
		code, d = doJSON(t, "POST", "/api/finance/responsibility/917", finance,
			M{"responsibility": "DRIVER", "note": "司机失约，酒吧无责"})
		mustOK(t, code, d, "判定司机责任")
		if d["deposit_status"] != "HELD" {
			t.Fatalf("司机责任应解除冻结为 HELD: %v", d)
		}
		if d["deposit_release_date"] != "2026-10-09" {
			t.Fatalf("释放日应按确认回收日 2026-10-02 +7 重算为 2026-10-09: %v", d["deposit_release_date"])
		}
		// 解冻后可正常释放
		code, d = doJSON(t, "POST", "/api/finance/settle/917", finance, M{"action": "release"})
		mustOK(t, code, d, "司机责任后释放押金")
		if d["released_amount"].(float64) != 700 {
			t.Fatalf("应全额释放 700: %v", d)
		}
		// 店长收到责任结论提醒
		_, da := doJSON(t, "GET", "/api/bar/alerts", bar2, nil)
		saw := false
		for _, x := range da["alerts"].([]interface{}) {
			m := x.(map[string]interface{})
			if m["kind"] == "RESPONSIBILITY" && m["keg_code"] == "KEG-0017" {
				saw = true
			}
		}
		if !saw {
			t.Fatalf("店长应收到 KEG-0017 责任判定提醒")
		}
	})

	t.Run("司机登记酒吧未备好空桶到场记录", func(t *testing.T) {
		// 801 是泡沫实验室(4)的预约，driver1 在上海，可登记
		code, d := doJSON(t, "POST", "/api/pickups/visit", driver1,
			M{"reservation_id": 801, "result": "BAR_NOT_READY", "note": "到店发现空桶还没整理"})
		mustOK(t, code, d, "登记酒吧未备货")
		// 酒吧店长不能登记 BAR_NOT_READY（角色不符）
		code, _ = doJSON(t, "POST", "/api/pickups/visit", bar1,
			M{"reservation_id": 801, "result": "BAR_NOT_READY"})
		if code != 403 {
			t.Fatalf("店长登记 BAR_NOT_READY 应 403, 实际 %d", code)
		}
		// 店长登记司机到店失败：801 属 bar1(4)
		code, d = doJSON(t, "POST", "/api/pickups/visit", bar1,
			M{"reservation_id": 801, "result": "DRIVER_FAILED", "note": "第二次又没来"})
		// 801 已被置为 BAR_NOT_READY，允许再被改登记（覆盖为 DRIVER_FAILED 场景也允许）
		if code != 200 {
			t.Fatalf("店长可对本店预约登记司机到店失败: %d %v", code, d)
		}
	})

	t.Run("桶追溯时间线含冻结与责任事件", func(t *testing.T) {
		code, d := doJSON(t, "GET", "/api/kegs/trace/KEG-0017", admin, nil)
		mustOK(t, code, d, "追溯 KEG-0017")
		types := map[string]bool{}
		for _, e := range d["events"].([]interface{}) {
			types[e.(map[string]interface{})["event_type"].(string)] = true
		}
		for _, want := range []string{"DEPOSIT_FROZEN", "VISIT_DRIVER_FAILED", "RESPONSIBILITY_CONFIRMED",
			"EMPTY_PROOF_UPLOADED", "PICKUP_TIME_CONFIRMED"} {
			if !types[want] {
				t.Fatalf("时间线缺少事件 %s: %v", want, types)
			}
		}
		cur := d["current"].(map[string]interface{})
		if cur["overdue_responsibility"] != "DRIVER" || cur["deposit_release_date"] != "2026-10-09" {
			t.Fatalf("追溯当前态责任/释放日错误: %v", cur)
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
