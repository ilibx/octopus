// Package circuitbreaker 提供熔断器实现，用于保护 LLM API 调用
// 当连续失败次数超过阈值时自动熔断，防止雪崩效应
package circuitbreaker

import (
	"errors"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/logger"
)

// State 表示熔断器的状态
type State int

const (
	// Closed 正常状态，允许请求通过
	Closed State = iota
	// Open 熔断状态，拒绝所有请求
	Open
	// HalfOpen 半开状态，允许一个探测请求
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

var (
	// ErrOpenState 熔断器打开时的错误
	ErrOpenState = errors.New("circuit breaker is open")
	// ErrHalfOpenState 熔断器半开时的错误（仅允许一个请求）
	ErrHalfOpenState = errors.New("circuit breaker is half-open, only one request allowed")
)

// Config 熔断器配置
type Config struct {
	// Name 熔断器名称（用于日志和监控）
	Name string `json:"name"`
	// FailureThreshold 连续失败次数阈值，达到后触发熔断
	FailureThreshold int `json:"failure_threshold"`
	// SuccessThreshold 半开状态下成功次数阈值，达到后关闭熔断
	SuccessThreshold int `json:"success_threshold"`
	// Timeout 熔断持续时间，超时后进入半开状态
	Timeout time.Duration `json:"timeout"`
	// HalfOpenMaxRequests 半开状态下允许的最大并发请求数
	HalfOpenMaxRequests int `json:"half_open_max_requests"`
}

// DefaultConfig 返回默认配置
func DefaultConfig(name string) Config {
	return Config{
		Name:                name,
		FailureThreshold:    5,
		SuccessThreshold:    2,
		Timeout:             30 * time.Second,
		HalfOpenMaxRequests: 1,
	}
}

// Breaker 熔断器结构体
type Breaker struct {
	config Config
	state  State
	mu     sync.RWMutex

	// 计数器
	failureCount int
	successCount int

	// 时间记录
	lastFailureTime time.Time
	openedAt        time.Time

	// 半开状态下的请求计数
	halfOpenRequests int
}

// NewBreaker 创建新的熔断器
func NewBreaker(config Config) *Breaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 2
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.HalfOpenMaxRequests <= 0 {
		config.HalfOpenMaxRequests = 1
	}

	return &Breaker{
		config: config,
		state:  Closed,
	}
}

// NewBreakerSimple 简化版创建函数
func NewBreakerSimple(name string, failureThreshold int, timeout time.Duration) *Breaker {
	return NewBreaker(Config{
		Name:             name,
		FailureThreshold: failureThreshold,
		Timeout:          timeout,
	})
}

// State 获取当前状态
func (b *Breaker) State() State {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// 检查是否需要从 Open 状态转换到 HalfOpen
	if b.state == Open {
		if time.Since(b.openedAt) >= b.config.Timeout {
			return HalfOpen
		}
		return Open
	}

	return b.state
}

// Allow 检查是否允许请求通过
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case Closed:
		return true

	case Open:
		// 检查是否超时
		if time.Since(b.openedAt) >= b.config.Timeout {
			logger.InfoCF("circuitbreaker", "Transitioning to half-open state",
				map[string]any{"name": b.config.Name})
			b.state = HalfOpen
			b.halfOpenRequests = 0
			return true
		}
		return false

	case HalfOpen:
		// 半开状态下限制并发请求数
		if b.halfOpenRequests < b.config.HalfOpenMaxRequests {
			b.halfOpenRequests++
			return true
		}
		return false

	default:
		return false
	}
}

// RecordSuccess 记录成功
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case Closed:
		// 重置失败计数
		b.failureCount = 0

	case HalfOpen:
		b.successCount++
		if b.successCount >= b.config.SuccessThreshold {
			logger.InfoCF("circuitbreaker", "Circuit breaker closed after successful probes",
				map[string]any{"name": b.config.Name, "success_count": b.successCount})
			b.state = Closed
			b.failureCount = 0
			b.successCount = 0
			b.halfOpenRequests = 0
		}
	}
}

// RecordFailure 记录失败
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failureCount++
	b.lastFailureTime = time.Now()

	switch b.state {
	case Closed:
		if b.failureCount >= b.config.FailureThreshold {
			logger.WarnCF("circuitbreaker", "Circuit breaker opened due to consecutive failures",
				map[string]any{
					"name":           b.config.Name,
					"failure_count":  b.failureCount,
					"threshold":      b.config.FailureThreshold,
				})
			b.state = Open
			b.openedAt = time.Now()
		}

	case HalfOpen:
		// 半开状态下任何失败都会重新打开熔断
		logger.WarnCF("circuitbreaker", "Circuit breaker re-opened after failure in half-open state",
			map[string]any{"name": b.config.Name})
		b.state = Open
		b.openedAt = time.Now()
		b.successCount = 0
		b.halfOpenRequests = 0
	}
}

// Execute 执行函数，自动处理熔断逻辑
func (b *Breaker) Execute(fn func() error) error {
	if !b.Allow() {
		return ErrOpenState
	}

	err := fn()
	if err != nil {
		b.RecordFailure()
		return err
	}

	b.RecordSuccess()
	return nil
}

// ExecuteWithFallback 执行函数，失败时使用降级函数
func (b *Breaker) ExecuteWithFallback(fn func() error, fallback func() error) error {
	if !b.Allow() {
		logger.DebugCF("circuitbreaker", "Executing fallback due to open circuit",
			map[string]any{"name": b.config.Name})
		if fallback != nil {
			return fallback()
		}
		return ErrOpenState
	}

	err := fn()
	if err != nil {
		b.RecordFailure()
		if fallback != nil {
			logger.DebugCF("circuitbreaker", "Executing fallback after function failure",
				map[string]any{"name": b.config.Name, "error": err.Error()})
			return fallback()
		}
		return err
	}

	b.RecordSuccess()
	return nil
}

// Reset 手动重置熔断器
func (b *Breaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	logger.InfoCF("circuitbreaker", "Circuit breaker manually reset",
		map[string]any{"name": b.config.Name})

	b.state = Closed
	b.failureCount = 0
	b.successCount = 0
	b.halfOpenRequests = 0
	b.openedAt = time.Time{}
}

// Stats 获取熔断器统计信息
func (b *Breaker) Stats() map[string]any {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return map[string]any{
		"name":               b.config.Name,
		"state":              b.State().String(),
		"failure_count":      b.failureCount,
		"success_count":      b.successCount,
		"failure_threshold":  b.config.FailureThreshold,
		"success_threshold":  b.config.SuccessThreshold,
		"timeout_seconds":    b.config.Timeout.Seconds(),
		"last_failure_time":  b.lastFailureTime,
		"opened_at":          b.openedAt,
		"half_open_requests": b.halfOpenRequests,
	}
}

// IsOpen 检查熔断器是否打开
func (b *Breaker) IsOpen() bool {
	return b.State() == Open
}

// IsClosed 检查熔断器是否关闭
func (b *Breaker) IsClosed() bool {
	return b.State() == Closed
}

// IsHalfOpen 检查熔断器是否半开
func (b *Breaker) IsHalfOpen() bool {
	return b.State() == HalfOpen
}
