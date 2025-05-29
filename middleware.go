// Copyright 2022 <mzh.scnu@qq.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/roc-n/tiny-balancer/proxy"
)

// #! 初级并发控制实现
func maxAllowedMiddleware(n uint) mux.MiddlewareFunc {
	sem := make(chan struct{}, n)
	acquire := func() { sem <- struct{}{} }
	release := func() { <-sem }

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			acquire()
			defer release()
			next.ServeHTTP(w, r)
		})
	}
}

// #! 令牌桶限流
type TokenBucket struct {
	capacity   int
	tokens     int
	rate       int // 令牌生成速率，个/秒
	lastRefill time.Time
	mu         sync.Mutex
}

func NewTokenBucket(capacity, rate int) *TokenBucket {
	tb := &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		rate:       rate,
		lastRefill: time.Now(),
	}
	return tb
}

func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elpased := now.Sub(tb.lastRefill).Seconds()
	newTokens := int(elpased * float64(tb.rate))
	if newTokens > 0 {
		tb.tokens = min(tb.capacity, tb.tokens+newTokens)
		tb.lastRefill = now
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

func tokenBucketMiddleware(tb *TokenBucket) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tb.Allow() {
				next.ServeHTTP(w, r)
			} else {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			}
		})
	}
}

// #! 漏桶限流
type leakyJob struct {
	w    http.ResponseWriter
	r    *http.Request
	next http.Handler
}

func leakyBucketMiddleware(bucketSize int, leakRate time.Duration) mux.MiddlewareFunc {
	queue := make(chan leakyJob, bucketSize)

	// 漏桶定时漏水
	go func() {
		for {
			time.Sleep(leakRate)
			select {
			case job := <-queue:
				job.next.ServeHTTP(job.w, job.r)
			default:
			}
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			job := leakyJob{w: w, r: r, next: next}
			select {
			case queue <- job:
				// 请求入队，goroutine进行处理
			default:
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			}
		})
	}
}

// 静态IP黑名单实现
func ipBlacklistMiddleware(blacklist map[string]struct{}) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := proxy.GetIP(r)
			if _, blocked := blacklist[ip]; blocked {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
