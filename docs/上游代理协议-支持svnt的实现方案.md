# 上游代理协议支持 SVNT 的实现方案

本文说明如何在 mihomo 中新增一个上游出站协议 `svnt`，用于连接支持 SVNT 协议的上游服务器。

这里的目标是：

- 在 mihomo 的 `proxies:` 中新增 `type: svnt`
- 让 mihomo 在出站时像使用 `socks5`、`trojan` 一样，能够连到 SVNT 服务器
- 按 SVNT 规定的消息格式完成建连协商
- 协商成功后透传业务流量

协议依据来自：

- [/vm/project/gitcnsind/SVNT/doc/svnt客户端与服务器的通信协议说明.md](/vm/project/gitcnsind/SVNT/doc/svnt客户端与服务器的通信协议说明.md)

同时结合了 SVNT 源码中的实际实现细节：

- 消息结构与收发：  
  [/vm/project/gitcnsind/SVNT/src/svnt/Common/Msg.go](/vm/project/gitcnsind/SVNT/src/svnt/Common/Msg.go)
- 代理连接流程：  
  [/vm/project/gitcnsind/SVNT/src/svnt/Adapter/Proxy/ProxyConnector.go](/vm/project/gitcnsind/SVNT/src/svnt/Adapter/Proxy/ProxyConnector.go)
- 控制/工作通道注册：  
  [/vm/project/gitcnsind/SVNT/src/svnt/Assist/ProxyServer.go](/vm/project/gitcnsind/SVNT/src/svnt/Assist/ProxyServer.go)
- 加密包装：  
  [/vm/project/gitcnsind/SVNT/src/svnt/Socket/CryptoConn.go](/vm/project/gitcnsind/SVNT/src/svnt/Socket/CryptoConn.go)

## 1. 先给结论

在 mihomo 里支持 `svnt`，建议按两阶段做：

### 第一阶段

实现一个最小可用的 `type: svnt`，只支持：

- TCP 承载
- `MSG_TYPE_TUNNEL` 建连
- `Encrypt = 0`
- 建连成功后纯透明 TCP 转发

也就是说，先把它做成一个“像 SOCKS5 一样的单连接出站协议”。

### 第二阶段

再考虑扩展：

- WebSocket 承载
- 更完整的 `PolicyID` / `InstancePath` / `IPPath` 控制
- UDP 支持
- `encrypt: 1`
- 与 SVNT 的控制通道 / 工作通道模型深度集成

对 mihomo 来说，第一阶段是合理的，第二阶段才是重度耦合。

## 1.1 已定稿边界

以下边界已经定稿，后续代码实现按此执行：

- 第一版只做 `TCP`
- 第一版只做 `MSG_TYPE_TUNNEL`
- 第一版只支持 `encrypt: 0`
- 第一版不支持 `WebSocket`
- 第一版不支持 `UDP`
- `key-data` 为必填配置
- `policy-id` 默认值为 `*`，同时允许显式配置
- `instance-id` 允许省略；省略时默认使用 `mihomo`
- `client-addr` / `instance-path` / `ip-path` 第一版只做最小自动填充
- SVNT 返回失败时，直接建连失败，并透出 `StatusString`
- mihomo 第一版采用“每条出站连接对应一条 SVNT 上游连接”的模型
- 第一版同时补示例配置文档

这意味着第一版的目标非常明确：

- 不是完整移植 SVNT 控制/工作通道体系
- 不是一次性覆盖所有承载层和数据面
- 而是先把 `type: svnt` 的 TCP 明文出站主路径打通

## 2. 为什么可以先做成单连接出站协议

SVNT 文档里写了两类重要路径：

1. 客户端直接发 `MSG_TYPE_TUNNEL` 建连，协商成功后进入透明转发  
   这条最接近 mihomo 当前的普通出站模型。

2. `REGISTOR_CONTROL` / `REGISTOR_WORK` / `WORK_FOR_PROXY`  
   这是 SVNT 自己的代理/控制平面，用于远程注册策略、复用工作通道。

对于 mihomo 新增一个上游协议的目标，第一条就够用了：

- mihomo 出站时，本来就是“为每条目标连接建立一个上游连接”
- `MSG_TYPE_TUNNEL` 也正好是“拿目标地址建一条隧道”

所以第一版不需要把 mihomo 改造成 SVNT 的“控制端 + 工作端”双通道架构。  
先复用 `MSG_TYPE_TUNNEL` 即可。

这也是本文推荐的实现方向。

## 3. 对应到 mihomo 的实现模型

在 mihomo 里，新增一个上游协议的典型接入点是：

- 新增 `adapter/outbound/svnt.go`
- 在 [adapter/parser.go](/vm/project/github/clash-meta/mihomo/adapter/parser.go) 注册 `case "svnt"`
- 在 [constant/adapters.go](/vm/project/github/clash-meta/mihomo/constant/adapters.go) 增加 `Svnt`

参考已有协议：

- `socks5`：  
  [adapter/outbound/socks5.go](/vm/project/github/clash-meta/mihomo/adapter/outbound/socks5.go)
- `http`：  
  [adapter/outbound/http.go](/vm/project/github/clash-meta/mihomo/adapter/outbound/http.go)
- `trojan`：  
  [adapter/outbound/trojan.go](/vm/project/github/clash-meta/mihomo/adapter/outbound/trojan.go)

从接入形态上，`svnt` 第一版最接近：

- 先连远端
- 发一个协议握手请求
- 服务端回复 OK
- 然后双方直接透传数据

所以它比 `trojan` / `socks5` 更像“带前置 JSON 握手的 TCP 隧道协议”。

## 4. SVNT 协议最关键的实现点

### 4.1 承载层

SVNT 底层可用：

- TCP
- WebSocket

这来自协议说明：

- [/vm/project/gitcnsind/SVNT/doc/svnt客户端与服务器的通信协议说明.md](/vm/project/gitcnsind/SVNT/doc/svnt客户端与服务器的通信协议说明.md)

第一版已定为只做 TCP。原因很直接：

- mihomo 现有出站协议大多默认先做 TCP 主路径
- WebSocket 会引入额外握手和包装层
- 先跑通裸 TCP 更容易验证 SVNT 应用层是否正确

### 4.2 消息格式

SVNT 的消息格式是：

1. 先写 4 字节大端 `int32`
2. 再写 JSON 内容

消息结构定义在：

- [/vm/project/gitcnsind/SVNT/src/svnt/Common/Msg.go](/vm/project/gitcnsind/SVNT/src/svnt/Common/Msg.go)

关键字段包括：

- `Magic`，固定 `SVNT`
- `Auth`
- `InstanceID`
- `MsgType`
- `Status` / `StatusString`
- `PolicyID`
- `Encrypt`
- `TargetAddr`
- `ClientAddr`
- `InstancePath`
- `IPPath`

这意味着在 mihomo 里需要自己实现一套最小 `svntMsg` 编解码，不建议直接硬依赖外部 SVNT 仓库。

### 4.3 鉴权

SVNT 校验：

- `Magic == "SVNT"`
- `Auth == crypto.Auth`

源码位置：

- [/vm/project/gitcnsind/SVNT/src/svnt/Common/Msg.go](/vm/project/gitcnsind/SVNT/src/svnt/Common/Msg.go)

这意味着 mihomo 的 `svnt` 出站配置必须至少提供：

- `key-data`

`key-data` 的语义不是“把一个现成的 `Auth` 字符串直接填进配置”，而是和 SVNT 自身保持一致：  
mihomo 侧根据 `KeyData` 直接参考 [/vm/project/gitcnsind/SVNT/src/svnt/crypto/crypto.go](/vm/project/gitcnsind/SVNT/src/svnt/crypto/crypto.go) 中 `CryptoEnvInitWithData(...)` 的逻辑生成本地 `crypto.Auth`，再把生成结果写入握手消息的 `Auth` 字段。

否则服务端会直接报：

- 消息验证不一致

### 4.4 建连消息

第一版要发送：

- `MsgType = MSG_TYPE_TUNNEL`

并携带：

- `PolicyID`
- `TargetAddr`
- `ClientAddr`
- `Encrypt`
- `InstancePath`
- `IPPath`

建连成功后服务端返回：

- `Status = MSG_STATUS_OK`
- `Encrypt` 最终协商结果

之后直接进入透明转发。

这部分行为和协议说明一致，也能在 SVNT 的 `TunnelConnector` / `Client` 行为里找到对应。

## 5. 推荐的 mihomo 配置设计

建议第一版定义成这样：

```yaml
proxies:
  - name: svnt-proxy
    type: svnt
    server: 192.168.100.10
    port: 9000
    key-data: your-key-data
    policy-id: "*"
    encrypt: 0
    instance-id: mihomo
    network: tcp
```

建议字段解释：

| 字段 | 是否建议第一版支持 | 说明 |
|---|---|---|
| `name` | 是 | 节点名 |
| `type` | 是 | 固定 `svnt` |
| `server` | 是 | SVNT 服务端地址 |
| `port` | 是 | SVNT 服务端端口 |
| `key-data` | 是 | 本地密钥数据；直接参考 `svnt/crypto/crypto.go` 的逻辑推导出 `MSG.Auth` |
| `policy-id` | 是 | 对应 `MSG.PolicyID`，默认 `*` |
| `encrypt` | 是 | 第一版固定只支持 `0` |
| `instance-id` | 建议 | 对应 `MSG.InstanceID` |
| `network` | 建议 | 第一版固定只接受 `tcp` |
| `client-addr` | 可选 | 不填则由 mihomo 自动推导或留空 |
| `instance-path` | 可选 | 默认自动构造最小值 |
| `ip-path` | 可选 | 默认自动构造最小值 |
| `ws-path` / `ws-host` | 第二阶段 | WebSocket 承载时再支持 |

## 6. 代码实现建议

## 6.1 新增文件

建议新增：

- `adapter/outbound/svnt.go`

如果代码稍多，可以拆成：

- `adapter/outbound/svnt.go`
- `adapter/outbound/svnt_msg.go`
- `adapter/outbound/svnt_crypto.go`

其中：

- `svnt.go` 负责 mihomo 适配器本身
- `svnt_msg.go` 负责消息收发和 JSON 编解码
- `svnt_crypto.go` 负责 `Encrypt=1` 的流包装

## 6.2 新增类型枚举

在：

- [constant/adapters.go](/vm/project/github/clash-meta/mihomo/constant/adapters.go)

里增加：

```go
Svnt
```

并在 `String()` 中增加：

```go
case Svnt:
    return "Svnt"
```

## 6.3 在 parser 中注册

在：

- [adapter/parser.go](/vm/project/github/clash-meta/mihomo/adapter/parser.go)

加：

```go
case "svnt":
    svntOption := &outbound.SvntOption{BasicOption: basicOption}
    err = decoder.Decode(mapping, svntOption)
    if err != nil {
        break
    }
    proxy, err = outbound.NewSvnt(*svntOption)
```

## 6.4 `SvntOption` 设计

第一版建议：

```go
type SvntOption struct {
    BasicOption
    Name       string `proxy:"name"`
    Server     string `proxy:"server"`
    Port       int    `proxy:"port"`
    KeyData    string `proxy:"key-data"`
    PolicyID   string `proxy:"policy-id,omitempty"`
    Encrypt    int    `proxy:"encrypt,omitempty"`
    InstanceID string `proxy:"instance-id,omitempty"`
    Network    string `proxy:"network,omitempty"`
}
```

第一版约束：

- `Network` 默认 `tcp`
- 第一版仅允许 `tcp`
- `KeyData` 不能为空
- 第一版 `Encrypt` 固定为 `0`

## 6.5 `Svnt` 结构体设计

建议：

```go
type Svnt struct {
    *Base
    option *SvntOption
}
```

如果后续要支持 WebSocket、更多状态缓存，再补字段。

## 6.6 `DialContext()` 的实现思路

`DialContext()` 第一版应做这些事：

1. `dialer.DialContext(ctx, "tcp", s.addr)` 连接 SVNT 服务器
2. 构造 `MSG_TYPE_TUNNEL` 请求
3. `TargetAddr` 使用 `metadata.RemoteAddress()`
4. `ClientAddr` 可用 `""` 或本地连接地址
5. `PolicyID` 使用配置值
6. `Encrypt` 使用配置值
7. 发送消息并读取响应
8. 检查响应 `Status == OK`
9. 如果响应中的 `Encrypt != 0`，直接返回不兼容错误
10. 返回 `NewConn(c, s)`

其中第 2 步之前，需要先用 `KeyData` 直接参考 `svnt/crypto/crypto.go` 中 `CryptoEnvInitWithData(...)` 的逻辑生成本地 `Auth`：

1. 直接把密钥字节作为 AES key
2. 用密钥字节的 MD5 前 16 字节生成 IV
3. 使用 `AES-CBC` 加密固定明文 `SVNTMSGAUTH12345`
4. 对密文做 Base64 编码
5. 把结果填入握手消息 `Auth`

这里应尽量按源码一比一实现，不自行抽象成“语义等价”的变体，避免和现网 SVNT 服务端出现细微不兼容。

这和现有出站协议的结构是一致的：

- 先连上游
- 再握手
- 再透传

## 6.7 消息结构建议本地重写

不要直接 import SVNT 仓库的 `Common.MSG`。

建议在 mihomo 内部定义最小兼容结构，例如：

```go
type svntMsg struct {
    Magic        string   `json:"Magic"`
    Auth         string   `json:"Auth"`
    InstanceID   string   `json:"InstanceID"`
    MsgType      int      `json:"MsgType"`
    MsgSubType   int      `json:"MsgSubType"`
    Status       int      `json:"Status"`
    StatusString string   `json:"StatusString"`
    PolicyID     string   `json:"PolicyID"`
    Encrypt      int      `json:"Encrypt"`
    CtrlID       int64    `json:"CtrlID"`
    TargetAddr   string   `json:"TargetAddr"`
    ClientAddr   string   `json:"ClientAddr"`
    DataString   string   `json:"DataString"`
    InstancePath []string `json:"InstancePath"`
    IPPath       []string `json:"IPPath"`
}
```

原因：

- 降低跨仓库耦合
- 避免把整个 SVNT 代码树拉进 mihomo
- 只保留出站握手实际需要的字段

## 6.8 加密怎么处理

SVNT 的 `Encrypt = 1` 后，数据通道会切换成加密包装。

SVNT 源码里用的是：

- `cipher.NewCFBEncrypter`
- `cipher.NewCFBDecrypter`

对应包装在：

- [/vm/project/gitcnsind/SVNT/src/svnt/Socket/CryptoConn.go](/vm/project/gitcnsind/SVNT/src/svnt/Socket/CryptoConn.go)

但这里有一个关键不确定点：

**仅从你给的协议说明里，还不能完整确定 mihomo 侧如何安全生成与服务器一致的 `crypto.Block` 和 `crypto.IV`。**

也就是说，第一版实现前必须进一步确认：

1. `crypto.Auth` 与加密密钥是否同源
2. `Block` / `IV` 如何协商或静态生成
3. 服务端是否只靠配置即可推导出相同密钥

就目前掌握的信息，第一版已经可以按 SVNT 现有实现直接落地：

- 直接参考 `svnt/crypto/crypto.go`，从 `key-data` 推导 `Auth`
- `encrypt: 0` 时仍然执行 `Auth` 一致性校验
- 不需要把 `Auth` 暴露成用户单独填写的配置项

当前已经定稿，第一版只支持：

- `encrypt: 0`

并把 `encrypt: 1` 明确留到第二阶段。

## 7. UDP 支持建议暂缓

从当前协议说明看，主流程强调的是：

- 建隧道
- 建连成功后透明转发

但没有像 SOCKS5/TUIC 那样把 UDP 建链细节明确写透。

对于 mihomo 来说，UDP 支持需要回答这些问题：

- `MSG_TYPE_TUNNEL` 后的数据流是否天然可承载 UDP
- 远端如何区分数据报边界
- 是走单独通道，还是复用现有透明隧道
- 加密模式下 UDP 如何封装

在没有更完整 SVNT 数据面说明前，不建议第一版实现 UDP。

因此第一版已定：

- 第一版 `SupportUDP() == false`
- 不实现 `ListenPacketContext()`

等 TCP 跑通后，再基于服务端代码补全。

## 8. WebSocket 承载建议放第二阶段

SVNT 支持底层 WebSocket，这在协议说明里已经写明。

但对于 mihomo 第一版来说，WebSocket 会增加：

- URL/path 设计
- Host/Header 设计
- Upgrade 流程
- 与 `svntMsg` 的 framing 叠加

而当前目标只是“支持 SVNT 上游协议”，不是“完整复刻 SVNT 的全部承载层”。

所以第一版已定：

- 第一版只做 `network: tcp`
- 第二阶段再考虑 `network: ws`

到第二阶段时，可参考 mihomo 里已有的 WebSocket 客户端实现，例如：

- [transport/vmess/websocket.go](/vm/project/github/clash-meta/mihomo/transport/vmess/websocket.go)

## 9. 分阶段实施计划

## 阶段一：TCP + 明文

目标：

- 支持 `type: svnt`
- 支持 TCP 承载
- `encrypt: 0`
- `MSG_TYPE_TUNNEL`
- 建连成功后透传 TCP

改动：

- `constant/adapters.go`
- `adapter/parser.go`
- `adapter/outbound/svnt.go`
- `docs/config.yaml` 增加最小示例

这是已经定稿并立即执行的第一步。

## 阶段二：补 `encrypt: 1`

前提：

- 先确认 SVNT 的密钥派生和 IV 规则

改动：

- 增加 `svnt_crypto.go`
- 实现 `net.Conn` 包装

## 阶段三：支持 WebSocket

前提：

- 明确 SVNT 的 WS 地址、路径、握手要求

改动：

- 增加 `network: ws`
- 增加 `ws-host`、`ws-path` 等配置

## 阶段四：评估 UDP

前提：

- 确认 SVNT 数据面能否稳定承载 UDP

## 10. 需要用户或协议方补充确认的信息

在真正开始编码前，建议先确认这些点：

1. `crypto.Auth` 的来源和配置方式  
   当前已经明确：应由 `key-data` 直接参考 `svnt/crypto/crypto.go` 中的实现生成，而不是让用户直接填写 `auth`

2. `encrypt=1` 时密钥和 IV 如何得到  
   这是是否能做加密版的核心

3. WebSocket 模式下的 URL / path / host 规则  
   文档只说底层可用 WS，但没有给出握手路径细节

4. 服务端是否允许一个外部客户端直接只发 `MSG_TYPE_TUNNEL` 就工作  
   从现有说明看答案是“可以”，但最好再由你确认目标服务端版本是否一致

5. 是否必须带 `PolicyID`，以及默认值能否用 `*`

其中最关键的是第 2 条。  
它会阻塞第二阶段 `encrypt: 1`，但不阻塞当前第一版。

## 11. 最终建议

对 mihomo 来说，SVNT 最合理的第一版不是完整移植 SVNT 的控制/工作通道体系，而是：

- 新增一个 `type: svnt`
- 走 TCP 连接
- 按 `MSG_TYPE_TUNNEL` 发起握手
- 握手成功后直接透传 TCP

这条路径改动最小，也最符合 mihomo 现有“上游代理协议”的抽象。

如果你准备继续推进实现，我建议下一步按这个范围落代码：

1. 先只做 `encrypt: 0`
2. 先只做 `network: tcp`
3. 先只做 `DialContext()`
4. 暂不做 UDP
5. 暂不做 WebSocket

等 TCP 明文跑通，再补加密和更复杂承载层。  
这是最稳的工程顺序。
