// Package queue 提供优先级任务队列实现
// 支持高/中/低三级优先级调度，高优先级任务可插队
package queue

import (
	"container/heap"
	"sync"
	"time"
)

// Priority 表示任务优先级
type Priority int

const (
	// PriorityHigh 高优先级（紧急任务）
	PriorityHigh Priority = 1
	// PriorityNormal 普通优先级（默认）
	PriorityNormal Priority = 2
	// PriorityLow 低优先级（后台任务）
	PriorityLow Priority = 3
)

func (p Priority) String() string {
	switch p {
	case PriorityHigh:
		return "high"
	case PriorityNormal:
		return "normal"
	case PriorityLow:
		return "low"
	default:
		return "unknown"
	}
}

// Item 队列项
type Item struct {
	ID         string    `json:"id"`
	Priority   Priority  `json:"priority"`
	Data       any       `json:"data,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	index      int       // heap 内部使用
}

// PriorityQueue 优先级队列（最小堆）
type PriorityQueue struct {
	items []*Item
	mu    sync.RWMutex
}

// Len 返回队列长度
func (pq *PriorityQueue) Len() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.items)
}

// Less 比较两个元素的优先级（数字越小优先级越高）
func (pq *PriorityQueue) Less(i, j int) bool {
	// 先比较优先级
	if pq.items[i].Priority != pq.items[j].Priority {
		return pq.items[i].Priority < pq.items[j].Priority
	}
	// 优先级相同时，先创建的优先
	return pq.items[i].CreatedAt.Before(pq.items[j].CreatedAt)
}

// Swap 交换两个元素
func (pq *PriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
	pq.items[i].index = i
	pq.items[j].index = j
}

// Push 添加元素
func (pq *PriorityQueue) Push(x any) {
	n := len(pq.items)
	item := x.(*Item)
	item.index = n
	pq.items = append(pq.items, item)
}

// Pop 弹出元素
func (pq *PriorityQueue) Pop() any {
	old := pq.items
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // 避免内存泄漏
	item.index = -1
	pq.items = old[0 : n-1]
	return item
}

// NewPriorityQueue 创建新的优先级队列
func NewPriorityQueue() *PriorityQueue {
	pq := &PriorityQueue{
		items: make([]*Item, 0),
	}
	heap.Init(pq)
	return pq
}

// Enqueue 入队
func (pq *PriorityQueue) Enqueue(id string, priority Priority, data any) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	item := &Item{
		ID:        id,
		Priority:  priority,
		Data:      data,
		CreatedAt: time.Now(),
	}
	heap.Push(pq, item)
}

// Dequeue 出队（返回优先级最高的元素）
func (pq *PriorityQueue) Dequeue() *Item {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return nil
	}

	item := heap.Pop(pq).(*Item)
	return item
}

// Peek 查看队首元素（不出队）
func (pq *PriorityQueue) Peek() *Item {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if len(pq.items) == 0 {
		return nil
	}

	return pq.items[0]
}

// Remove 移除指定 ID 的元素
func (pq *PriorityQueue) Remove(id string) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for i, item := range pq.items {
		if item.ID == id {
			heap.Remove(pq, i)
			return true
		}
	}
	return false
}

// Size 返回队列大小
func (pq *PriorityQueue) Size() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.items)
}

// IsEmpty 检查队列是否为空
func (pq *PriorityQueue) IsEmpty() bool {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.items) == 0
}

// Clear 清空队列
func (pq *PriorityQueue) Clear() {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.items = make([]*Item, 0)
}

// List 列出所有元素（用于调试）
func (pq *PriorityQueue) List() []*Item {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	result := make([]*Item, len(pq.items))
	for i, item := range pq.items {
		result[i] = item
	}
	return result
}

// Stats 返回队列统计信息
func (pq *PriorityQueue) Stats() map[string]any {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	highCount := 0
	normalCount := 0
	lowCount := 0

	for _, item := range pq.items {
		switch item.Priority {
		case PriorityHigh:
			highCount++
		case PriorityNormal:
			normalCount++
		case PriorityLow:
			lowCount++
		}
	}

	return map[string]any{
		"total":        len(pq.items),
		"high_count":   highCount,
		"normal_count": normalCount,
		"low_count":    lowCount,
	}
}
