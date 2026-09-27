package main

import (
	"database/sql"
	"net/http"
	"strings"
)

// /api/orders  GET 列表 / POST 创建待配送订单
func handleOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		listOrders(w, r)
		return
	}
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 GET/POST")
		return
	}
	createOrder(w, r)
}

func listOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT o.id, o.order_no, b.id, b.name, c.name, COALESCE(d.name,'直营'),
		       COALESCE(dr.name,'未指派'), o.is_festival, COALESCE(o.festival_note,''),
		       o.deposit_term_days, o.status, o.created_at, COALESCE(text(o.delivered_at),'')
		FROM orders o
		JOIN bars b ON b.id=o.bar_id
		JOIN cities c ON c.id=o.city_id
		LEFT JOIN dealers d ON d.id=o.dealer_id
		LEFT JOIN drivers dr ON dr.id=o.driver_id
		ORDER BY o.id DESC`)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	list := scanRows(rows, func(rs rowScanner) (M, error) {
		var id, barID, term int64
		var orderNo, bar, city, dealer, driver, note, delivered, status string
		var festival bool
		var at sql.NullTime
		if err := rs.Scan(&id, &orderNo, &barID, &bar, &city, &dealer, &driver,
			&festival, &note, &term, &status, &at, &delivered); err != nil {
			return nil, err
		}
		m := M{"id": id, "order_no": orderNo, "bar_id": barID, "bar": bar, "city": city,
			"dealer": dealer, "driver": driver, "is_festival": festival, "festival_note": note,
			"deposit_term_days": term, "status": status, "created_at": tsStr(at),
			"delivered_at": delivered}

		irows, _ := db.Query(`
			SELECT be.name, be.style, ba.batch_no, ba.status, COALESCE(oi.keg_id,0), oi.qty
			FROM order_items oi JOIN batches ba ON ba.id=oi.batch_id
			JOIN beers be ON be.id=ba.beer_id
			WHERE oi.order_id=$1 ORDER BY oi.id`, id)
		m["items"] = scanRows(irows, func(x rowScanner) (M, error) {
			var kegID, qty int64
			var beer, style, batchNo, status string
			if err := x.Scan(&beer, &style, &batchNo, &status, &kegID, &qty); err != nil {
				return nil, err
			}
			return M{"beer": beer, "style": style, "batch_no": batchNo, "batch_status": status,
				"keg_id": kegID, "qty": qty}, nil
		})
		return m, nil
	})
	writeOK(w, M{"orders": list})
}

func createOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BarID           int64  `json:"bar_id"`
		DriverID        int64  `json:"driver_id"`
		IsFestival      bool   `json:"is_festival"`
		FestivalNote    string `json:"festival_note"`
		DepositTermDays int    `json:"deposit_term_days"`
		Items           []struct {
			BatchID int64 `json:"batch_id"`
			Qty     int   `json:"qty"`
		} `json:"items"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if req.BarID == 0 || len(req.Items) == 0 {
		fail(w, 400, "酒吧与配送明细必填")
		return
	}
	if req.DepositTermDays <= 0 {
		req.DepositTermDays = 30
	}
	u := currentUser(r)

	orderID := int64(0)
	err := runTx(func(tx *sql.Tx) error {
		var cityID, dealerID sql.NullInt64
		if err := tx.QueryRow(`SELECT city_id, dealer_id FROM bars WHERE id=$1`, req.BarID).
			Scan(&cityID, &dealerID); err != nil {
			return errBad("酒吧不存在")
		}
		var orderNo string
		if err := tx.QueryRow(`SELECT 'ORD-'||to_char(now(),'MMDD')||'-'||lpad((COALESCE(max(id),0)+1)::text,3,'0')
			FROM orders`).Scan(&orderNo); err != nil {
			return err
		}
		if err := tx.QueryRow(`INSERT INTO orders
			(order_no,bar_id,dealer_id,driver_id,city_id,is_festival,festival_note,deposit_term_days,status,created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'PLANNED',$9) RETURNING id`,
			orderNo, req.BarID, nullableID(dealerID.Int64), nullableID(req.DriverID), cityID.Int64,
			req.IsFestival, nullStr(req.FestivalNote), req.DepositTermDays, u.ID).Scan(&orderID); err != nil {
			return err
		}
		for _, it := range req.Items {
			if it.BatchID == 0 || it.Qty <= 0 {
				continue
			}
			var status string
			if err := tx.QueryRow(`SELECT status FROM batches WHERE id=$1`, it.BatchID).Scan(&status); err != nil {
				return errBad("批次不存在")
			}
			if _, err := tx.Exec(`INSERT INTO order_items (order_id,batch_id,qty) VALUES ($1,$2,$3)`,
				orderID, it.BatchID, it.Qty); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if e, ok := err.(*badRequest); ok {
			fail(w, 400, e.Error())
		} else {
			fail(w, 500, err.Error())
		}
		return
	}
	writeOK(w, M{"order_id": orderID})
}

// POST /api/orders/dispatch/{id}  酒厂发货：分配具体酒桶、生成在店周期
func handleDispatchOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	id := tailID(r.URL.Path, "/api/orders/dispatch/")
	var req struct {
		DriverID int64    `json:"driver_id"`
		Receiver string   `json:"bar_receiver"`
		KegCodes []string `json:"keg_codes"` // 可选：整单指定酒桶（按明细顺序展开）
	}
	_ = decodeJSON(r, &req)

	var dispatched []M
	err := runTx(func(tx *sql.Tx) error {
		var status string
		var barID, cityID, term int64
		var oldDriver sql.NullInt64
		err := tx.QueryRow(`SELECT status, bar_id, city_id, deposit_term_days, driver_id FROM orders WHERE id=$1`, id).
			Scan(&status, &barID, &cityID, &term, &oldDriver)
		if err == sql.ErrNoRows {
			return errBad("订单不存在")
		}
		if err != nil {
			return err
		}
		if status != "PLANNED" {
			return errBad("仅待配送订单可以发货，当前状态: " + status)
		}
		driverID := req.DriverID
		if driverID == 0 {
			driverID = oldDriver.Int64
		}
		if driverID == 0 {
			return errBad("请指定配送司机")
		}
		if strings.TrimSpace(req.Receiver) == "" {
			return errBad("请填写酒吧收货人")
		}

		// 展开明细
		type item struct{ id, batchID, qty int64 }
		var items []item
		irows, err := tx.Query(`SELECT id, batch_id, qty FROM order_items WHERE order_id=$1 ORDER BY id FOR UPDATE`, id)
		if err != nil {
			return err
		}
		for irows.Next() {
			var it item
			if err := irows.Scan(&it.id, &it.batchID, &it.qty); err != nil {
				return err
			}
			items = append(items, it)
		}
		irows.Close()

		// 停售/召回批次拦截
		for _, it := range items {
			var bs string
			if err := tx.QueryRow(`SELECT status FROM batches WHERE id=$1`, it.batchID).Scan(&bs); err != nil {
				return errBad("批次不存在")
			}
			if bs != "NORMAL" {
				var bn string
				_ = tx.QueryRow(`SELECT batch_no FROM batches WHERE id=$1`, it.batchID).Scan(&bn)
				return errBad("批次 " + bn + " 已暂停销售/召回，订单禁止配送，请更换批次")
			}
		}

		codeIdx := 0
		for _, it := range items {
			for n := 0; n < int(it.qty); n++ {
				var kegID int64
				var capacity float64
				var code string
				if codeIdx < len(req.KegCodes) {
					err = tx.QueryRow(`SELECT id, capacity_liters, keg_code FROM kegs
						WHERE keg_code=$1 AND asset_status='IN_STOCK' FOR UPDATE`,
						req.KegCodes[codeIdx]).Scan(&kegID, &capacity, &code)
				} else {
					err = tx.QueryRow(`SELECT id, capacity_liters, keg_code FROM kegs
						WHERE asset_status='IN_STOCK' ORDER BY id LIMIT 1 FOR UPDATE`).
						Scan(&kegID, &capacity, &code)
				}
				codeIdx++
				if err == sql.ErrNoRows {
					return errBad("仓库可用空桶不足，无法完成发货")
				}
				if err != nil {
					return err
				}

				deposit := 700.0
				if capacity < 25 {
					deposit = 500
				}
				var cycleID int64
				if err := tx.QueryRow(`INSERT INTO keg_cycles
					(keg_id,batch_id,order_id,bar_id,driver_id,city_id,deposit_amount,deposit_due_date,stage,bar_receiver)
					VALUES ($1,$2,$3,$4,$5,$6,$7,CURRENT_DATE+$8::int,'DELIVERED',$9) RETURNING id`,
					kegID, it.batchID, id, barID, driverID, cityID, deposit, term, req.Receiver).
					Scan(&cycleID); err != nil {
					return err
				}
				if _, err := tx.Exec(`UPDATE kegs SET asset_status='AT_BAR', current_bar_id=$1, current_cycle_id=$2 WHERE id=$3`,
					barID, cycleID, kegID); err != nil {
					return err
				}
				if _, err := tx.Exec(`UPDATE order_items SET keg_id=$1 WHERE id=$2`,
					kegID, it.id); err != nil {
					return err
				}
				if err := logKegEvent(tx, kegID, cycleID, "DELIVERED", "酒厂/司机",
					M{"order_id": id, "bar_id": barID, "driver_id": driverID, "receiver": req.Receiver}); err != nil {
					return err
				}
				dispatched = append(dispatched, M{"keg_code": code, "cycle_id": cycleID, "deposit": deposit})
			}
		}

		if _, err := tx.Exec(`UPDATE orders SET status='DELIVERED', driver_id=$1, delivered_at=now() WHERE id=$2`,
			driverID, id); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if e, ok := err.(*badRequest); ok {
			fail(w, 400, e.Error())
		} else {
			fail(w, 500, err.Error())
		}
		return
	}
	writeOK(w, M{"order_id": id, "dispatched": dispatched})
}

// ---------- 小工具 ----------

type badRequest struct{ msg string }

func (e *badRequest) Error() string { return e.msg }
func errBad(s string) error         { return &badRequest{s} }

func nullableID(id int64) interface{} {
	if id == 0 {
		return nil
	}
	return id
}
func nullableID2(id int64) interface{} { return nullableID(id) }
func nullStr(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
