package unit

import (
	"fmt"
	"strings"
	"sync"

	"ergo.services/ergo/lib"
)

type failures struct {
	mu        sync.RWMutex
	rules     map[string]*failureRule
	counters  map[string]int // method call counts
	events    lib.QueueMPSC
	component string
}

// failureRule defines how a method should fail
type failureRule struct {
	Error     error
	AfterCall int    // fail after N calls (0 = immediate, removes rule after first failure)
	Pattern   string // optional pattern matching for args
}

func newFailures(events lib.QueueMPSC, component string) *failures {
	return &failures{
		rules:     make(map[string]*failureRule),
		counters:  make(map[string]int),
		events:    events,
		component: component,
	}
}

type Failures interface {
	SetMethodFailure(method string, err error)
	SetMethodFailureAfter(method string, afterCalls int, err error)
	SetMethodFailureOnce(method string, err error)
	SetMethodFailurePattern(method, pattern string, err error)
	ClearMethodFailures()
	ClearMethodFailure(method string)
	GetMethodCallCount(method string) int
	CheckMethodFailure(method string, args ...any) error
}

// SetMethodFailure configures a method to fail with the given error
func (f *failures) SetMethodFailure(method string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules[method] = &failureRule{Error: err}
}

// SetMethodFailureAfter configures a method to fail after N calls
func (f *failures) SetMethodFailureAfter(method string, afterCalls int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules[method] = &failureRule{
		Error:     err,
		AfterCall: afterCalls,
	}
}

// SetMethodFailureOnce configures a method to fail only once
func (f *failures) SetMethodFailureOnce(method string, err error) {
	f.SetMethodFailureAfter(method, 0, err)
}

// SetMethodFailurePattern configures a method to fail when args match pattern
func (f *failures) SetMethodFailurePattern(method, pattern string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules[method] = &failureRule{
		Error:   err,
		Pattern: pattern,
	}
}

// CheckMethodFailure checks if a method should fail and returns the error
func (f *failures) CheckMethodFailure(method string, args ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	rule, exists := f.rules[method]
	if !exists {
		return nil
	}

	// call counter
	f.counters[method]++

	// check if we should fail after N calls
	if rule.AfterCall > 0 && f.counters[method] <= rule.AfterCall {
		return nil // Not yet time to fail
	}

	// check pattern matching
	if rule.Pattern != "" && !f.matchesPattern(rule.Pattern, args...) {
		return nil // Pattern doesn't match
	}

	// remove rule if it's a one-time failure (AfterCall = 0)
	if rule.AfterCall == 0 {
		delete(f.rules, method)
	}

	// log failure event
	if f.events != nil {
		f.events.Push(FailureEvent{
			Component: f.component,
			Method:    method,
			Args:      args,
			Error:     rule.Error,
		})
	}

	return rule.Error
}

// ClearMethodFailures removes all configured failures
func (f *failures) ClearMethodFailures() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules = make(map[string]*failureRule)
	f.counters = make(map[string]int)
}

// ClearMethodFailure removes a specific method failure
func (f *failures) ClearMethodFailure(method string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rules, method)
	delete(f.counters, method)
}

// GetMethodCallCount returns how many times a method has been called
func (f *failures) GetMethodCallCount(method string) int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.counters[method]
}

// matchesPattern checks if args match the failure pattern
func (f *failures) matchesPattern(pattern string, args ...any) bool {
	if len(args) == 0 {
		return pattern == ""
	}

	// simple pattern matching - can be enhanced
	argStr := fmt.Sprintf("%v", args)
	return strings.Contains(argStr, pattern)
}

// FailureEvent captures when a method failure is triggered
type FailureEvent struct {
	Component string // "node", "process", "network", "registrar", "log"
	Method    string
	Args      []any
	Error     error
}

func (e FailureEvent) Type() string {
	return "method_failure"
}

func (e FailureEvent) String() string {
	return fmt.Sprintf("FailureEvent(component=%s, method=%s, error=%s)", e.Component, e.Method, e.Error)
}
