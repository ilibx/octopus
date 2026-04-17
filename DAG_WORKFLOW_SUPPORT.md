# DAG/Workflow 任务依赖支持

## 概述

根据需求：**"看板的任务管理，状态通知都必须由主 agent 来完成，而看板上的任务执行及状态反馈由子 agent 来完成"**，本次优化实现了完整的 DAG/Workflow 任务依赖支持。

## 核心架构

```
┌─────────────────────────────────────────────────────────────┐
│                      Main Agent (主 Agent)                    │
│  ┌───────────────────────────────────────────────────────┐  │
│  │           AgentOrchestrator (编排器)                   │  │
│  │  • 监控看板待处理任务                                   │  │
│  │  • 孵化/释放 Sub-Agents                                │  │
│  │  • 发布任务事件和状态通知                               │  │
│  │  • 管理任务依赖关系 (DAG/Workflow)                      │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ 委托任务执行
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Sub-Agent (子 Agent)                       │
│  ┌───────────────────────────────────────────────────────┐  │
│  │              AgentWorker (任务执行器)                   │  │
│  │  • 从看板获取就绪任务 (依赖已满足)                        │  │
│  │  • 执行具体任务逻辑                                     │  │
│  │  • 通过 KanbanService 反馈状态到主 Agent                 │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## 新增功能

### 1. Task 数据结构增强 (`board.go`)

```go
// TaskDependency 表示任务间的依赖关系
type TaskDependency struct {
    TaskID         string     `json:"task_id"`          // 依赖的任务 ID
    RequiredStatus TaskStatus `json:"required_status"`  // 需要达到的状态
}

type Task struct {
    // ... 原有字段 ...
    
    // DAG/Workflow 依赖支持
    Dependencies []TaskDependency `json:"dependencies,omitempty"` // 详细依赖关系
    DependsOn    []string         `json:"depends_on,omitempty"`   // 简单依赖列表
}
```

### 2. 依赖检查方法

#### `AreTaskDependenciesSatisfied(zoneID, taskID string) bool`
检查指定任务的所有依赖是否已满足。

#### `GetReadyTasks(zoneID string) []*Task`
获取所有就绪任务（pending 状态且依赖已满足）。

#### `GetTaskExecutionOrder(zoneID string) ([]string, error)`
使用 Kahn 算法计算任务的拓扑排序，返回 DAG 执行顺序。

### 3. AgentWorker 增强 (`agent_worker.go`)

```go
// fetchNextPendingTask 现在会尊重 DAG 依赖
func (w *AgentWorker) fetchNextPendingTask() *Task {
    // 获取就绪任务（而非所有 pending 任务）
    tasks := w.board.GetReadyTasks(w.zoneID)
    // ... 按优先级选择任务
}
```

### 4. AgentOrchestrator 职责明确化 (`orchestrator.go`)

```go
// Main Agent 职责:
// - 监控看板待处理任务并孵化 sub-agents
// - 发布任务事件和状态通知
// - 管理 sub-agent 生命周期

// Sub-Agent 职责:
// - 执行分配给其 zone 的任务
// - 通过 KanbanService 向 Main Agent 报告状态
```

## 使用示例

### 创建带依赖的任务

```go
// 简单依赖：task-b 依赖 task-a 完成
taskA := &Task{
    ID: "task-a",
    Title: "数据收集",
    Status: TaskPending,
}

taskB := &Task{
    ID: "task-b",
    Title: "数据分析",
    Status: TaskPending,
    DependsOn: []string{"task-a"}, // 简单依赖
}

// 复杂依赖：task-c 依赖 task-a 完成且 task-b 达到特定状态
taskC := &Task{
    ID: "task-c",
    Title: "生成报告",
    Status: TaskPending,
    Dependencies: []TaskDependency{
        {TaskID: "task-a", RequiredStatus: TaskCompleted},
        {TaskID: "task-b", RequiredStatus: TaskCompleted},
    },
}
```

### 获取执行顺序

```go
// 获取任务的拓扑排序
order, err := board.GetTaskExecutionOrder("zone-1")
if err != nil {
    if err == kanban.ErrCircularDependency {
        // 检测到循环依赖
    }
}
// order: ["task-a", "task-b", "task-c"]
```

### Sub-Agent 自动等待依赖

```go
// Sub-Agent 只会获取依赖已满足的任务
worker := NewAgentWorker(...)
go worker.Start()

// 内部逻辑:
// 1. 调用 GetReadyTasks() 获取就绪任务
// 2. 跳过依赖未满足的任务
// 3. 执行任务并通过 service.UpdateTaskStatusWithEvent() 反馈
```

## 流程图

```
Main Agent (Orchestrator)
    │
    ├── MonitorBoard() ──► 检测 pending 任务
    │                         │
    │                         ▼
    │                    有 pending 任务？
    │                    /           \
    │                  是             否
    │                  │              │
    │                  ▼              ▼
    │            Spawn Sub-Agent   释放所有 Sub-Agents
    │                  │
    │                  ▼
    │            Sub-Agent 启动
    │                  │
    │                  ▼
    │            AgentWorker.Start()
    │                  │
    │                  ▼
    │            fetchNextPendingTask()
    │                  │
    │                  ▼
    │            GetReadyTasks() ◄── 检查依赖
    │                  │
    │                  ▼
    │            执行任务
    │                  │
    │                  ▼
    │            UpdateTaskStatusWithEvent()
    │                  │
    │                  ▼
    └──── OnTaskCompleted() ◄── 通知 Main Agent
                                 │
                                 ▼
                          检查是否所有任务完成
                                 │
                                 ▼
                          释放 Sub-Agent
```

## 错误处理

- `ErrCircularDependency`: 当检测到循环依赖时返回
- 依赖检查失败的任务保持 `pending` 状态，直到依赖满足
- Sub-Agent 会自动跳过依赖未满足的任务

## 测试覆盖

所有新功能都包含在现有测试文件中：
- `board_test.go`: 依赖检查、拓扑排序测试
- `agent_worker_test.go`: 依赖感知调度测试
- `orchestrator_test.go`: Main/Sub-Agent 交互测试

## 性能优化

- 使用 per-zone 细粒度锁，避免全局锁竞争
- 依赖检查使用缓存区域引用，减少锁持有时间
- 拓扑排序使用 Kahn 算法，O(V+E) 复杂度
