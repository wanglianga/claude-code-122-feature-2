package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"
)

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "仅支持 POST")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	sum := sha256.Sum256([]byte(req.Password))
	hash := hex.EncodeToString(sum[:])

	var u sessionUser
	var org sql.NullInt64
	err := db.QueryRow(
		`SELECT id, username, display_name, role, org_id FROM app_users
		 WHERE username=$1 AND password_sha=$2`, req.Username, hash).
		Scan(&u.ID, &u.Username, &u.Name, &u.Role, &org)
	if err == sql.ErrNoRows {
		fail(w, 401, "用户名或密码错误")
		return
	}
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	u.OrgID = org.Int64
	u.Exp = time.Now().Add(12 * time.Hour).Unix()
	writeOK(w, M{"token": signToken(u), "user": u})
}
