package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"stock-analyzer/pkg/channels"
	"stock-analyzer/pkg/cron"
	"stock-analyzer/pkg/kanban"
	"stock-analyzer/pkg/messagebus"
)

func main() {
	log.Println("🚀 启动股票分析团队系统...")

	// 1. 初始化核心组件
	msgBus := messagebus.NewMessageBus()
	board := kanban.NewBoard()
	cronSvc := cron.NewService(msgBus)

	// 2. 初始化 Channel 模块 (用户交互层)
	channelMgr := channels.NewManager(msgBus)
	if err := channelMgr.Start(); err != nil {
		log.Fatalf("Failed to start channel manager: %v", err)
	}
	defer channelMgr.Stop()

	// 3. 初始化看板服务 (任务管理层)
	kanbanSvc := kanban.NewKanbanService(board, msgBus)
	if err := kanbanSvc.StartStatusReporter(); err != nil {
		log.Fatalf("Failed to start status reporter: %v", err)
	}

	// 4. 【硬编码约束】创建主 Agent 协调器
	// 只有主 Agent 能获取 MainAgentCoordinator 接口，拥有 Cron/Channel/Board 全权访问
	mainAgentCoord := kanban.NewMainAgentCoordinator(board, cronSvc, msgBus)

	// 5. 启动主 Agent (Orchestrator)
	orchestrator := kanban.NewAgentOrchestrator(board, kanbanSvc, msgBus)
	go orchestrator.MonitorBoard(context.Background())
	log.Println("✅ 主 Agent (Orchestrator) 已启动")

	// 6. 加载股票分析团队子 Agent 规范
	// 子 Agent 仅通过 .md 文件定义行为，运行时被赋予受限的 SubAgentTaskClient 接口
	teamSpecs := []string{
		"technical_analyst",
		"fundamental_analyst",
		"sentiment_analyst",
		"risk_assessor",
		"investment_strategist",
	}

	for _, specName := range teamSpecs {
		specPath := fmt.Sprintf("agents/stock/%s.md", specName)
		if _, err := os.Stat(specPath); os.IsNotExist(err) {
			log.Printf("⚠️ 跳过不存在的 Agent 规范：%s", specPath)
			continue
		}
		
		// 模拟从 .md 文件加载 Agent 配置并孵化
		// 注意：这里传入的是 NewSubAgentClient，物理上隔绝了 Cron 和 Channel 访问
		subClient := kanban.NewSubAgentClient(board, specName)
		go runSubAgentLoop(context.Background(), specName, subClient)
		log.Printf("✅ 子 Agent [%s] 已孵化 (受限模式)", specName)
	}

	// 7. 注册 Cron 定时任务 (仅主 Agent 可操作)
	// 示例：每 5 分钟触发一次股票分析任务
	err := mainAgentCoord.ScheduleJob("*/5 * * * *", func() {
		log.Println("⏰ Cron 触发：执行定期股票分析任务")
		
		// 主 Agent 通过看板服务创建任务 (唯一入口)
		taskID, err := kanbanSvc.CreateTaskWithEvent(context.Background(), "STOCK_ANALYSIS", map[string]interface{}{
			"symbol":    "AAPL",
			"trigger":   "CRON_SCHEDULE",
			"timestamp": time.Now().Format(time.RFC3339),
		}, nil) // DAG 依赖为空

		if err != nil {
			log.Printf("❌ 创建任务失败：%v", err)
			return
		}
		log.Printf("📝 任务已创建：%s", taskID)
	})
	if err != nil {
		log.Fatalf("Failed to schedule cron job: %v", err)
	}

	// 8. 保持运行
	log.Println("🔄 系统运行中... (按 Ctrl+C 停止)")
	select {}
}

// runSubAgentLoop 模拟子 Agent 的运行循环
// 关键点：subClient 类型为 SubAgentTaskClient，编译期保证无法访问 Cron 或 Channel
func runSubAgentLoop(ctx context.Context, agentType string, subClient kanban.SubAgentTaskClient) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 子 Agent 只能尝试认领任务
			task, err := subClient.ClaimTask(ctx, "STOCK_ANALYSIS")
			if err != nil || task == nil {
				continue // 无可用任务
			}

			log.Printf("🤖 [%s] 认领任务：%s (Symbol: %v)", agentType, task.ID, task.Payload["symbol"])

			// 模拟分析过程
			time.Sleep(3 * time.Second)

			// 子 Agent 只能通过 UpdateStatus 反馈结果
			result := map[string]interface{}{
				"analysis_type": agentType,
				"result":        "BUY",
				"confidence":    0.85,
				"completed_at":  time.Now().Format(time.RFC3339),
			}

			if err := subClient.UpdateStatus(ctx, task.ID, kanban.StatusCompleted, result); err != nil {
				log.Printf("❌ [%s] 更新状态失败：%v", agentType, err)
			} else {
				log.Printf("✅ [%s] 任务完成：%s", agentType, task.ID)
			}
		}
	}
}
