# Octopus 数据流转详细说明

本文档详细描述 Octopus 系统中数据的完整流转过程，包括任务创建、分配、执行和完成的整个生命周期。

---

## 1. 核心数据模型

### 1.1 数据结构关系图

```
┌──────────────────┐
│   KanbanBoard    │
│  - ID: string    │
│  - Name: string  │
│  - Zones: map    │◄───────┐
└────────┬─────────┘        │
         │                  │
         ▼                  │
┌──────────────────┐        │
│      Zone        │        │
│  - ID: string    │        │
│  - AgentType     │        │
│  - Tasks: []     │◄───┐   │
└────────┬─────────┘    │   │
         │              │   │
         ▼              │   │
┌──────────────────┐    │   │
│      Task        │    │   │
│  - ID: string    │    │   │
│  - Status        │    │   │
│  - Priority      │    │   │
│  - Result        │    │   │
└────────┬─────────┘    │   │
         │              │   │
         │              │   │
         ▼              │   │
┌──────────────────┐    │   │
│   TaskEvent      │    │   │
│  - Type          │    │   │
│  - ZoneID        │    │   │
│  - TaskID        │    │   │
│  - Status        │    │   │
└────────┬─────────┘    │   │
         │              │   │
         ▼              │   │
┌──────────────────┐    │   │
│   MessageBus     │────┘   │
│  - Subjects      │        │
│  - Subscribers   │        │
└────────┬─────────┘        │
         │                  │
         ▼                  │
┌──────────────────┐        │
│  Orchestrator    │────────┘
│  - Board         │
│  - ActiveAgents  │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│   AgentWorker    │
│  - ZoneID        │
│  - AgentInstance │
│  - CurrentTasks  │
└──────────────────┘
```

---

## 2. 任务创建流程

### 2.1 流程图

```
用户/API
   │
   ▼
┌─────────────────┐
│ CreateTask API  │ POST /kanban/tasks
└────────┬────────┘
         │ task data
         ▼
┌─────────────────┐
│ KanbanService   │
│ CreateTaskWithEvent()
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌─────────┐ ┌──────────┐
│  Board  │ │ Event Bus│
│ AddTask │ │ Publish  │
└────┬────┘ └────┬─────┘
     │           │
     │ task      │ task_created event
     │ created   │
     ▼           ▼
┌─────────┐ ┌──────────────┐
│  Zone   │ │ Orchestrator │
│ Tasks[] │ │ MonitorBoard │
└─────────┘ └──────┬───────┘
                   │
                   ▼
            ┌─────────────┐
            │ checkAndSpawn│
            │ Agents()     │
            └──────┬──────┘
```

### 2.2 详细步骤

#### 步骤 1: API 接收请求
```go
// HTTP Handler 接收创建任务请求
POST /kanban/tasks
{
  "zone_id": "research",
  "task_id": "task_001",
  "title": "研究市场趋势",
  "description": "分析最新的市场报告...",
  "priority": 1,
  "metadata": {"source": "user_input"}
}
```

#### 步骤 2: Service 层处理
```go
// pkg/kanban/service.go:76-84
func (s *KanbanService) CreateTaskWithEvent(...) (*Task, error) {
    // 2.1 添加到看板
    task, err := s.board.AddTask(zoneID, taskID, title, description, priority, metadata)
    
    // 2.2 发布事件
    s.PublishTaskEvent("task_created", zoneID, taskID, TaskPending, title, "", "")
    
    return task, nil
}
```

#### 步骤 3: Board 存储任务
```go
// pkg/kanban/board.go:103-137
func (k *KanbanBoard) AddTask(...) (*Task, error) {
    k.mu.Lock()
    defer k.mu.Unlock()
    
    // 3.1 获取目标区域
    zone, ok := k.Zones[zoneID]
    
    // 3.2 创建任务对象
    task := &Task{
        ID:          taskID,
        Title:       title,
        Description: description,
        Status:      TaskPending,
        Priority:    priority,
        CreatedAt:   time.Now(),
    }
    
    // 3.3 添加到区域任务列表
    zone.Tasks = append(zone.Tasks, task)
    
    return task, nil
}
```

#### 步骤 4: 发布事件
```go
// pkg/kanban/service.go:46-73
func (s *KanbanService) PublishTaskEvent(...) {
    event := TaskEvent{
        Type:      "task_created",
        BoardID:   s.board.ID,
        ZoneID:    zoneID,
        TaskID:    taskID,
        Status:    TaskPending,
        Timestamp: time.Now().Unix(),
    }
    
    // 序列化并发布到消息总线
    data, _ := json.Marshal(event)
    s.msgBus.Publish("kanban.events", string(data))
}
```

---

## 3. Agent 孵化流程

### 3.1 流程图

```
Orchestrator.MonitorBoard()
         │
         ▼
    ┌────────┐
    │ ticker │ 每 2 秒触发
    └───┬────┘
        │
        ▼
┌─────────────────┐
│ GetPendingTasks │
└────────┬────────┘
         │
         ▼
    ┌────────┐
    │ 遍历   │ 每个有待处理任务的 Zone
    └───┬────┘
        │
        ▼
┌─────────────────┐
│ HasActiveAgent? │─── Yes ───► 跳过
└────────┬────────┘
         │ No
         ▼
┌─────────────────┐
│ GetZoneAgentType│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ spawnAgentForZone│
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌─────────┐ ┌──────────┐
│Registry │ │ Worker   │
│AddAgent │ │ Start    │
└─────────┘ └──────────┘
```

### 3.2 详细步骤

#### 步骤 1: 持续监控看板
```go
// pkg/kanban/orchestrator.go:41-54
func (o *AgentOrchestrator) MonitorBoard(ctx context.Context) {
    ticker := time.NewTicker(2 * time.Second)
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            o.checkAndSpawnAgents()  // 检查并孵化 Agent
        }
    }
}
```

#### 步骤 2: 检查待处理任务
```go
// pkg/kanban/orchestrator.go:57-112
func (o *AgentOrchestrator) checkAndSpawnAgents() {
    o.mu.Lock()
    defer o.mu.Unlock()
    
    // 2.1 获取所有待处理任务
    pendingTasks := o.board.GetPendingTasks()
    
    // 2.2 如果没有任务，释放所有 Agent
    if len(pendingTasks) == 0 {
        o.ReleaseAllAgents()
        return
    }
    
    // 2.3 遍历每个有待处理任务的区域
    for zoneID, tasks := range pendingTasks {
        // 2.4 检查是否已有活跃 Agent
        if o.activeAgents[zoneID] != "" {
            continue
        }
        
        // 2.5 检查区域是否有运行中的任务
        if o.board.HasActiveAgent(zoneID) {
            continue
        }
        
        // 2.6 获取所需 Agent 类型
        agentType, _ := o.board.GetZoneAgentType(zoneID)
        
        // 2.7 孵化新 Agent
        o.spawnAgentForZone(zoneID, agentType)
    }
}
```

#### 步骤 3: 孵化子 Agent
```go
// pkg/kanban/orchestrator.go:115-151
func (o *AgentOrchestrator) spawnAgentForZone(zoneID, agentType string) error {
    // 3.1 生成唯一 Agent ID
    agentID := fmt.Sprintf("%s_%s", agentType, zoneID)
    
    // 3.2 检查是否已存在
    if _, exists := o.agentRegistry.GetAgent(agentID); exists {
        o.activeAgents[zoneID] = agentID
        return nil
    }
    
    // 3.3 创建 Agent 配置
    agentCfg := &config.AgentConfig{
        ID:      agentID,
        Name:    fmt.Sprintf("Subagent for zone %s", zoneID),
        Default: false,
    }
    
    // 3.4 添加到注册表
    addedID, err := o.agentRegistry.AddAgent(agentCfg, ...)
    
    // 3.5 记录活跃 Agent
    o.activeAgents[zoneID] = addedID
    
    return nil
}
```

---

## 4. 任务执行流程

### 4.1 流程图

```
AgentWorker.Start()
     │
     ▼
┌─────────────────┐
│ 启动多个 worker │ maxConcurrency 个协程
│ goroutines      │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ processTasksLoop│
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌─────────┐ ┌──────────┐
│ ticker  │ │ ctx.Done │
│ 500ms   │ │ (停止)   │
└────┬────┘ └──────────┘
     │
     ▼
┌─────────────────┐
│ tryProcessNextTask│
└────────┬────────┘
     │
     ▼
┌─────────────────┐
│ 检查并发数限制   │
└────────┬────────┘
     │
     ▼
┌─────────────────┐
│ fetchNextPendingTask│
└────────┬────────┘
     │
     ▼
┌─────────────────┐
│ claimTask       │
└────────┬────────┘
     │
     ▼
┌─────────────────┐
│ executeTask     │
└────────┬────────┘
     │
     ▼
┌─────────────────┐
│ runTaskExecution│
└────────┬────────┘
     │
     ▼
┌─────────────────┐
│ 更新任务状态     │
└────────┬────────┘
```

### 4.2 详细步骤

#### 步骤 1: 启动 Worker
```go
// pkg/kanban/agent_worker.go:54-73
func (w *AgentWorker) Start() {
    logger.InfoCF("agent_worker", "Starting agent worker", ...)
    
    var wg sync.WaitGroup
    // 1.1 启动多个并发 worker
    for i := 0; i < w.maxConcurrency; i++ {
        wg.Add(1)
        go func(workerNum int) {
            defer wg.Done()
            w.processTasksLoop(workerNum)
        }(i)
    }
    
    wg.Wait()
}
```

#### 步骤 2: 任务处理循环
```go
// pkg/kanban/agent_worker.go:76-90
func (w *AgentWorker) processTasksLoop(workerNum int) {
    ticker := time.NewTicker(500 * time.Millisecond)
    
    for {
        select {
        case <-w.ctx.Done():
            return  // 优雅退出
        case <-ticker.C:
            w.tryProcessNextTask(workerNum)
        }
    }
}
```

#### 步骤 3: 尝试处理下一个任务
```go
// pkg/kanban/agent_worker.go:92-115
func (w *AgentWorker) tryProcessNextTask(workerNum int) {
    // 3.1 检查并发限制
    w.mu.RLock()
    if len(w.currentTasks) >= w.maxConcurrency {
        w.mu.RUnlock()
        return
    }
    w.mu.RUnlock()
    
    // 3.2 获取下一个待处理任务
    task := w.fetchNextPendingTask()
    if task == nil {
        return
    }
    
    // 3.3 认领任务
    if !w.claimTask(task.ID) {
        return
    }
    
    // 3.4 异步执行任务
    go w.executeTask(task, workerNum)
}
```

#### 步骤 4: 获取待处理任务
```go
// pkg/kanban/agent_worker.go:117-133
func (w *AgentWorker) fetchNextPendingTask() *Task {
    // 4.1 从看板获取所有 pending 任务
    tasks := w.board.GetTasksByStatus(w.zoneID, TaskPending)
    if len(tasks) == 0 {
        return nil
    }
    
    // 4.2 选择优先级最高的任务
    var highestPriorityTask *Task
    for _, task := range tasks {
        if highestPriorityTask == nil || 
           task.Priority > highestPriorityTask.Priority {
            highestPriorityTask = task
        }
    }
    
    return highestPriorityTask
}
```

#### 步骤 5: 执行任务
```go
// pkg/kanban/agent_worker.go:156-199
func (w *AgentWorker) executeTask(task *Task, workerNum int) {
    defer w.releaseTask(task.ID)
    
    // 5.1 更新状态为 running
    w.service.UpdateTaskStatusWithEvent(
        w.zoneID, task.ID, TaskRunning, "", "")
    
    // 5.2 执行实际任务逻辑
    result, err := w.runTaskExecution(task)
    
    // 5.3 根据结果更新状态
    if err != nil {
        w.service.UpdateTaskStatusWithEvent(
            w.zoneID, task.ID, TaskFailed, "", err.Error())
    } else {
        w.service.UpdateTaskStatusWithEvent(
            w.zoneID, task.ID, TaskCompleted, result, "")
    }
}
```

#### 步骤 6: 调用 Agent 执行
```go
// pkg/kanban/agent_worker.go:201-230
func (w *AgentWorker) runTaskExecution(task *Task) (string, error) {
    // 6.1 构建 prompt
    prompt := fmt.Sprintf("Task: %s\nDescription: %s", 
        task.Title, task.Description)
    
    // 6.2 添加元数据
    if len(task.Metadata) > 0 {
        for k, v := range task.Metadata {
            prompt += fmt.Sprintf("\n  %s: %s", k, v)
        }
    }
    
    // 6.3 使用 Agent 实例执行
    ctx, cancel := context.WithTimeout(w.ctx, 5*time.Minute)
    defer cancel()
    
    result, err := w.processTaskWithAgent(ctx, prompt, task.ID)
    
    return result, err
}
```

---

## 5. 任务完成与 Agent 释放

### 5.1 流程图

```
Task Completed
     │
     ▼
┌─────────────────┐
│ UpdateTaskStatus│
│ (completed)     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Publish Event   │
│ task_completed  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Orchestrator    │
│ OnTaskCompleted │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 检查区域是否还有 │
│ 未完成的任务     │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
  有任务     无任务
    │         │
    │         ▼
    │   ┌─────────────────┐
    │   │ RemoveAgent     │
    │   │ 释放子 Agent     │
    │   └─────────────────┘
    │
    ▼
继续执行
```

### 5.2 详细步骤

#### 步骤 1: 检测任务完成
```go
// pkg/kanban/orchestrator.go:154-193
func (o *AgentOrchestrator) OnTaskCompleted(zoneID, taskID string) {
    o.mu.Lock()
    defer o.mu.Unlock()
    
    // 1.1 获取区域
    zone, err := o.board.GetZone(zoneID)
    
    // 1.2 检查所有任务是否都已完成
    allCompleted := true
    for _, task := range zone.Tasks {
        if task.Status != TaskCompleted && 
           task.Status != TaskFailed {
            allCompleted = false
            break
        }
    }
    
    // 1.3 如果全部完成，释放 Agent
    if allCompleted {
        if agentID, exists := o.activeAgents[zoneID]; exists {
            // 从注册表移除
            o.agentRegistry.RemoveAgent(agentID)
            
            // 从活跃列表删除
            delete(o.activeAgents, zoneID)
        }
    }
}
```

---

## 6. 周期任务触发流程

### 6.1 设计说明

**重要变更**：Cron 服务触发的任务不再直接创建看板任务，而是先发送到主 Agent，由主 Agent 根据任务性质智能选择通知渠道。

**流程优势**：
- 主 Agent 可以分析任务内容，决定使用哪些 Channel（单个或多个）进行通知
- 实现更灵活的通知策略，避免硬编码的通知逻辑
- 支持动态调整通知渠道组合

### 6.2 流程图

```
CronService.runLoop()
     │
     ▼
┌─────────────────┐
│ 每秒检查一次     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ checkJobs()     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 遍历所有启用的   │
│ Cron Job        │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ NextRunAt <= Now│─── No ───► 跳过
└────────┬────────┘
         │ Yes
         ▼
┌─────────────────┐
│ 清除 NextRunAt  │
│ (防止重复执行)   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ onJob(handler)  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 发送到主 Agent   │◄────── 新增步骤
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 主 Agent 分析    │
│ 任务性质         │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 智能选择 Channel │
│ (单个或多个)     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 通过选定渠道     │
│ 发送通知         │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 计算下次运行时间 │
└─────────────────┘
```

### 6.3 集成示例

```go
// 设置 Cron 任务处理器
cronService.SetOnJob(func(job *CronJob) (string, error) {
    // 解析 payload
    var payload CronPayload
    json.Unmarshal([]byte(job.Payload.Message), &payload)
    
    // 发送到主 Agent，由主 Agent 处理通知逻辑
    mainAgent.ProcessCronTask(&CronTask{
        ID:      job.ID,
        Message: payload.Message,
        Metadata: map[string]string{
            "cron_job_id": job.ID,
            "schedule":    job.Schedule,
        },
    })
    
    return "Task sent to main agent", nil
})

// 主 Agent 处理 Cron 任务的示例逻辑
func (a *MainAgent) ProcessCronTask(task *CronTask) error {
    // 1. 分析任务性质
    channels := a.selectChannelsForTask(task)
    
    // 2. 通过选定的渠道发送通知
    for _, ch := range channels {
        ch.SendNotification(task.Message)
    }
    
    return nil
}

// 智能选择渠道的逻辑
func (a *MainAgent) selectChannelsForTask(task *CronTask) []Channel {
    var selected []Channel
    
    // 根据任务元数据或内容选择合适的渠道
    if task.Metadata["priority"] == "high" {
        // 高优先级任务：使用所有可用渠道
        selected = a.allChannels
    } else if task.Metadata["type"] == "alert" {
        // 告警类任务：使用即时通讯渠道
        selected = a.instantChannels // Telegram, Discord, etc.
    } else {
        // 普通任务：使用默认渠道
        selected = a.defaultChannels
    }
    
    return selected
}
```

---

## 7. 事件驱动通信

### 7.1 事件类型和数据流

```
┌──────────────────────────────────────────────────────────┐
│                    Message Bus                            │
│                                                           │
│  Subject: "kanban.events"                                │
│  ├── task_created                                        │
│  ├── task_updated                                        │
│  └── task_completed                                      │
│                                                           │
│  Subject: "channel.broadcast"                            │
│  └── 格式化后的状态报告                                   │
│                                                           │
│  Subscribers:                                            │
│  ├── KanbanService (StartStatusReporter)                │
│  ├── Channels (Telegram, HTTP, etc.)                    │
│  └── External Systems                                    │
└──────────────────────────────────────────────────────────┘
```

### 7.2 事件订阅和处理

```go
// pkg/kanban/service.go:200-232
func (s *KanbanService) StartStatusReporter(ctx context.Context) {
    go func() {
        handler := func(msg string) {
            var event TaskEvent
            json.Unmarshal([]byte(msg), &event)
            
            // 格式化报告
            report := fmt.Sprintf("📋 Task Update: %s\nZone: %s\nStatus: %s",
                event.Title, event.ZoneID, event.Status)
            
            if event.Result != "" {
                report += fmt.Sprintf("\nResult: %s", event.Result)
            }
            
            // 广播到所有渠道
            s.msgBus.Publish("channel.broadcast", report)
        }
        
        s.SubscribeToEvents(handler)
        
        <-ctx.Done()
    }()
}
```

---

## 8. 数据持久化

### 8.1 内存数据结构

所有核心数据（Board、Zones、Tasks）都存储在内存中，通过 mutex 保证并发安全。

### 8.2 持久化策略

| 数据类型 | 持久化方式 | 位置 |
|---------|-----------|------|
| Cron Jobs | JSON 文件 | `CRON_STORE_PATH` |
| Agent Config | 配置文件 | `config.yaml` |
| Templates | Markdown 文件 | `KANBAN_TEMPLATE_DIR` |
| Events | 可选：消息队列 | Redis/Kafka |

### 8.3 恢复机制

系统重启时：
1. 从配置文件加载 Agent 定义
2. 从 cron.json 恢复周期任务
3. 从模板目录加载 Agent 模板
4. 看板状态从头开始（不持久化）

---

## 9. 性能考虑

### 9.1 并发控制

- **Board 级锁**: 所有看板操作使用 `sync.RWMutex`
- **Zone 级优化**: 可以改进为按 Zone 分片锁
- **Worker 限流**: 通过 `maxConcurrency` 控制并发任务数

### 9.2 内存管理

- **任务清理**: 完成的任务应该定期归档或删除
- **缓存策略**: 模板加载器实现 LRU 缓存
- **事件缓冲**: 消息总线应该有容量限制

### 9.3 延迟优化

| 操作 | 当前延迟 | 优化建议 |
|------|---------|---------|
| Agent 孵化 | ~100ms | 预热的 Agent 池 |
| 任务轮询 | 500ms | 事件驱动替代轮询 |
| 状态同步 | <10ms | 保持不变 |

---

## 10. 故障处理

### 10.1 常见故障场景

1. **Agent 执行失败**
   - 任务状态标记为 failed
   - 错误信息记录到 task.Error
   - 触发告警事件

2. **消息总线断开**
   - 事件丢失（当前设计）
   - 建议：实现事件持久化和重试

3. **看板数据不一致**
   - Mutex 保护防止竞态条件
   - 建议：增加数据校验机制

### 10.2 恢复策略

```go
// 优雅关闭示例
func (w *AgentWorker) Stop() {
    w.cancel()  // 发送停止信号
    
    // 等待当前任务完成（最多 10 秒）
    timeout := time.After(10 * time.Second)
    ticker := time.NewTicker(100 * time.Millisecond)
    
    for {
        select {
        case <-timeout:
            return  // 强制关闭
        case <-ticker.C:
            if w.GetActiveTasks() == 0 {
                return  // 正常关闭
            }
        }
    }
}
```

---

## 总结

Octopus 的数据流转设计遵循以下原则：

1. **事件驱动**: 所有状态变更都通过事件通知
2. **松耦合**: 组件间通过消息总线通信
3. **并发安全**: 使用 mutex 保护共享数据
4. **弹性伸缩**: 根据负载自动调整资源
5. **可追溯性**: 完整的日志和事件记录

这种设计使得系统既保持了高性能，又具有良好的可扩展性和可维护性。
