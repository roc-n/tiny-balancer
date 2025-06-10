# tiny-balancer

`tiny-balancer` 基于[mini-balancer](https://github.com/wanzo-mini/mini-balancer.git)构建，实现基础的web负载均衡器。

支持以下负载均衡算法: 
* `round-robin`
* `random`
* `power of 2 random choice`
* `consistent hash`
* `consistent hash with bounded`
* `ip-hash`
* `least-load`

## 改进
- 实现综合考虑并发与延迟的p2c-ewma算法
- 实现基于令牌桶/漏桶的限流算法
- 设计IP黑名单限制访问

## 运行
配置 `config.yaml`文件，编译并运行如下： 
```shell
> ./mini-balancer

___ _ _  _ _   _ ___  ____ _    ____ _  _ ____ ____ ____ 
 |  | |\ |  \_/  |__] |__| |    |__| |\ | |    |___ |__/ 
 |  | | \|   |   |__] |  | |___ |  | | \| |___ |___ |  \                                        

Schema: http
Port: 8089
Health Check: true
Location:
        Route: /
        Proxy Pass: [http://192.168.1.1 http://192.168.1.2:1015 https://192.168.1.2 http://my-server.com]
        Mode: round-robin

```


每个balancer均实现`Balancer` 接口:
```go
type Balancer interface {
	Add(string)
	Remove(string)
	Balance(string) (string, error)
	Inc(string)
	Done(string)
	RequestCtx() func(string) 
}
```

## TODO
- 健康状态检测仅使用TCP连接判断，且多个服务部署在同一个机器上会造成冗余，考虑更换为细粒度检测方法，比如针对每个服务的响应进行判断