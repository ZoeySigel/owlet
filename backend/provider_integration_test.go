package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type providerTransport func(*http.Request) (*http.Response, error)

func (f providerTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProviderJobSettlement(t *testing.T) {
	// No paid requests: intercept the fixed official endpoint, while using a real
	// isolated PostgreSQL schema to check the job and ledger transition together.
	a := testApp(t)
	ctx := context.Background()
	t.Setenv("MODEL_MODE", "bigmodel")
	t.Setenv("PAID_CALLS_VERIFIED", "true")
	t.Setenv("BIGMODEL_API_KEY", "test-key")
	t.Setenv("BIGMODEL_TEXT_MODEL", "test-model")
	t.Setenv("TEXT_CONTEXT_TOKENS", "200000")
	t.Setenv("TEXT_MAX_TOKENS", "2048")
	t.Setenv("TEXT_INPUT_PER_MILLION_MICRO", "500000")
	t.Setenv("TEXT_OUTPUT_PER_MILLION_MICRO", "3000000")
	t.Setenv("TEXT_RESERVE_MICRO", "106144")
	valid := `{"title":"商品","caption":"真实介绍","tags":"好物","pages":[{"title":"一","text":"一"},{"title":"二","text":"二"},{"title":"三","text":"三"},{"title":"四","text":"四"}]}`
	for _, tc := range []struct {
		name, draft, state string
		status             int
		done               bool
		charge             int64
	}{
		{"success", valid, "succeeded", 200, true, 110},
		{"invalid draft billed", `{}`, "failed", 200, true, 110},
		{"incomplete stream retained", valid, "outcome_unknown", 200, false, 0},
		{"http failure retained", "", "outcome_unknown", 429, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uid, jid := testUser(t, a), id()
			day, month := "day:"+jid, "month:"+jid
			_, err := a.db.Exec(ctx, `INSERT INTO quotas(key,cap,reserved) VALUES($1,5000000,106144),($2,50000000,106144)`, day, month)
			if err != nil {
				t.Fatal(err)
			}
			meta := map[string]any{"mode": "bigmodel", "model": "test-model", "snapshot": map[string]string{"name": "商品"}, "input_rate": 500000, "output_rate": 3000000, "max_tokens": 2048}
			_, err = a.db.Exec(ctx, `INSERT INTO jobs(id,owner_id,idem,kind,input,state,reserve,day_key,month_key,result) VALUES($1,$2,$1,'text','{}','running',106144,$3,$4,$5)`, jid, uid, day, month, meta)
			if err != nil {
				t.Fatal(err)
			}
			old := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = old })
			calls := 0
			http.DefaultTransport = providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if r.URL.Host != "open.bigmodel.cn" || payload["model"] != "test-model" || payload["request_id"] != jid || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Fatal("unexpected provider request")
				}
				chunk, _ := json.Marshal(map[string]any{"id": "upstream-id", "choices": []any{map[string]any{"delta": map[string]string{"content": tc.draft}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 20}})
				body := "data: " + string(chunk) + "\n\n"
				if tc.done {
					body += "data: [DONE]\n\n"
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			a.runJob(ctx, jid)
			var state string
			var amount, reserved, spent int64
			var entries int
			if err := a.db.QueryRow(ctx, `SELECT state,charged FROM jobs WHERE id=$1`, jid).Scan(&state, &amount); err != nil {
				t.Fatal(err)
			}
			a.db.QueryRow(ctx, `SELECT reserved,spent FROM quotas WHERE key=$1`, day).Scan(&reserved, &spent)
			a.db.QueryRow(ctx, `SELECT count(*) FROM ledger WHERE job_id=$1`, jid).Scan(&entries)
			if state != tc.state || amount != tc.charge || spent != tc.charge || calls != 1 {
				t.Fatalf("state=%s amount=%d spent=%d calls=%d", state, amount, spent, calls)
			}
			if tc.state == "outcome_unknown" {
				if reserved != 106144 || entries != 0 {
					t.Fatal(reserved, entries)
				}
			} else if reserved != 0 || entries != 1 {
				t.Fatal(reserved, entries)
			}
		})
	}
}
