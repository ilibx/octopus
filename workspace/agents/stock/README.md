# 股票分析团队 (Stock Analysis Team)

## 📋 概述

本目录包含一个完整的股票分析团队实现，由 **1 个主 Agent** 和 **5 个子 Agent** 组成，所有 Agent 均以 `.md` 格式定义规范。系统严格遵循以下架构约束：

### 核心约束

1. **任务来源限制**: 只有 Cron 定时任务和 Channel 用户请求可以向主 Agent 下发任务
2. **状态通知集中化**: 所有任务状态更新和结果反馈必须由主 Agent 统一发布
3. **职责分离**: 主 Agent 负责任务管理和协调，子 Agent 负责具体执行和状态反馈
4. **DAG/Workflow 支持**: 任务之间可以定义依赖关系，按拓扑顺序执行

## 📁 文件结构

```
agents/stock/
├── main_agent.md            # 主 Agent 完整规范
├── technical_analyst.md     # 技术分析子 Agent
├── fundamental_analyst.md   # 基本面分析子 Agent
├── sentiment_analyst.md     # 舆情分析子 Agent
├── risk_assessor.md         # 风险评估子 Agent
└── investment_strategist.md # 投资策略子 Agent
```

## 🏗️ 团队组成

### 主 Agent (Main Agent)

| 文件 | 说明 |
|------|------|
| `main_agent.md` | 主 Agent 完整规范，包括职责、工作流、输入输出格式等 |

**核心职责**:
- ✅ 接收来自 Cron 和 Channel 的任务
- ✅ 分解任务并构建 DAG
- ✅ 孵化和调度子 Agent
- ✅ 整合分析结果
- ✅ 通过多个 Channel 反馈最终报告

### 子 Agent (Sub-Agents)

| 文件 | Agent 名称 | 职责 |
|------|-----------|------|
| `technical_analyst.md` | Technical Analyst | 技术分析 (图表、指标、趋势) |
| `fundamental_analyst.md` | Fundamental Analyst | 基本面分析 (财报、估值、竞争力) |
| `sentiment_analyst.md` | Sentiment Analyst | 舆情分析 (新闻、社交媒体、情绪) |
| `risk_assessor.md` | Risk Assessor | 风险评估 (市场风险、公司风险、压力测试) |
| `investment_strategist.md` | Investment Strategist | 投资策略 (综合建议、仓位管理、止盈止损) |

## 🔄 工作流程

### 典型 DAG 工作流

```
┌─────────────────┐
│  用户请求/定时   │
│     任务触发     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Main Agent    │
│  任务解析与验证  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   构建任务 DAG   │
└────────┬────────┘
         │
    ┌────┴────┬────────────┬────────────┐
    │         │            │            │
    ▼         ▼            ▼            ▼
┌───────┐ ┌───────┐  ┌───────┐  ┌───────┐
│技术   │ │基本面 │  │舆情   │  │风险   │
│分析   │ │分析   │  │分析   │  │评估   │
│(并行) │ │(并行) │  │(并行) │  │(并行) │
└───┬───┘ └───┬───┘  └───┬───┘  └───┬───┘
    │         │          │          │
    └─────────┴──────────┴──────────┘
                  │
                  ▼
         ┌─────────────────┐
         │  投资策略生成    │
         │ (依赖前 4 个完成) │
         └────────┬────────┘
                  │
                  ▼
         ┌─────────────────┐
         │  结果整合与发布  │
         └────────┬────────┘
                  │
                  ▼
         ┌─────────────────┐
         │  多渠道反馈用户  │
         └─────────────────┘
```

### 任务执行步骤

1. **任务接收**
   - Cron 定时任务 → `cron_integration.go`
   - Channel 用户请求 → `channels/kanban_integration.go`
   - 两者都调用 `KanbanService.CreateTaskWithEvent()`

2. **任务入队**
   - 任务添加到看板 (Board)
   - 发布 `task.created` 事件到 MessageBus

3. **Main Agent 监控**
   - `Orchestrator.MonitorBoard()` 持续监控
   - 检测到 pending 任务后检查依赖
   - 孵化对应的 Sub-Agent Worker

4. **Sub-Agent 执行**
   - Sub-Agent 认领任务
   - 执行具体分析逻辑 (根据 `.md` 规范)
   - 通过 `UpdateTaskStatusWithEvent()` 反馈状态

5. **结果整合**
   - Main Agent 收集所有 Sub-Agent 结果
   - 验证完整性和一致性
   - 生成综合报告

6. **多渠道反馈**
   - 发布 `task.completed` 事件
   - 推送到原始请求 Channel
   - 广播到所有订阅 Channel
   - WebSocket 实时推送 (如启用)

## 📊 任务依赖示例

```json
{
  "task_id": "analysis_001",
  "stock_code": "600519",
  "subtasks": [
    {
      "id": "tech_001",
      "type": "technical_analysis",
      "depends_on": []
    },
    {
      "id": "fund_001",
      "type": "fundamental_analysis",
      "depends_on": []
    },
    {
      "id": "sent_001",
      "type": "sentiment_analysis",
      "depends_on": []
    },
    {
      "id": "risk_001",
      "type": "risk_assessment",
      "depends_on": []
    },
    {
      "id": "strat_001",
      "type": "investment_strategy",
      "depends_on": ["tech_001", "fund_001", "sent_001", "risk_001"]
    }
  ]
}
```

## 🚀 使用示例

### 场景 1: 用户通过 Channel 请求单只股票分析

**用户输入:**
```
/analyze 600519 贵州茅台 full
```

**系统处理:**
1. Channel Integration 解析命令
2. 创建分析任务 (source=channel)
3. Main Agent 接收任务
4. 并行孵化 4 个分析 Agent
5. 等待完成后孵化策略 Agent
6. 整合结果并反馈给用户

### 场景 2: Cron 定时批量分析

**Cron 配置:**
```yaml
jobs:
  - name: daily_stock_analysis
    schedule: "0 9 * * *"  # 每个交易日早上 9 点
    config:
      stocks: ["600519", "000858", "000568"]
      analysis_types: ["full"]
```

**系统处理:**
1. Cron 触发任务
2. 为每只股票创建分析任务 (source=cron)
3. Main Agent 并行处理多只股票
4. 结果发布到订阅的 Channel

### 场景 3: 订阅股票的重大事件分析

**用户操作:**
1. 用户订阅 600519 的重大事件通知
2. 公司发布财报
3. Cron 检测到财报发布
4. 触发专项分析任务
5. 结果推送给所有订阅用户

## ✅ 合规性检查清单

确保所有实现严格遵守以下约束:

- [x] 任务只能由 Cron 或 Channel 创建
- [x] 所有状态更新通过 Main Agent 发布
- [x] Sub-Agent 不直接访问 MessageBus
- [x] DAG 依赖关系得到正确执行
- [x] 结果同时反馈到请求 Channel 和订阅 Channel
- [x] 循环依赖被检测并阻止
- [x] 任务超时和错误得到妥善处理

## 📖 相关文档

- `/workspace/pkg/kanban/` - 看板核心实现
- `/workspace/pkg/cron/` - Cron 定时任务
- `/workspace/pkg/channels/` - Channel 用户通道
- `/workspace/CODE_REVIEW_REPORT.md` - Code Review 报告
- `/workspace/COMPLIANCE_REPORT.md` - 合规性审查报告
