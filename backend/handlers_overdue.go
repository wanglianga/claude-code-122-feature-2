package main

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/lib/pq"
)

// addBarAlert 写入一条店长提醒
func addBarAlert(tx *sql.Tx, barID int64, kind string, cycleID, refID int64, title, body string) error {
	var cid interface{}
	if cycleID != 0 {
		cid = cycleID
	}
	var rid interface{}
	if refID != 0 {
		rid = refID
	}
	_, err := tx.Exec(`INSERT INTO bar_alerts (bar_id,kind,cycle_id,ref_id,title,body)
		VALUES ($1,$2,$3,$4,$5,$6)`, barID, kind, cid, rid, title, body)
	return err
}

// POST /api/overdue/scan  平台扫描：超过账期未归还空桶 → 按桶冻结押金并提醒店长预约回收
// body 可选 {cycle_ids: [...]}；不传则冻结全部逾期未闭环桶
func handleOverdueScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	var req struct {
		CycleIDs []int64 `json:"cycle_ids"`
	}
	_ = decodeJSON(r, &req)

	frozen := []M{}
	err := runTx(func(tx *sql.Tx) error {
		q := `SELECT c.id, c.keg_id, c.bar_id, k.keg_code, b.name,
		              c.deposit_amount, c.deposit_due_date,
		              (CURRENT_DATE - c.deposit_due_date)
		       FROM keg_cycles c
		       JOIN kegs k ON k.id=c.keg_id
		       JOIN bars b ON b.id=c.bar_id
		       WHERE NOT c.deposit_frozen AND c.deposit_status='HELD'
		         AND c.liability_party IS NULL
		         AND c.deposit_due_date < CURRENT_DATE
		         AND c.stage NOT IN ('WASHED','FAILED') AND NOT c.lost`
		args := []interface{}{}
		if len(req.CycleIDs) > 0 {
			q += " AND c.id = ANY($1)"
			args = append(args, pq.Array(req.CycleIDs))
		}
		q += " ORDER BY c.deposit_due_date, c.id"
		rows, err := tx.Query(q, args...)
		if err != nil {
			return err
		}
		type fr struct {
			id, kegID, barID, overdueDays int64
			deposit                       float64
			code, bar                     string
			due                           time.Time
		}
		var list []fr
		for rows.Next() {
			var x fr
			if err := rows.Scan(&x.id, &x.kegID, &x.barID, &x.code, &x.bar,
				&x.deposit, &x.due, &x.overdueDays); err != nil {
				return err
			}
			list = append(list, x)
		}
		rows.Close()

		for _, x := range list {
			if _, err := tx.Exec(`UPDATE keg_cycles
				SET deposit_frozen=TRUE, frozen_at=now(), late_return=TRUE WHERE id=$1`, x.id); err != nil {
				return err
			}
			var freezeID int64
			if err := tx.QueryRow(`INSERT INTO deposit_freezes
				(cycle_id,keg_id,bar_id,deposit_amount,original_due_date,overdue_days)
				VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
				x.id, x.kegID, x.barID, x.deposit, x.due, x.overdueDays).Scan(&freezeID); err != nil {
				return err
			}
			if err := addBarAlert(tx, x.barID, "OVERDUE_FREEZE", x.id, freezeID,
				"空桶逾期："+x.code+" 押金已被平台冻结",
				"该桶已超过约定归还日 "+x.due.Format("2006-01-02")+"（逾期 "+itoa(x.overdueDays)+
					" 天），平台已按桶编号冻结押金 ¥"+itoa(int64(x.deposit))+
					"。请尽快确认空桶已打空并预约回收；司机到店失败或酒吧未备好空桶都会影响逾期责任判定。"); err != nil {
				return err
			}
			if err := logKegEvent(tx, x.kegID, x.id, "DEPOSIT_FROZEN", "平台",
				M{"freeze_id": freezeID, "original_due_date": x.due.Format("2006-01-02"),
					"overdue_days": x.overdueDays, "deposit": x.deposit}); err != nil {
				return err
			}
			frozen = append(frozen, M{"freeze_id": freezeID, "cycle_id": x.id, "keg_code": x.code,
				"bar": x.bar, "deposit_amount": x.deposit, "overdue_days": x.overdueDays})
		}
		return nil
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeOK(w, M{"frozen": frozen, "frozen_count": len(frozen)})
}

// POST /api/driver/visit  司机到店登记
// body: {reservation_id, cycle_ids?（不传=整单）, result: READY|BAR_NOT_READY|DRIVER_FAILED, note}
func handleDriverVisit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	var req struct {
		ReservationID int64   `json:"reservation_id"`
		CycleIDs      []int64 `json:"cycle_ids"`
		Result        string  `json:"result"`
		Note          string  `json:"note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if req.ReservationID == 0 {
		fail(w, 400, "缺少回收预约")
		return
	}
	switch req.Result {
	case "READY", "BAR_NOT_READY", "DRIVER_FAILED":
	default:
		fail(w, 400, "到店结果应为 READY/BAR_NOT_READY/DRIVER_FAILED")
		return
	}
	u := currentUser(r)

	recorded := []M{}
	err := runTx(func(tx *sql.Tx) error {
		var barID, cityID int64
		var rvStatus string
		err := tx.QueryRow(`SELECT rv.bar_id, b.city_id, rv.status
			FROM return_reservations rv JOIN bars b ON b.id=rv.bar_id
			WHERE rv.id=$1`, req.ReservationID).
			Scan(&barID, &cityID, &rvStatus)
		if err == sql.ErrNoRows {
			return errBad("回收预约不存在")
		}
		if err != nil {
			return err
		}
		driverID := u.OrgID
		if u.Role == "admin" && driverID == 0 {
			driverID = 1
		}
		// 预约清单（或指定子集）
		q := `SELECT rc.cycle_id, c.keg_id, k.keg_code, c.stage
			FROM reservation_cycles rc
			JOIN keg_cycles c ON c.id=rc.cycle_id
			JOIN kegs k ON k.id=c.keg_id
			WHERE rc.reservation_id=$1`
		args := []interface{}{req.ReservationID}
		if len(req.CycleIDs) > 0 {
			q += " AND rc.cycle_id = ANY($2)"
			args = append(args, pq.Array(req.CycleIDs))
		}
		rows, err := tx.Query(q, args...)
		if err != nil {
			return err
		}
		type item struct {
			cycleID, kegID int64
			code, stage    string
		}
		var items []item
		for rows.Next() {
			var x item
			if err := rows.Scan(&x.cycleID, &x.kegID, &x.code, &x.stage); err != nil {
				return err
			}
			items = append(items, x)
		}
		rows.Close()
		if len(items) == 0 {
			return errBad("预约清单中没有可登记的酒桶")
		}

		for _, x := range items {
			if x.stage == "WASHED" || x.stage == "FAILED" {
				continue // 已闭环桶不再登记到场
			}
			if _, err := tx.Exec(`INSERT INTO driver_visits
				(reservation_id,cycle_id,driver_id,bar_id,city_id,result,note)
				VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''))`,
				req.ReservationID, x.cycleID, driverID, barID, cityID,
				req.Result, req.Note); err != nil {
				return err
			}
			switch req.Result {
			case "BAR_NOT_READY":
				if _, err := tx.Exec(`UPDATE keg_cycles
					SET bar_not_ready_count=bar_not_ready_count+1 WHERE id=$1`, x.cycleID); err != nil {
					return err
				}
				if err := addBarAlert(tx, barID, "RETURN_REMINDER", x.cycleID, 0,
					"司机已到店但空桶未备好："+x.code,
					"司机按预约到店回收时该桶尚未备好，本次到场记录会计入酒吧方逾期责任，请尽快打空并重新确认回收时间。"); err != nil {
					return err
				}
				if err := logKegEvent(tx, x.kegID, x.cycleID, "VISIT_BAR_NOT_READY", "司机",
					M{"reservation_id": req.ReservationID, "note": req.Note}); err != nil {
					return err
				}
			case "DRIVER_FAILED":
				if _, err := tx.Exec(`UPDATE keg_cycles
					SET driver_fail_count=driver_fail_count+1 WHERE id=$1`, x.cycleID); err != nil {
					return err
				}
				if err := addBarAlert(tx, barID, "RETURN_REMINDER", x.cycleID, 0,
					"司机到店失败，回收需改期："+x.code,
					"司机本次未能到店完成回收（车辆/调度等原因），该到场记录作为酒吧方免责依据；请重新确认回收时间，押金冻结责任将据此判定。"); err != nil {
					return err
				}
				if err := logKegEvent(tx, x.kegID, x.cycleID, "VISIT_DRIVER_FAILED", "司机",
					M{"reservation_id": req.ReservationID, "note": req.Note}); err != nil {
					return err
				}
			default:
				if err := logKegEvent(tx, x.kegID, x.cycleID, "VISIT_READY", "司机",
					M{"reservation_id": req.ReservationID, "note": req.Note}); err != nil {
					return err
				}
			}
			recorded = append(recorded, M{"cycle_id": x.cycleID, "keg_code": x.code, "result": req.Result})
		}
		return nil
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeOK(w, M{"recorded": recorded, "count": len(recorded)})
}

// POST /api/cycles/photo/{id}  逾期冻结后店长上传"已打空但未回收"现场照片
func handleEmptyPhoto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	id := tailID(r.URL.Path, "/api/cycles/photo/")
	var req struct {
		PhotoData string `json:"photo_data"`
		Note      string `json:"note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !strings.HasPrefix(req.PhotoData, "data:image/") {
		fail(w, 400, "照片需为 data:image 格式（浏览器现场拍照/上传）")
		return
	}
	if len(req.PhotoData) > 3_000_000 {
		fail(w, 400, "照片过大，请压缩后再上传（<2MB）")
		return
	}
	u := currentUser(r)

	var photoID int64
	err := runTx(func(tx *sql.Tx) error {
		var kegID, barID int64
		var frozen bool
		var stage string
		if err := tx.QueryRow(`SELECT keg_id, bar_id, deposit_frozen, stage
			FROM keg_cycles WHERE id=$1 FOR UPDATE`, id).
			Scan(&kegID, &barID, &frozen, &stage); err == sql.ErrNoRows {
			return errBad("流转记录不存在")
		} else if err != nil {
			return err
		}
		if u.Role != "admin" && barID != u.OrgID {
			return errBad("只能上传本店酒桶的现场照片")
		}
		if !frozen {
			return errBad("仅押金已逾期冻结的酒桶需要上传现场举证照片")
		}
		if stage == "WASHED" || stage == "FAILED" {
			return errBad("该桶已闭环，无需上传照片")
		}
		if err := tx.QueryRow(`INSERT INTO empty_photos (cycle_id,bar_id,photo_data,note)
			VALUES ($1,$2,$3,NULLIF($4,'')) RETURNING id`,
			id, barID, req.PhotoData, req.Note).Scan(&photoID); err != nil {
			return err
		}
		return logKegEvent(tx, kegID, id, "EMPTY_PHOTO_UPLOADED", "酒吧",
			M{"photo_id": photoID, "note": req.Note})
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeOK(w, M{"photo_id": photoID})
}

// POST /api/cycles/confirm-return  店长确认回收时间（逾期冻结桶重新排期回收）
// body: {cycle_ids:[...], pickup_date:'YYYY-MM-DD'}
func handleConfirmReturn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	var req struct {
		CycleIDs   []int64 `json:"cycle_ids"`
		PickupDate string  `json:"pickup_date"`
		Note       string  `json:"note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if len(req.CycleIDs) == 0 || req.PickupDate == "" {
		fail(w, 400, "请选择空桶并确认回收日期")
		return
	}
	u := currentUser(r)

	resID := int64(0)
	err := runTx(func(tx *sql.Tx) error {
		var barID int64
		for _, cid := range req.CycleIDs {
			var cBar int64
			var stage string
			var frozen bool
			if err := tx.QueryRow(`SELECT bar_id, stage, deposit_frozen
				FROM keg_cycles WHERE id=$1 FOR UPDATE`, cid).
				Scan(&cBar, &stage, &frozen); err != nil {
				return errBad("流转记录不存在")
			}
			if u.Role != "admin" && cBar != u.OrgID {
				return errBad("只能确认本店酒桶的回收时间")
			}
			if stage != "EMPTY" && stage != "RESERVED" {
				return errBad("请先将酒桶标记打空，再确认回收时间（当前: " + stage + "）")
			}
			if !frozen {
				return errBad("仅逾期冻结的酒桶需要在此重新确认回收时间，普通空桶请使用“预约回收”")
			}
			barID = cBar
		}
		// 旧的待取桶预约作废
		if _, err := tx.Exec(`UPDATE return_reservations SET status='CANCELLED'
			WHERE status='WAITING' AND id IN (
				SELECT reservation_id FROM reservation_cycles WHERE cycle_id = ANY($1))`,
			pq.Array(req.CycleIDs)); err != nil {
			return err
		}
		if err := tx.QueryRow(`INSERT INTO return_reservations
			(cycle_id,bar_id,pickup_date,empty_count,note,status)
			VALUES ($1,$2,$3::date,$4,NULLIF($5,''),'WAITING') RETURNING id`,
			req.CycleIDs[0], barID, req.PickupDate, len(req.CycleIDs), req.Note).Scan(&resID); err != nil {
			return err
		}
		for _, cid := range req.CycleIDs {
			if _, err := tx.Exec(`INSERT INTO reservation_cycles (reservation_id,cycle_id) VALUES ($1,$2)
				ON CONFLICT DO NOTHING`, resID, cid); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE keg_cycles
				SET stage='RESERVED', return_reserved_at=now(), return_confirmed_at=now()
				WHERE id=$1`, cid); err != nil {
				return err
			}
			var kegID int64
			_ = tx.QueryRow(`SELECT keg_id FROM keg_cycles WHERE id=$1`, cid).Scan(&kegID)
			if err := logKegEvent(tx, kegID, cid, "RETURN_TIME_CONFIRMED", "酒吧",
				M{"reservation_id": resID, "pickup_date": req.PickupDate}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeOK(w, M{"reservation_id": resID})
}

// POST /api/finance/liability/{cycleId}  逾期责任确认并重算押金释放时间
// body: {party: BAR|DRIVER|NONE（不传则按到场/照片证据自动建议）, note}
func handleLiabilityConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	id := tailID(r.URL.Path, "/api/finance/liability/")
	var req struct {
		Party string `json:"party"`
		Note  string `json:"note"`
	}
	_ = decodeJSON(r, &req)
	if req.Party != "" && req.Party != "BAR" && req.Party != "DRIVER" && req.Party != "NONE" {
		fail(w, 400, "责任方应为 BAR/DRIVER/NONE")
		return
	}
	u := currentUser(r)

	resp := M{}
	err := runTx(func(tx *sql.Tx) error {
		var kegID, barID, notReady, driverFail int64
		var deposit float64
		var frozen bool
		var liability sql.NullString
		if err := tx.QueryRow(`SELECT keg_id, bar_id, deposit_amount, deposit_frozen,
			COALESCE(liability_party,''), bar_not_ready_count, driver_fail_count
			FROM keg_cycles WHERE id=$1 FOR UPDATE`, id).
			Scan(&kegID, &barID, &deposit, &frozen, &liability, &notReady, &driverFail); err != nil {
			return errBad("流转记录不存在")
		}
		if !frozen {
			return errBad("该桶押金未处于逾期冻结状态，无需责任确认")
		}
		if liability.Valid && liability.String != "" {
			return errBad("该桶逾期责任已确认，不能重复判定")
		}

		// 已采信的"账期内已打空"照片
		var approvedPhoto int
		_ = tx.QueryRow(`SELECT count(*) FROM empty_photos
			WHERE cycle_id=$1 AND status='APPROVED'`, id).Scan(&approvedPhoto)

		party := req.Party
		suggest := ""
		if party == "" {
			switch {
			case driverFail > 0 && notReady == 0:
				party, suggest = "NONE", "司机到店失败且酒吧无未备货记录，酒吧方免责"
			case notReady > 0:
				party, suggest = "BAR", "司机到店时空桶未备好，逾期责任在酒吧"
			case approvedPhoto > 0:
				party, suggest = "NONE", "店长已上传打空现场照片并经采信，回收延误属物流环节"
			default:
				party, suggest = "BAR", "无酒吧免责证据，逾期责任默认在酒吧"
			}
		}

		// 重新计算押金释放日：
		//  BAR   → 取桶/回收日之后 7 天冷静期；DRIVER/NONE（酒吧无责）→ 当日即具备释放条件
		var releaseDue time.Time
		if party == "BAR" {
			if err := tx.QueryRow(`SELECT
				GREATEST(
					COALESCE((SELECT max(rv.pickup_date) FROM return_reservations rv
						JOIN reservation_cycles rc ON rc.reservation_id=rv.id
						WHERE rc.cycle_id=$1 AND rv.status IN ('WAITING','PICKED')),
					CURRENT_DATE),
				CURRENT_DATE) + 7`, id).Scan(&releaseDue); err != nil {
				return err
			}
		} else {
			if err := tx.QueryRow(`SELECT CURRENT_DATE`).Scan(&releaseDue); err != nil {
				return err
			}
		}

		note := req.Note
		if note == "" {
			note = suggest
		}
		if _, err := tx.Exec(`UPDATE keg_cycles
			SET liability_party=$1, liability_note=$2, liability_confirmed_by=$3,
			    liability_confirmed_at=now(), release_due_date=$4, deposit_frozen=FALSE
			WHERE id=$5`, party, note, u.ID, releaseDue, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE deposit_freezes SET status='RESOLVED', resolved_at=now()
			WHERE cycle_id=$1 AND status='FROZEN'`, id); err != nil {
			return err
		}
		partyText := map[string]string{"BAR": "酒吧方（未备好空桶/延迟归还）",
			"DRIVER": "司机/物流方", "NONE": "酒吧方无责"}[party]
		if err := addBarAlert(tx, barID, "LIABILITY_RESULT", id, 0,
			"逾期责任已确认：押金释放时间重新计算",
			"该逾期空桶责任判定为："+partyText+"。押金新释放日为 "+
				releaseDue.Format("2006-01-02")+"，届时完成回收/清洗后押金将按计划释放。说明："+note); err != nil {
			return err
		}
		if err := logKegEvent(tx, kegID, id, "LIABILITY_CONFIRMED", "财务",
			M{"party": party, "note": note, "release_due_date": releaseDue.Format("2006-01-02"),
				"bar_not_ready": notReady, "driver_failed": driverFail, "photo_approved": approvedPhoto}); err != nil {
			return err
		}
		resp = M{"party": party, "suggestion": suggest, "release_due_date": releaseDue.Format("2006-01-02")}
		return nil
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeOK(w, resp)
}

// POST /api/finance/photos/{id}  财务采信/驳回店长上传的打空现场照片
// body: {action: approve|reject, note}
func handlePhotoReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	id := tailID(r.URL.Path, "/api/finance/photos/")
	var req struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	_ = decodeJSON(r, &req)
	status := ""
	switch req.Action {
	case "approve":
		status = "APPROVED"
	case "reject":
		status = "REJECTED"
	default:
		fail(w, 400, "动作应为 approve/reject")
		return
	}
	u := currentUser(r)

	err := runTx(func(tx *sql.Tx) error {
		var cycleID, barID, kegID int64
		var oldStatus string
		if err := tx.QueryRow(`SELECT p.cycle_id, p.bar_id, p.status, c.keg_id
			FROM empty_photos p JOIN keg_cycles c ON c.id=p.cycle_id
			WHERE p.id=$1 FOR UPDATE`, id).Scan(&cycleID, &barID, &oldStatus, &kegID); err != nil {
			return errBad("照片不存在")
		}
		if oldStatus != "PENDING" {
			return errBad("该照片已审核")
		}
		if _, err := tx.Exec(`UPDATE empty_photos SET status=$1, reviewed_by=$2, reviewed_at=now(),
			review_note=NULLIF($3,'') WHERE id=$4`, status, u.ID, req.Note, id); err != nil {
			return err
		}
		alertTitle, alertBody := "现场照片已采信", "您上传的已打空未回收现场照片已被财务采信，将作为逾期责任判定中酒吧方的免责/减责依据。"
		if status == "REJECTED" {
			alertTitle, alertBody = "现场照片未被采信", "您上传的现场照片未通过核实（"+req.Note+"），不能作为逾期免责依据，请补充更清晰的照片或尽快预约回收。"
		}
		if err := addBarAlert(tx, barID, "PHOTO_REVIEWED", cycleID, id, alertTitle, alertBody); err != nil {
			return err
		}
		return logKegEvent(tx, kegID, cycleID, "EMPTY_PHOTO_REVIEWED", "财务",
			M{"photo_id": id, "status": status, "note": req.Note})
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeOK(w, M{"ok": true, "status": status})
}

// GET /api/finance/photo-queue  逾期冻结桶的现场举证照片审核队列
func handlePhotoQueue(w http.ResponseWriter, r *http.Request) {
	q := `SELECT p.id, p.cycle_id, k.keg_code, b.id, b.name, p.photo_data,
	       COALESCE(p.note,''), p.status, COALESCE(text(p.created_at),''),
	       COALESCE(text(p.reviewed_at),''), COALESCE(p.review_note,''),
	       c.bar_not_ready_count, c.driver_fail_count
		FROM empty_photos p
		JOIN keg_cycles c ON c.id=p.cycle_id
		JOIN kegs k ON k.id=c.keg_id
		JOIN bars b ON b.id=p.bar_id`
	args := []interface{}{}
	if r.URL.Query().Get("status") != "" {
		q += " WHERE p.status=$1"
		args = append(args, r.URL.Query().Get("status"))
	} else {
		// 默认：待审核在前，其余最近记录在后
		q += " ORDER BY (p.status='PENDING') DESC, p.id DESC LIMIT 50"
		rows, err := db.Query(q)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeOK(w, M{"photos": scanPhotoRows(rows)})
		return
	}
	q += " ORDER BY p.id DESC LIMIT 50"
	rows, err := db.Query(q, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeOK(w, M{"photos": scanPhotoRows(rows)})
}

func scanPhotoRows(rs *sql.Rows) []M {
	return scanRows(rs, func(x rowScanner) (M, error) {
		var id, cycleID, barID, notReady, driverFail int64
		var code, bar, data, note, status, createdAt, reviewedAt, reviewNote string
		if err := x.Scan(&id, &cycleID, &code, &barID, &bar, &data, &note, &status,
			&createdAt, &reviewedAt, &reviewNote, &notReady, &driverFail); err != nil {
			return nil, err
		}
		return M{"id": id, "cycle_id": cycleID, "keg_code": code, "bar_id": barID, "bar": bar,
			"photo_data": data, "note": note, "status": status, "created_at": createdAt,
			"reviewed_at": reviewedAt, "review_note": reviewNote,
			"bar_not_ready_count": notReady, "driver_fail_count": driverFail}, nil
	})
}

// GET /api/cycles/photos/{cycleId}  查看某只桶的现场照片（本店店长/管理员）
func handleCyclePhotos(w http.ResponseWriter, r *http.Request) {
	id := tailID(r.URL.Path, "/api/cycles/photos/")
	u := currentUser(r)
	var barID int64
	if err := db.QueryRow(`SELECT bar_id FROM keg_cycles WHERE id=$1`, id).Scan(&barID); err != nil {
		fail(w, 400, "流转记录不存在")
		return
	}
	if u.Role != "admin" && u.Role != "finance" && u.Role != "brewery" && barID != u.OrgID {
		fail(w, 403, "只能查看本店酒桶照片")
		return
	}
	rows, err := db.Query(`SELECT id, photo_data, COALESCE(note,''), status,
		COALESCE(text(created_at),''), COALESCE(text(reviewed_at),''), COALESCE(review_note,'')
		FROM empty_photos WHERE cycle_id=$1 ORDER BY id DESC`, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	list := scanRows(rows, func(x rowScanner) (M, error) {
		var pid int64
		var data, note, status, at, reviewedAt, reviewNote string
		if err := x.Scan(&pid, &data, &note, &status, &at, &reviewedAt, &reviewNote); err != nil {
			return nil, err
		}
		return M{"id": pid, "photo_data": data, "note": note, "status": status,
			"created_at": at, "reviewed_at": reviewedAt, "review_note": reviewNote}, nil
	})
	writeOK(w, M{"photos": list})
}

// GET /api/bar/alerts  当前店长的逾期/责任提醒（admin 可带 bar_id）
func handleBarAlerts(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	barID := u.OrgID
	if r.URL.Query().Get("bar_id") != "" && u.Role == "admin" {
		barID = atoi64(r.URL.Query().Get("bar_id"))
	}
	rows, err := db.Query(`SELECT a.id, a.kind, COALESCE(a.cycle_id,0), a.title, a.body,
		COALESCE(text(a.created_at),''), a.ack_at IS NOT NULL, COALESCE(k.keg_code,'')
		FROM bar_alerts a
		LEFT JOIN keg_cycles c ON c.id=a.cycle_id
		LEFT JOIN kegs k ON k.id=c.keg_id
		WHERE a.bar_id=$1 ORDER BY a.id DESC LIMIT 50`, barID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	list := scanRows(rows, func(rs rowScanner) (M, error) {
		var id, cycleID int64
		var kind, title, body, at, code string
		var acked bool
		if err := rs.Scan(&id, &kind, &cycleID, &title, &body, &at, &acked, &code); err != nil {
			return nil, err
		}
		return M{"id": id, "kind": kind, "cycle_id": cycleID, "title": title, "body": body,
			"created_at": at, "acked": acked, "keg_code": code}, nil
	})
	unacked := 0
	for _, a := range list {
		if !a["acked"].(bool) {
			unacked++
		}
	}
	writeOK(w, M{"alerts": list, "unacked_count": unacked})
}

// POST /api/bar/alerts/ack/{id}
func handleAckBarAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	id := tailID(r.URL.Path, "/api/bar/alerts/ack/")
	u := currentUser(r)
	res, err := db.Exec(`UPDATE bar_alerts SET ack_by=$1, ack_at=now()
		WHERE id=$2 AND ack_at IS NULL AND ($3='admin' OR bar_id=$4)`,
		u.ID, id, u.Role, u.OrgID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 400, "提醒不存在、已确认或不属于本店")
		return
	}
	writeOK(w, M{"ok": true})
}
