# kratos-consul-discovery

## 项目简介
1. Kratos  https://github.com/bilibili/kratos 的一个`discovery` 组件, 跟默认的 discovery 组件功能一样，只是使用了 consul 作为服务发现中心
2. not ready for production

## demo

```go
import (
 discovery "github.com/cocktail18/kratos-consul-discovery"
)

const AppID = "demo"

// consul 的配置使用环境变量


// 实例化
dis, err := discovery.NewConsulDiscovery(discovery.Config{Zone:"zone01", Env:"dev", Region:"region01"})
if err != nil {
    panic(err)
}
// 注册为 resolver
resolver.Register(dis)


// 服务注册
ip := "127.0.0.1" // NOTE: 必须拿到您实例节点的真实IP，
port := "9000" // NOTE: 必须拿到您实例grpc监听的真实端口，warden默认监听9000
hn, _ := os.Hostname()
ins := &naming.Instance{
    Zone:     "zone01",
    Env:      env.DeployEnv,
    AppID:    AppID,
    Hostname: hn,
    Addrs: []string{
        "grpc://" + ip + ":" + port,
    },
}
deRegister, err := dis.Register(context.Background(), ins)
if err != nil {
    panic(err)
}

// 服务调用
client := warden.NewClient(nil)
conn, err := client.Dial(context.Background(), "consul://default/"+AppID)
if err != nil {
	panic(err)
}
```

## consul 环境变量参考
```go
// HTTPAddrEnvName defines an environment variable name which sets
// the HTTP address if there is no -http-addr specified.
HTTPAddrEnvName = "CONSUL_HTTP_ADDR"

// HTTPTokenEnvName defines an environment variable name which sets
// the HTTP token.
HTTPTokenEnvName = "CONSUL_HTTP_TOKEN"

// HTTPTokenFileEnvName defines an environment variable name which sets
// the HTTP token file.
HTTPTokenFileEnvName = "CONSUL_HTTP_TOKEN_FILE"

// HTTPAuthEnvName defines an environment variable name which sets
// the HTTP authentication header.
HTTPAuthEnvName = "CONSUL_HTTP_AUTH"

// HTTPSSLEnvName defines an environment variable name which sets
// whether or not to use HTTPS.
HTTPSSLEnvName = "CONSUL_HTTP_SSL"

// HTTPCAFile defines an environment variable name which sets the
// CA file to use for talking to Consul over TLS.
HTTPCAFile = "CONSUL_CACERT"

// HTTPCAPath defines an environment variable name which sets the
// path to a directory of CA certs to use for talking to Consul over TLS.
HTTPCAPath = "CONSUL_CAPATH"

// HTTPClientCert defines an environment variable name which sets the
// client cert file to use for talking to Consul over TLS.
HTTPClientCert = "CONSUL_CLIENT_CERT"

// HTTPClientKey defines an environment variable name which sets the
// client key file to use for talking to Consul over TLS.
HTTPClientKey = "CONSUL_CLIENT_KEY"

// HTTPTLSServerName defines an environment variable name which sets the
// server name to use as the SNI host when connecting via TLS
HTTPTLSServerName = "CONSUL_TLS_SERVER_NAME"

// HTTPSSLVerifyEnvName defines an environment variable name which sets
// whether or not to disable certificate checking.
HTTPSSLVerifyEnvName = "CONSUL_HTTP_SSL_VERIFY"

// GRPCAddrEnvName defines an environment variable name which sets the gRPC
// address for consul connect envoy. Note this isn't actually used by the api
// client in this package but is defined here for consistency with all the
// other ENV names we use.
GRPCAddrEnvName = "CONSUL_GRPC_ADDR"
```


