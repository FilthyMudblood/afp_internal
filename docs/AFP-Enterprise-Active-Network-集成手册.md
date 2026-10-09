# AFP-Enterprise 集成手册（Active Network）

面向：Active Network 开发团队  
版本：2026-10-08  
范围：本地规划器 + MCP / HTTP / 内部工具。工具真正执行前加闸。

---

## 1. 先看这一页

AFP-Enterprise 不是新规划器，也不是新 MCP。

它是规划器旁边的一台 **sidecar**。规划器在发出工具调用之前问它一句：

> 这一步能不能跑？

三种回答：

| 回执 | 规划器该做什么 |
|------|----------------|
| 放行 | 照常调工具 |
| 延迟 | 先睡 `delay_ms`，再继续 |
| 隔离 | **不要**发这次 I/O，停图或标注阻断，禁止无治理重试 |

你们现有的 LangGraph / MCP / HTTP 工具都留着。只在「即将出网或即将打本地工具」这一跳前面加两行：上报状态 → 预检。

```text
规划器 / 工具节点
        │  1. 上报深度、上下文字节
        │  2. PreFlightCheck（突发提示）
        ▼
   AFP sidecar（主机内核）
        │
        ▼
   放行 / 延迟 / 隔离
        │
        ▼
   真正的 lookup / HTTP / MCP
```

内核按顺序看四件事，命中就返回：紧急停机 → 深度闸（默认 >10）→ 压力硬隔离（≥0.95）→ 隔离–恢复机。Active Network 第一期只需要接上前三段；恢复机写在规格里，不要当已经验过。

---

## 2. 接入原则

1. **闸在工具之前，不在工具里面。** 不要去改 MCP server 或 HTTP handler 的业务逻辑。
2. **先写后探。** 每次 hop 先 `ReportInternalState`，再 `PreFlightCheck`。只探不写，深度闸会看错旧值。
3. **一台规划器进程配一台 sidecar。** 两台规划器共用一台 sidecar 会互相覆盖深度。这是已知行为，不是 bug。
4. **隔离 ≠ 抛到上层去重试。** 隔离后必须停，或把 `afp_blocked` 写进图状态后走结束边。
5. **不要解析 prompt。** sidecar 不读自然语言、不读 tool schema。它只看深度、并发、内存、上下文体积、即将扇出的任务数。

---

## 3. 部署 sidecar

开发机最小启动（仓库根目录）：

```bash
mkdir -p /tmp/afp
export AFP_IPC_SOCKET=/tmp/afp/agent.sock
go run ./cmd/sidecar
```

规划器进程必须能看见同一个套接字：

```bash
export AFP_IPC_SOCKET=/tmp/afp/agent.sock
export PYTHONPATH=sdk/python
```

常用环境变量：

| 变量 | 默认 | 含义 |
|------|------|------|
| `AFP_IPC_SOCKET` | `/var/run/afp/agent.sock` | 规划器 ↔ sidecar 的 Unix 套接字 |
| `AFP_MAX_RECURSION_DEPTH` | `10` | 上报深度大于此值立刻隔离 |
| `AFP_ENTROPY_LIMIT` | `0.95` | 四轴压力最大值达到此线硬隔离 |
| `AFP_MAX_CONTEXT_BYTES` | `512MiB` | 上报上下文字节的分母 |
| `AFP_SDK_FAIL_MODE` | `open` | sidecar 挂了：`open` 放行，`closed` 拒绝 |
| `AFP_SDK_RPC_TIMEOUT_MS` | `50` | 预检 RPC 超时 |

生产形态：每个规划器 Pod 一个 sidecar 容器，共享 volume 上的 socket。不要全公司共用一个 sidecar。

---

## 4. Python SDK

包在 `sdk/python/afp_sdk/`。规划器服务加：

```bash
export PYTHONPATH=sdk/python
# 或把 sdk/python 装进你们的 venv
```

导入：

```python
from afp_sdk import AFPSidecarClient, AFPQuotaExceededError
from afp_sdk.langgraph import afp_governed_node
from afp_sdk.tools import afp_governed
```

### 4.1 已有 LangGraph：包一层节点

这是 Active Network 最常见的接入方式。

```python
from afp_sdk.langgraph import afp_governed_node

@afp_governed_node(estimated_tasks=1, on_quota_exceeded="annotate")
def tool_node(state: dict) -> dict:
    # 这里才是原来的 lookup / HTTP / MCP
    return call_your_tool(state)
```

装饰器会自动：

1. 从图状态读 `recursion_depth`（没有就当 0）
2. 读 `context_memory_bytes`；没有则按 `messages` 内容估字节
3. `report_state` → `pre_flight_check`
4. 隔离时：`annotate` 把 `afp_blocked=True` 和原因写回 state；`raise` 则抛 `AFPQuotaExceededError`

图上加一条结束边：

```python
def should_continue(state: dict) -> str:
    if state.get("afp_blocked"):
        return "stop"
    return "next"
```

已经编好的图，不想改每个节点定义，可以事后包：

```python
from afp_sdk.langgraph import govern_state_graph_nodes

govern_state_graph_nodes(graph, node_names=["tools", "planner"])
```

规划器自己要维护深度。每进一层 +1，新任务从 0 开始：

```python
state["recursion_depth"] = state.get("recursion_depth", 0) + 1
```

### 4.2 普通工具函数 / MCP 客户端

没有 LangGraph 时，包在真正 I/O 前：

```python
from afp_sdk.tools import afp_governed

@afp_governed(estimated_tasks=1)
def http_get(url: str) -> bytes:
    return your_http_client.get(url)
```

注意：`@afp_governed` **只探不写**。深度闸要生效，调用前仍须：

```python
client = AFPSidecarClient()
client.report_state(
    current_recursion_depth=depth,
    context_memory_bytes=ctx_bytes,
)
result = http_get(url)  # 装饰器内部会 PreFlightCheck
```

一次 `expand` 即将扇出 N 个 GET 时，**在 expand 本身**把 `estimated_tasks=N` 传进去，不要等 GET 已经发出去。N=48 时压力是 `48/50=0.96`，会硬隔离，48 个 GET 一张都不会发。

### 4.3 手写两行（最小）

```python
from afp_sdk import AFPSidecarClient, AFPQuotaExceededError

afp = AFPSidecarClient()
afp.report_state(current_recursion_depth=depth, context_memory_bytes=ctx_bytes)
try:
    afp.pre_flight_check(estimated_tasks=1, trace_id=trace_id)
except AFPQuotaExceededError as exc:
    stop_graph(reason=str(exc))
    return
do_tool_io()
```

`pre_flight_check` 在延迟回执时会 **自己 sleep**，返回后即可继续。隔离才抛异常。

---

## 5. 三种回执：规划器必须遵守

| sidecar 动作 | SDK 行为 | 你们必须做 |
|--------------|----------|------------|
| `PERMISSIVE` | 返回 `True` | 调工具 |
| `THROTTLED` | sleep `delay_ms` 后返回 `True` | 不要自己再 sleep 一遍；不要跳过 sleep |
| `ISOLATED` | 抛 `AFPQuotaExceededError` | 停 I/O。用 `annotate` 或捕获后结束图 |

禁止：

- 隔离后立刻换一条 MCP 路径再打（绕闸）
- 隔离后当普通异常交给框架无限 retry
- 预检失败（socket 挂了）时在 `fail_mode=open` 下以为「闸已经生效」——那是放行降级

---

## 6. 必须上报的三个数

sidecar 不读代码。它只信你们报的数。

| 字段 | 谁填 | 怎么填 |
|------|------|--------|
| `current_recursion_depth` | 规划器 | 当前图深度。新 job 从 0 起。死循环会涨到 11 被闸 |
| `context_memory_bytes` | 规划器 | 当前上下文体积。有 messages 就按内容估 |
| `estimated_tasks` | 即将扇出的那一跳 | 马上要发出的工具数。单次调用填 1；expand 填即将排队的 GET 数 |

漏报深度：深度闸失灵，旧深度还可能误杀下一个无关任务。  
漏报突发：48 扇出会直接打出去。  
报了但不在工具前探：等于没接。

---

## 7. 一台规划器，一台 sidecar

状态挂在 **主机 sidecar** 上，不是挂在某个 agent id 上。当前实现会忽略对端身份字段。

| 布局 | 结果 |
|------|------|
| 规划器 A、B 各一台 sidecar | A 隔离不影响 B |
| A、B 共用一台 sidecar | B 上报深度 0 会盖掉 A 留下的 11，B 会被放行 |

Active Network 若多个 planner 进程同机：每个进程一个 socket 路径，或每个 Pod 一个 sidecar。不要图省事共用。

隔离之后，深度不会因为「这次请求结束了」而清掉。必须有一次新的 `report_state(depth=0)` 才会放行。这是设计，不是泄漏。

---

## 8. 默认阈值（先不要改）

| 项 | 值 | 第一期会不会碰到 |
|----|----|------------------|
| 深度闸 | > 10 隔离 | 会。循环规划会打到 |
| 工具并发分母 | 50 | 突发提示用 |
| 硬隔离 | max(工具, 内存, 上下文, 突发) ≥ 0.95 | 会。48/50=0.96 |
| 告警延迟 | ≥ 0.75 时 500–2000 ms | 第一期 traces 通常走不到 |
| 安全带 | 0.40 | 恢复机用，第一期走不到 |
| 隔离驻留 / 观察期 | 64 / 128 epoch | 规格，第一期未验收 |

第一期验收只要求：有界任务能做完、死循环和 48 突发能停、误杀为 0。不要把延迟带和恢复机写进「已上线能力」。

---

## 9. sidecar 挂了怎么办

`AFP_SDK_FAIL_MODE`：

| 值 | 行为 | 什么时候用 |
|----|------|------------|
| `open`（默认） | IPC 失败则放行，打日志 | 开发、灰度。闸挂了业务还能跑 |
| `closed` | IPC 失败抛 `AFPInfrastructureError` | 生产若认为「宁停勿跑」 |

灰度建议：先 `open`，看日志里有没有 IPC 失败；稳定后再考虑关键链路 `closed`。

---

## 10. 接入验收（一条非核心链路即可）

不要一上来挂全网。选一条 **非核心** 规划器链路，开关对照。

准备：

1. 该规划器单独 sidecar  
2. 节点或工具已包 `afp_governed_node` / `afp_governed`  
3. 深度会随 hop +1，新任务从 0 起  
4. 扇出点传入真实 `estimated_tasks`

必看四件事：

| 检查 | 通过标准 |
|------|----------|
| 有界任务 | 短查询、正常多 hop 能做完，误杀 = 0 |
| 死循环 | 深度到 11 停，不要堆到框架 `recursion_limit` 才爆 |
| 突发 | expand 48 时 0 个 GET 出去 |
| 残留 | 隔离后不改深度再探仍拒绝；新任务报 0 能做 |

不合格就拆闸，不要开到核心支付 / 核心检索。

---

## 11. 常见坑

**只包了工具，没包规划节点。** 规划空转加深度，工具还没调就已经该隔离。两个都要在 I/O 前探。

**`estimated_tasks` 永远填 1。** expand 打出去 48 个 GET 时闸看不见风暴。

**多规划器共用 socket。** 上线第二天出现「A 隔离了 B 却突然好了」——是 B 把深度写成 0。

**隔离当 retry。** LangGraph / 你们自己的重试器会把隔离变成更狠的风暴。结束边或 annotate。

**以为接了 MCP 网关就等于接了 AFP。** MCP 管「能不能说」；AFP 管「这一跳能不能执行」。两层都要，顺序是：MCP/权限通过之后、真正 I/O 之前问 sidecar。

**把 500 节点仿真或 0.5B 小模型当生产证据。** 那是附录。Active Network 只信你们自己链路上的四项验收。

---

## 12. 不要对业务方这么说

不要说：AFP 已经防止资源耗尽、已经按身份隔离、已经验证 LLM agent、已经是生产安全。

可以说：

> 在这条规划器链路上，工具执行前会问 sidecar。测过的正常任务能做完；测过的死循环和突发会停。闸记住上次上报的深度，直到下一次上报覆盖。

---

## 13. 给排期的最小切片

| 顺序 | 工作 | 完成定义 |
|------|------|----------|
| D1 | 开发机拉起 sidecar + 一条 demo 图 | socket 通，放行能调工具 |
| D2 | 包 Active Network 一条非核心 LangGraph 工具节点 | 有界任务误杀 0 |
| D3 | 深度 +1 与新任务报 0 | 循环能停，新任务能做 |
| D4 | 扇出点传 `estimated_tasks` | 大 expand 被拦在工具前 |
| D5 | 每规划器独立 sidecar | 互不覆盖 |
| D6 | 日志：回执、原因、depth、estimated_tasks | 出事能对照 |

D2 不过，不要做 D5。

---

## 14. 对接人清单

规划器同学：图状态里的 `recursion_depth`、结束边、禁止隔离后重试。  
工具同学：HTTP/MCP 客户端只在 I/O 前探；expand 传突发数。  
平台同学：每 Pod 一个 sidecar、socket 权限、`FAIL_MODE`、指标。  
不要让业务同学去改 sidecar 内核。
