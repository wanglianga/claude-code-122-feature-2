package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// GET /api/kegs?status=&batch_id=&bar_id=&q=
func handleListKegs(w http.ResponseWriter, r *http.Request) {
	q := `SELECT k.id, k.keg_code, k.capacity_liters, k.asset_status, k.refillable,
		COALESCE(b.name,''), COALESCE(ba.batch_no,''), COALESCE(be.style,''),
		COALESCE(c.stage,''), COALESCE(c.deposit_status,''),
		COALESCE(c.id,0)
		FROM kegs k
		LEFT JOIN keg_cycles c ON c.id=k.current_cycle_id
		LEFT JOIN bars b ON b.id=c.bar_id
		LEFT JOIN batches ba ON ba.id=c.batch_id
		LEFT JOIN beers be ON be.id=ba.beer_id
		WHERE 1=1`
	args := []interface{}{}
	if s := r.URL.Query().Get("status"); s != "" {
		q += " AND k.asset_status=$" + strconv.Itoa(len(args)+1)
		args = append(args, s)
	}
	if s := r.URL.Query().Get("batch_id"); s != "" {
		q += " AND c.batch_id=$" + strconv.Itoa(len(args)+1)
		args = append(args, atoi64(s))
	}
	if s := r.URL.Query().Get("bar_id"); s != "" {
		q += " AND c.bar_id=$" + strconv.Itoa(len(args)+1)
		args = append(args, atoi64(s))
	}
	if s := r.URL.Query().Get("q"); s != "" {
		q += " AND k.keg_code ILIKE $" + strconv.Itoa(len(args)+1)
		args = append(args, "%"+s+"%")
	}
	q += " ORDER BY k.keg_code"

	rows, err := db.Query(q, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	list := scanRows(rows, func(rs rowScanner) (M, error) {
		var id, cycleID int64
		var code, status, bar, batch, style, stage, depositStatus string
		var capacity float64
		var refillable bool
		if err := rs.Scan(&id, &code, &capacity, &status, &refillable,
			&bar, &batch, &style, &stage, &depositStatus, &cycleID); err != nil {
			return nil, err
		}
		return M{
			"id": id, "keg_code": code, "capacity_liters": capacity,
			"asset_status": status, "refillable": refillable,
			"bar": bar, "batch_no": batch, "style": style,
			"stage": stage, "deposit_status": depositStatus, "cycle_id": cycleID,
		}, nil
	})
	writeOK(w, M{"kegs": list})
}

func pad4(id int64) string {
	s := strconv.FormatInt(id, 10)
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}

// POST /api/kegs/register  酒厂登记新桶
func handleRegisterKegs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	var req struct {
		Count    int     `json:"count"`
		Capacity float64 `json:"capacity_liters"`
		Prefix   string  `json:"prefix"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if req.Count <= 0 || req.Count > 200 {
		fail(w, 400, "登记数量需在 1~200 之间")
		return
	}
	if req.Capacity <= 0 {
		req.Capacity = 30
	}
	if req.Prefix == "" {
		req.Prefix = "KEG"
	}

	created := []string{}
	err := runTx(func(tx *sql.Tx) error {
		for i := 0; i < req.Count; i++ {
			var id int64
			// 预取序列值作为桶编号与主键，保证并发下编号唯一
			if err := tx.QueryRow(`SELECT nextval(pg_get_serial_sequence('kegs','id'))`).Scan(&id); err != nil {
				return err
			}
			code := req.Prefix + "-" + pad4(id)
			if _, err := tx.Exec(`INSERT INTO kegs (id,keg_code,capacity_liters) VALUES ($1,$2,$3)`,
				id, code, req.Capacity); err != nil {
				return err
			}
			created = append(created, code)
		}
		return nil
	})
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeOK(w, M{"created": created, "count": len(created)})
}

// GET /api/kegs/trace/{code}  按桶追溯时间线
func handleKegTrace(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/api/kegs/trace/")
	if code == "" {
		fail(w, 400, "缺少酒桶编号")
		return
	}
	var kegID int64
	var capacity float64
	var assetStatus string
	var refillable bool
	err := db.QueryRow(`SELECT id, capacity_liters, asset_status, refillable FROM kegs WHERE keg_code=$1`, code).
		Scan(&kegID, &capacity, &assetStatus, &refillable)
	if err == sql.ErrNoRows {
		fail(w, 404, "酒桶不存在: "+code)
		return
	}
	if err != nil {
		fail(w, 500, err.Error())
		return
	}

	current := M{}
	var cid, batchID, barID, driverID, orderID sql.NullInt64
	var stage, depositStatus, receiver, reason sql.NullString
	var deposit, released, deducted float64
	var dueDate sql.NullTime
	var late, missing, lost, deformed bool
	err = db.QueryRow(`SELECT id, batch_id, bar_id, driver_id, order_id, stage,
		deposit_amount, deposit_due_date, deposit_status, released_amount, deducted_amount,
		COALESCE(deduct_reason,''), COALESCE(bar_receiver,''), late_return, missing_scan, lost, deformed
		FROM keg_cycles WHERE keg_id=$1 ORDER BY id DESC LIMIT 1`, kegID).
		Scan(&cid, &batchID, &barID, &driverID, &orderID, &stage,
			&deposit, &dueDate, &depositStatus, &released, &deducted, &reason, &receiver,
			&late, &missing, &lost, &deformed)
	if err == nil {
		current = M{
			"cycle_id": cid.Int64, "batch_id": batchID.Int64, "bar_id": barID.Int64,
			"driver_id": driverID.Int64, "order_id": orderID.Int64, "stage": stage.String,
			"deposit_amount": deposit, "deposit_due_date": dateStr(dueDate),
			"deposit_status": depositStatus.String, "released_amount": released,
			"deducted_amount": deducted, "deduct_reason": reason.String,
			"bar_receiver": receiver.String,
			"late_return":  late, "missing_scan": missing, "lost": lost, "deformed": deformed,
		}
		var batchNo, beerStyle, beerName, barName string
		_ = db.QueryRow(`SELECT ba.batch_no, be.style, be.name FROM batches ba JOIN beers be ON be.id=ba.beer_id WHERE ba.id=$1`, batchID.Int64).
			Scan(&batchNo, &beerStyle, &beerName)
		current["batch_no"] = batchNo
		current["style"] = beerStyle
		current["beer"] = beerName
		if barID.Int64 != 0 {
			_ = db.QueryRow(`SELECT name FROM bars WHERE id=$1`, barID.Int64).Scan(&barName)
			current["bar"] = barName
		}
	}

	rows, _ := db.Query(`SELECT event_type, detail, COALESCE(actor,''), created_at
		FROM keg_events WHERE keg_id=$1 ORDER BY created_at, id`, kegID)
	events := scanRows(rows, func(rs rowScanner) (M, error) {
		var etype, actor string
		var detail []byte
		var at sql.NullTime
		if err := rs.Scan(&etype, &detail, &actor, &at); err != nil {
			return nil, err
		}
		var d M
		_ = json.Unmarshal(detail, &d)
		return M{"event_type": etype, "detail": d, "actor": actor, "at": tsStr(at)}, nil
	})

	writeOK(w, M{
		"keg": M{
			"id": kegID, "keg_code": code, "capacity_liters": capacity,
			"asset_status": assetStatus, "refillable": refillable,
		},
		"current": current,
		"events":  events,
	})
}
