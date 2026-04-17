# 严格约束下的任务管理架构

## 核心原则

**严格遵守以下约束：**
1. **只有 Cron 和 Channel 能对主 Agent 下发任务**
2. **主 Agent 能根据任务需要同时通过多个 Channel 反馈结果**
3. **看板上的任务执行及状态反馈由子 Agent 完成**
4. **任务之间可以有 DAG/Workflow 依赖关系**

## 架构图

```
┌─────────────────┐      ┌──────────────────┐
│     Cron        │      │     Channel      │
│   (定时任务)     │      │   (用户命令)      │
└────────┬────────┘      └────────┬─────────┘
         │                        │
         │  创建任务               │ 创建任务
         ▼                        ▼
┌─────────────────────────────────────────────────┐
│              Main Agent (主 Agent)               │
│  ┌─────────────────────────────────────────┐    │
│  │        AgentOrchestrator                │    │
│  │  - MonitorBoard() 监控待处理任务          │    │
│  │  - spawnAgentForZone() 孵化子 Agent       │    │
│  │  - OnTaskCompleted() 接收完成通知         │    │
│  │  - PublishTaskEvent() 发布状态通知        │    │
│  └─────────────────────────────────────────┘    │
│                      │                          │
│                      ▼                          │
│  ┌─────────────────────────────────────────┐    │
│  │           KanbanBoard                   │    │
│  │  - Zones (分区)                          │    │
│  │  - Tasks with DAG dependencies          │    │
│  │  - Per-zone locks                       │    │
│  └─────────────────────────────────────────┘    │
└─────────────────────────────────────────────────┘
                      │
                      │ 分配任务
                      ▼
┌─────────────────────────────────────────────────┐
│            Sub-Agents (子 Agents)                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────┐ │
│  │ AgentWorker │  │ AgentWorker │  │   ...   │ │
│  │  Zone A     │  │  Zone B     │  │         │ │
│  │             │  │             │  │         │ │
│  │ - executeTask() 执行任务      │  │         │ │
│  │ - UpdateTaskStatusWithEvent() │  │         │ │
│  │   反馈状态到主 Agent          │  │         │ │
│  └─────────────┘  └─────────────┘  └─────────┘ │
└─────────────────────────────────────────────────┘
                      │
                      │ 状态更新事件
                      ▼
┌─────────────────────────────────────────────────┐
│         MessageBus (事件总线)                    │
│  - kanban.events                                │
│  - channel.broadcast                            │
└─────────────────────────────────────────────────┘
                      │
                      │ 广播状态通知
                      ▼
         ┌────────────────────────┐
         │   Multiple Channels    │
         │  (Feishu/Slack/etc.)   │
         │  向用户反馈任务状态      │
         └────────────────────────┘
```

## 职责划分

### Main Agent (主 Agent) 职责

**唯一入口：**
- ✅ 通过 `CronKanbanIntegration` 接收 Cron 定时任务
- ✅ 通过 `Channel Integration` 接收用户命令任务

**核心管理职能：**
1. **任务管理**
   - `MonitorBoard()` - 持续监控看板待处理任务
   - `checkAndSpawnAgents()` - 检查并孵化子 Agent
   - 维护任务队列和优先级

2. **子 Agent 生命周期管理**
   - `spawnAgentForZone()` - 为特定 Zone 孵化子 Agent
   - `ReleaseAllAgents()` - 释放所有子 Agent
   - `OnTaskCompleted()` - 接收子 Agent 完成通知

3. **状态通知**
   - `PublishTaskEvent()` - 发布所有任务事件
   - 通过 MessageBus 广播到多个 Channel
   - WebSocket 实时推送

**严禁：**
- ❌ 直接执行具体任务
- ❌ 直接调用 LLM Provider
- ❌ 绕过 Cron/Channel 创建任务

### Sub-Agent (子 Agent) 职责

**唯一入口：**
- ✅ 由 Main Agent 的 `AgentOrchestrator` 孵化

**核心执行职能：**
1. **任务执行**
   - `executeTask()` - 执行具体任务逻辑
   - `runTaskExecution()` - 调用 LLM Provider
   - 遵守 DAG 依赖关系

2. **状态反馈**
   - `UpdateTaskStatusWithEvent()` - 报告任务状态
   - 通过 `KanbanService` 反馈给 Main Agent
   - 不直接发布事件

**严禁：**
- ❌ 直接创建任务
- ❌ 直接发布状态事件
- ❌ 绕过 Main Agent 协调

## 数据流

### 任务创建流程（仅 Cron/Channel）

```
Cron Job / Channel Command
         │
         ▼
┌─────────────────┐
│ CreateTaskWithEvent() │ ← 唯一入口
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  board.AddTask() │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ PublishTaskEvent() │ → MessageBus → Channels
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ MonitorBoard()  │ ← Main Agent 发现新任务
└─────────────────┘
```

### 任务执行流程（Sub-Agent）

```
Main Agent 孵化 Sub-Agent
         │
         ▼
┌─────────────────┐
│ fetchNextPendingTask() │ ← 检查 DAG 依赖
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  executeTask()  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ UpdateTaskStatusWithEvent() │
│   - TaskRunning             │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ runTaskExecution() │ → LLM Provider
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ UpdateTaskStatusWithEvent() │
│   - TaskCompleted/Failed    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ OnTaskCompleted() │ ← Main Agent 接收通知
└─────────────────┘
```

### 状态通知流程（多 Channel 广播）

```
Sub-Agent 更新状态
         │
         ▼
┌─────────────────┐
│ PublishTaskEvent() │ ← Main Agent 发布
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   MessageBus    │
│ kanban.events   │
└────────┬────────┘
         │
         ├──────────────┬──────────────┐
         ▼              ▼              ▼
┌─────────────┐ ┌─────────────┐ ┌─────────────┐
│   Feishu    │ │    Slack    │ │  Telegram   │
│  Channel    │ │   Channel   │ │   Channel   │
└─────────────┘ └─────────────┘ └─────────────┘
         │              │              │
         └──────────────┴──────────────┘
                        │
                        ▼
              用户收到状态通知
```

## DAG/Workflow 支持

### 依赖类型

1. **简单依赖** (`DependsOn`)
   ```go
   task.DependsOn = []string{"task-1", "task-2"}
   // 默认要求依赖任务状态为 completed
   ```

2. **详细依赖** (`Dependencies`)
   ```go
   task.Dependencies = []TaskDependency{
       {TaskID: "task-1", RequiredStatus: TaskCompleted},
       {TaskID: "task-2", RequiredStatus: TaskRunning},
   }
   ```

### 依赖检查流程

```
Sub-Agent 获取任务
         │
         ▼
┌─────────────────┐
│ GetReadyTasks() │ ← 仅返回 pending + 依赖满足的任务
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ AreTaskDependenciesSatisfied() │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
   Yes       No
    │         │
    ▼         ▼
  可执行    等待依赖
```

### 拓扑排序

```go
// 获取任务的正确执行顺序
order, err := board.GetTaskExecutionOrder(zoneID)
// 使用 Kahn 算法检测循环依赖
// 如果存在循环依赖，返回 ErrCircularDependency
```

## 代码实现要点

### 1. 任务创建限制

**✅ 允许：**
```go
// cron_integration.go - Cron 创建任务
func (i *CronKanbanIntegration) handleCronJob(job *cron.CronJob) {
    task, err := i.service.CreateTaskWithEvent(...)
}

// channels/integration.go - Channel 创建任务  
func (h *ChannelHandler) OnCommand(cmd *Command) {
    task, err := kanbanService.CreateTaskWithEvent(...)
}
```

**❌ 禁止：**
```go
// 任何其他地方直接调用 board.AddTask()
// HTTP API 提供任务创建端点
// Sub-Agent 创建新任务
```

### 2. 状态发布限制

**✅ 允许：**
```go
// orchestrator.go - Main Agent 发布事件
func (o *AgentOrchestrator) OnTaskCompleted(zoneID, taskID string) {
    // 记录日志，协调子 Agent 释放
}

// service.go - 统一发布入口
func (s *KanbanService) UpdateTaskStatusWithEvent(...) {
    s.board.UpdateTaskStatus(...)
    s.PublishTaskEvent(...) // ← 唯一发布点
}
```

**❌ 禁止：**
```go
// Sub-Agent 直接调用 msgBus.Publish()
// 绕过 KanbanService 直接更新 board
```

### 3. 多 Channel 反馈

```go
// service.go - StartStatusReporter
func (s *KanbanService) StartStatusReporter(ctx context.Context) {
    handler := func(msg string) {
        // 发布到 message bus
        s.msgBus.Publish("channel.broadcast", report)
    }
    s.SubscribeToEvents(handler)
}

// channels/manager.go - 监听广播
func (m *Manager) Start() {
    m.msgBus.Subscribe("channel.broadcast", func(msg string) {
        // 发送到所有活跃的 channel
        for _, ch := range m.activeChannels {
            ch.SendMessage(...)
        }
    })
}
```

## 验证清单

### 代码审查要点

- [ ] 确认 `board.AddTask()` 只在 `service.CreateTaskWithEvent()` 中调用
- [ ] 确认 `CreateTaskWithEvent()` 只在 `cron_integration.go` 和 `channels/*` 中调用
- [ ] 确认 `PublishTaskEvent()` 只在 `service.go` 中调用
- [ ] 确认 Sub-Agent 只通过 `UpdateTaskStatusWithEvent()` 反馈状态
- [ ] 确认 HTTP API 只提供读操作，无任务创建端点
- [ ] 确认 `MessageBus` 的 `kanban.events` 主题只由 Main Agent 发布
- [ ] 确认 `channel.broadcast` 主题被所有活跃 Channel 订阅

### 运行时验证

```bash
# 1. 验证 Cron 任务创建
curl -X POST http://localhost:8080/cron/jobs \
  -d '{"schedule":"*/5 * * * *","payload":{"zone_id":"default","title":"Test"}}'

# 2. 验证 Channel 命令
# 在 Feishu/Slack 中发送：/kanban create task "Test Task"

# 3. 验证 HTTP 只读
curl http://localhost:8080/kanban  # ✓ 应该工作
curl -X POST http://localhost:8080/kanban/tasks \
  -d '{"zone_id":"default","title":"Test"}'  # ✗ 应该返回 405

# 4. 验证多 Channel 通知
# 观察 Feishu、Slack、Telegram 都收到状态更新
```

## 文件清单

### 核心实现
- `pkg/kanban/orchestrator.go` - Main Agent 编排器
- `pkg/kanban/agent_worker.go` - Sub-Agent 执行器
- `pkg/kanban/board.go` - 看板与 DAG 支持
- `pkg/kanban/service.go` - 统一服务层
- `pkg/kanban/cron_integration.go` - Cron 集成
- `pkg/channels/manager.go` - Channel 管理
- `pkg/channels/integration.go` - Channel-Kanban 集成 (需实现)

### 测试
- `pkg/kanban/orchestrator_test.go`
- `pkg/kanban/agent_worker_test.go`
- `pkg/kanban/board_test.go`
- `pkg/kanban/service_test.go`
- `pkg/kanban/cron_integration_test.go`

## 总结

本架构严格遵守以下约束：

1. **任务创建单一入口**：仅 Cron 和 Channel 可以创建任务
2. **Main/Sub 职责分离**：Main Agent 管理协调，Sub-Agent 专注执行
3. **状态通知集中化**：所有事件由 Main Agent 统一发布
4. **多 Channel 广播**：支持同时向多个渠道反馈结果
5. **DAG/Workflow 支持**：任务间可有复杂依赖关系

任何违反这些约束的代码都应被视为 Bug 并立即修复。
