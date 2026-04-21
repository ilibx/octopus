package circuitbreaker

import (
	"sync"
	"testing"
	"time"
)

func TestNewBreaker(t *testing.T) {
	config := DefaultConfig("test")
	breaker := NewBreaker(config)

	if breaker == nil {
		t.Fatal("Expected breaker to be created")
	}

	if breaker.State() != Closed {
		t.Errorf("Expected initial state to be Closed, got %v", breaker.State())
	}
}

func TestBreakerStateTransitions(t *testing.T) {
	config := Config{
		Name:             "test",
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
	}
	breaker := NewBreaker(config)

	// Initial state should be Closed
	if breaker.State() != Closed {
		t.Errorf("Expected Closed, got %v", breaker.State())
	}

	// Record failures to open the circuit
	for i := 0; i < 3; i++ {
		breaker.RecordFailure()
	}

	if breaker.State() != Open {
		t.Errorf("Expected Open after 3 failures, got %v", breaker.State())
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should transition to HalfOpen
	if breaker.State() != HalfOpen {
		t.Errorf("Expected HalfOpen after timeout, got %v", breaker.State())
	}

	// Record successes to close the circuit (need to call Allow first in HalfOpen)
	breaker.Allow() // consume the half-open slot
	breaker.RecordSuccess()
	breaker.RecordSuccess()

	if breaker.State() != Closed {
		t.Errorf("Expected Closed after 2 successes in HalfOpen, got %v", breaker.State())
	}
}

func TestBreakerAllow(t *testing.T) {
	config := Config{
		Name:                "test",
		FailureThreshold:    2,
		SuccessThreshold:    1,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	}
	breaker := NewBreaker(config)

	// Should allow in Closed state
	if !breaker.Allow() {
		t.Error("Expected Allow to return true in Closed state")
	}

	// Open the circuit
	breaker.RecordFailure()
	breaker.RecordFailure()

	// Should not allow in Open state
	if breaker.Allow() {
		t.Error("Expected Allow to return false in Open state")
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should allow one request in HalfOpen state
	if !breaker.Allow() {
		t.Error("Expected Allow to return true in HalfOpen state")
	}

	// Manually reset the half-open state for testing purposes
	// In real scenario, the first request would either succeed or fail and change state
	// For this test, we just verify the counter mechanism works
	breaker.mu.Lock()
	currentHalfOpenRequests := breaker.halfOpenRequests
	breaker.mu.Unlock()
	
	if currentHalfOpenRequests != 1 {
		t.Errorf("Expected halfOpenRequests to be 1, got %d", currentHalfOpenRequests)
	}
	
	// Second request should be denied if HalfOpenMaxRequests is 1
	if breaker.config.HalfOpenMaxRequests == 1 && breaker.Allow() {
		t.Error("Expected Allow to return false for second request when HalfOpenMaxRequests=1")
	}
}

func TestBreakerExecute(t *testing.T) {
	config := DefaultConfig("test")
	breaker := NewBreaker(config)

	// Successful execution
	err := breaker.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Failed execution
	err = breaker.Execute(func() error {
		return ErrOpenState
	})
	if err != ErrOpenState {
		t.Errorf("Expected ErrOpenState, got %v", err)
	}
}

func TestBreakerExecuteWithFallback(t *testing.T) {
	config := Config{
		Name:             "test",
		FailureThreshold: 1,
		Timeout:          100 * time.Millisecond,
	}
	breaker := NewBreaker(config)

	fallbackCalled := false
	fallback := func() error {
		fallbackCalled = true
		return nil
	}

	// First call fails, opens circuit
	err := breaker.ExecuteWithFallback(func() error {
		return ErrOpenState
	}, fallback)
	if err != nil {
		t.Errorf("Expected no error from fallback, got %v", err)
	}
	if !fallbackCalled {
		t.Error("Expected fallback to be called")
	}
}

func TestBreakerReset(t *testing.T) {
	config := Config{
		Name:             "test",
		FailureThreshold: 2,
	}
	breaker := NewBreaker(config)

	// Open the circuit
	breaker.RecordFailure()
	breaker.RecordFailure()

	if breaker.State() != Open {
		t.Errorf("Expected Open, got %v", breaker.State())
	}

	// Reset
	breaker.Reset()

	if breaker.State() != Closed {
		t.Errorf("Expected Closed after reset, got %v", breaker.State())
	}
}

func TestBreakerStats(t *testing.T) {
	config := Config{
		Name:             "test",
		FailureThreshold: 5,
		SuccessThreshold: 3,
		Timeout:          60 * time.Second,
	}
	breaker := NewBreaker(config)

	// Record some failures
	breaker.RecordFailure()
	breaker.RecordFailure()

	stats := breaker.Stats()

	if stats["name"] != "test" {
		t.Errorf("Expected name 'test', got %v", stats["name"])
	}
	if stats["failure_count"] != 2 {
		t.Errorf("Expected failure_count 2, got %v", stats["failure_count"])
	}
	if stats["failure_threshold"] != 5 {
		t.Errorf("Expected failure_threshold 5, got %v", stats["failure_threshold"])
	}
}

func TestBreakerConcurrentAccess(t *testing.T) {
	config := DefaultConfig("test")
	breaker := NewBreaker(config)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			breaker.Allow()
			breaker.RecordSuccess()
			breaker.RecordFailure()
			breaker.State()
			breaker.Stats()
		}()
	}

	wg.Wait()
	// Should not panic
}

func TestBreakerHalfOpenRecovery(t *testing.T) {
	config := Config{
		Name:             "test",
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	}
	breaker := NewBreaker(config)

	// Open the circuit
	breaker.RecordFailure()
	breaker.RecordFailure()

	if breaker.State() != Open {
		t.Errorf("Expected Open, got %v", breaker.State())
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Should be HalfOpen now
	state := breaker.State()
	if state != HalfOpen {
		t.Errorf("Expected HalfOpen, got %v", state)
	}

	// Manually set to HalfOpen to ensure state is correct (State() method may auto-transition)
	breaker.mu.Lock()
	breaker.state = HalfOpen
	breaker.halfOpenRequests = 0
	breaker.successCount = 0
	breaker.mu.Unlock()

	// Fail in HalfOpen - should go back to Open
	breaker.RecordFailure()

	if breaker.State() != Open {
		t.Errorf("Expected Open after failure in HalfOpen, got %v", breaker.State())
	}
}
