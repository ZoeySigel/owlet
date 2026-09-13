package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const dailyCap int64 = 5_000_000
const monthlyCap int64 = 50_000_000

var china = time.FixedZone("Asia/Shanghai", 8*3600)

func periods(owner string, t time.Time) (string, string) {
	t = t.In(china)
	return "user:" + owner + ":" + t.Format("2006-01-02"), "global:" + t.Format("2006-01")
}

type JobInput struct {
	ProjectID string `json:"project_id"`
	Kind      string `json:"kind"`
	Prompt    string `json:"prompt"`
	Page      int    `json:"page"`
}
type Job struct {
	ID      string          `json:"id"`
	Owner   string          `json:"owner_id"`
	Project *string         `json:"project_id"`
	Kind    string          `json:"kind"`
	State   string          `json:"state"`
	Input   json.RawMessage `json:"input"`
	Result  json.RawMessage `json:"result"`
	Error   string          `json:"error"`
	Reserve int64           `json:"reserve"`
	Charged int64           `json:"charged"`
	Created time.Time       `json:"created_at"`
}

func price(kind string) (int64, error) {
	if kind != "text" && kind != "image" {
		return 0, errors.New("任务类型无效")
	}
	if env("MODEL_MODE", "mock") == "mock" {
		if kind == "text" {
			return 10_000, nil
		}
		return 100_000, nil
	}
	if env("PAID_CALLS_VERIFIED", "false") != "true" {
		return 0, errors.New("真实调用未启用：需先核实账号权限、费率和调用费用上界")
	}
	if kind == "image" {
		p := integer("IMAGE_PRICE_MICRO", 0)
		if p > 0 && p <= dailyCap {
			return p, nil
		}
	} else {
		p := integer("TEXT_RESERVE_MICRO", 0)
		contextTokens := integer("TEXT_CONTEXT_TOKENS", 0)
		inputRate, outputRate := integer("TEXT_INPUT_PER_MILLION_MICRO", 0), integer("TEXT_OUTPUT_PER_MILLION_MICRO", 0)
		if contextTokens < 1 || contextTokens > 2_000_000 || inputRate < 1 || outputRate < 1 || inputRate > 1_000_000_000_000 || outputRate > 1_000_000_000_000 {
			return 0, errors.New("未核实文本模型最大上下文及费率")
		}
		upper := (contextTokens*inputRate + integer("TEXT_MAX_TOKENS", 2048)*outputRate + 999999) / 1_000_000
		if p < upper {
			return 0, errors.New("文本预留额度低于模型最大上下文和输出费用上界")
		}
		if p > 0 && p <= dailyCap && integer("TEXT_INPUT_PER_MILLION_MICRO", 0) > 0 && integer("TEXT_OUTPUT_PER_MILLION_MICRO", 0) > 0 && integer("TEXT_MAX_TOKENS", 2048) > 0 && integer("TEXT_MAX_TOKENS", 2048) <= 4096 && env("BIGMODEL_TEXT_MODEL", "") != "" {
			return p, nil
		}
	}
	return 0, errors.New("模型计费配置不完整")
}
func (a *App) quote(w http.ResponseWriter, r *http.Request) {
	var in JobInput
	if !read(w, r, &in) {
		return
	}
	p, e := price(in.Kind)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	reply(w, 200, map[string]any{"reserve": p, "mode": env("MODEL_MODE", "mock"), "calls": 1})
}
func reserveQuota(ctx context.Context, tx pgx.Tx, day, month string, amount int64) error {
	// Fixed lock order: global month, then user day. All quota transitions use it.
	for _, b := range []struct {
		k string
		c int64
	}{{month, monthlyCap}, {day, dailyCap}} {
		if _, e := tx.Exec(ctx, `INSERT INTO quotas(key,cap) VALUES($1,$2) ON CONFLICT DO NOTHING`, b.k, b.c); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE quotas SET reserved=reserved+$1 WHERE key=$2 AND spent+reserved+$1<=cap`, amount, b.k)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return errors.New("额度不足，请查看个人日额度和全站月额度")
		}
	}
	return nil
}
func (a *App) createJob(w http.ResponseWriter, r *http.Request) {
	if free, e := diskFree(a.dir); e != nil || free < 512<<20 {
		fail(w, 503, "存储空间不足，暂时无法开始生成")
		return
	}
	var in JobInput
	if !read(w, r, &in) {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 120 {
		fail(w, 400, "缺少有效幂等键")
		return
	}
	if len(in.Prompt) > 12000 {
		fail(w, 400, "补充要求过长")
		return
	}
	if in.Page < -1 || in.Page > 3 || (in.Kind == "image" && utf8.RuneCountInString(in.Prompt) > 900) {
		fail(w, 400, "页码无效或背景描述超过 900 字符")
		return
	}
	p, e := price(in.Kind)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	ctx := r.Context()
	tx, e := a.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "数据库不可用")
		return
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(728420)`); e != nil {
		fail(w, 503, "提交暂不可用")
		return
	}
	// Lock the owner to serialize idempotency checks and account disable races.
	var disabled bool
	e = tx.QueryRow(ctx, `SELECT disabled FROM users WHERE id=$1 FOR UPDATE`, user(r).ID).Scan(&disabled)
	if e != nil || disabled {
		fail(w, 403, "账号已禁用")
		return
	}
	var existing string
	var old json.RawMessage
	e = tx.QueryRow(ctx, `SELECT id,input FROM jobs WHERE owner_id=$1 AND idem=$2`, user(r).ID, key).Scan(&existing, &old)
	if e == nil {
		var o JobInput
		json.Unmarshal(old, &o)
		if o != in {
			fail(w, 409, "幂等键已用于其他请求")
			return
		}
		reply(w, 200, map[string]string{"id": existing})
		return
	}
	var paused string
	e = tx.QueryRow(ctx, `SELECT value FROM settings WHERE key='paused' FOR SHARE`).Scan(&paused)
	if e != nil || paused == "true" {
		fail(w, 409, "全站生成已暂停")
		return
	}
	var body json.RawMessage
	e = tx.QueryRow(ctx, `SELECT body FROM documents WHERE id=$1 AND owner_id=$2 AND kind='project' FOR SHARE`, in.ProjectID, user(r).ID).Scan(&body)
	if e != nil {
		fail(w, 404, "项目不存在")
		return
	}
	day, month := periods(user(r).ID, time.Now())
	if e = reserveQuota(ctx, tx, day, month, p); e != nil {
		fail(w, 409, e.Error())
		return
	}
	snapshot, err := generationSnapshot(body)
	if err != nil || len(snapshot)+len(in.Prompt) > 16000 {
		fail(w, 400, "创作资料过长，请缩短后重试")
		return
	}
	raw, _ := json.Marshal(in)
	jid := id()
	_, e = tx.Exec(ctx, `INSERT INTO jobs(id,owner_id,project_id,idem,kind,input,reserve,day_key,month_key,result) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, jid, user(r).ID, in.ProjectID, key, in.Kind, raw, p, day, month, map[string]any{"snapshot": json.RawMessage(snapshot), "mode": env("MODEL_MODE", "mock"), "input_rate": integer("TEXT_INPUT_PER_MILLION_MICRO", 0), "output_rate": integer("TEXT_OUTPUT_PER_MILLION_MICRO", 0), "model": env("BIGMODEL_TEXT_MODEL", ""), "max_tokens": integer("TEXT_MAX_TOKENS", 2048)})
	if e != nil || tx.Commit(ctx) != nil {
		fail(w, 500, "任务提交失败")
		return
	}
	reply(w, 201, map[string]string{"id": jid})
}
func (a *App) usage(w http.ResponseWriter, r *http.Request) {
	day, month := periods(user(r).ID, time.Now())
	out := map[string]any{"mode": env("MODEL_MODE", "mock")}
	for _, b := range []struct {
		k, n string
		c    int64
	}{{day, "daily", dailyCap}, {month, "monthly", monthlyCap}} {
		var spent, res int64
		a.db.QueryRow(r.Context(), `SELECT spent,reserved FROM quotas WHERE key=$1`, b.k).Scan(&spent, &res)
		out[b.n] = map[string]int64{"cap": b.c, "spent": spent, "reserved": res, "remaining": b.c - spent - res}
	}
	reply(w, 200, out)
}
func (a *App) listJobs(w http.ResponseWriter, r *http.Request) {
	q := `SELECT id,owner_id,project_id,kind,state,input,result,error,reserve,charged,created_at FROM jobs WHERE owner_id=$1 ORDER BY created_at DESC LIMIT 100`
	args := []any{user(r).ID}
	if user(r).Admin && r.URL.Query().Get("all") == "true" {
		q = `SELECT id,owner_id,project_id,kind,state,input,result,error,reserve,charged,created_at FROM jobs ORDER BY created_at DESC LIMIT 100`
		args = nil
	}
	rows, e := a.db.Query(r.Context(), q, args...)
	if e != nil {
		fail(w, 500, "读取任务失败")
		return
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var j Job
		rows.Scan(&j.ID, &j.Owner, &j.Project, &j.Kind, &j.State, &j.Input, &j.Result, &j.Error, &j.Reserve, &j.Charged, &j.Created)
		out = append(out, j)
	}
	reply(w, 200, out)
}
func (a *App) cancelJob(w http.ResponseWriter, r *http.Request) {
	var state string
	e := a.db.QueryRow(r.Context(), `UPDATE jobs SET cancel_requested=true WHERE id=$1 AND owner_id=$2 AND state IN ('queued','running') RETURNING state`, r.PathValue("id"), user(r).ID).Scan(&state)
	if e != nil {
		fail(w, 409, "任务不存在或已结束")
		return
	}
	reply(w, 200, map[string]string{"message": "已请求取消。已发出的调用仍可能计费。"})
}
func (a *App) emit(ctx context.Context, jid string, b any) {
	raw, _ := json.Marshal(b)
	a.db.Exec(ctx, `INSERT INTO events(job_id,body) VALUES($1,$2)`, jid, raw)
}
func (a *App) events(w http.ResponseWriter, r *http.Request) {
	var yes bool
	a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1 AND owner_id=$2)`, r.PathValue("id"), user(r).ID).Scan(&yes)
	if !yes {
		fail(w, 404, "任务不存在")
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	last, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		rows, e := a.db.Query(r.Context(), `SELECT id,body FROM events WHERE job_id=$1 AND id>$2 ORDER BY id LIMIT 100`, r.PathValue("id"), last)
		if e != nil {
			return
		}
		for rows.Next() {
			var raw []byte
			rows.Scan(&last, &raw)
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", last, raw)
		}
		rows.Close()
		fmt.Fprint(w, ": heartbeat\n\n")
		fl.Flush()
		var state string
		a.db.QueryRow(r.Context(), `SELECT state FROM jobs WHERE id=$1`, r.PathValue("id")).Scan(&state)
		if state != "queued" && state != "running" {
			fmt.Fprintf(w, "event: done\ndata: %q\n\n", state)
			fl.Flush()
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}
func (a *App) finish(ctx context.Context, jid, state string, result any, note string, amount int64, reconcile bool) error {
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var reserve int64
	var day, month, current string
	var settled bool
	e = tx.QueryRow(ctx, `SELECT reserve,day_key,month_key,state,settled FROM jobs WHERE id=$1 FOR UPDATE`, jid).Scan(&reserve, &day, &month, &current, &settled)
	if e != nil {
		return e
	}
	if settled {
		return nil
	}
	if reconcile && current != "outcome_unknown" {
		return errors.New("只能核对结果不明的调用")
	}
	if amount < 0 {
		return errors.New("金额无效")
	}
	for _, key := range []string{month, day} {
		_, e = tx.Exec(ctx, `UPDATE quotas SET reserved=reserved-$1,spent=spent+$2 WHERE key=$3`, reserve, amount, key)
		if e != nil {
			return e
		}
	}
	raw := []byte(`{}`)
	if result != nil {
		raw, e = json.Marshal(result)
		if e != nil {
			return e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE jobs SET state=$1,result=result||$2::jsonb,error=$3,charged=$4,settled=true,updated_at=now() WHERE id=$5`, state, raw, note, amount, jid)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO ledger(job_id,amount,note) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, jid, amount, note)
	if e != nil {
		return e
	}
	if state == "succeeded" {
		tx.Exec(ctx, `UPDATE documents SET updated_at=now(),expires_at=now()+interval '30 days' WHERE id=(SELECT project_id FROM jobs WHERE id=$1)`, jid)
	}
	if amount > reserve {
		tx.Exec(ctx, `UPDATE settings SET value='true' WHERE key='paused'`)
	}
	if e = tx.Commit(ctx); e == nil {
		a.emit(ctx, jid, map[string]any{"state": state, "result": result})
	}
	return e
}
func (a *App) claim(ctx context.Context) (string, error) {
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	var jid, owner, day, month string
	var res int64
	var cancel, disabled bool
	e = tx.QueryRow(ctx, `SELECT j.id,j.owner_id,j.day_key,j.month_key,j.reserve,j.cancel_requested,u.disabled FROM jobs j JOIN users u ON u.id=j.owner_id WHERE j.state='queued' ORDER BY j.created_at LIMIT 1 FOR UPDATE OF j SKIP LOCKED`).Scan(&jid, &owner, &day, &month, &res, &cancel, &disabled)
	if e != nil {
		return "", e
	}
	var paused string
	tx.QueryRow(ctx, `SELECT value FROM settings WHERE key='paused'`).Scan(&paused)
	if cancel || disabled || paused == "true" {
		tx.Rollback(ctx)
		return "", a.finish(ctx, jid, "cancelled", nil, "任务在调用前取消或暂停", 0, false)
	}
	nd, nm := periods(owner, time.Now())
	if nd != day || nm != month {
		for _, key := range []string{month, day} {
			if _, e = tx.Exec(ctx, `UPDATE quotas SET reserved=reserved-$1 WHERE key=$2`, res, key); e != nil {
				return "", e
			}
		}
		if e = reserveQuota(ctx, tx, nd, nm, res); e != nil {
			tx.Rollback(ctx)
			return "", a.finish(ctx, jid, "failed", nil, "跨期后的额度不足，未调用模型", 0, false)
		}
	}
	_, e = tx.Exec(ctx, `UPDATE jobs SET state='running',day_key=$1,month_key=$2,updated_at=now() WHERE id=$3`, nd, nm, jid)
	if e != nil {
		return "", e
	}
	return jid, tx.Commit(ctx)
}
func (a *App) worker(ctx context.Context) {
	// Session advisory lock permits one worker even during rolling restarts.
	conn, e := a.db.Acquire(ctx)
	if e != nil {
		return
	}
	defer conn.Release()
	var locked bool
	for !locked {
		if conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(728419)`).Scan(&locked) != nil {
			return
		}
		if !locked {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(728419)`)
	a.recoverStarted(ctx)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.recoverDownload(ctx)
			jid, e := a.claim(ctx)
			if e == nil && jid != "" {
				a.runJob(ctx, jid)
			}
		}
	}
}
