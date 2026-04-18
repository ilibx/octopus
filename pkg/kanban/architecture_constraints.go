package kanban

import (
	"context"
	"fmt"

	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/cron"
)

// =============================================================================
// 硬编码架构约束 (Hardcoded Architectural Constraints)
// =============================================================================
// 本文件定义了系统的核心交互边界，通过接口隔离和依赖注入强制实施：
//
// 1. MainAgentContext: 仅主 Agent 可获取。包含 Cron, Channel, Board 的全权访问。
// 2. SubAgentContext: 仅子 Agent 可获取。**仅**包含 Board 的任务认领与状态更新接口。
//    - 无 Cron 访问权限
//    - 无 Channel 访问权限
//    - 无全局 Board 读取权限 (只能操作已认领的任务)
// =============================================================================

// SubAgentTaskClient 是子代理唯一能接触的接口集合。
// 它被刻意设计为功能受限，物理上隔绝了与 Cron 和 Channel 的交互可能。
type SubAgentTaskClient interface {
	// ClaimTask 尝试认领一个特定类型的任务。
	// 子代理只能通过此方法获取工作，不能直接查询看板全貌。
	ClaimTask(ctx context.Context, taskType string) (*Task, error)

	// UpdateStatus 更新当前正在执行的任务状态。
	// 这是子代理反馈结果的唯一途径。
	UpdateStatus(ctx context.Context, taskID string, status TaskStatus, result map[string]interface{}) error

	// Heartbeat 发送心跳以保持任务持有权。
	Heartbeat(ctx context.Context, taskID string) error
}

// MainAgentCoordinator 是主代理独有的接口集合。
// 拥有系统最高权限，负责调度、通知和整体协调。
type MainAgentCoordinator interface {
	SubAgentTaskClient // 主代理也具备子代理的所有能力（用于测试或特殊处理）

	// Cron 访问
	GetCronService() *cron.CronService
	ScheduleJob(spec string, job func()) error

	// Channel 访问
	GetMessageBus() *bus.MessageBus
	PublishToChannels(eventType string, payload interface{}) error

	// Board 全局访问 (子代理不可用)
	GetBoardSnapshot() *KanbanBoard
	ForceAssignTask(taskID string, agentID string) error
}

// =============================================================================
// 实现类
// =============================================================================

// subAgentClientImpl 是 SubAgentTaskClient 的唯一实现。
// 注意：它只持有了 board 的弱引用（通过特定的安全方法），完全没有 cron 或 msgbus 的字段。
type subAgentClientImpl struct {
	board *KanbanBoard
	agentID string
}

func NewSubAgentClient(board *KanbanBoard, agentID string) SubAgentTaskClient {
	return &subAgentClientImpl{
		board: board,
		agentID: agentID,
	}
}

func (s *subAgentClientImpl) ClaimTask(ctx context.Context, taskType string) (*Task, error) {
	// 硬编码逻辑：子代理只能“盲”认领，不能指定具体任务ID，防止抢占
	return s.board.ClaimPendingTaskByType(s.agentID, taskType)
}

func (s *subAgentClientImpl) UpdateStatus(ctx context.Context, taskID string, status TaskStatus, result map[string]interface{}) error {
	// 硬编码逻辑：验证任务确实属于该子代理
	task := s.board.GetTask(taskID)
	if task == nil || task.AssignedTo != s.agentID {
		return fmt.Errorf("unauthorized: task %s not assigned to agent %s", taskID, s.agentID)
	}
	return s.board.UpdateTaskStatus(taskID, status, result)
}

func (s *subAgentClientImpl) Heartbeat(ctx context.Context, taskID string) error {
	task := s.board.GetTask(taskID)
	if task == nil || task.AssignedTo != s.agentID {
		return fmt.Errorf("unauthorized heartbeat")
	}
	s.board.RefreshTaskLease(taskID)
	return nil
}

// mainAgentCoordinatorImpl 是主代理的实现。
// 它持有所有核心组件的引用。
type mainAgentCoordinatorImpl struct {
	board     *KanbanBoard
	cronSvc   *cron.CronService
	msgBus    *bus.MessageBus
	agentID   string
}

func NewMainAgentCoordinator(board *KanbanBoard, cronSvc *cron.CronService, msgBus *bus.MessageBus) MainAgentCoordinator {
	return &mainAgentCoordinatorImpl{
		board:   board,
		cronSvc: cronSvc,
		msgBus:  msgBus,
		agentID: "MAIN_AGENT",
	}
}

// 实现 SubAgentTaskClient 接口 (委托给 board)
func (m *mainAgentCoordinatorImpl) ClaimTask(ctx context.Context, taskType string) (*Task, error) {
	return m.board.ClaimPendingTaskByType(m.agentID, taskType)
}
func (m *mainAgentCoordinatorImpl) UpdateStatus(ctx context.Context, taskID string, status TaskStatus, result map[string]interface{}) error {
	return m.board.UpdateTaskStatus(taskID, status, result)
}
func (m *mainAgentCoordinatorImpl) Heartbeat(ctx context.Context, taskID string) error {
	m.board.RefreshTaskLease(taskID)
	return nil
}

// 实现 MainAgentCoordinator 特有接口
func (m *mainAgentCoordinatorImpl) GetCronService() *cron.CronService {
	return m.cronSvc
}

func (m *mainAgentCoordinatorImpl) ScheduleJob(spec string, job func()) error {
	_, err := m.cronSvc.AddJob(spec, cron.FuncJob(job))
	return err
}

func (m *mainAgentCoordinatorImpl) GetMessageBus() *bus.MessageBus {
	return m.msgBus
}

func (m *mainAgentCoordinatorImpl) PublishToChannels(eventType string, payload interface{}) error {
	// 主代理通过 MessageBus 广播，由 Channel 模块监听并推送给用户
	return m.msgBus.Publish("channel.broadcast", map[string]interface{}{
		"type":    eventType,
		"payload": payload,
		"source":  "MAIN_AGENT",
	})
}

func (m *mainAgentCoordinatorImpl) GetBoardSnapshot() *KanbanBoard {
	return m.board
}

func (m *mainAgentCoordinatorImpl) ForceAssignTask(taskID string, agentID string) error {
	return m.board.ForceAssign(taskID, agentID)
}
