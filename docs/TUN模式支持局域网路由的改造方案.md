# TUN 模式支持局域网路由的改造方案

本文讨论一个明确问题：

- 在 mihomo 的 TUN 模式下，希望将某个局域网目标地址导入 TUN
- 例如只把 `192.168.100.235/32` 导入 `Meta` 这个 TUN 网卡
- 但实际发现，仅靠当前 `tun.auto-route + route-address` 配置并不稳定生效
- 手工执行 `ip route add 192.168.100.235 dev Meta` 后，流量才会进入 mihomo

这说明当前实现对“同网段局域网主机路由导入 TUN”的支持不完整。

本文只写问题分析和代码改造方案，不直接修改代码。

## 1. 问题现象

以如下配置为例：

```yaml
tun:
  enable: true
  stack: system
  auto-route: true
  auto-detect-interface: true
  route-address:
    - 192.168.100.235/32
```

预期行为：

- 访问 `192.168.100.235` 的流量进入 TUN
- 其他流量不进入 TUN

实际行为：

- 访问 `192.168.100.235` 时，流量并没有进入 mihomo
- 手工执行下面的命令后，流量才能进入 mihomo：

```bash
sudo ip route add 192.168.100.235 dev Meta
```

这个现象说明，问题不在 `rules` 层，而在系统路由层。

## 2. 根因分析

### 2.1 同网段主机默认会命中直连路由

如果机器本身就在 `192.168.100.0/24` 网段，例如出口网卡是：

```text
192.168.100.x/24 dev ens33
```

那么系统通常已经存在一条直连路由：

```text
192.168.100.0/24 dev ens33 scope link
```

对于目标 `192.168.100.235`，内核会认为：

- 该目标在本地二层可达
- 应直接从物理网卡发 ARP 并发包

因此，如果 TUN 侧没有更明确、更高优先级的主机路由，这类流量往往不会进入 TUN。

### 2.2 当前 `route-address` 更像“自定义导流范围”，不是“强制插入主机路由”

当前仓库里，`route-address` 的注释是：

- 启用 `auto-route` 时使用自定义路由而不是默认路由

参考：

- [docs/config.yaml](/vm/project/github/clash-meta/mihomo/docs/config.yaml:150)

当前配置会被传入 `TunOptions`：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:396)

但从现象看，当前 `auto-route + route-address` 并没有确保对同网段单主机地址写入一条足够强的：

```text
ip route add <host>/32 dev <tun>
```

或者等价的策略路由结果。

所以：

- 对公网、非直连网段，这套行为可能足够
- 对同网段局域网主机，这套行为不够稳定

### 2.3 手工 `ip route add ... dev Meta` 证明了解决方向

下面这条命令一旦生效：

```bash
ip route add 192.168.100.235 dev Meta
```

就说明内核已经被明确告知：

- 访问这个目标时，不要走原物理网卡的局域网直连路径
- 直接把它交给 `Meta` TUN 设备

因此，代码改造的核心目标应该是：

在特定条件下，由 mihomo 自动补齐这类“局域网主机路由到 TUN”的系统路由。

## 3. 当前代码相关位置

### 3.1 `tun` 配置字段定义

`route-address`、`route-exclude-address` 等字段定义在：

- [config/config.go](/vm/project/github/clash-meta/mihomo/config/config.go:267)
- [listener/config/tun.go](/vm/project/github/clash-meta/mihomo/listener/config/tun.go:12)

### 3.2 配置进入 `general.Tun`

配置解析后进入：

- [config/config.go](/vm/project/github/clash-meta/mihomo/config/config.go:1647)

### 3.3 最终传入 `TunOptions`

`route-address`、`route-exclude-address` 等配置最终传给 `TunOptions`：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:384)

尤其是：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:396)
- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:398)

### 3.4 TUN 设备创建与启动

TUN 设备创建和协议栈启动在：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:472)
- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:496)
- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:501)

如果要补系统路由，时机通常应在：

- TUN 设备已创建
- 已知实际 TUN 名称
- 路由目标集已确定

之后再执行。

## 4. 改造目标

目标不是泛化重写整个 `auto-route` 逻辑，而是补齐一个缺口：

- 当 `route-address` 中包含局域网目标，尤其是 `/32` 主机路由
- 且该目标可能被现有直连局域网路由吞掉时
- 自动为这些目标补一条明确指向 TUN 的主机路由

最小目标：

- `192.168.100.235/32` 这类目标不再需要用户手工执行 `ip route add ... dev Meta`

更理想的目标：

- 对所有同网段 LAN 主机目标都能稳定导入 TUN
- 且不破坏现有非 LAN 目标的行为

## 5. 方案设计

这里给出三个方案，从保守到激进排列。

## 5.1 方案 A：自动为 `route-address` 中的 IPv4 `/32` 目标补主机路由

### 设计思路

当满足以下条件时：

- `tun.enable = true`
- `auto-route = true`
- `route-address` 中存在 IPv4 `/32`

则在 TUN 启动完成后，自动执行等价逻辑：

```bash
ip route replace <dst>/32 dev <tunName>
```

例如：

```bash
ip route replace 192.168.100.235/32 dev Meta
```

### 优点

- 改动小
- 行为直观
- 直接解决当前暴露出来的问题
- 不需要用户增加额外脚本

### 缺点

- 只覆盖 `/32` 主机路由场景
- 对更大 LAN 网段是否需要同样处理，没有统一答案
- 需要谨慎处理路由清理

### 适合度

这是最推荐的第一步实现。

## 5.2 方案 B：自动识别“与出口网卡同网段”的目标，并补精确主机路由

### 设计思路

在 TUN 初始化期间：

1. 获取默认出口接口及其地址
2. 判断 `route-address` 中的目标是否与出口接口地址同网段
3. 若同网段且目标是主机地址，则自动补：

```bash
ip route replace <dst>/32 dev <tunName>
```

### 优点

- 更符合“只在需要时介入”
- 不会对所有 `/32` 目标一刀切

### 缺点

- 逻辑更复杂
- 需要依赖接口探测结果稳定
- 多出口、多地址、策略路由场景下边界更多

### 适合度

适合作为方案 A 之后的增强，而不是第一版。

## 5.3 方案 C：增加显式配置项，允许用户声明“强制写主机路由到 TUN”

### 设计思路

新增配置，例如：

```yaml
tun:
  enable: true
  auto-route: true
  route-address:
    - 192.168.100.235/32
  force-route-address:
    - 192.168.100.235/32
```

或者更明确：

```yaml
tun:
  enable: true
  auto-route: true
  route-address:
    - 192.168.100.235/32
  route-address-via-tun:
    - 192.168.100.235/32
```

对应行为是：

- 对这些地址额外补系统主机路由，明确指向 TUN 设备

### 优点

- 行为显式
- 用户可控
- 不改现有默认行为

### 缺点

- 增加新的配置面
- 文档和兼容性成本更高
- 第一版实现门槛比方案 A 高

### 适合度

如果希望长期维护、避免“自动猜测”，这是更干净的产品化方案。

## 6. 推荐实现顺序

推荐分两步走。

### 第一步

先实现方案 A：

- 对 `route-address` 中的 IPv4 `/32` 自动补 `ip route replace ... dev <tun>`

理由：

- 当前问题就是单主机 LAN 目标
- 修改范围最小
- 最快能验证正确性

### 第二步

如果后续发现需要更细化控制，再评估方案 C：

- 增加显式配置项
- 将“是否强制写 TUN 主机路由”开放给用户控制

## 7. 建议代码落点

建议把改造落在 `listener/sing_tun` 这一层，而不是配置解析层。

原因：

- 这里已经拿到了最终的 `TunOptions`
- 这里知道真实的 `tunName`
- 这里最接近 TUN 生命周期
- 这里便于在 `Close()` 时同步清理路由

优先落点建议：

- [listener/sing_tun/server.go](/vm/project/github/clash-meta/mihomo/listener/sing_tun/server.go:472) 之后
- 即 TUN 设备创建成功、`tunName` 明确之后

可行的结构是新增辅助函数，例如：

```go
func setupExtraHostRoutes(tunName string, routePrefixes []netip.Prefix) error
func cleanupExtraHostRoutes(tunName string, routePrefixes []netip.Prefix)
```

职责：

- 只处理符合条件的主机路由
- 使用 `ip route replace`
- 在关闭监听器时清理自己加的路由

## 8. 实现细节建议

### 8.1 只处理 `/32`

第一版建议只处理 IPv4 `/32`：

- 风险可控
- 语义明确
- 与用户当前需求完全对齐

### 8.2 用 `replace` 不用 `add`

建议使用：

```bash
ip route replace <dst>/32 dev <tunName>
```

原因：

- 幂等
- 重启或热更新时更稳
- 避免“路由已存在”报错

### 8.3 只在 Linux 启用

这类逻辑明显是 Linux 专属。

应限制在：

- `runtime.GOOS == "linux"`

其他平台直接跳过。

### 8.4 路由清理必须成对实现

既然自动补了：

```bash
ip route replace ...
```

在 TUN 关闭时就应考虑清理：

```bash
ip route del ...
```

否则容易残留脏路由。

### 8.5 错误策略

建议：

- 路由补充失败时，打印明确 warning / error
- 但要区分是否阻止 TUN 启动

初步建议：

- 默认记错误并返回失败，让用户明确知道当前配置没有按预期接管该路由

因为这类失败会直接导致功能失效，静默容忍意义不大。

## 9. 风险与边界

### 9.1 可能影响本地局域网直连行为

一旦把 LAN 主机路由强制指向 TUN，就意味着：

- 原本直接从物理网卡二层访问的主机
- 现在先进入 mihomo

这是预期行为，但必须被明确认知。

### 9.2 多出口接口场景更复杂

如果主机上存在：

- 多块物理网卡
- 多个同网段地址
- 复杂策略路由

那么“局域网主机是否应强制进 TUN”可能并不简单。

这也是为什么第一版建议只做最小修复，不做过度自动推断。

### 9.3 与 `auto-redirect` 的关系

当前问题已经证明，即便不开 `auto-redirect`，只要系统里明确存在：

```bash
ip route add 192.168.100.235 dev Meta
```

流量就可以进入 TUN。

因此这个改造不应强依赖 `auto-redirect`。

## 10. 结论

当前 TUN 模式对“同网段局域网主机导入 TUN”的支持存在缺口。

已有验证表明：

```bash
ip route add 192.168.100.235 dev Meta
```

能够直接修复问题。

因此，最小且务实的代码改造方向是：

- 在 Linux 下
- 当 `auto-route` 启用
- 且 `route-address` 中包含 IPv4 `/32` 主机目标时
- 由 mihomo 自动补一条 `ip route replace <dst>/32 dev <tunName>`

推荐先按这个方向做第一版实现，再决定是否增加显式配置项做更细粒度控制。
