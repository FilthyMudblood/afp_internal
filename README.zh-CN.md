# AFP-Core（企业）

GitHub 仓库名仍是 Aegis Fabric Protocol。本目录是 **企业 / Path A** 协议。

[![Docker Publish](https://github.com/FilthyMudblood/aegis-fabric/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/FilthyMudblood/aegis-fabric/actions/workflows/docker-publish.yml)
[![GHCR Sidecar](https://img.shields.io/badge/GHCR-sidecar-2496ED?logo=docker&logoColor=white)](https://ghcr.io/filthymudblood/aegis-fabric-sidecar)
[![GHCR Operator](https://img.shields.io/badge/GHCR-operator-2496ED?logo=docker&logoColor=white)](https://ghcr.io/filthymudblood/aegis-fabric-operator)
[![GHCR Demo Agent](https://img.shields.io/badge/GHCR-demo--agent-2496ED?logo=docker&logoColor=white)](https://ghcr.io/filthymudblood/afp-demo-agent)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

### **企业 Agent-to-Agent 的物理刹车片**

*Path A：在行政域内管住自己的 planner*

> **"TCP 治理数据包，AFP 治理优化器"**
> *(TCP governs packets. AFP governs optimizers.)*

**Aegis Fabric Protocol（AFP）** 是面向**企业 agent-to-agent** 的**后果持久层（CPL）**：插在「Agent 能通信」和「这支舰队能存活」之间的运行时刹车。参考实现为 K8s Sidecar。

开放网格下的邻居治理（陌生人、治理头、身份绑定拒绝）是**分开写的姊妹协议 AFP-Exchange**（`../AFP-Exchange`）。本仓库不实现、不评价那一套。双栖节点仍应共用 allow / delay / isolate 字母表，以免 split-brain——那是共享内核，不是一个 sidecar 加开关。

English · [`README.md`](README.md) · 白皮书 · **[v2 Protocol Edition（英文）](docs/whitepaper-v2/whitepaper-v2-protocol-edition.md)** · [v1 Zenodo 存档](https://zenodo.org/records/20674352)

关于这一执行边界设计的一篇短文曾投往 NeurIPS 2026 Agents in the Wild，未被录用。审稿要求用真实 planner/tool 轨迹代替合成网格；这部分工作正在本仓库推进。

### 市场空白

Multi-agent 栈解决了**规划**（LangGraph、CrewAI）和**传输**（gRPC、Kafka、MCP、ASP）。都没解决**协调运行时**：

> *这一步能不能执行？失控之后，后果能不能记住？*

| 层次 | 状态 |
|------|------|
| 传输 | 已有解 — 字节送达 |
| 语义协作 | 演进中 — ASP、A2A、MCP |
| **协调运行时法则** | **空白 — AFP 填这一格** |

| 场景 | AFP 回答什么 |
|------|----------------|
| **企业 multi-agent（本仓库）** | 在 Pod 内、出网前掐灭 planner 失控 —— 避免 OOM、Token 燃烧、重试雪崩 |
| **开放网格（姊妹协议 AFP-Exchange）** | 陌生人准入与身份绑定拒绝：另文另仓库，本仓库不实现、不评价 |

> *Agent 学会了说话，网络学会了路由。**还没有人在优化器提交之前踩刹车，并记住它失控过。***

本仓库是 **closed mesh / Path A**（mTLS，以 PreFlight 为主）。**open exchange**（GovernanceHeader、CVP、陌生人税）见姊妹协议 [AFP-Exchange](../AFP-Exchange)，不要当成同一个 sidecar 的开关。白皮书 §4 仍保留规格讨论，不等于本仓库已交付。

### 为什么 Agent 堆叠需要「有记忆」

**Agent 堆叠**——`Planner → A → B → C → …`——风险随跳数复利。每一步单独看可能都「合法」，但深度、队列压力、上下文在累积，不一定表现为某一条「坏消息」：

```text
堆叠风险  ≈  深度 × 分支 × 上下文 × peer 传染
```

按请求放行的网关会**遗忘**。优化器会把工作拆成无数语法合法的小步来绕过。CPL 把后果绑在 **agent/peer 身份**上，堆叠失控无法靠「每一跳都很礼貌」来洗白。

| 对象 | CPL 管什么 | CPL 不判断什么 |
|------|------------|----------------|
| **失控** | 递归环、意图爆发、上下文雪崩逼近 OOM | — |
| **物理恶意**（开放网） | peer 洪水、hit-and-run 陌生人——CVP、陌生人税、gossip | 消息内容的语义善恶 |
| **允许的堆叠** | 策略内的 `Planner → A → B → C` | 每一步业务上是否「该做」 |

### 生产里：偶发还是常态？

| 风险类型 | 企业 multi-agent | 开放 P2P 网 |
|----------|------------------|-------------|
| **堆叠失控** | **常见** — 分解与委派是优化器默认行为，不是罕见 bug | **常见** — 传染放大本地失控 |
| **物理恶意** | **相对少** — 多为误配、滥用、prompt 注入；非常态对抗 peer | **需当真** — 陌生人、洪水、hit-and-run |

失控有两种形态：**偶发尖峰**（某次 replan 死循环、某次 burst）和**结构性渗漏**（7×24 agent fleet 无运行时刹车，持续漏钱）。AFP 针对的是**堆叠的规模律**——跳数越多，物理越先穿——不是「Agent 天天作恶」的恐吓叙事。

> **实话实说：** 不是因为 Agent 每天都很坏才需要 AFP，而是因为**堆叠后的优化器 routinely 跑赢 retry、Token 看板和按请求限流**。

### 为什么 retry、Token 上限和监控不够

`max_retries`、`recursion_limit`、Token 预算是**必要遥测**，但对 multi-agent 堆叠**不足以当刹车**：

| 手段 | 能帮到 | 结构性缺口 |
|------|--------|------------|
| **Retry / 循环计数** | 单进程、单图里的环 | 随 session 清零；看不见跨 Agent 链；一次 retry 内部仍可能爆发 |
| **Token / 成本封顶** | 花超了再停 | **事后** — 步骤已提交才计数；内部队列可能尚未体现为 Token |
| **指标与告警** | 发现事故 | **先执行后告警** — 至少晚一个 cycle；无 peer 级持久后果 |
| **框架内护栏** | 开发时自声明 | 进程内、不可移植、入站 mesh 不可执法 |

```text
监控：  步骤已提交 → 数 Token → 告警 → 停（本轮）
CPL：   下一步之前 → PreFlight → 节流/隔离（且记住）
```

**互补，不替代：** 保留 Token 预算与看板；在优化器 **commit** 处叠加 CPL，后果跨 hop、跨 session **可持久**。

### 根因：幻觉、优化器行为，还是恶意？

AFP **不是**反幻觉产品，不判断某一步是否「事实错误」。

| 失效模式 | 常见根因 | 幻觉扮演的角色 |
|----------|----------|----------------|
| **堆叠失控** | 优化器默认行为——分解、replan、retry、委派——**不必**先产生虚假陈述 | **有时加剧** — 编造工具、误判未完成、错误再委派 |
| **物理恶意**（开放网） | Prompt 注入、滥用、对抗 peer、hit-and-run | **通常无关** — 多为故意或结构性问题，不是「模型说错话」 |

死循环常常发生在模型**忠实执行图逻辑**时：`失败重试`、`拆子任务`、`再问一个 Agent`。幻觉可能**点燃**多余 hop；**堆叠几何**不论根因都会放大。

```text
Main agent    →  策略：「委派并 replan」     →  广度 / 深度
Sub-agent     →  本地工具爆发或递归        →  单跳熵压
幻觉          →  选错下一步                →  可能点火；很少是唯一原因
```

**CPL 卡的是物理，不是认识论：** PreFlight 读深度、熵、burst——不读 LLM「信不信上一句」。事实核查与幻觉缓解属于 **L5 / 应用层**；AFP 是无论认知根因如何都会触发的 **L2 刹车**。

---

## 痛点

AI Agent 不是 HTTP 客户端，而是**主动优化器**——规划、递归、拆任务、外部化成本。

| 症状 | 传统基础设施为何失效 |
|------|---------------------|
| **Planner 死循环** | LangGraph 继续跑，进程活着，没有 CrashLoop |
| **意图爆炸** | 上万内部 Task 不出网，防火墙看不见 |
| **上下文雪崩** | 内存压力在 Pod 内累积，L7 网关来不及 |
| **Argent Signaling Protocol (ASP)** | HTTP 返回 508 时，优化器早已提交 |

这不是网络问题，是**优化器治理**问题。

## 解法

在每个 Agent Pod 旁部署 **Go Sidecar**。LangGraph 节点执行前，Python SDK 通过**微秒级 UDS** 问 Sidecar：

> *"这个意图物理上安全吗？"*

不安全 → 源头 **ISOLATED**。无 OOM、无级联重试、无静默 Token 燃烧。

```text
Application intent  →  UDS PreFlightCheck  →  ALLOW | THROTTLE | ISOLATED
                              ↑
                    CRD law + gRPC injunction
```

## CPL 本质是什么？

**后果持久层（CPL）** 不是又一个网关或审批界面。它是 AFP 在**运行时边界**上执法的层——**带记忆的物理约束**，在 intent 变成不可逆动作**之前**生效。

### 三要素

| 维度 | CPL 是什么 |
|------|------------|
| **空间** | 贴在**执行边界**（Planner ↔ Sidecar）——带外，不走 HTTP/ASP 带内通道 |
| **时间** | **意图前（Pre-intent）**——工具调用、委派、出网 I/O 之前 |
| **状态** | `PERMISSIVE` · `THROTTLED` · `ISOLATED` 跨 scheduling epoch 持续，直到 FSM 恢复 |

```text
请求结束  ≠  后果清除
```

Sidecar 通过每节点唯一的 **SEA（单一执法权威）** 实现 CPL。

### 解决的本质问题

危险已搬进**优化器内部**：递归、任务爆发、上下文膨胀往往**没有流量**，却在烧 CPU、内存和 Token。TCP、HTTP、ASP 看的是**消息**，不是**优化轨迹**。按请求放行/拒绝会**遗忘**；优化器会把工作拆成无数「合法小步」来绕过。

CPL 回答一个问题：

> **谁在优化器「动手优化」之前管它？**

### 怎么工作

```text
ReportInternalState（深度、上下文字节）
        ↓
PreFlight（同步）→ EntropyMonitor → SEA + FSM
        ↓
PERMISSIVE | THROTTLED + delay | ISOLATED
```

1. **测量** — 本地物理量：递归深度、熵负载、爆发 hint（不单信自报）
2. **关卡** — 同步 PreFlight；Planner 必须等裁决
3. **记忆** — FSM 按 agent/peer 持久；隔离不会因下一次「礼貌会话」自动解除
4. **非对称恢复** — 跌落瞬时；Isolated → Probation 需 `k_isolation`（64）个 epoch，且后续 DROP **不刷新** penalty 时钟；Probation → Permissive 需 `k_probation`（128）**且** `CVP ≥ 0.8`。试探期中途熵尖峰会再隔离（防 thrashing）。

### 开放网 Gossip（最小版）

首次隔离时可向高 CVP 的 core relay 发出带签的 `TopologyWarning`。**入站：** 无签名或 ed25519 验签失败的流言当噪声丢弃；伪造签名会崖式扣减声称 reporter 的 CVP。跨节点线传输仍在加固（见 [`ROADMAP.md`](ROADMAP.md)）。

理论：[白皮书 §3 FSM](docs/whitepaper-v2/whitepaper-v2-protocol-edition.md#pre-intent-enforcement) · [§4 gossip](docs/whitepaper-v2/whitepaper-v2-protocol-edition.md#open-network-topology)

### 「防止坏 intent」在这里指什么

AFP **不判断** intent 在语义或道德上是否「坏」。它拦截的是**物理上不可持续**的优化器行为：

| 病理 | CPL 响应 |
|------|----------|
| 递归委派环（`A→D→F→A`） | 超过 `maxRecursionDepth` → **ISOLATED** |
| 意图爆发（上万内部 Task） | 熵 / burst 压力 → **THROTTLED** 或熔断 |
| 上下文雪崩逼近 OOM | 内存 + 上下文字节 → **THROTTLED** / **ISOLATED** |

摩擦在**提交之前**施加，且**后果可持久**——失控轨迹无法靠拆成语法合法的小步来逃避。

**主要对象是堆叠失控**，不是语义上的「坏 intent」。本仓库的企业协议管的是自家 planner 链。物理恶意陌生人（过载导出、传染、换马甲）属于 **AFP-Exchange**，不是本仓库的交付主张。

理论全文：[白皮书 v2 §2 CPL](docs/whitepaper-v2/whitepaper-v2-protocol-edition.md#consequence-persistence-layer) · [§3 意图前执法](docs/whitepaper-v2/whitepaper-v2-protocol-edition.md#pre-intent-enforcement)

---

## AFP 与 Argent Signaling Protocol (ASP) 的区别

**一句话：** [Argent Signaling Protocol (ASP)](https://zenodo.org/records/20674352) 及同类协议管的是 *Agent 怎么对话*；AFP 管的是 *这个意图能不能物理执行* —— 在出网之前、在 HTTP 之前、在优化器提交之前。

ASP 等应用层信令解决的是**路口交通灯**问题：

- 发现与能力交换
- 多轮会话与协作语义
- 对已暴露意图的协商

这是必要基础设施，但**不足以**保证单 Pod 内的物理安全：

| 故障模式 | ASP / 带内信令 | L7 网关 / WAF | **AFP（带外 CPL）** |
|----------|----------------|---------------|---------------------|
| Planner 无限递归 | 会话仍可能「合法」 | 尚无 HTTP 可检 | **UDS PreFlight 在下一节点拦截** |
| 万级内部 Task 爆发 | 无流量可信令 | 防火墙无包可拦 | **熵限 / 深度限，I/O 前熔断** |
| 上下文雪崩逼近 OOM | 会话 ACTIVE，探针仍绿 | QPS 限流，非 bytes×深度 | **EntropyMonitor + cgroup 感知** |
| 集群紧急钳制 | 策略变更是对话层 | 按路由改配置 | **Kill Switch + CRD 亚秒推流** |

```text
         协作语义层                    物理后果层
         ──────────                    ──────────
         ASP · A2A 协议        +       AFP Sidecar · PreFlightCheck
         （谁跟谁说什么）              （这个意图能不能跑？）
                    │                          │
                    └──── 互补，不是替代 ──────┘
```

**立场：** 信令不是敌人；把它当作*唯一*防线才是架构失误。AFP 位于应用协议**之下** —— 类似 Envoy 之于 HTTP、cgroup 之于进程：带外、微秒级、fail-closed。

详见白皮书 v2 · [完整协议版](docs/whitepaper-v2/whitepaper-v2-protocol-edition.md)

---

## 协议定位

AFP **不替代** gRPC、HTTP、Kafka、NATS、MCP 或 LangGraph。它**叠加**在你已有的传输层与框架之上。

### 常被混为一谈的三种「Agent 通信」

| 层次 | 例子 | 归属 |
|------|------|------|
| **对话（Conversation）** | 多轮 Chat、Prompt、会话状态 | LLM / 应用层 |
| **数据传递（Data passing）** | `{ task_result, confidence }` 走 gRPC、Kafka、MCP | 既有消息栈 |
| **运行时信令（Runtime signaling）** | PreFlight 裁决、GovernanceHeader 认证、持久 FSM | **AFP（L2–L4）** |

把三者统称「Agent 通信协议」是 AFP 要修正的架构误区。AFP 管的是**运行时资格与后果**，不管聊天语法，也不管业务载荷 schema。

### 企业真正要约束的是什么

AFP **不禁止** Agent A → Agent B，而是约束**不可持续的优化轨迹**：

| 允许 | 在运行时边界拦截 |
|------|------------------|
| 策略内的 Planner → A → B → C | 超过 `maxRecursionDepth` 的递归环（A → D → F → A） |
| 熵预算内的委派 | 意图爆发、上下文雪崩逼近 OOM |
| 既有传输上的对等流量 | 会话仍「合法」但物理上失控的涌现行为 |

这是**物理后果**，不是工作流审批。AFP 是**运行时边界**（Sidecar + SEA），不是工作流引擎，也不是中央编排器。

### Sidecar Mesh，而非中央网关

零信任不要求单一 choke-point 网关。AFP 采用 **Service Mesh 模式**：

```text
Agent  →  AFP Sidecar  →  mTLS  →  AFP Sidecar  →  Agent
```

信任 enforcement 在 **Sidecar**（本地 PreFlight、入站 GovernanceHeader）——与每 Pod 旁 Envoy 同一问责模型。中央网关与 Sidecar Mesh 都可以是零信任；AFP 选择**每节点执法**。

**开放 profile 附加：** 入站陌生人税 + CVP 地板；带签拓扑流言（验签失败即丢）。CVP 仍是**每个 Sidecar 的本地账本**——多副本不共享全局后果存储。

### 三个 Plane（勿混用）

| Plane | 例子 | AFP 角色 |
|-------|------|----------|
| **Agent 控制面** | Planner、LangGraph DAG、工具图 | L1 可观测；AFP **不规定** |
| **数据面** | 载荷、缓存、流式 | gRPC/Kafka/MCP 承载；**超出范围** |
| **协调面** | 资格、后果、摩擦、依赖信任 | **L2–L4 核心** — CPL、SEA、CVP、Policy Surface |

> *说明：* K8s 文档里的「control plane」指 Operator / Policy Controller（L3），是**策略管理**，不是 Agent 的 Planner 控制面。

### AFP 回答的问题

企业已有成熟的**字节怎么传**。AFP 回答一个更小、更易落地的问题：

> **当 Agent 已经能够通信时，如何让每次协调具有一致的运行时语义、且后果可持久？**

传输负责送达。AFP 负责**提交之前的治理**。

---

## 10 分钟快速起步

### 前置条件

- Docker、[kind](https://kind.sigs.k8s.io/) 或 Minikube
- `kubectl`、`make`

### 一行命令

```bash
git clone https://github.com/FilthyMudblood/aegis-fabric.git && cd aegis-fabric
make kind-quickstart
```

构建完整栈（sidecar · operator · policy-controller · demo-agent），加载到 kind，apply 清单并运行拦截演示。

### 买家秀（Aha! Moment）

```bash
kubectl -n afp-system logs -f deploy/afp-agent-node -c agent-core
```

**几秒内应看到：**

```text
afp-demo-agent: waiting for sidecar IPC at /var/run/afp/agent.sock
afp-demo-agent: sidecar socket ready
--- langgraph planner demo (initial_depth=10) ---
[AFP SDK] LangGraph node blocked: afp-core: recursion depth exceeded physical limit, intent loop detected
annotated-stop: afp-core: recursion depth exceeded physical limit, intent loop detected
```

| 日志 | 含义 |
|------|------|
| `socket ready` | `emptyDir` UDS — Python 与 Go Sidecar 微秒级 IPC，无 TCP |
| `node blocked` | 触碰 `maxRecursionDepth: 10`，意图在摇篮里被掐灭 |
| `annotated-stop` | 优雅降级，无崩溃、无 OOM |

### 亚秒级控制面

**A — 改 CRD（声明式，<1s 推流）：**

```bash
kubectl patch afpclusterpolicy enterprise-default --type merge \
  -p '{"spec":{"maxRecursionDepth":5,"entropyLimit":0.80}}'
kubectl -n afp-system logs -f deploy/afp-agent-node -c afp-sidecar
```

**B — Kill Switch（运维指令）：**

```bash
kubectl -n afp-system port-forward svc/afp-policy-controller 8090:8090 &
go run ./cmd/controlplane/policyctl --controller 127.0.0.1:8090 --kill-switch
go run ./cmd/controlplane/policyctl --controller 127.0.0.1:8090 --clear
```

---

## 架构一览

### 三层联邦

| 层 | 职责 |
|----|------|
| **L1 应用层** | `@afp_governed_node` 治理意图 |
| **L2 数据面** | UDS PreFlightCheck · EntropyMonitor |
| **L3 控制面** | CRD · Operator · Policy Controller |

### 双源合并策略模型

```text
Base Layer（法律）          Overlay Layer（指令）
CRD → Operator → ConfigMap    gRPC StreamPolicyUpdates
     → fsnotify (~60s)              ↓
                            Kill Switch / 紧急钳制
                    ─────────────────────────────
                    Controller 宕机 → ConfigMap 兜底
```

| 来源 | 延迟 | 用途 |
|------|------|------|
| **Base** | ~60s | 持久化真源，Fail-Safe 底线 |
| **Overlay** | 亚秒级 | CRD 推流、Kill Switch、事故响应 |

安全：Sidecar 用 **ServiceAccount Token** + `TokenReview` 校验。

---

## GHCR 镜像

`main` 分支 push 自动发布：

```bash
docker pull ghcr.io/filthymudblood/aegis-fabric-sidecar:latest
docker pull ghcr.io/filthymudblood/aegis-fabric-operator:latest
docker pull ghcr.io/filthymudblood/afp-demo-agent:latest
```

---

## 企业运维要点

- **`AFP_SDK_FAIL_MODE=closed`** — 生产默认，Sidecar 不可达时停止意图生成
- **`entropyLimit`** — 默认 0.95，预防性熵压熔断
- **`on_quota_exceeded="annotate"`** — LangGraph 优雅降级，写入 `afp_blocked`

详见 [`deploy/kubernetes/README.md`](deploy/kubernetes/README.md)

---

## 实证

Monte Carlo：**1,000 × 500 节点 × 5% 恶意** — Baseline 存活 0.4% vs AFP **100%**

```bash
go run ./cmd/demo/simulator
```

---

## 状态

| 阶段 | 交付 |
|------|------|
| **Phase 1** | 数据面 · SDK · LangGraph · K8s 伴生 · CRD Operator · demo-agent |
| **Phase 2** | 策略推流 · Operator 桥接 · TokenReview · revision 续传 · **mTLS** · **状态回写** · **删除传播** · GHCR CI |

**代码冻结于 PR-6c。** 生产加固：[`ROADMAP.md`](ROADMAP.md) · 理论：[白皮书 v2.0 协议版](docs/whitepaper-v2/whitepaper-v2-protocol-edition.md) · v1 存档：[Zenodo](https://zenodo.org/records/20674352)

---

## 许可证

Apache License 2.0 — 详见 [LICENSE](LICENSE)。
