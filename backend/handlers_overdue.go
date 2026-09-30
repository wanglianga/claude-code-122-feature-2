package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"
)

// 空桶逾期押金冻结全链路：
//  1. 平台按桶编号冻结超账期未归还桶的押金，并提醒店长预约回收
//  2. 司机到店但酒吧未备好空桶 / 酒吧备好但司机未到店，分别登记到场记录，影响逾期责任判定
//  3. 冻结后店长可上传"已打空但未回收"现场照片、确认回收时间
//  4. 财务按凭证判定逾期责任：司机责任解除冻结，押金释放时间重新计算；酒吧责任维持冻结可扣款

const maxPhotoBytes = 6 * 1024 * 1024 // 演示环境照片以内联 data URI 存储，单张上限 6MB

// ---------- 桶级催还/责任提醒 ----------

// GET /api/bar/alerts  当前用户可见的桶级提醒（逾期冻结/责任结论/回收时间确认）
func handleBarAlerts(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	q := `SELECT a.id, a.cycle_id, k.keg_code, b.name, a.kind, a.title, a.body,
	       COALESCE(text(a.created_at),''), a.ack_at IS NOT NULL
	      FROM bar_alerts a
	      JOIN keg_cycles c ON c.id=a.cycle_id
	      JOIN kegs k ON k.id=c.keg_id
	      JOIN bars b ON b.id=a.bar_id
	      WHERE ($1=0 OR a.bar_id=$1)
	      ORDER BY a.id DESC LIMIT 100`
	var barFilter int64
	switch u.Role {
	case "bar_manager":
		barFilter = u.OrgID
	case "admin", "brewery", "finance", "qc":
		if s := r.URL.Query().Get("bar_id"); s != "" {
			barFilter = atoi64(s)
		}
	default:
		barFilter = -1 // 司机/清洗厂/经销商不看桶级提醒
	}
	rows, err := db.Query(q, barFilter)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	list := scanRows(rows, func(rs rowScanner) (M, error) {
		var id, cid int64
		var code, bar, kind, title, body, createdAt string
		var acked bool
		if err := rs.Scan(&id, &cid, &code, &bar, &kind, &title, &body, &createdAt, &acked); err != nil {
			return nil, err
		}
		return M{"id": id, "cycle_id": cid, "keg_code": code, "bar": bar,
			"kind": kind, "title": title, "body": body,
			"created_at": createdAt, "acked": acked}, nil
	})
	writeOK(w, M{"alerts": list})
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

// ---------- 逾期冻结 ----------

// POST /api/finance/freeze-overdue  按桶编号冻结逾期未归还桶押金；不传 cycle_ids 则冻结全部符合条件桶
// body: {cycle_ids?: [..]}
func handleFreezeOverdue(w http.ResponseWriter, r *http.Request) {
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
		q := `SELECT c.id, c.keg_id, k.keg_code, c.bar_id, c.deposit_amount,
		              c.deposit_due_date, c.stage, b.name
		       FROM keg_cycles c JOIN kegs k ON k.id=c.keg_id
		       JOIN bars b ON b.id=c.bar_id
		       WHERE c.deposit_status='HELD'
		         AND c.deposit_due_date < CURRENT_DATE
		         AND c.stage NOT IN ('RETURNED','WASHED','FAILED')`
		args := []interface{}{}
		if len(req.CycleIDs) > 0 {
			q += ` AND c.id = ANY($1)`
			args = append(args, req.CycleIDs)
		}
		q += ` ORDER BY c.id`
		rows, err := tx.Query(q, args...)
		if err != nil {
			return err
		}
		type cand struct {
			cid, kegID, barID int64
			code, bar, stage  string
			deposit           float64
			due               sql.NullTime
		}
		var cands []cand
		for rows.Next() {
			var x cand
			if err := rows.Scan(&x.cid, &x.kegID, &x.code, &x.barID, &x.deposit, &x.due, &x.stage, &x.bar); err != nil {
				rows.Close()
				return err
			}
			cands = append(cands, x)
		}
		rows.Close()

		for _, x := range cands {
			if _, err := tx.Exec(`UPDATE keg_cycles
				SET deposit_status='FROZEN', deposit_frozen_at=now(), late_return=TRUE
				WHERE id=$1`, x.cid); err != nil {
				return err
			}
			// 同一桶只发一次冻结催还提醒
			var n int
			if err := tx.QueryRow(`SELECT count(*) FROM bar_alerts
				WHERE cycle_id=$1 AND kind='OVERDUE_FREEZE'`, x.cid).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				hint := "请尽快在「我的在店桶」中预约回收并确认回收时间"
				if x.stage == "RESERVED" {
					hint = "您已预约回收，请与司机重新确认到店时间"
				}
				body := "该桶已超过约定押金账期（" + dateStr(x.due) + "）仍未归还，平台已按桶编号冻结押金 ¥" +
					trimFloat(x.deposit) + "。" + hint + "；可上传已打空现场照片说明情况，逾期责任确认后押金释放时间将重新计算。"
				if _, err := tx.Exec(`INSERT INTO bar_alerts (cycle_id,bar_id,kind,title,body)
					VALUES ($1,$2,'OVERDUE_FREEZE',$3,$4)`, x.cid, x.barID,
					"押金冻结提醒："+x.code+" 已超账期未归还", body); err != nil {
					return err
				}
			}
			if err := logKegEvent(tx, x.kegID, x.cid, "DEPOSIT_FROZEN", "平台",
				M{"deposit": x.deposit, "due": dateStr(x.due), "bar": x.bar}); err != nil {
				return err
			}
			frozen = append(frozen, M{"cycle_id": x.cid, "keg_code": x.code,
				"bar": x.bar, "deposit_amount": x.deposit})
		}
		return nil
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeOK(w, M{"frozen": frozen, "count": len(frozen)})
}

// ---------- 逾期责任判定 ----------

// POST /api/finance/responsibility/{cycleId}
// body: {responsibility: BAR|DRIVER|CLEARED, note, release_days?}
// 司机责任/无责：解除冻结，押金释放时间按"确认回收时间 + release_days（默认 7 天）"重新计算
// 酒吧责任：维持冻结，财务可继续扣款/部分扣款
func handleResponsibility(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	id := tailID(r.URL.Path, "/api/finance/responsibility/")
	var req struct {
		Responsibility string `json:"responsibility"`
		Note           string `json:"note"`
		ReleaseDays    int    `json:"release_days"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if req.Responsibility != "BAR" && req.Responsibility != "DRIVER" && req.Responsibility != "CLEARED" {
		fail(w, 400, "责任方应为 BAR（酒吧未备货）/ DRIVER（司机到店失败）/ CLEARED（无责）")
		return
	}
	if req.ReleaseDays <= 0 {
		req.ReleaseDays = 7 // 回收并清洗合格后 7 日内释放
	}

	u := currentUser(r)
	resp := M{}
	err := runTx(func(tx *sql.Tx) error {
		var kegID, barID int64
		var status, code string
		var deposit float64
		if err := tx.QueryRow(`SELECT c.keg_id, c.bar_id, c.deposit_status, c.deposit_amount, k.keg_code
			FROM keg_cycles c JOIN kegs k ON k.id=c.keg_id
			WHERE c.id=$1 FOR UPDATE`, id).Scan(&kegID, &barID, &status, &deposit, &code); err != nil {
			return errBad("流转记录不存在")
		}
		if status != "FROZEN" {
			return errBad("仅逾期冻结中的押金需要责任判定，当前状态: " + status)
		}

		// 释放时间锚点：店长确认的回收时间（取最近一次有效预约日），无或已过期则按今天
		var anchor sql.NullTime
		_ = tx.QueryRow(`SELECT max(pickup_date) FROM return_reservations
			WHERE cycle_id=$1 AND status<>'CANCELLED'`, id).Scan(&anchor)
		var releaseDate string
		if err := tx.QueryRow(`SELECT (CASE WHEN $1::date IS NULL OR $1::date < CURRENT_DATE
			THEN CURRENT_DATE ELSE $1::date END + $2::int)::text`,
			nullableTime(anchor), req.ReleaseDays).Scan(&releaseDate); err != nil {
			return err
		}

		newStatus := "FROZEN"
		conclusion := ""
		switch req.Responsibility {
		case "BAR":
			newStatus = "FROZEN"
			conclusion = "酒吧未备好空桶，逾期责任由酒吧承担，押金维持冻结等待财务结算"
		case "DRIVER":
			newStatus = "HELD"
			conclusion = "司机已到店失败（酒吧已备货），逾期不由酒吧承担，押金解除冻结"
		case "CLEARED":
			newStatus = "HELD"
			conclusion = "双方无责/已协商厘清，押金解除冻结"
		}

		if _, err := tx.Exec(`UPDATE keg_cycles
			SET deposit_status=$1, overdue_responsibility=$2, responsibility_note=NULLIF($3,''),
			    responsibility_by=$4, responsibility_at=now(), deposit_release_date=$5
			WHERE id=$6`, newStatus, req.Responsibility, req.Note, u.ID, releaseDate, id); err != nil {
			return err
		}

		title := "逾期责任判定结果：" + code
		body := conclusion + "。押金释放时间重新计算为 " + releaseDate + "。"
		if req.Note != "" {
			body += " 备注：" + req.Note
		}
		if _, err := tx.Exec(`INSERT INTO bar_alerts (cycle_id,bar_id,kind,title,body)
			VALUES ($1,$2,'RESPONSIBILITY',$3,$4)`, id, barID, title, body); err != nil {
			return err
		}
		if err := logKegEvent(tx, kegID, id, "RESPONSIBILITY_CONFIRMED", "财务",
			M{"responsibility": req.Responsibility, "deposit_status": newStatus,
				"release_date": releaseDate, "note": req.Note}); err != nil {
			return err
		}
		resp = M{"deposit_status": newStatus, "overdue_responsibility": req.Responsibility,
			"deposit_release_date": releaseDate}
		return nil
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeOK(w, resp)
}

// ---------- 司机到场记录 ----------

// POST /api/pickups/visit  到场异常登记
// BAR_NOT_READY：司机已到店但酒吧未备好空桶（driver/admin 登记）
// DRIVER_FAILED：酒吧已备好但司机未按约到店（bar_manager/admin 登记）
// body: {reservation_id, result, note}
func handlePickupVisit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	var req struct {
		ReservationID int64  `json:"reservation_id"`
		Result        string `json:"result"`
		Note          string `json:"note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if req.ReservationID == 0 {
		fail(w, 400, "缺少回收预约")
		return
	}
	if req.Result != "BAR_NOT_READY" && req.Result != "DRIVER_FAILED" {
		fail(w, 400, "到场结果应为 BAR_NOT_READY（酒吧未备货）或 DRIVER_FAILED（司机到店失败）")
		return
	}
	u := currentUser(r)
	if req.Result == "BAR_NOT_READY" && u.Role != "admin" && u.Role != "driver" {
		fail(w, 403, "仅到场司机可登记「酒吧未备好空桶」")
		return
	}
	if req.Result == "DRIVER_FAILED" && u.Role != "admin" && u.Role != "bar_manager" {
		fail(w, 403, "仅酒吧店长可登记「司机到店失败」")
		return
	}

	evType := map[string]string{"BAR_NOT_READY": "VISIT_BAR_NOT_READY", "DRIVER_FAILED": "VISIT_DRIVER_FAILED"}
	err := runTx(func(tx *sql.Tx) error {
		var barID, driverID sql.NullInt64
		var status string
		if err := tx.QueryRow(`SELECT bar_id, driver_id, status FROM return_reservations
			WHERE id=$1 FOR UPDATE`, req.ReservationID).
			Scan(&barID, &driverID, &status); err != nil {
			return errBad("回收预约不存在")
		}
		if status == "PICKED" || status == "CANCELLED" {
			return errBad("该预约已取桶/已取消，不能再登记到场异常")
		}
		cycleID, kegID, code := cycleOfReservation(tx, req.ReservationID)
		if cycleID == 0 {
			return errBad("该预约没有关联酒桶")
		}

		switch req.Result {
		case "BAR_NOT_READY":
			if u.Role != "admin" && u.Role != "driver" {
				return errBad("仅到场司机可登记「酒吧未备好空桶」")
			}
			if u.Role == "driver" {
				if !driverInCity(tx, u.OrgID, barID.Int64) {
					return errBad("只能登记本市酒吧的到场记录")
				}
				driverID = sql.NullInt64{Int64: u.OrgID, Valid: true}
			}
		case "DRIVER_FAILED":
			if u.Role != "admin" && u.Role != "bar_manager" {
				return errBad("仅酒吧店长可登记「司机到店失败」")
			}
			if u.Role == "bar_manager" && barID.Int64 != u.OrgID {
				return errBad("只能登记本店预约")
			}
		}

		if _, err := tx.Exec(`INSERT INTO driver_visits
			(reservation_id,cycle_id,bar_id,driver_id,result,note,created_by)
			VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7)`,
			req.ReservationID, cycleID, barID.Int64, driverID, req.Result, req.Note, u.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE return_reservations
			SET status=$1, visit_note=NULLIF($2,''), visited_at=now(),
			    driver_id=COALESCE(driver_id,$3)
			WHERE id=$4`, req.Result, req.Note, driverID.Int64, req.ReservationID); err != nil {
			return err
		}
		actor := "司机"
		if u.Role == "bar_manager" {
			actor = "酒吧"
		}
		return logKegEvent(tx, kegID, cycleID, evType[req.Result], actor,
			M{"reservation_id": req.ReservationID, "keg_code": code, "note": req.Note})
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeOK(w, M{"ok": true})
}

// ---------- 现场照片申诉 ----------

// /api/cycles/proof/{id}：POST 店长上传照片，GET 财务/店长查看凭证
func handleProofSubroute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleUploadProof(w, r)
	case http.MethodGet:
		handleListProofs(w, r)
	default:
		fail(w, 405, "仅支持 GET/POST")
	}
}

// POST /api/cycles/proof/{id}  冻结后店长上传"已打空但未回收"现场照片
// body: {photo_data: "data:image/...;base64,...", note}
func handleUploadProof(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	id := tailID(r.URL.Path, "/api/cycles/proofs/")
	var req struct {
		PhotoData string `json:"photo_data"`
		Note      string `json:"note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if len(req.PhotoData) < 32 || !startsWith(req.PhotoData, "data:image/") {
		fail(w, 400, "请上传图片（data:image 格式）")
		return
	}
	if len(req.PhotoData) > maxPhotoBytes {
		fail(w, 400, "照片过大，请压缩到 6MB 以内")
		return
	}
	u := currentUser(r)
	if u.Role != "admin" && u.Role != "bar_manager" {
		fail(w, 403, "仅酒吧店长可上传现场照片")
		return
	}

	var proofID int64
	err := runTx(func(tx *sql.Tx) error {
		var kegID, barID int64
		var stage string
		if err := tx.QueryRow(`SELECT keg_id, bar_id, stage FROM keg_cycles WHERE id=$1 FOR UPDATE`, id).
			Scan(&kegID, &barID, &stage); err != nil {
			return errBad("流转记录不存在")
		}
		if u.Role != "admin" && barID != u.OrgID {
			return errBad("只能为本店酒桶上传现场照片")
		}
		if stage == "RETURNED" || stage == "WASHED" || stage == "FAILED" {
			return errBad("该桶已回收闭环，无需上传未回收照片")
		}
		if err := tx.QueryRow(`INSERT INTO empty_proofs (cycle_id,bar_id,photo_data,note,created_by)
			VALUES ($1,$2,$3,NULLIF($4,''),$5) RETURNING id`,
			id, barID, req.PhotoData, req.Note, u.ID).Scan(&proofID); err != nil {
			return err
		}
		return logKegEvent(tx, kegID, id, "EMPTY_PROOF_UPLOADED", "酒吧", M{"note": req.Note})
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeOK(w, M{"proof_id": proofID})
}

// GET /api/cycles/proofs/{id}  现场照片 + 到场历史（责任判定凭证）
func handleListProofs(w http.ResponseWriter, r *http.Request) {
	id := tailID(r.URL.Path, "/api/cycles/proofs/")
	u := currentUser(r)

	var barID int64
	var code string
	err := db.QueryRow(`SELECT c.bar_id, k.keg_code FROM keg_cycles c
		JOIN kegs k ON k.id=c.keg_id WHERE c.id=$1`, id).Scan(&barID, &code)
	if err == sql.ErrNoRows {
		fail(w, 404, "流转记录不存在")
		return
	}
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if u.Role == "bar_manager" && barID != u.OrgID {
		fail(w, 403, "只能查看本店酒桶凭证")
		return
	}
	switch u.Role {
	case "bar_manager", "admin", "finance", "brewery":
	default:
		fail(w, 403, "当前角色无权查看现场凭证")
		return
	}

	rows, _ := db.Query(`SELECT p.id, COALESCE(p.note,''), p.photo_data,
		COALESCE(text(p.created_at),''), COALESCE(au.display_name,'')
		FROM empty_proofs p
		LEFT JOIN app_users au ON au.id=p.created_by
		WHERE p.cycle_id=$1 ORDER BY p.id`, id)
	proofs := scanRows(rows, func(rs rowScanner) (M, error) {
		var pid int64
		var note, photo, at, by string
		if err := rs.Scan(&pid, &note, &photo, &at, &by); err != nil {
			return nil, err
		}
		return M{"id": pid, "note": note, "photo_data": photo, "created_at": at, "created_by": by}, nil
	})

	vrows, _ := db.Query(`SELECT v.result, COALESCE(v.note,''), COALESCE(text(v.visited_at),''),
		COALESCE(d.name,'')
		FROM driver_visits v LEFT JOIN drivers d ON d.id=v.driver_id
		WHERE v.cycle_id=$1 ORDER BY v.id`, id)
	visits := scanRows(vrows, func(rs rowScanner) (M, error) {
		var result, note, at, driver string
		if err := rs.Scan(&result, &note, &at, &driver); err != nil {
			return nil, err
		}
		return M{"result": result, "note": note, "visited_at": at, "driver": driver}, nil
	})

	writeOK(w, M{"cycle_id": id, "keg_code": code, "proofs": proofs, "visits": visits})
}

// ---------- 店长确认回收时间 ----------

// POST /api/cycles/confirm-pickup  店长确认回收时间（有异常预约则改约，无预约则直接新建）
// body: {cycle_id, reservation_id?, pickup_date, note}
func handleConfirmPickup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	var req struct {
		CycleID       int64  `json:"cycle_id"`
		ReservationID int64  `json:"reservation_id"`
		PickupDate    string `json:"pickup_date"`
		Note          string `json:"note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if req.CycleID == 0 || req.PickupDate == "" {
		fail(w, 400, "请选择酒桶并确认回收日期")
		return
	}
	if _, err := parseDate(req.PickupDate); err != nil {
		fail(w, 400, "回收日期格式应为 YYYY-MM-DD")
		return
	}
	u := currentUser(r)

	resID := int64(0)
	err := runTx(func(tx *sql.Tx) error {
		var kegID, barID int64
		var stage string
		if err := tx.QueryRow(`SELECT keg_id, bar_id, stage FROM keg_cycles WHERE id=$1 FOR UPDATE`,
			req.CycleID).Scan(&kegID, &barID, &stage); err != nil {
			return errBad("流转记录不存在")
		}
		if u.Role != "admin" && barID != u.OrgID {
			return errBad("只能确认本店酒桶的回收时间")
		}
		if stage != "EMPTY" && stage != "RESERVED" {
			return errBad("该桶当前阶段不能确认回收时间: " + stage)
		}

		var barName, code string
		_ = tx.QueryRow(`SELECT name FROM bars WHERE id=$1`, barID).Scan(&barName)
		_ = tx.QueryRow(`SELECT keg_code FROM kegs WHERE id=$1`, kegID).Scan(&code)

		// 指定了预约：校验归属并改约
		if req.ReservationID != 0 {
			var rvBar int64
			var rvStatus string
			if err := tx.QueryRow(`SELECT bar_id, status FROM return_reservations
				WHERE id=$1 FOR UPDATE`, req.ReservationID).Scan(&rvBar, &rvStatus); err != nil {
				return errBad("回收预约不存在")
			}
			if rvBar != barID {
				return errBad("预约与酒桶不属于同一家酒吧")
			}
			if rvStatus == "PICKED" || rvStatus == "CANCELLED" {
				return errBad("该预约已结束，请新建回收预约")
			}
			if _, err := tx.Exec(`UPDATE return_reservations
				SET pickup_date=$1, note=COALESCE(NULLIF($2,''),note), status='WAITING', visited_at=NULL
				WHERE id=$3`, req.PickupDate, req.Note, req.ReservationID); err != nil {
				return err
			}
			resID = req.ReservationID
		} else {
			// 未指定：找最近一张未结束的预约改约，否则新建
			var found int64
			_ = tx.QueryRow(`SELECT id FROM return_reservations
				WHERE cycle_id=$1 AND status NOT IN ('PICKED','CANCELLED')
				ORDER BY id DESC LIMIT 1`, req.CycleID).Scan(&found)
			if found != 0 {
				if _, err := tx.Exec(`UPDATE return_reservations
					SET pickup_date=$1, note=COALESCE(NULLIF($2,''),note), status='WAITING', visited_at=NULL
					WHERE id=$3`, req.PickupDate, req.Note, found); err != nil {
					return err
				}
				resID = found
			} else {
				if err := tx.QueryRow(`INSERT INTO return_reservations
					(cycle_id,bar_id,pickup_date,empty_count,note,status)
					VALUES ($1,$2,$3,1,NULLIF($4,''),'WAITING') RETURNING id`,
					req.CycleID, barID, req.PickupDate, req.Note).Scan(&resID); err != nil {
					return err
				}
				if _, err := tx.Exec(`INSERT INTO reservation_cycles (reservation_id,cycle_id)
					VALUES ($1,$2) ON CONFLICT DO NOTHING`, resID, req.CycleID); err != nil {
					return err
				}
			}
		}

		if _, err := tx.Exec(`UPDATE keg_cycles
			SET stage='RESERVED', return_reserved_at=COALESCE(return_reserved_at,now()),
			    pickup_confirmed_at=now(), pickup_confirmed_by=$1, pickup_confirm_note=NULLIF($2,'')
			WHERE id=$3`, u.ID, req.Note, req.CycleID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO bar_alerts (cycle_id,bar_id,kind,title,body)
			VALUES ($1,$2,'PICKUP_CONFIRMED',$3,$4)`,
			req.CycleID, barID,
			"店长已确认回收时间："+code+" · "+req.PickupDate,
			barName+" 已确认于 "+req.PickupDate+" 交回空桶，请安排司机按时到店。"+
				ifEmpty(req.Note, "")); err != nil {
			return err
		}
		return logKegEvent(tx, kegID, req.CycleID, "PICKUP_TIME_CONFIRMED", "酒吧",
			M{"reservation_id": resID, "pickup_date": req.PickupDate, "note": req.Note})
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeOK(w, M{"reservation_id": resID, "pickup_date": req.PickupDate})
}

// ---------- 辅助 ----------

func cycleOfReservation(tx *sql.Tx, reservationID int64) (cycleID, kegID int64, code string) {
	_ = tx.QueryRow(`SELECT c.id, c.keg_id, k.keg_code FROM return_reservations rv
		JOIN keg_cycles c ON c.id=rv.cycle_id
		JOIN kegs k ON k.id=c.keg_id WHERE rv.id=$1`, reservationID).
		Scan(&cycleID, &kegID, &code)
	if cycleID == 0 {
		_ = tx.QueryRow(`SELECT rc.cycle_id, c.keg_id, k.keg_code FROM reservation_cycles rc
			JOIN keg_cycles c ON c.id=rc.cycle_id
			JOIN kegs k ON k.id=c.keg_id
			WHERE rc.reservation_id=$1 ORDER BY rc.cycle_id LIMIT 1`, reservationID).
			Scan(&cycleID, &kegID, &code)
	}
	return cycleID, kegID, code
}

func driverInCity(tx *sql.Tx, driverID, barID int64) bool {
	var n int
	_ = tx.QueryRow(`SELECT count(*) FROM drivers d JOIN bars b ON b.city_id=d.city_id
		WHERE d.id=$1 AND b.id=$2`, driverID, barID).Scan(&n)
	return n > 0
}

func nullableTime(t sql.NullTime) interface{} {
	if !t.Valid {
		return nil
	}
	return t.Time.Format("2006-01-02")
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

func ifEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return " 备注：" + s
}
