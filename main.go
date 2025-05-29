package main

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/roc-n/tiny-balancer/proxy"
)

func main() {
	config, err := ReadConfig("config.yaml")
	if err != nil {
		log.Fatalf("read config error: %s", err)
	}

	err = config.Validation()
	if err != nil {
		log.Fatalf("verify config error: %s", err)
	}

	router := mux.NewRouter()
	for _, l := range config.Location {
		httpProxy, err := proxy.NewHTTPProxy(l.ProxyPass, l.BalanceMode)
		if err != nil {
			log.Fatalf("create proxy error: %s", err)
		}
		// start health check
		if config.HealthCheck {
			httpProxy.HealthCheck(config.HealthCheckInterval)
		}
		router.Handle(l.Pattern, httpProxy)
	}

	// #! IP黑名单处理 ###########
	if len(config.IPBlacklist) > 0 {
		blacklist := make(map[string]struct{}, len(config.IPBlacklist))
		for _, ip := range config.IPBlacklist {
			blacklist[ip] = struct{}{}
		}
		router.Use(ipBlacklistMiddleware(blacklist))
	}

	// #! 限流处理 ###########
	// 同一时间并发数限制
	if config.MaxAllowed > 0 {
		router.Use(maxAllowedMiddleware(config.MaxAllowed))
	}
	// 令牌桶限制
	if config.TokenBucketLimit.Enabled {
		tb := NewTokenBucket(config.TokenBucketLimit.Capacity, config.TokenBucketLimit.Rate)
		router.Use(tokenBucketMiddleware(tb))
	}
	// 漏桶限制
	if config.LeakyBucket.Enabled {
		leakRate := config.LeakyBucket.Rate * int(time.Second)
		router.Use(leakyBucketMiddleware(config.LeakyBucket.Capacity, time.Duration(leakRate)))
	}

	svr := http.Server{
		Addr:    ":" + strconv.Itoa(config.Port),
		Handler: router,
	}

	// print config detail
	config.Print()

	// listen and serve
	if config.Schema == "http" {
		err := svr.ListenAndServe()
		if err != nil {
			log.Fatalf("listen and serve error: %s", err)
		}
	} else if config.Schema == "https" {
		err := svr.ListenAndServeTLS(config.SSLCertificate, config.SSLCertificateKey)
		if err != nil {
			log.Fatalf("listen and serve error: %s", err)
		}
	}
}
