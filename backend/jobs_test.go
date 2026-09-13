package main

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testApp(t *testing.T) *App {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	bootstrap, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	ns := "test_" + id()
	if _, e = bootstrap.Exec(ctx, "CREATE SCHEMA "+ns); e != nil {
		t.Fatal(e)
	}
	cfg, e := pgxpool.ParseConfig(url)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = ns
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		db.Close()
		bootstrap.Exec(context.Background(), "DROP SCHEMA "+ns+" CASCADE")
		bootstrap.Close()
	})
	return &App{db: db, dir: t.TempDir(), origin: "http://localhost:5173"}
}
func testUser(t *testing.T, a *App) string {
	t.Helper()
	uid := id()
	if _, e := a.db.Exec(context.Background(), `INSERT INTO users(id,username,password) VALUES($1,$1,'test')`, uid); e != nil {
		t.Fatal(e)
	}
	return uid
}
func TestPeriods(t *testing.T) {
	d, m := periods("u", time.Date(2026, 9, 30, 16, 0, 0, 0, time.UTC))
	if d != "user:u:2026-10-01" || m != "global:2026-10" {
		t.Fatal(d, m)
	}
}
func TestPaidFailsClosed(t *testing.T) {
	t.Setenv("MODEL_MODE", "bigmodel")
	t.Setenv("PAID_CALLS_VERIFIED", "false")
	if _, e := price("text"); e == nil {
		t.Fatal("paid enabled without verification")
	}
}
func TestConcurrentQuotaReservation(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	day, month := "test-day:"+id(), "test-month:"+id()
	a.db.Exec(ctx, `INSERT INTO quotas(key,cap) VALUES($1,100000),($2,100000)`, day, month)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, e := a.db.Begin(ctx)
			if e != nil {
				t.Error(e)
				return
			}
			defer tx.Rollback(ctx)
			if e = reserveQuota(ctx, tx, day, month, 100000); e == nil {
				if tx.Commit(ctx) == nil {
					accepted.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d, wanted 1", accepted.Load())
	}
	for _, key := range []string{day, month} {
		var res int64
		a.db.QueryRow(ctx, `SELECT reserved FROM quotas WHERE key=$1`, key).Scan(&res)
		if res != 100000 {
			t.Fatalf("reserve %d", res)
		}
	}
}
func TestSettlementIdempotent(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	uid := testUser(t, a)
	day, month := "day:"+id(), "month:"+id()
	jid := id()
	a.db.Exec(ctx, `INSERT INTO quotas(key,cap,reserved) VALUES($1,5000000,100000),($2,50000000,100000)`, day, month)
	_, e := a.db.Exec(ctx, `INSERT INTO jobs(id,owner_id,idem,kind,input,state,reserve,day_key,month_key) VALUES($1,$2,$1,'image','{}','running',100000,$3,$4)`, jid, uid, day, month)
	if e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 2; n++ {
		if e = a.finish(ctx, jid, "succeeded", nil, "test", 80000, false); e != nil {
			t.Fatal(e)
		}
	}
	var res, spent int64
	a.db.QueryRow(ctx, `SELECT reserved,spent FROM quotas WHERE key=$1`, day).Scan(&res, &spent)
	if res != 0 || spent != 80000 {
		t.Fatal(res, spent)
	}
	var count int
	a.db.QueryRow(ctx, `SELECT count(*) FROM ledger WHERE job_id=$1`, jid).Scan(&count)
	if count != 1 {
		t.Fatal(count)
	}
}
func TestUnknownRetainsReservation(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	uid := testUser(t, a)
	day, month := "day:"+id(), "month:"+id()
	jid := id()
	a.db.Exec(ctx, `INSERT INTO quotas(key,cap,reserved) VALUES($1,5000000,100000),($2,50000000,100000)`, day, month)
	a.db.Exec(ctx, `INSERT INTO jobs(id,owner_id,idem,kind,input,state,dispatched,reserve,day_key,month_key) VALUES($1,$2,$1,'image','{}','outcome_unknown',true,100000,$3,$4)`, jid, uid, day, month)
	if e := a.finish(ctx, jid, "failed", nil, "reconciled", 40000, true); e != nil {
		t.Fatal(e)
	}
	var spent, res int64
	a.db.QueryRow(ctx, `SELECT spent,reserved FROM quotas WHERE key=$1`, day).Scan(&spent, &res)
	if spent != 40000 || res != 0 {
		t.Fatal(spent, res)
	}
}
func TestDocumentAssetIsolation(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	one := testUser(t, a)
	two := testUser(t, a)
	aid := id()
	a.db.Exec(ctx, `INSERT INTO assets(id,owner_id,mime,size) VALUES($1,$2,'image/png',1)`, aid, one)
	raw := json.RawMessage(`{"pages":[{"assetId":"` + aid + `"}]}`)
	var body any
	json.Unmarshal(raw, &body)
	if a.checkAssets(ctx, two, body) {
		t.Fatal("foreign asset accepted")
	}
	if !a.checkAssets(ctx, one, body) {
		t.Fatal("own asset rejected")
	}
}

func TestCrossMonthReservation(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	uid := testUser(t, a)
	jid := id()
	oldDay, oldMonth := "user:"+uid+":2001-01-01", "global:2001-01"
	a.db.Exec(ctx, `INSERT INTO quotas(key,cap,reserved) VALUES($1,5000000,100000),($2,50000000,100000)`, oldDay, oldMonth)
	_, e := a.db.Exec(ctx, `INSERT INTO jobs(id,owner_id,idem,kind,input,reserve,day_key,month_key) VALUES($1,$2,$1,'image','{}',100000,$3,$4)`, jid, uid, oldDay, oldMonth)
	if e != nil {
		t.Fatal(e)
	}
	got, e := a.claim(ctx)
	if e != nil || got != jid {
		t.Fatal(got, e)
	}
	day, month := periods(uid, time.Now())
	for _, key := range []string{oldDay, oldMonth} {
		var n int64
		a.db.QueryRow(ctx, `SELECT reserved FROM quotas WHERE key=$1`, key).Scan(&n)
		if n != 0 {
			t.Fatal("old reservation retained", n)
		}
	}
	for _, key := range []string{day, month} {
		var n int64
		a.db.QueryRow(ctx, `SELECT reserved FROM quotas WHERE key=$1`, key).Scan(&n)
		if n != 100000 {
			t.Fatal("new reservation missing", n)
		}
	}
}
func TestRestartDoesNotRedispatchUnknown(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	uid := testUser(t, a)
	for _, dispatched := range []bool{false, true} {
		jid := id()
		a.db.Exec(ctx, `INSERT INTO jobs(id,owner_id,idem,kind,input,state,dispatched,reserve,day_key,month_key) VALUES($1,$2,$1,'image','{}','running',$3,100000,'d','m')`, jid, uid, dispatched)
	}
	if e := a.recoverStarted(ctx); e != nil {
		t.Fatal(e)
	}
	var queued, unknown int
	a.db.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE state='queued' AND NOT dispatched`).Scan(&queued)
	a.db.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE state='outcome_unknown' AND dispatched AND NOT settled`).Scan(&unknown)
	if queued != 1 || unknown != 1 {
		t.Fatal(queued, unknown)
	}
}
func TestCancellationReleasesUnspent(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	uid := testUser(t, a)
	day, month := periods(uid, time.Now())
	jid := id()
	a.db.Exec(ctx, `INSERT INTO quotas(key,cap,reserved) VALUES($1,5000000,100000),($2,50000000,100000)`, day, month)
	a.db.Exec(ctx, `INSERT INTO jobs(id,owner_id,idem,kind,input,reserve,day_key,month_key,cancel_requested) VALUES($1,$2,$1,'image','{}',100000,$3,$4,true)`, jid, uid, day, month)
	a.claim(ctx)
	var state string
	var dispatched bool
	a.db.QueryRow(ctx, `SELECT state,dispatched FROM jobs WHERE id=$1`, jid).Scan(&state, &dispatched)
	if state != "cancelled" || dispatched {
		t.Fatal(state, dispatched)
	}
	var reserved int64
	a.db.QueryRow(ctx, `SELECT reserved FROM quotas WHERE key=$1`, day).Scan(&reserved)
	if reserved != 0 {
		t.Fatal(reserved)
	}
}
func TestCleanupKeepsSharedAsset(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	uid := testUser(t, a)
	shared, unused := id(), id()
	a.db.Exec(ctx, `INSERT INTO assets(id,owner_id,mime,size,created_at) VALUES($1,$3,'image/png',1,now()-interval '40 days'),($2,$3,'image/png',1,now()-interval '40 days')`, shared, unused, uid)
	project, product := id(), id()
	a.db.Exec(ctx, `INSERT INTO documents(id,owner_id,kind,body,expires_at) VALUES($1,$3,'project',$4,now()-interval '1 day'),($2,$3,'product',$4,NULL)`, project, product, uid, map[string]string{"assetId": shared})
	a.cleanupOnce(ctx)
	var exists bool
	a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM documents WHERE id=$1)`, project).Scan(&exists)
	if exists {
		t.Fatal("expired project retained")
	}
	a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM assets WHERE id=$1)`, shared).Scan(&exists)
	if !exists {
		t.Fatal("shared asset deleted")
	}
	a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM assets WHERE id=$1)`, unused).Scan(&exists)
	if exists {
		t.Fatal("unused asset retained")
	}
}
func TestDownloadBlocksPrivateAddress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, e := downloadImage(ctx, "https://127.0.0.1/private"); e == nil {
		t.Fatal("private address allowed")
	}
}
