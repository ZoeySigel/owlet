package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

var loginGuard = struct {
	sync.Mutex
	attempts map[string][]time.Time
}{attempts: make(map[string][]time.Time)}
var passwordSlots = make(chan struct{}, 2)

func limitedLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		// Reverse proxy must remove client-supplied forwarding headers. Do not trust them here.
		now := time.Now()
		loginGuard.Lock()
		if len(loginGuard.attempts) > 1000 {
			for key, ts := range loginGuard.attempts {
				if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > time.Minute {
					delete(loginGuard.attempts, key)
				}
			}
		}
		ts := loginGuard.attempts[host]
		fresh := ts[:0]
		for _, t := range ts {
			if now.Sub(t) < time.Minute {
				fresh = append(fresh, t)
			}
		}
		denied := len(fresh) >= 20
		if !denied {
			fresh = append(fresh, now)
		}
		loginGuard.attempts[host] = fresh
		loginGuard.Unlock()
		if denied {
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "登录尝试过于频繁，请稍后重试")
			return
		}
		select {
		case passwordSlots <- struct{}{}:
			defer func() { <-passwordSlots }()
			next(w, r)
		default:
			fail(w, 429, "登录服务繁忙，请稍后重试")
		}
	}
}
