# 主/子 Agent 架构总结

## 核心原则

> **看板的任务管理，状态通知都必须由主 agent 来完成，而看板上的任务执行及状态反馈由子 agent 来完成**

## 角色职责

### Main Agent (主 Agent)

| 职责 | 实现位置 | 说明 |
|------|---------|------|
| **任务管理** | `AgentOrchestrator` | 监控看板、检测待处理任务 |
| **状态通知** | `KanbanService.PublishTaskEvent()` | 发布任务创建/更新/完成事件 |
| **Sub-Agent 生命周期** | `spawnAgentForZone()`, `ReleaseAllAgents()` | 孵化和释放子 agent |
| **依赖管理** | `board.GetReadyTasks()` | 维护 DAG/Workflow 依赖关系 |
| **事件协调** | `OnTaskCompleted()` | 接收子 agent 完成通知 |

### Sub-Agent (子 Agent)

| 职责 | 实现位置 | 说明 |
|------|---------|------|
| **任务执行** | `AgentWorker.executeTask()` | 执行具体任务逻辑 |
| **状态反馈** | `service.UpdateTaskStatusWithEvent()` | 通过 KanbanService 报告状态 |
| **依赖感知调度** | `fetchNextPendingTask()` → `GetReadyTasks()` | 只获取依赖已满足的任务 |
| **并发控制** | `maxConcurrency` | 按配置控制并发执行数 |

## 数据流

```
┌─────────────────┐
│   Main Agent    │
│ (Orchestrator)  │
└────────┬────────┘
         │ 1. 监控到 pending 任务
         ▼
┌─────────────────┐
│ Spawn Sub-Agent │
└────────┬────────┘
         │ 2. 启动
         ▼
┌─────────────────┐
│  Sub-Agent      │
│ (AgentWorker)   │
└────────┬────────┘
         │ 3. GetReadyTasks() - 检查依赖
         │ 4. 执行任务
         ▼
┌─────────────────┐
│ UpdateTaskStatus│
│ WithEvent()     │ ──► 5. 发布事件到 MessageBus
└────────┬────────┘                │
         │                         ▼
         │             ┌───────────────────┐
         │             │  Main Agent       │
         └────────────►│  OnTaskCompleted()│
                       └───────────────────┘
```

## 关键代码变更

### 1. board.go - DAG/Workflow 支持

```go
// Task 新增依赖字段
type Task struct {
    // ...
    Dependencies []TaskDependency `json:"dependencies,omitempty"`
    DependsOn    []string         `json:"depends_on,omitempty"`
}

// 依赖检查
func (k *KanbanBoard) AreTaskDependenciesSatisfied(zoneID, taskID string) bool
func (k *KanbanBoard) GetReadyTasks(zoneID string) []*Task
func (k *KanbanBoard) GetTaskExecutionOrder(zoneID string) ([]string, error)
```

### 2. orchestrator.go - Main Agent 职责

```go
type AgentOrchestrator struct {
    mainAgentID string  // 明确标识主 agent
    // ...
}

// 主 agent 孵化子 agent
func (o *AgentOrchestrator) spawnAgentForZone(zoneID, agentType string) error

// 主 agent 接收子 agent 完成通知
func (o *AgentOrchestrator) OnTaskCompleted(zoneID, taskID string)
```

### 3. agent_worker.go - Sub-Agent 职责

```go
// Sub-Agent 只获取就绪任务（依赖已满足）
func (w *AgentWorker) fetchNextPendingTask() *Task {
    tasks := w.board.GetReadyTasks(w.zoneID)
    // ...
}

// Sub-Agent 通过 service 反馈状态
func (w *AgentWorker) executeTask(task *Task, workerNum int) {
    // 执行任务
    result, err := w.runTaskExecution(task)
    
    // 反馈状态到主 agent
    w.service.UpdateTaskStatusWithEvent(w.zoneID, task.ID, status, result, err)
}
```

## 文件统计

| 文件 | 行数 | 主要功能 |
|------|------|---------|
| `board.go` | ~500 | 看板数据结构 + DAG 依赖管理 |
| `orchestrator.go` | ~280 | Main Agent 编排逻辑 |
| `agent_worker.go` | ~300 | Sub-Agent 任务执行 |
| `service.go` | ~260 | 事件发布 + WebSocket |
| **总计** | **~4800** | 完整 kanban 系统 |

## 测试覆盖

- `board_test.go`: 依赖检查、拓扑排序
- `orchestrator_test.go`: Main/Sub-Agent 交互
- `agent_worker_test.go`: 依赖感知调度
- `service_test.go`: 事件发布

## 使用示例

```go
// 1. 创建看板（指定主 agent）
board := NewKanbanBoard("board-1", "项目看板", "main-agent-001")

// 2. 创建 zone
board.CreateZone("dev", "开发区", "开发任务", "developer")

// 3. 添加带依赖的任务
board.AddTask("dev", "task-1", "设计", "...", 1, nil)
board.AddTask("dev", "task-2", "实现", "...", 2, nil, 
    DependsOn: []string{"task-1"})

// 4. 主 agent 启动编排器
orchestrator := NewAgentOrchestrator(board, registry, msgBus, cfg, provider)
go orchestrator.MonitorBoard(ctx)

// 5. 子 agent 自动孵化并执行任务
// - 检测到 task-1 pending → 孵化 sub-agent
// - sub-agent 执行 task-1 → 反馈完成
// - task-2 依赖满足 → sub-agent 执行 task-2
```

## 优势

1. **职责分离**: 主 agent 专注管理和协调，子 agent 专注执行
2. **依赖安全**: DAG 确保任务按正确顺序执行
3. **可扩展**: 支持多个 zone 并行，每个 zone 独立 sub-agent
4. **可观测**: 所有状态变更通过事件总线，支持 WebSocket 实时推送
