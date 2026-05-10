<h1 align="center">
  <img src="Meta.png" alt="Meta Kennel" width="200">
  <br>Meta Kernel<br>
</h1>

<h3 align="center">另一个 Mihomo 内核。</h3>

<p align="center">
  <a href="https://goreportcard.com/report/github.com/MetaCubeX/mihomo">
    <img src="https://goreportcard.com/badge/github.com/MetaCubeX/mihomo?style=flat-square">
  </a>
  <img src="https://img.shields.io/github/go-mod/go-version/MetaCubeX/mihomo/Alpha?style=flat-square">
  <a href="https://github.com/MetaCubeX/mihomo/releases">
    <img src="https://img.shields.io/github/release/MetaCubeX/mihomo/all.svg?style=flat-square">
  </a>
  <a href="https://github.com/MetaCubeX/mihomo">
    <img src="https://img.shields.io/badge/release-Meta-00b4f0?style=flat-square">
  </a>
</p>

## 功能特性

- 支持带认证的本地 HTTP/HTTPS/SOCKS 服务器
- 支持 VMess、VLESS、Shadowsocks、Trojan、Snell、TUIC、Hysteria、SVNT 协议
- 内置 DNS 服务器，尽可能降低 DNS 污染攻击的影响，支持 DoH/DoT 上游与 Fake IP
- 支持基于域名、GEOIP、IPCIDR 或进程的规则分流，将流量转发到不同节点
- 支持远程策略组，可按延迟实现自动故障转移、负载均衡或自动选择节点
- 支持远程 provider，可远程获取节点列表，无需在配置中硬编码
- 支持 Netfilter TCP 重定向，可通过 `iptables` 将 Mihomo 部署到你的网络网关
- 提供完整的 HTTP RESTful API 控制接口

## 面板

本项目有一个原生适配的 Web 控制面板，可查看 [metacubexd](https://github.com/MetaCubeX/metacubexd)。

## 配置示例

配置示例位于 [/docs/config.yaml](https://github.com/MetaCubeX/mihomo/blob/Alpha/docs/config.yaml)。

## 文档

项目文档见 [mihomo Docs](https://wiki.metacubex.one/)。

## 开发说明

环境要求：
[Go 1.20 或更高版本](https://go.dev/dl/)

构建 mihomo：

```shell
git clone https://github.com/MetaCubeX/mihomo.git
cd mihomo && go mod download
go build -o sdc-mihomo
```

如果无法连接 GitHub，可先设置 Go 代理：

```shell
go env -w GOPROXY=https://goproxy.io,direct
```

使用 gVisor TUN 栈构建：

```shell
go build -tags with_gvisor
```

### IPTABLES 配置

适用于支持 `iptables` 的 Linux 系统。

```yaml
# 启用 TPROXY 监听器
tproxy-port: 9898

iptables:
  enable: true # 默认为 false
  inbound-interface: eth0 # 入站网卡，默认是 'lo'
```

## 调试

可参考 [wiki](https://wiki.metacubex.one/api/#debug) 了解如何使用调试 API。

## 致谢

- [Dreamacro/clash](https://github.com/Dreamacro/clash)
- [SagerNet/sing-box](https://github.com/SagerNet/sing-box)
- [riobard/go-shadowsocks2](https://github.com/riobard/go-shadowsocks2)
- [v2ray/v2ray-core](https://github.com/v2ray/v2ray-core)
- [WireGuard/wireguard-go](https://github.com/WireGuard/wireguard-go)
- [yaling888/clash-plus-pro](https://github.com/yaling888/clash)

## 许可证

本软件基于 GPL-3.0 许可证发布。

**另外，任何与 `MetaCubeX` 无关联的下游项目，名称中不得包含 `mihomo` 一词。**
