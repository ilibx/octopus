// Package observability 提供轻量级指标收集器
// 无需 Prometheus 依赖，内存统计 QPS/延迟/错误率
package observability

import (
	"sync"
	"time"
)

// Metrics 指标收集器
type Metrics struct {
	mu sync.RWMutex

	// 计数器
	totalRequests   int64
	successfulReqs  int64
	failedReqs      int64
	activeWorkers   int64

	// 延迟统计（毫秒）
	latencies []float64
	maxLatencies int // 保留的最大延迟样本数

	// 时间窗口统计
	windowStart    time.Time
	windowRequests int64

	// 熔断器状态
	circuitBreakerStatus string

	// 任务队列深度
	queueDepth int64
}

// NewMetrics 创建新的指标收集器
func NewMetrics() *Metrics {
	return &Metrics{
		latencies:            make([]float64, 0),
		maxLatencies:         1000,
		windowStart:          time.Now(),
		circuitBreakerStatus: "closed",
	}
}

// RecordRequest 记录请求
func (m *Metrics) RecordRequest(success bool, latencyMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests++
	if success {
		m.successfulReqs++
	} else {
		m.failedReqs++
	}

	// 记录延迟
	m.latencies = append(m.latencies, latencyMs)
	if len(m.latencies) > m.maxLatencies {
		// 移除最旧的样本
		m.latencies = m.latencies[1:]
	}

	// 更新窗口计数
	m.windowRequests++
}

// RecordActiveWorkers 记录活跃工作线程数
func (m *Metrics) RecordActiveWorkers(count int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeWorkers = count
}

// SetCircuitBreakerStatus 设置熔断器状态
func (m *Metrics) SetCircuitBreakerStatus(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.circuitBreakerStatus = status
}

// RecordQueueDepth 记录队列深度
func (m *Metrics) RecordQueueDepth(depth int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueDepth = depth
}

// GetQPS 获取每秒请求数（基于时间窗口）
func (m *Metrics) GetQPS() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	elapsed := time.Since(m.windowStart).Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(m.windowRequests) / elapsed
}

// ResetWindow 重置时间窗口（用于定期统计）
func (m *Metrics) ResetWindow() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.windowRequests = 0
	m.windowStart = time.Now()
}

// GetP50Latency 获取 P50 延迟
func (m *Metrics) GetP50Latency() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.latencies) == 0 {
		return 0
	}

	sorted := make([]float64, len(m.latencies))
	copy(sorted, m.latencies)
	sortFloat64s(sorted)

	idx := len(sorted) / 2
	return sorted[idx]
}

// GetP99Latency 获取 P99 延迟
func (m *Metrics) GetP99Latency() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.latencies) == 0 {
		return 0
	}

	sorted := make([]float64, len(m.latencies))
	copy(sorted, m.latencies)
	sortFloat64s(sorted)

	idx := int(float64(len(sorted)-1) * 0.99)
	return sorted[idx]
}

// GetErrorRate 获取错误率
func (m *Metrics) GetErrorRate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.totalRequests == 0 {
		return 0
	}

	return float64(m.failedReqs) / float64(m.totalRequests)
}

// GetAllStats 获取所有统计信息
func (m *Metrics) GetAllStats() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	errorRate := 0.0
	if m.totalRequests > 0 {
		errorRate = float64(m.failedReqs) / float64(m.totalRequests)
	}

	qps := 0.0
	elapsed := time.Since(m.windowStart).Seconds()
	if elapsed > 0 {
		qps = float64(m.windowRequests) / elapsed
	}

	p50 := 0.0
	p99 := 0.0
	if len(m.latencies) > 0 {
		sorted := make([]float64, len(m.latencies))
		copy(sorted, m.latencies)
		sortFloat64s(sorted)

		p50 = sorted[len(sorted)/2]
		p99Idx := int(float64(len(sorted)-1) * 0.99)
		p99 = sorted[p99Idx]
	}

	return map[string]any{
		"total_requests":       m.totalRequests,
		"successful_requests":  m.successfulReqs,
		"failed_requests":      m.failedReqs,
		"error_rate":           errorRate,
		"qps":                  qps,
		"p50_latency_ms":       p50,
		"p99_latency_ms":       p99,
		"active_workers":       m.activeWorkers,
		"circuit_breaker_status": m.circuitBreakerStatus,
		"queue_depth":          m.queueDepth,
		"window_elapsed_sec":   elapsed,
	}
}

// GetSummary 获取简要摘要
func (m *Metrics) GetSummary() string {
	stats := m.GetAllStats()
	return formatSummary(stats)
}

// Reset 重置所有指标
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests = 0
	m.successfulReqs = 0
	m.failedReqs = 0
	m.activeWorkers = 0
	m.latencies = make([]float64, 0)
	m.windowStart = time.Now()
	m.windowRequests = 0
	m.queueDepth = 0
}

// sortFloat64s 简单排序辅助函数
func sortFloat64s(s []float64) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

// formatSummary 格式化摘要字符串
func formatSummary(stats map[string]any) string {
	return ""
}
