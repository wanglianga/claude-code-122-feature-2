package main

import "database/sql"

// rows 是 *sql.Rows 的别名，方便 scanRows 回调签名
type rowScanner = *sql.Rows

// scanRows 通用行扫描
func scanRows(rs *sql.Rows, f func(rowScanner) (M, error)) []M {
	out := []M{}
	if rs == nil {
		return out
	}
	defer rs.Close()
	for rs.Next() {
		m, err := f(rs)
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	return out
}

func dateStr(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format("2006-01-02")
}

func tsStr(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format("2006-01-02 15:04")
}

func nstr(s sql.NullString) string { return s.String }
func nint64(n sql.NullInt64) int64 { return n.Int64 }
