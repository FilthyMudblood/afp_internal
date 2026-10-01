# Aegis Fabric Protocol (AFP)

[![Docker Publish](https://github.com/FilthyMudblood/aegis-fabric/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/FilthyMudblood/aegis-fabric/actions/workflows/docker-publish.yml)
[![GHCR Sidecar](https://img.shields.io/badge/GHCR-sidecar-2496ED?logo=docker&logoColor=white)](https://ghcr.io/filthymudblood/aegis-fabric-sidecar)
[![GHCR Operator](https://img.shields.io/badge/GHCR-operator-2496ED?logo=docker&logoColor=white)](https://ghcr.io/filthymudblood/aegis-fabric-operator)
[![GHCR Demo Agent](https://img.shields.io/badge/GHCR-demo--agent-2496ED?logo=docker&logoColor=white)](https://ghcr.io/filthymudblood/afp-demo-agent)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

### **企业级 Agent 网络的物理刹车片**

> **"TCP 治理数据包，AFP 治理优化器"**
> *(TCP governs packets. AFP governs optimizers.)*

**Aegis Fabric Protocol（AFP）** 是 Kubernetes 原生的**后果持久层（CPL）**——伴生 Sidecar 在意图变成不可逆网络 I/O **之前**，掐灭规划死循环、意图爆发与递归委派风暴。

English · [`README.md`](README.md) · 白皮书 · **[v2 协议版](docs/whitepaper-v2/)** · [v1 Zenodo 存档](https://zenodo.org/records/20674352)

关于这一执行边界设计的一篇短文曾投往 NeurIPS 2026 Agents in the Wild，未被录用。审稿要求用真实 planner/tool 轨迹代替合成网格；这部分工作正在本仓库推进。

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

详见白皮书 v2 · [第 1 章 — 优化危机](docs/whitepaper-v2/chapter-01-optimization-crisis.md) · [完整目录](docs/whitepaper-v2/)

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

**代码冻结于 PR-6c。** 生产加固：[`ROADMAP.md`](ROADMAP.md) · 理论：[白皮书 v2.0 协议版](docs/whitepaper-v2/) · v1 存档：[Zenodo](https://zenodo.org/records/20674352)

---

## 许可证

Apache License 2.0 — 详见 [LICENSE](LICENSE)。
