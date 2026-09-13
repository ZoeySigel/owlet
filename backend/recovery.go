package main

import (
	"context"
	"encoding/json"
	"time"
)

func (a *App) recoverStarted(ctx context.Context) error {
	_, e := a.db.Exec(ctx, `UPDATE jobs SET state=CASE WHEN dispatched THEN 'outcome_unknown' ELSE 'queued' END,error=CASE WHEN dispatched THEN '进程中断，等待核对上游结果' ELSE '' END WHERE state='running'`)
	return e
}

// Retry only an existing image download. Never issue another generation request.
func (a *App) recoverDownload(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	var jid, owner, raw string
	var reserve int64
	e := a.db.QueryRow(ctx, `SELECT id,owner_id,result->>'download_url',reserve FROM jobs WHERE state='outcome_unknown' AND result ? 'download_url' AND NOT settled AND updated_at<now()-interval '60 seconds' ORDER BY updated_at LIMIT 1`).Scan(&jid, &owner, &raw, &reserve)
	if e != nil {
		return
	}
	a.db.Exec(ctx, `UPDATE jobs SET updated_at=now() WHERE id=$1`, jid)
	data, e := downloadImage(ctx, raw)
	if e != nil {
		return
	}
	aid, e := a.saveAsset(ctx, owner, data)
	if e != nil {
		return
	}
	a.finish(ctx, jid, "succeeded", map[string]string{"assetId": aid}, "恢复已有图片下载，没有重复生成", reserve, false)
}
func generationSnapshot(raw json.RawMessage) (string, error) {
	var b map[string]any
	if e := json.Unmarshal(raw, &b); e != nil {
		return "", e
	}
	selected := map[string]any{}
	for _, key := range []string{"name", "selling", "audience", "price", "note", "title", "caption", "tags", "pages", "brand"} {
		if v, ok := b[key]; ok {
			selected[key] = v
		}
	}
	out, e := json.Marshal(selected)
	return string(out), e
}
