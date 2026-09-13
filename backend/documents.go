package main

import (
	"context"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	_ "golang.org/x/image/webp"
)

type Document struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	Body     json.RawMessage `json:"body"`
	Revision int             `json:"revision"`
	Updated  time.Time       `json:"updated_at"`
	Expires  *time.Time      `json:"expires_at"`
}

func (a *App) listDocs(w http.ResponseWriter, r *http.Request) {
	rows, e := a.db.Query(r.Context(), `SELECT id,kind,body,revision,updated_at,expires_at FROM documents WHERE owner_id=$1 ORDER BY updated_at DESC`, user(r).ID)
	if e != nil {
		fail(w, 500, "读取失败")
		return
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var d Document
		if rows.Scan(&d.ID, &d.Kind, &d.Body, &d.Revision, &d.Updated, &d.Expires) == nil {
			out = append(out, d)
		}
	}
	reply(w, 200, out)
}
func validKind(k string) bool { return k == "brand" || k == "product" || k == "project" }
func (a *App) checkAssets(ctx context.Context, owner string, v any) bool {
	return checkAssetsWith(a.db, ctx, owner, v)
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func checkAssetsWith(q rowQuerier, ctx context.Context, owner string, v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, v := range x {
			if k == "assetId" || k == "backgroundId" || k == "logoId" {
				if s, ok := v.(string); ok && s != "" {
					var yes bool
					if q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM assets WHERE id=$1 AND owner_id=$2)`, s, owner).Scan(&yes) != nil || !yes {
						return false
					}
				}
			}
			if !checkAssetsWith(q, ctx, owner, v) {
				return false
			}
		}
	case []any:
		for _, v := range x {
			if !checkAssetsWith(q, ctx, owner, v) {
				return false
			}
		}
	}
	return true
}
func (a *App) saveDoc(w http.ResponseWriter, r *http.Request) {
	var d Document
	if !read(w, r, &d) {
		return
	}
	if !validKind(d.Kind) {
		fail(w, 400, "资料类型无效")
		return
	}
	var body map[string]any
	if json.Unmarshal(d.Body, &body) != nil || body == nil {
		fail(w, 400, "内容无效")
		return
	}
	tx, e := a.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "数据库不可用")
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(728420)`); e != nil {
		fail(w, 503, "保存暂不可用")
		return
	}
	if !checkAssetsWith(tx, r.Context(), user(r).ID, body) {
		fail(w, 400, "素材不存在或不属于当前账号")
		return
	}
	var expires *time.Time
	if d.Kind == "project" {
		v := time.Now().Add(30 * 24 * time.Hour)
		expires = &v
	}
	if r.Method == "POST" {
		d.ID = id()
		d.Revision = 1
		e = tx.QueryRow(r.Context(), `INSERT INTO documents(id,owner_id,kind,body,expires_at) VALUES($1,$2,$3,$4,$5) RETURNING updated_at,expires_at`, d.ID, user(r).ID, d.Kind, d.Body, expires).Scan(&d.Updated, &d.Expires)
	} else {
		d.ID = r.PathValue("id")
		e = tx.QueryRow(r.Context(), `UPDATE documents SET body=$1,revision=revision+1,updated_at=now(),expires_at=$2 WHERE id=$3 AND owner_id=$4 AND revision=$5 AND kind=$6 RETURNING revision,updated_at,expires_at`, d.Body, expires, d.ID, user(r).ID, d.Revision, d.Kind).Scan(&d.Revision, &d.Updated, &d.Expires)
	}
	if e != nil {
		fail(w, 409, "保存冲突，请刷新后重试（品牌资料每人仅一份）")
		return
	}
	if d.Kind == "project" {
		_, e = tx.Exec(r.Context(), `INSERT INTO versions(document_id,revision,body) VALUES($1,$2,$3)`, d.ID, d.Revision, d.Body)
	}
	if e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "保存失败")
		return
	}
	reply(w, 200, d)
}
func (a *App) deleteDoc(w http.ResponseWriter, r *http.Request) {
	tx, e := a.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "数据库不可用")
		return
	}
	defer tx.Rollback(r.Context())
	var owner string
	e = tx.QueryRow(r.Context(), `SELECT owner_id FROM documents WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&owner)
	if e != nil || owner != user(r).ID {
		fail(w, 404, "项目不存在")
		return
	}
	var active bool
	tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM jobs WHERE project_id=$1 AND state IN ('queued','running','outcome_unknown'))`, r.PathValue("id")).Scan(&active)
	if active {
		fail(w, 409, "请先取消或核对该项目的生成任务")
		return
	}
	tx.Exec(r.Context(), `DELETE FROM documents WHERE id=$1`, r.PathValue("id"))
	if tx.Commit(r.Context()) != nil {
		fail(w, 500, "删除失败")
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func (a *App) versions(w http.ResponseWriter, r *http.Request) {
	rows, e := a.db.Query(r.Context(), `SELECT v.revision,v.body,v.created_at FROM versions v JOIN documents d ON d.id=v.document_id WHERE d.id=$1 AND d.owner_id=$2 ORDER BY v.revision DESC LIMIT 100`, r.PathValue("id"), user(r).ID)
	if e != nil {
		fail(w, 500, "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var rev int
		var b json.RawMessage
		var t time.Time
		rows.Scan(&rev, &b, &t)
		out = append(out, map[string]any{"revision": rev, "body": b, "created_at": t})
	}
	reply(w, 200, out)
}
func (a *App) saveAsset(ctx context.Context, owner string, data []byte) (string, error) {
	if free, e := diskFree(a.dir); e != nil || free < 512<<20 {
		return "", os.ErrPermission
	}
	cfg, format, e := image.DecodeConfig(strings.NewReader(string(data)))
	if e != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 20_000_000 {
		return "", os.ErrInvalid
	}
	mime := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "webp": "image/webp"}[format]
	if mime == "" {
		return "", os.ErrInvalid
	}
	aid := id()
	if e = os.WriteFile(filepath.Join(a.dir, aid), data, 0600); e != nil {
		return "", e
	}
	_, e = a.db.Exec(ctx, `INSERT INTO assets(id,owner_id,mime,size) VALUES($1,$2,$3,$4)`, aid, owner, mime, len(data))
	if e != nil {
		os.Remove(filepath.Join(a.dir, aid))
		return "", e
	}
	return aid, nil
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
	if e := r.ParseMultipartForm(1 << 20); e != nil {
		fail(w, 400, "图片过大，最大 10 MB")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	f, _, e := r.FormFile("file")
	if e != nil {
		fail(w, 400, "请选择图片")
		return
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (10<<20)+1))
	if e != nil || len(b) > 10<<20 {
		fail(w, 400, "图片过大")
		return
	}
	aid, e := a.saveAsset(r.Context(), user(r).ID, b)
	if e != nil {
		fail(w, 400, "仅支持不超过 20MP 的 JPG、PNG、WebP 图片，或存储空间不足")
		return
	}
	reply(w, 201, map[string]string{"id": aid})
}
func (a *App) asset(w http.ResponseWriter, r *http.Request) {
	var mime string
	e := a.db.QueryRow(r.Context(), `SELECT mime FROM assets WHERE id=$1 AND owner_id=$2`, r.PathValue("id"), user(r).ID).Scan(&mime)
	if e != nil {
		fail(w, 404, "素材不存在")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, filepath.Join(a.dir, r.PathValue("id")))
}
func (a *App) adminList(w http.ResponseWriter, r *http.Request) {
	if !user(r).Admin {
		fail(w, 403, "需要管理员权限")
		return
	}
	rows, e := a.db.Query(r.Context(), `SELECT id,username,disabled,admin FROM users ORDER BY created_at`)
	if e != nil {
		fail(w, 500, "读取失败")
		return
	}
	out := []map[string]any{}
	for rows.Next() {
		var id, n string
		var d, b bool
		rows.Scan(&id, &n, &d, &b)
		out = append(out, map[string]any{"id": id, "username": n, "disabled": d, "admin": b})
	}
	rows.Close()
	var paused string
	a.db.QueryRow(r.Context(), `SELECT value FROM settings WHERE key='paused'`).Scan(&paused)
	logs := []map[string]any{}
	rows, e = a.db.Query(r.Context(), `SELECT l.job_id,l.amount,l.note,l.created_at,u.username FROM ledger l JOIN jobs j ON j.id=l.job_id JOIN users u ON u.id=j.owner_id ORDER BY l.created_at DESC LIMIT 100`)
	if e == nil {
		for rows.Next() {
			var id, n, un string
			var amount int64
			var t time.Time
			rows.Scan(&id, &amount, &n, &t, &un)
			logs = append(logs, map[string]any{"job_id": id, "amount": amount, "note": n, "username": un, "created_at": t})
		}
		rows.Close()
	}
	reply(w, 200, map[string]any{"users": out, "paused": paused == "true", "ledger": logs})
}
func (a *App) adminAction(w http.ResponseWriter, r *http.Request) {
	if !user(r).Admin {
		fail(w, 403, "需要管理员权限")
		return
	}
	var p struct {
		ID, Password     string
		Disabled, Paused bool
		Amount           int64
	}
	if !read(w, r, &p) {
		return
	}
	tx, e := a.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "数据库不可用")
		return
	}
	defer tx.Rollback(r.Context())
	result := map[string]any{"ok": true}
	switch r.PathValue("action") {
	case "invite":
		token := id()
		_, e = tx.Exec(r.Context(), `INSERT INTO invitations(hash) VALUES($1)`, hash(token))
		result["invite"] = token
	case "user":
		if p.ID == user(r).ID {
			fail(w, 400, "不能禁用当前管理员")
			return
		}
		_, e = tx.Exec(r.Context(), `UPDATE users SET disabled=$1 WHERE id=$2`, p.Disabled, p.ID)
		if e == nil {
			_, e = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, p.ID)
		}
	case "password":
		if !passwordOK(p.Password) {
			fail(w, 400, "密码需 12–72 字节")
			return
		}
		h, _ := bcrypt.GenerateFromPassword([]byte(p.Password), 12)
		_, e = tx.Exec(r.Context(), `UPDATE users SET password=$1 WHERE id=$2`, string(h), p.ID)
		if e == nil {
			_, e = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, p.ID)
		}
	case "pause":
		v := "false"
		if p.Paused {
			v = "true"
		}
		_, e = tx.Exec(r.Context(), `UPDATE settings SET value=$1 WHERE key='paused'`, v)
	case "reconcile":
		tx.Rollback(r.Context())
		if p.Amount < 0 {
			fail(w, 400, "金额无效")
			return
		}
		if e = a.finish(r.Context(), p.ID, "failed", nil, "管理员核对上游账单", p.Amount, true); e != nil {
			fail(w, 409, e.Error())
			return
		}
		a.db.Exec(r.Context(), `INSERT INTO audit(actor,action,target) VALUES($1,'reconcile',$2)`, user(r).ID, p.ID)
		reply(w, 200, result)
		return
	default:
		fail(w, 404, "操作不存在")
		return
	}
	if e != nil {
		fail(w, 500, "操作失败")
		return
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO audit(actor,action,target) VALUES($1,$2,$3)`, user(r).ID, r.PathValue("action"), p.ID)
	if e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "操作失败")
		return
	}
	reply(w, 200, result)
}
func (a *App) cleanup(ctx context.Context) {
	a.cleanupOnce(ctx)
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			a.cleanupOnce(ctx)
		}
	}
}
func (a *App) cleanupOnce(ctx context.Context) {
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(728420)`); e != nil {
		return
	}
	if _, e = tx.Exec(ctx, `DELETE FROM sessions WHERE expires_at<now()`); e != nil {
		return
	}
	if _, e = tx.Exec(ctx, `DELETE FROM documents d WHERE kind='project' AND expires_at<now() AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.project_id=d.id AND j.state IN ('queued','running','outcome_unknown'))`); e != nil {
		return
	}
	if _, e = tx.Exec(ctx, `UPDATE jobs SET input='{}',result='{}' WHERE settled AND updated_at<now()-interval '30 days' AND (input<>'{}'::jsonb OR result<>'{}'::jsonb)`); e != nil {
		return
	}
	rows, e := tx.Query(ctx, `DELETE FROM assets a WHERE created_at<now()-interval '30 days' AND NOT EXISTS(SELECT 1 FROM documents d WHERE d.body::text LIKE '%'||a.id||'%') AND NOT EXISTS(SELECT 1 FROM versions v WHERE v.body::text LIKE '%'||a.id||'%') AND NOT EXISTS(SELECT 1 FROM jobs j WHERE (NOT j.settled OR j.updated_at>now()-interval '30 days') AND j.result::text LIKE '%'||a.id||'%') RETURNING id`)
	if e != nil {
		return
	}
	var removed []string
	for rows.Next() {
		var aid string
		if rows.Scan(&aid) != nil {
			rows.Close()
			return
		}
		removed = append(removed, aid)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return
	}
	if _, e = tx.Exec(ctx, `DELETE FROM events WHERE created_at<now()-interval '30 days'`); e != nil {
		return
	}
	if tx.Commit(ctx) != nil {
		return
	}
	for _, aid := range removed {
		os.Remove(filepath.Join(a.dir, aid))
	}
}
