# TUN 模式工作原理

本文说明 mihomo 在 TUN 模式下，如何把系统流量导入 mihomo 进程、交给规则引擎处理，再把结果返回给应用。

说明基于当前仓库实现，不是泛化描述。

## 1. TUN 模式不是监听端口

TUN 模式和 `mixed-port`、`socks-port`、`redir-port`、`tproxy-port` 不一样。

这些端口模式的共同点是：

- mihomo 监听一个端口
- 应用或内核把连接转发到这个端口
- mihomo 在端口入口处接收连接

而 TUN 模式不是这样。

TUN 模式的核心是：

- mihomo 创建一个虚拟网卡
- 操作系统把目标流量路由到这个虚拟网卡
- mihomo 从虚拟网卡读取 IP 包

所以，TUN 模式导流依赖的是：

- 虚拟网卡
- 路由
- mihomo 内部协议栈

不是依赖普通监听端口。

## 2. 流量进入 mihomo 的整体路径

整体链路如下：

1. 应用发起网络请求
2. 操作系统根据路由规则，决定把流量发往 TUN 虚拟网卡
3. 内核把 IP 包写入 TUN 设备
4. mihomo 从 TUN 设备读取这些 IP 包
5. mihomo 内部协议栈将 IP 包还原成 TCP 连接或 UDP 数据包
6. mihomo 把这些连接/数据包交给规则引擎
7. 命中规则后，选择 `DIRECT` 或某个代理节点出站
8. 远端响应返回后，mihomo 再把数据写回 TUN
9. 内核把响应交还给原应用

对应用来说，看起来像是在正常访问网络；对 mihomo 来说，它实际接管了这部分 IP 流量。

## 3. mihomo 如何创建 TUN 设备

TUN 入口在：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:136)

创建 TUN 设备的位置在：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:472)

Linux/非 Windows 下实际调用：

- [listener/sing_tun/server_notwindows.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server_notwindows.go:9)

也就是通过 `tun.New(options)` 创建虚拟网卡。

如果配置里没有显式指定合法的设备名，mihomo 会先计算一个可用的 TUN 名称。

相关代码：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:116)

## 4. 系统为什么会把流量发给这个 TUN

关键不在“应用知道 mihomo 存在”，而在“操作系统的路由表认为这些包应该走 TUN 网卡”。

TUN 配置会在配置解析阶段进入 `general.Tun`：

- [config/config.go](/vm/project/github/clash-meta/mihomo/config/config.go:1640)

当 `tun.enable: true` 后，执行器会重建 TUN 监听：

- [hub/executor/executor.go](/vm/project/github/clash-meta/mihomo/hub/executor/executor.go:212)

如果配置了：

```yaml
tun:
  auto-route: true
```

则 `sing-tun` 会自动配置路由，让系统把匹配到的流量送到 TUN 设备。

也就是说：

- 应用还是正常往外发包
- 内核查路由
- 命中的流量被送到 TUN
- mihomo 从 TUN 收到这些包

这就是“流量被导入 mihomo”的本质。

## 5. mihomo 如何从 TUN 中读取流量

创建完 TUN 设备后，mihomo 会组装 `StackOptions`：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:482)

关键字段包括：

- `Tun: tunIf`
- `TunOptions: tunOptions`
- `Handler: handler`

随后创建并启动协议栈：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:496)
- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:501)

这里的 `tun.NewStack(...)` 会在内部持续读取 TUN 设备中的原始 IP 包，并根据包内容还原出：

- TCP 连接
- UDP 数据流
- 部分 DNS 流量

## 6. 流量如何进入 mihomo 的规则引擎

TUN 协议栈读到流量后，不会直接连代理，而是先交给统一的入口处理器。

处理器由这里创建：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:298)

它本质上是一个 `ListenerHandler`，最终把 TCP/UDP 流量交给统一的 `Tunnel` 接口。

### TCP 路径

TCP 新连接进入：

- [listener/sing/sing.go](/vm/project/github/clash-meta/mihomo/listener/sing/sing.go:122)

在这里会构造元数据，例如：

- 源地址
- 目的地址
- 入站类型（这里是 `TUN`）

然后调用：

- [listener/sing/sing.go](/vm/project/github/clash-meta/mihomo/listener/sing/sing.go:145)

即：

```go
h.Tunnel.HandleTCPConn(conn, cMetadata)
```

到这里，TCP 流量就正式进入 mihomo 的规则处理流程。

### UDP 路径

UDP 数据在这里进入统一处理：

- [listener/sing/sing.go](/vm/project/github/clash-meta/mihomo/listener/sing/sing.go:226)

最后调用：

- [listener/sing/sing.go](/vm/project/github/clash-meta/mihomo/listener/sing/sing.go:241)

即：

```go
h.Tunnel.HandleUDPPacket(cPacket, cMetadata)
```

这意味着 TUN、SOCKS、REDIR、TPROXY 等不同入口，后续都会汇入同一套规则引擎和代理选择逻辑。

## 7. DNS 劫持在 TUN 模式里的作用

如果配置了：

```yaml
tun:
  dns-hijack:
    - 0.0.0.0:53
```

那么发往 53 端口的 DNS 请求会优先被识别出来，而不是按普通业务流量转发。

相关实现：

- [listener/sing_tun/dns.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/dns.go:22)

对于 DNS 请求，mihomo 会走内部 DNS 处理逻辑，例如：

- `resolver.RelayDnsConn(...)`
- `relayDnsPacket(...)`

相关代码：

- [listener/sing_tun/dns.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/dns.go:31)
- [listener/sing_tun/dns.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/dns.go:99)

需要注意：

- `dns-hijack` 只影响 DNS 请求
- 它不等于“所有流量都被导入”
- 真正决定业务流量是否进入 TUN 的仍然是路由和 TUN 本身

## 8. 规则命中后发生了什么

当流量进入 `HandleTCPConn` 或 `HandleUDPPacket` 后，就会按 mihomo 现有规则进行匹配。

例如配置：

```yaml
rules:
  - DOMAIN-SUFFIX,baidu.com,socks5-proxy
  - MATCH,DIRECT
```

则行为是：

- 访问 `baidu.com` 命中 `socks5-proxy`
- 其他流量命中 `DIRECT`

这时 mihomo 会自己负责真正的出站动作：

- 如果命中代理节点，则由 mihomo 主动连接代理服务器
- 如果命中 `DIRECT`，则由 mihomo 直接连接目标地址

也就是说，应用和目标站点之间已经不是直接通信，而是变成：

- 应用 <-> TUN <-> mihomo <-> 代理或目标站点

## 9. 响应数据如何回到应用

远端返回数据后，mihomo 会把处理后的流量重新写回 TUN 协议栈，由内核再交还给原应用。

因此，从应用视角看：

- 自己发起了一个普通连接
- 收到了正常响应

但中间实际经过了：

- 路由重定向
- 虚拟网卡
- mihomo 规则引擎
- 代理或直连出站

## 10. TUN 模式与 TPROXY 模式的区别

两者都能把流量交给 mihomo，但方式不同。

### TUN 模式

- 依赖虚拟网卡
- 依赖系统路由把流量送进虚拟网卡
- mihomo 从 TUN 设备读取原始 IP 包

### TPROXY 模式

- 依赖 iptables / policy routing
- 依赖 netfilter 在内核里改写流量处理路径
- mihomo 通过 TPROXY 入口接收被内核转交的连接或数据包

简单说：

- TUN 是“把流量送到一张虚拟网卡上，再由 mihomo 接”
- TPROXY 是“用内核流量劫持机制把流量转交给 mihomo”

## 11. 为什么 TUN 模式常常更容易理解

因为它的模型更直接：

- 创建虚拟网卡
- 配路由
- 把 IP 包读进来
- 按规则处理
- 把结果写回去

相较之下，TPROXY 更依赖复杂的内核转发路径和 iptables 规则。

## 12. 对当前仓库配置的实际含义

像下面这样的最小配置：

```yaml
tun:
  enable: true
  stack: system
  dns-hijack:
    - 0.0.0.0:53
  auto-route: true
  auto-detect-interface: true

proxies:
  - name: socks5-proxy
    type: socks5
    server: 192.168.100.198
    port: 8899

rules:
  - DOMAIN-SUFFIX,baidu.com,socks5-proxy
  - MATCH,DIRECT
```

含义是：

1. mihomo 创建并启用 TUN 虚拟网卡
2. 自动配置路由，让系统流量进入这个 TUN
3. DNS 请求优先交给 mihomo 处理
4. 进入 mihomo 的业务流量按规则匹配
5. `baidu.com` 走 `192.168.100.198:8899`
6. 其他流量直连

## 13. 一句话总结

TUN 模式把流量导入 mihomo 的方式是：

通过虚拟网卡和系统路由，让内核把 IP 包交给 mihomo，而不是通过普通代理端口或域名级 iptables 规则把流量塞进进程。

## 14. 能否指定哪些流量进入 TUN

可以。

当前仓库里的 `tun` 配置支持对“哪些流量被导入 TUN”做单独控制。相关字段定义在：

- [config/config.go](/vm/project/github/clash-meta/mihomo/config/config.go:267)
- [listener/config/tun.go](/vm/project/github/clash-meta/mihomo/listener/config/tun.go:12)

可用控制项包括：

- `route-address`
- `route-exclude-address`
- `route-address-set`
- `route-exclude-address-set`
- `include-interface`
- `exclude-interface`
- `include-uid`
- `exclude-uid`
- `exclude-src-port`
- `exclude-dst-port`
- Android/mac 相关过滤项

这些配置控制的是：

- 哪些流量被路由到 TUN 网卡
- 哪些流量绕过 TUN，直接由系统原路径处理

需要注意的是，这一层是“导流层”，不是 `rules` 那一层。

也就是说：

- 先决定流量是否进入 TUN
- 再决定进入 mihomo 以后是 `DIRECT` 还是走某个代理节点

## 15. TUN 导流是否支持按域名控制

不能像 `rules` 一样直接按域名控制。

TUN 导流本质上依赖路由和过滤条件，所以更适合按这些维度控制：

- 目标 IP / 网段
- 接口
- UID
- 端口
- MAC 地址

例如：

- `192.168.100.0/24` 可以直接指定是否进入 TUN
- `192.168.100.20/32` 可以直接排除
- 某个 Linux 用户发出的流量可以直接排除
- 某个目标端口的流量可以直接排除

但 `baidu.com` 这种域名不能在 TUN 导流阶段直接匹配。

如果要实现“只让 baidu 相关流量进 TUN”，通常只能选择两种思路：

1. 让较大范围流量先进 TUN，再由 `rules` 决定只有 `baidu.com` 走代理
2. 在系统层先把 `baidu.com` 解析成 IP 集，再用手工路由或防火墙规则把这些 IP 导入 TUN

第二种方案的本质仍然是“按 IP 导流”，不是“运行时按域名导流”。

## 16. 路由如何决定哪些流量进入 TUN

TUN 的导流关键点不在应用，而在系统路由。

当启用 TUN 后，内核会根据路由规则决定：

- 哪些包要发往真实物理网卡
- 哪些包要发往 TUN 虚拟网卡

只有被路由到 TUN 的流量，mihomo 才能看到。

相关配置最终会进入：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:395)

其中包括：

- `Inet4RouteAddress`
- `Inet6RouteAddress`
- `Inet4RouteExcludeAddress`
- `Inet6RouteExcludeAddress`
- `IncludeInterface`
- `ExcludeInterface`
- `IncludeUID`
- `ExcludeUID`
- `ExcludeSrcPort`
- `ExcludeDstPort`

这说明在当前实现里，TUN 入口的导流范围本身就是可配置的。

## 17. `route-address` 和 `route-exclude-address`

这是最常用的一组导流控制项。

### `route-address`

表示“哪些目标网段应该被路由到 TUN”。

例如：

```yaml
tun:
  enable: true
  auto-route: true
  route-address:
    - 192.168.100.0/24
```

含义是：

- 只有发往 `192.168.100.0/24` 的流量会被导入 TUN

再例如：

```yaml
tun:
  enable: true
  auto-route: true
  route-address:
    - 0.0.0.0/1
    - 128.0.0.0/1
```

这是一个常见写法，表示：

- 用两个半区覆盖大部分 IPv4 默认路由

### `route-exclude-address`

表示“即使大范围流量进 TUN，这些目标仍然要绕过 TUN”。

例如：

```yaml
tun:
  enable: true
  auto-route: true
  route-address:
    - 0.0.0.0/0
  route-exclude-address:
    - 192.168.100.20/32
```

含义是：

- 默认所有 IPv4 流量进入 TUN
- 但发往 `192.168.100.20` 的流量不进入 TUN

## 18. `route-address-set` 和 `route-exclude-address-set`

这是基于规则集控制导流范围的方式，但本质仍然是基于目标 IP CIDR。

文档示例在：

- [docs/config/config.yaml](/vm/project/github/clash-meta/mihomo/docs/config/config.yaml:139)

注释已经说明其适用条件：

- 仅支持 Linux
- 需要 `nftables`
- 需要启用 `auto-route`
- 需要启用 `auto-redirect`

配置形态例如：

```yaml
tun:
  enable: true
  auto-route: true
  auto-redirect: true
  route-address-set:
    - ruleset-1
  route-exclude-address-set:
    - ruleset-2
```

含义是：

- 把 `ruleset-1` 中的目标 IP CIDR 加入导流范围
- 把 `ruleset-2` 中的目标 IP CIDR 从导流范围中排除

这不是直接按域名工作，而是把规则集里的 CIDR 下沉到导流层。

## 19. 接口、UID、端口级别的导流控制

当前实现还支持从这些维度控制是否进入 TUN：

- 接口：`include-interface` / `exclude-interface`
- 用户：`include-uid` / `exclude-uid`
- 端口：`exclude-src-port` / `exclude-dst-port`

示例：

```yaml
tun:
  enable: true
  auto-route: true
  route-address:
    - 0.0.0.0/0
  exclude-interface:
    - docker0
  exclude-uid:
    - 0
  exclude-dst-port:
    - 22
```

含义是：

- 默认所有 IPv4 流量进入 TUN
- 但和 `docker0` 接口相关的流量不导入
- root 用户发起的流量不导入
- 目标端口是 `22` 的流量不导入

这类配置在需要保护 SSH、容器网络、系统服务时很有用。

## 20. 自动路由和手动路由的区别

### 自动路由

对应：

```yaml
tun:
  auto-route: true
```

含义是由 `sing-tun` 自动创建并维护策略路由，把匹配流量送入 TUN。

优点：

- 配置简单
- 不需要手工维护 `ip rule` / `ip route`
- 适合大多数单机使用场景

缺点：

- 可见性不如手工配置直观
- 调试时需要额外查看系统下发的实际路由

### 手动路由

对应思路是：

- `tun.enable: true`
- `auto-route: false`
- 由用户自己手工配置系统路由和策略路由

例如自己维护：

- `ip rule`
- `ip route`
- 更复杂的策略路由表
- 外部 `nftables` / `iptables` 配套逻辑

优点：

- 完全可控
- 适合网关、旁路由、多出口、容器网络等复杂场景

缺点：

- 配置复杂
- 更容易出现环路、漏流量、优先级错误

## 21. `auto-route` 与 `auto-redirect` 的区别

这两个选项容易混淆，但作用不同。

### `auto-route`

负责：

- 自动配置路由
- 决定哪些流量被送入 TUN

### `auto-redirect`

负责：

- 自动配置额外的重定向规则
- 主要用于 Linux 下更复杂的透明导流场景

当前实现明确要求：

- 启用 `auto-redirect` 时，必须先启用 `auto-route`

相关代码：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:421)

文档注释则说明：

- `auto-redirect` 主要用于 Linux
- 在路由器场景下通常更有价值

参考：

- [docs/config/config.yaml](/vm/project/github/clash-meta/mihomo/docs/config/config.yaml:137)

## 22. 实际配置建议

### 场景一：只让某个网段进入 TUN

```yaml
tun:
  enable: true
  auto-route: true
  route-address:
    - 192.168.100.0/24
```

### 场景二：默认都进入 TUN，但排除某个地址

```yaml
tun:
  enable: true
  auto-route: true
  route-address:
    - 0.0.0.0/0
  route-exclude-address:
    - 192.168.100.20/32
```

### 场景三：默认都进入 TUN，但保护 SSH 和容器网络

```yaml
tun:
  enable: true
  auto-route: true
  route-address:
    - 0.0.0.0/0
  exclude-interface:
    - docker0
  exclude-dst-port:
    - 22
```

### 场景四：只想让 `baidu.com` 走代理

推荐做法不是“只让 baidu 进入 TUN”，而是：

1. 让足够范围的流量进入 TUN
2. 再用 `rules` 控制只有 `baidu.com` 走代理

例如：

```yaml
tun:
  enable: true
  auto-route: true

rules:
  - DOMAIN-SUFFIX,baidu.com,socks5-proxy
  - MATCH,DIRECT
```

这样更符合 mihomo 当前实现能力，也更容易维护。

## 23. 局域网 `/32` 主机路由的特殊处理

对于同网段局域网目标，单靠：

```yaml
tun:
  auto-route: true
  route-address:
    - 192.168.100.235/32
```

在很多 Linux 环境里仍然可能被现有的局域网直连路由抢走。

原因是内核通常已经有类似这样的直连路由：

```text
192.168.100.0/24 dev ens33 scope link
```

于是访问 `192.168.100.235` 时，流量会优先从物理网卡直接出去，而不是进入 TUN。

当前仓库已经补了第一版修复：

- Linux 下
- `tun.auto-route: true`
- `route-address` 中包含 IPv4 `/32`

时，mihomo 会在 TUN 启动成功后自动补一条等价于下面的主机路由：

```bash
ip route replace 192.168.100.235/32 dev Meta
```

关闭 TUN 时则会尝试删除这条主机路由。

对应实现位于：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:506)
- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:665)

因此，对于“只把某个同网段 LAN 主机导入 TUN”的场景，当前仓库已经具备基础支持，不再要求用户手工执行 `ip route add ... dev Meta`。
