package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// M 是 JSON 响应的便捷类型
type M = map[string]interface{}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOK(w http.ResponseWriter, v interface{}) {
	if v == nil {
		v = M{"ok": true}
	}
	writeJSON(w, 200, v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, M{"error": msg})
}

func decodeJSON(r *http.Request, dst interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("请求体 JSON 解析失败: " + err.Error())
	}
	return nil
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// tailID 取 /api/xxx/<id> 中的 id
func tailID(path, prefix string) int64 {
	return atoi64(strings.TrimPrefix(path, prefix))
}

// ---------- 鉴权（HMAC 无状态 token） ----------

type ctxKey string

const userCtx ctxKey = "user"

type sessionUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	OrgID    int64  `json:"org_id"`
	Exp      int64  `json:"exp"`
}

func tokenSecret() []byte {
	s := os.Getenv("KEG_TOKEN_SECRET")
	if s == "" {
		s = "kegplatform-dev-secret"
	}
	return []byte(s)
}

// token 格式: <userID>.<expUnix>.<hex(HMAC)>
func signToken(u sessionUser) string {
	payload := strconv.FormatInt(u.ID, 10) + "." + strconv.FormatInt(u.Exp, 10)
	mac := hmac.New(sha256.New, tokenSecret())
	mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

func parseToken(t string) (sessionUser, bool) {
	var u sessionUser
	parts := strings.Split(t, ".")
	if len(parts) != 3 {
		return u, false
	}
	mac := hmac.New(sha256.New, tokenSecret())
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return u, false
	}
	u.ID = atoi64(parts[0])
	u.Exp = atoi64(parts[1])
	if time.Now().Unix() > u.Exp {
		return u, false
	}
	return u, true
}

func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		tok := strings.TrimPrefix(h, "Bearer ")
		u, ok := parseToken(tok)
		if !ok {
			fail(w, 401, "未登录或登录已过期")
			return
		}
		// 每次请求补齐用户信息
		var org sql.NullInt64
		err := db.QueryRow(`SELECT username, display_name, role, org_id FROM app_users WHERE id=$1`, u.ID).
			Scan(&u.Username, &u.Name, &u.Role, &org)
		if err == sql.ErrNoRows {
			fail(w, 401, "账号不存在")
			return
		}
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		u.OrgID = org.Int64
		ctx := context.WithValue(r.Context(), userCtx, u)
		next(w, r.WithContext(ctx))
	}
}

func currentUser(r *http.Request) sessionUser {
	v := r.Context().Value(userCtx)
	return v.(sessionUser)
}

func requireRole(roles string, next http.HandlerFunc) http.HandlerFunc {
	allowed := map[string]bool{}
	for _, x := range strings.Fields(roles) {
		allowed[x] = true
	}
	return func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if !allowed[u.Role] {
			fail(w, 403, "当前角色无权操作（需要: "+roles+"）")
			return
		}
		next(w, r)
	}
}
