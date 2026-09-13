package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schema string

type App struct {
	db          *pgxpool.Pool
	dir, origin string
	secure      bool
}
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Admin    bool   `json:"admin"`
}
type ctxKey struct{}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func integer(k string, d int64) int64 {
	n, e := strconv.ParseInt(os.Getenv(k), 10, 64)
	if e != nil {
		return d
	}
	return n
}
func id() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func hash(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func reply(w http.ResponseWriter, n int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(n)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, n int, s string) { reply(w, n, map[string]string{"error": s}) }
func read(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	if d.Decode(v) != nil {
		fail(w, 400, "请求格式无效")
		return false
	}
	return true
}
func user(r *http.Request) User { return r.Context().Value(ctxKey{}).(User) }
func (a *App) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie("owlet_session")
		if e != nil {
			fail(w, 401, "请先登录")
			return
		}
		var u User
		e = a.db.QueryRow(r.Context(), `SELECT u.id,u.username,u.admin FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.hash=$1 AND s.expires_at>now() AND NOT u.disabled`, hash(c.Value)).Scan(&u.ID, &u.Username, &u.Admin)
		if e != nil {
			fail(w, 401, "登录已过期，请重新登录")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	}
}
func (a *App) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Origin") != a.origin {
			fail(w, 403, "请求来源不匹配")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func passwordOK(s string) bool { return len(s) >= 12 && len(s) <= 72 }
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var p struct{ Username, Password, Invite string }
	if !read(w, r, &p) {
		return
	}
	p.Username = strings.TrimSpace(p.Username)
	if strings.HasSuffix(r.URL.Path, "register") {
		if len(p.Username) < 3 || len(p.Username) > 40 || !passwordOK(p.Password) {
			fail(w, 400, "用户名需 3–40 字符，密码需 12–72 字节")
			return
		}
		tx, e := a.db.Begin(r.Context())
		if e != nil {
			fail(w, 503, "数据库不可用")
			return
		}
		defer tx.Rollback(r.Context())
		var used *string
		e = tx.QueryRow(r.Context(), `SELECT used_by FROM invitations WHERE hash=$1 FOR UPDATE`, hash(p.Invite)).Scan(&used)
		if e != nil || used != nil {
			fail(w, 400, "邀请码无效或已使用")
			return
		}
		uid := id()
		pw, _ := bcrypt.GenerateFromPassword([]byte(p.Password), 12)
		if _, e = tx.Exec(r.Context(), `INSERT INTO users(id,username,password) VALUES($1,$2,$3)`, uid, p.Username, string(pw)); e != nil {
			fail(w, 409, "用户名不可用")
			return
		}
		tx.Exec(r.Context(), `UPDATE invitations SET used_by=$1 WHERE hash=$2`, uid, hash(p.Invite))
		if tx.Commit(r.Context()) != nil {
			fail(w, 500, "注册失败")
			return
		}
	}
	var u User
	var pw string
	var disabled bool
	e := a.db.QueryRow(r.Context(), `SELECT id,username,admin,password,disabled FROM users WHERE username=$1`, p.Username).Scan(&u.ID, &u.Username, &u.Admin, &pw, &disabled)
	if e != nil || bcrypt.CompareHashAndPassword([]byte(pw), []byte(p.Password)) != nil || disabled {
		time.Sleep(250 * time.Millisecond)
		fail(w, 401, "用户名或密码错误")
		return
	}
	token := id()
	_, e = a.db.Exec(r.Context(), `INSERT INTO sessions VALUES($1,$2,$3)`, hash(token), u.ID, time.Now().Add(7*24*time.Hour))
	if e != nil {
		fail(w, 500, "登录失败")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "owlet_session", Value: token, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode, MaxAge: 604800})
	reply(w, 200, u)
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	c, e := r.Cookie("owlet_session")
	if e == nil {
		a.db.Exec(r.Context(), `DELETE FROM sessions WHERE hash=$1`, hash(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "owlet_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode})
	reply(w, 200, map[string]bool{"ok": true})
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, e := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if e != nil {
		log.Fatal("DATABASE_URL 无效")
	}
	cfg.MaxConns = 5
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec(ctx, schema); e != nil {
		log.Fatal(e)
	}
	a := &App{db: db, dir: env("DATA_DIR", "./data"), origin: env("PUBLIC_ORIGIN", "http://localhost:5173"), secure: env("COOKIE_SECURE", "false") == "true"}
	os.MkdirAll(a.dir, 0700)
	if pw := os.Getenv("ADMIN_PASSWORD"); pw != "" {
		if !passwordOK(pw) {
			log.Fatal("管理员密码至少 12 字节")
		}
		h, _ := bcrypt.GenerateFromPassword([]byte(pw), 12)
		_, e = db.Exec(ctx, `INSERT INTO users(id,username,password,admin) VALUES($1,$2,$3,true) ON CONFLICT(username) DO NOTHING`, id(), env("ADMIN_USERNAME", "admin"), string(h))
		if e != nil {
			log.Fatal(e)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if db.Ping(r.Context()) != nil {
			fail(w, 503, "数据库不可用")
			return
		}
		reply(w, 200, map[string]string{"status": "ok", "mode": env("MODEL_MODE", "mock")})
	})
	mux.HandleFunc("POST /api/auth/register", limitedLogin(a.login))
	mux.HandleFunc("POST /api/auth/login", limitedLogin(a.login))
	mux.HandleFunc("POST /api/auth/logout", a.auth(a.logout))
	mux.HandleFunc("GET /api/me", a.auth(func(w http.ResponseWriter, r *http.Request) { reply(w, 200, user(r)) }))
	mux.HandleFunc("GET /api/documents", a.auth(a.listDocs))
	mux.HandleFunc("POST /api/documents", a.auth(a.saveDoc))
	mux.HandleFunc("PUT /api/documents/{id}", a.auth(a.saveDoc))
	mux.HandleFunc("DELETE /api/documents/{id}", a.auth(a.deleteDoc))
	mux.HandleFunc("GET /api/documents/{id}/versions", a.auth(a.versions))
	mux.HandleFunc("POST /api/assets", a.auth(a.upload))
	mux.HandleFunc("GET /api/assets/{id}", a.auth(a.asset))
	mux.HandleFunc("GET /api/usage", a.auth(a.usage))
	mux.HandleFunc("POST /api/quotes", a.auth(a.quote))
	mux.HandleFunc("POST /api/jobs", a.auth(a.createJob))
	mux.HandleFunc("GET /api/jobs", a.auth(a.listJobs))
	mux.HandleFunc("POST /api/jobs/{id}/cancel", a.auth(a.cancelJob))
	mux.HandleFunc("GET /api/jobs/{id}/events", a.auth(a.events))
	mux.HandleFunc("GET /api/admin", a.auth(a.adminList))
	mux.HandleFunc("POST /api/admin/{action}", a.auth(a.adminAction))
	go a.worker(ctx)
	go a.cleanup(ctx)
	srv := &http.Server{Addr: env("LISTEN_ADDR", "127.0.0.1:8080"), Handler: a.middleware(mux), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 16}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	fmt.Println("Owlet listening", srv.Addr, "mode", env("MODEL_MODE", "mock"))
	if e = srv.ListenAndServe(); !errors.Is(e, http.ErrServerClosed) {
		log.Fatal(e)
	}
}
