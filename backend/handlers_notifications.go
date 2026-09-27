package main

import (
	"net/http"
)

// GET /api/notifications  当前用户可见通知（停售/召回）
func handleNotifications(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)

	// 角色可见范围
	where := ""
	args := []interface{}{}
	switch u.Role {
	case "bar_manager":
		where = "(audience='ALL' OR (audience='BAR' AND target_id=$1))"
		args = append(args, u.OrgID)
	case "dealer":
		where = "(audience='ALL' OR (audience='DEALER_STOCK' AND target_id=$1))"
		args = append(args, u.OrgID)
	case "driver":
		where = "(audience='ALL' OR audience='PENDING_ORDER')"
	default:
		where = "TRUE"
	}

	q := `SELECT n.id, ba.batch_no, be.name, be.style, n.audience, n.title, n.body,
	       COALESCE(text(n.created_at),''), n.ack_at IS NOT NULL
		FROM notifications n
		JOIN batches ba ON ba.id=n.batch_id
		JOIN beers be ON be.id=ba.beer_id
		WHERE ` + where + `
		ORDER BY n.id DESC LIMIT 100`
	rows, err := db.Query(q, args...)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	list := scanRows(rows, func(rs rowScanner) (M, error) {
		var id int64
		var batchNo, beer, style, audience, title, body, createdAt string
		var acked bool
		if err := rs.Scan(&id, &batchNo, &beer, &style, &audience, &title, &body, &createdAt, &acked); err != nil {
			return nil, err
		}
		return M{"id": id, "batch_no": batchNo, "beer": beer, "style": style,
			"audience": audience, "title": title, "body": body,
			"created_at": createdAt, "acked": acked}, nil
	})
	writeOK(w, M{"notifications": list})
}

// POST /api/notifications/ack/{id}
func handleAckNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	id := tailID(r.URL.Path, "/api/notifications/ack/")
	u := currentUser(r)
	_, err := db.Exec(`UPDATE notifications SET ack_by=$1, ack_at=now()
		WHERE id=$2 AND ack_at IS NULL`, u.ID, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeOK(w, M{"ok": true})
}
