# Octopus 看板模式架构设计

## 概述

本文档描述了 Octopus 项目中主 Agent 与子 Agent 通过看板 (Kanban) 进行任务协作的架构设计。

## 核心概念

### 1. 看板 board (Kanban Board)
- 主 Agent 维护的全局任务协调中心
- 由多个独立区域 (Zone) 组成
- 每个区域对应一类特定任务类型

### 2. 区域 (Zone)
- 看板上的功能划分区域
- 每个区域有明确的职责和所需的 Agent 类型
- 包含该区域的所有任务及其状态

### 3. 任务 (Task)
- 最小的工作单元
- 状态包括：pending, running, in_progress, completed, failed, blocked
- 包含结果、错误信息等元数据

### 4. 主 Agent (Main Agent)
- 看板的维护者
- 负责任务接收、拆分和规划
- 监控看板状态，孵化子 Agent
- 向发布渠道汇报任务状态

### 5. 子 Agent (Sub-Agent)
- 由主 Agent 动态孵化
- 自主完成看板区域匹配的任务
- 完成后将状态和结果发布到对应看板区域

## 架构组件

### pkg/kanban/board.go
```go
type KanbanBoard struct {
    ID          string
    Name        string
    Zones       map[string]*Zone
    MainAgentID string
}

type Zone struct {
    ID          string
    Name        string
    Description string
    Tasks       []*Task
    AgentType   string  // 需要的 Agent 类型
    Active      bool
}

type Task struct {
    ID          string
    Title       string
    Description string
    Status      TaskStatus
    Priority    int
    AssignedTo  string  // 负责执行的 Agent ID
    Result      string
    Error       string
}
```

**核心功能:**
- `CreateZone()`: 创建新的功能区域
- `AddTask()`: 添加任务到指定区域
- `UpdateTaskStatus()`: 更新任务状态
- `GetPendingTasks()`: 获取所有待处理任务
- `HasActiveAgent()`: 检查区域是否有活跃 Agent

### pkg/kanban/orchestrator.go
```go
type AgentOrchestrator struct {
    board         *KanbanBoard
    agentRegistry *agent.AgentRegistry
    activeAgents  map[string]string  // zoneID -> agentID
}
```

**核心功能:**
- `MonitorBoard()`: 持续监控看板状态
- `checkAndSpawnAgents()`: 检查并孵化所需 Agent
- `spawnAgentForZone()`: 为区域创建子 Agent
- `OnTaskCompleted()`: 处理任务完成事件

### pkg/kanban/service.go
```go
type KanbanService struct {
    board   *KanbanBoard
    msgBus  *bus.MessageBus
}
```

**核心功能:**
- `PublishTaskEvent()`: 发布任务事件
- `CreateTaskWithEvent()`: 创建任务并发布事件
- `UpdateTaskStatusWithEvent()`: 更新状态并发布事件
- `StartStatusReporter()`: 向渠道汇报状态变化
- `HTTPHandler()`: 提供 REST API 接口

## 工作流程

### 1. 任务接收流程
```
Channel → Main Agent → Kanban Board
                          ↓
                    Create Zone (if needed)
                          ↓
                    Add Task (status: pending)
                          ↓
                    Publish Event
```

### 2. 子 Agent 孵化流程
```
Orchestrator Monitor Loop (every 2s)
        ↓
Check Pending Tasks per Zone
        ↓
No Active Agent in Zone?
        ↓ YES
Load agent.md Template
        ↓
Create Agent Instance
        ↓
Register to AgentRegistry
        ↓
Start Agent Loop
        ↓
Mark Zone as Active
```

### 3. 任务执行流程
```
Sub-Agent wakes up
        ↓
Poll Assigned Zone for Pending Tasks
        ↓
Claim Task (status: running)
        ↓
Execute Task Logic
        ↓
Update Task (status: completed/failed, result/error)
        ↓
Publish Event → Main Agent → Channels
        ↓
Check if all tasks in zone completed
        ↓ YES
Release Agent (optional)
```

### 4. 状态汇报流程
```
Task Status Changed
        ↓
KanbanService Publishes Event
        ↓
StatusReporter Listens
        ↓
Format Report Message
        ↓
Publish to channel.broadcast
        ↓
All Channels Receive Update
```

## API 接口

### REST Endpoints

```
GET /kanban
  - 返回整个看板状态

GET /kanban/zones/{zoneID}
  - 返回指定区域的详细信息

GET /kanban/tasks?zone={zoneID}&status={status}
  - 查询任务列表，支持按状态过滤
```

### 事件总线主题

```
kanban.events
  - 所有任务变更事件
  
channel.broadcast
  - 格式化后的状态报告，供所有渠道订阅
```

## 配置示例

```yaml
kanban:
  enabled: true
  board_id: "main"
  board_name: "Octopus Main Board"
  
  zones:
    - id: "research"
      name: "Research & Analysis"
      description: "Information gathering and analysis tasks"
      agent_type: "researcher"
      
    - id: "coding"
      name: "Code Development"
      description: "Programming and code review tasks"
      agent_type: "developer"
      
    - id: "writing"
      name: "Content Writing"
      description: "Document and content creation"
      agent_type: "writer"
      
    - id: "automation"
      name: "Task Automation"
      description: "Recurring and automated workflows"
      agent_type: "automator"
```

## 子 Agent 模板 (agent.md)

```markdown
# Agent: {{.AgentType}}

## Role
You are a specialized {{.AgentType}} agent responsible for tasks in the {{.ZoneName}} zone.

## Responsibilities
- Monitor assigned zone for pending tasks
- Execute tasks autonomously
- Report results back to kanban board

## Tools Available
{{.Tools}}

## Workflow
1. Check zone for pending tasks
2. Claim task by updating status to 'running'
3. Execute task using available tools
4. Update task with result or error
5. Wait for next assignment
```

## 周期任务支持

通过集成 `pkg/cron` 模块，看板支持周期任务:

```go
// 在主 Agent 中注册周期任务
cronService.AddJob("0 */6 * * *", func() {
    kanban.CreateTaskWithEvent(
        "automation",
        fmt.Sprintf("health_check_%d", time.Now().Unix()),
        "System Health Check",
        "Perform routine system health verification",
        PriorityNormal,
        nil,
    )
})
```

## 优势

1. **解耦**: 主 Agent 与子 Agent 通过看板异步通信
2. **可扩展**: 可动态添加新的区域和 Agent 类型
3. **可观测**: 所有任务状态透明可见
4. **弹性**: Agent 可根据负载动态孵化/释放
5. **多渠道**: 统一的状态汇报机制支持多种通信渠道

## 待实现功能

1. [ ] 动态 Agent 孵化逻辑 (加载 agent.md 模板)
2. [ ] Agent 生命周期管理 (启动/停止/重启)
3. [ ] 任务优先级调度算法
4. [ ] Agent 间通信机制
5. [ ] 持久化看板状态
6. [ ] Web UI 可视化界面
7. [ ] 任务历史记录和审计日志

## 集成点

- `pkg/bus`: 消息总线用于事件分发
- `pkg/agent`: Agent 实例管理和执行
- `pkg/channels`: 多渠道状态汇报
- `pkg/cron`: 周期任务调度
- `pkg/health`: HTTP 健康检查和 API
