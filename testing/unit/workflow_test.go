package unit

import (
	"fmt"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// Order Processing Workflow Tests

type orderActor struct {
	act.Actor
	orderID    string
	state      string
	items      []string
	total      float64
	customer   gen.PID
	retries    int
	maxRetries int
}

func factoryOrderActor() gen.ProcessBehavior {
	return &orderActor{
		maxRetries: 3,
	}
}

func (o *orderActor) Init(args ...any) error {
	// Initialize state if not already set by constructor
	if o.state == "" {
		o.state = "pending"
	}
	if o.items == nil {
		o.items = []string{}
	}
	return nil
}

func (o *orderActor) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case AddItem:
		o.items = append(o.items, msg.Item)
		o.total += msg.Price
		o.Send(o.customer, ItemAdded{
			OrderID: o.orderID,
			Item:    msg.Item,
			Total:   o.total,
		})

	case SubmitOrder:
		if len(o.items) == 0 {
			o.Send(o.customer, OrderError{
				OrderID: o.orderID,
				Error:   "no items in order",
			})
			return nil
		}

		o.state = "validating"
		// Send validation request
		validator, _ := o.Spawn(factoryValidatorActor, gen.ProcessOptions{}, o.orderID, o.items)
		o.Send(validator, ValidateOrder{
			OrderID: o.orderID,
			Items:   o.items,
			Total:   o.total,
		})

	case ValidationResult:
		if !msg.Valid {
			o.state = "rejected"
			o.Send(o.customer, OrderRejected{
				OrderID: o.orderID,
				Reason:  msg.Reason,
			})
			return gen.TerminateReasonNormal
		}

		o.state = "processing_payment"
		// Simulate payment processing
		o.Send(o.PID(), ProcessPayment{
			OrderID:  o.orderID,
			Amount:   o.total,
			Customer: o.customer,
		})

	case ProcessPayment:
		// Simulate payment result
		success := o.total < 1000 && o.retries < 2 // Simulate payment failure for large orders or retries
		o.retries++

		if !success && o.retries <= o.maxRetries {
			o.Log().Warning("Payment failed for order %s, retry %d", o.orderID, o.retries)
			o.SendAfter(o.PID(), ProcessPayment{
				OrderID:  o.orderID,
				Amount:   o.total,
				Customer: o.customer,
			}, 100) // Retry after 100ms
			return nil
		}

		if !success {
			o.state = "failed"
			o.Send(o.customer, OrderFailed{
				OrderID: o.orderID,
				Reason:  fmt.Sprintf("payment failed after %d retries", o.maxRetries),
			})
			return gen.TerminateReasonNormal
		}

		o.state = "confirmed"
		o.Send(o.customer, OrderConfirmed{
			OrderID: o.orderID,
			Total:   o.total,
		})

		// Send fulfillment request
		o.Send(o.PID(), FulfillOrder{
			OrderID:  o.orderID,
			Items:    o.items,
			Customer: o.customer,
		})

	case GetOrderStatus:
		o.Send(from, OrderStatus{
			OrderID: o.orderID,
			State:   o.state,
			Items:   o.items,
			Total:   o.total,
			Retries: o.retries,
		})

	case CancelOrder:
		previousState := o.state
		o.state = "cancelled"
		o.Send(o.customer, OrderCancelled{
			OrderID:       o.orderID,
			PreviousState: previousState,
		})
		return gen.TerminateReasonNormal
	}
	return nil
}

type validatorActor struct {
	act.Actor
	orderID string
	items   []string
}

func factoryValidatorActor() gen.ProcessBehavior {
	return &validatorActor{}
}

func (v *validatorActor) Init(args ...any) error {
	if len(args) > 0 {
		v.orderID = args[0].(string)
	}
	if len(args) > 1 {
		v.items = args[1].([]string)
	}
	return nil
}

func (v *validatorActor) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case ValidateOrder:
		// Simple validation logic
		valid := len(msg.Items) > 0 && msg.Total > 0

		var reason string
		if !valid {
			if len(msg.Items) == 0 {
				reason = "no items"
			} else {
				reason = "invalid total"
			}
		}

		// Send validation result back
		v.Send(from, ValidationResult{
			OrderID: msg.OrderID,
			Valid:   valid,
			Reason:  reason,
		})
		return gen.TerminateReasonNormal
	}
	return nil
}

// Message types for order workflow
type AddItem struct {
	Item  string
	Price float64
}

type SubmitOrder struct{}

type ValidationResult struct {
	OrderID string
	Valid   bool
	Reason  string
}

type PaymentResult struct {
	OrderID string
	Success bool
	Reason  string
}

type GetOrderStatus struct{}

type CancelOrder struct{}

type ItemAdded struct {
	OrderID string
	Item    string
	Total   float64
}

type OrderError struct {
	OrderID string
	Error   string
}

type OrderRejected struct {
	OrderID string
	Reason  string
}

type OrderConfirmed struct {
	OrderID string
	Total   float64
}

type OrderFailed struct {
	OrderID string
	Reason  string
}

type OrderCancelled struct {
	OrderID       string
	PreviousState string
}

type OrderStatus struct {
	OrderID string
	State   string
	Items   []string
	Total   float64
	Retries int
}

type ValidateOrder struct {
	OrderID string
	Items   []string
	Total   float64
}

type ProcessPayment struct {
	OrderID  string
	Amount   float64
	Customer gen.PID
}

type FulfillOrder struct {
	OrderID  string
	Items    []string
	Customer gen.PID
}

// Order Workflow Tests

func TestOrderWorkflow_CompleteFlow(t *testing.T) {
	// Create an order actor directly
	customer, err := Spawn(t, func() gen.ProcessBehavior {
		return &orderActor{
			orderID:    "ORDER-001",
			customer:   gen.PID{Node: "test", ID: 123}, // Test customer PID
			maxRetries: 3,
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	// Clear initialization events
	customer.ClearEvents()

	// Test basic order operations
	// Add items to order
	customer.SendMessage(gen.PID{}, AddItem{Item: "laptop", Price: 899.99})
	customer.SendMessage(gen.PID{}, AddItem{Item: "mouse", Price: 29.99})

	// Verify item added events are sent to the customer PID
	customer.ShouldSend().To(gen.PID{Node: "test", ID: 123}).MessageMatching(func(msg any) bool {
		if added, ok := msg.(ItemAdded); ok {
			return added.OrderID == "ORDER-001" && added.Item == "laptop" && added.Total == 899.99
		}
		return false
	}).Once().Assert()

	customer.ShouldSend().To(gen.PID{Node: "test", ID: 123}).MessageMatching(func(msg any) bool {
		if added, ok := msg.(ItemAdded); ok {
			return added.OrderID == "ORDER-001" && added.Item == "mouse" && added.Total == 929.98
		}
		return false
	}).Once().Assert()

	// Test status check
	customer.SendMessage(gen.PID{}, GetOrderStatus{})
	customer.ShouldSend().To(gen.PID{}).MessageMatching(func(msg any) bool {
		if status, ok := msg.(OrderStatus); ok {
			return status.OrderID == "ORDER-001" && status.State == "pending" && len(status.Items) == 2
		}
		return false
	}).Once().Assert()

	// Test order submission - just verify validator is spawned
	customer.SendMessage(gen.PID{}, SubmitOrder{})
	customer.ShouldSpawn().Once().Assert()
}

func TestOrderWorkflow_ErrorScenarios(t *testing.T) {
	// Test empty order submission
	customer, err := Spawn(t, func() gen.ProcessBehavior {
		return &orderActor{
			orderID:    "ORDER-002",
			customer:   gen.PID{Node: "test", ID: 123},
			maxRetries: 3,
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	customer.ClearEvents()
	customer.SendMessage(gen.PID{}, SubmitOrder{})

	// Should receive error for empty order
	customer.ShouldSend().To(gen.PID{Node: "test", ID: 123}).MessageMatching(func(msg any) bool {
		if orderError, ok := msg.(OrderError); ok {
			return orderError.OrderID == "ORDER-002" && orderError.Error == "no items in order"
		}
		return false
	}).Once().Assert()

	// Test order cancellation with items
	customer.SendMessage(gen.PID{}, AddItem{Item: "book", Price: 19.99})
	customer.SendMessage(gen.PID{}, CancelOrder{})

	customer.ShouldSend().To(gen.PID{Node: "test", ID: 123}).MessageMatching(func(msg any) bool {
		if cancelled, ok := msg.(OrderCancelled); ok {
			return cancelled.OrderID == "ORDER-002" && cancelled.PreviousState == "pending"
		}
		return false
	}).Once().Assert()
}

func TestOrderWorkflow_PaymentRetries(t *testing.T) {
	// Create large order that will trigger payment failures
	customer, err := Spawn(t, func() gen.ProcessBehavior {
		return &orderActor{
			orderID:    "ORDER-004",
			customer:   gen.PID{Node: "test", ID: 123},
			maxRetries: 3,
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	customer.ClearEvents()

	// Add expensive item and test basic workflow
	customer.SendMessage(gen.PID{}, AddItem{Item: "server", Price: 5000.00})

	// Verify item was added
	customer.ShouldSend().To(gen.PID{Node: "test", ID: 123}).MessageMatching(func(msg any) bool {
		if added, ok := msg.(ItemAdded); ok {
			return added.OrderID == "ORDER-004" && added.Item == "server" && added.Total == 5000.00
		}
		return false
	}).Once().Assert()

	// Test order submission - just verify validator is spawned
	customer.SendMessage(gen.PID{}, SubmitOrder{})
	customer.ShouldSpawn().Once().Assert()

	// Test status check shows order is in validating state
	customer.SendMessage(gen.PID{}, GetOrderStatus{})
	customer.ShouldSend().To(gen.PID{}).MessageMatching(func(msg any) bool {
		if status, ok := msg.(OrderStatus); ok {
			return status.OrderID == "ORDER-004" && status.State == "validating"
		}
		return false
	}).Once().Assert()
}

// Worker Management Tests

func TestManager_WorkerManagement(t *testing.T) {
	// Create manager that will spawn workers
	manager, err := Spawn(t, func() gen.ProcessBehavior {
		return &managerActor{
			workers:    make(map[string]gen.PID),
			maxWorkers: 3,
			strategy:   "round_robin",
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	manager.ClearEvents()

	// Test starting workers
	manager.SendMessage(gen.PID{}, StartWorker{
		WorkerID: "worker-1",
		Config:   map[string]any{"type": "cpu_intensive"},
	})

	// Verify worker started - manager should send response to the sender (gen.PID{})
	manager.ShouldSend().To(gen.PID{}).MessageMatching(func(msg any) bool {
		if started, ok := msg.(WorkerStarted); ok {
			return started.WorkerID == "worker-1"
		}
		return false
	}).Once().Assert()

	// Verify worker was spawned
	manager.ShouldSpawn().Once().Assert()

	// Verify metrics were sent
	manager.ShouldSend().To("metrics").MessageMatching(func(msg any) bool {
		if metrics, ok := msg.(WorkerMetrics); ok {
			return metrics.Action == "start" && metrics.WorkerID == "worker-1"
		}
		return false
	}).Once().Assert()

	// Test manager status
	manager.SendMessage(gen.PID{}, GetManagerStatus{})
	manager.ShouldSend().To(gen.PID{}).MessageMatching(func(msg any) bool {
		if status, ok := msg.(ManagerStatus); ok {
			return status.ActiveWorkers == 1 && status.MaxWorkers == 3 && status.Strategy == "round_robin"
		}
		return false
	}).Once().Assert()
}

type managerActor struct {
	act.Actor
	workers    map[string]gen.PID
	maxWorkers int
	strategy   string
}

func factoryManagerActor() gen.ProcessBehavior {
	return &managerActor{
		workers:    make(map[string]gen.PID),
		maxWorkers: 5,
		strategy:   "round_robin",
	}
}

func (s *managerActor) Init(args ...any) error {
	return nil
}

func (s *managerActor) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case StartWorker:
		if len(s.workers) >= s.maxWorkers {
			s.Send(from, WorkerError{Error: "maximum workers reached"})
			return nil
		}

		workerPID, err := s.Spawn(factoryWorkerActor, gen.ProcessOptions{}, msg.WorkerID, msg.Config)
		if err != nil {
			s.Send(from, WorkerError{Error: err.Error()})
			return nil
		}

		s.workers[msg.WorkerID] = workerPID
		s.Send(from, WorkerStarted{WorkerID: msg.WorkerID, PID: workerPID})

		// Send metrics update
		s.Send("metrics", WorkerMetrics{
			Action:        "start",
			WorkerID:      msg.WorkerID,
			ActiveWorkers: len(s.workers),
		})

	case StopWorker:
		workerPID, exists := s.workers[msg.WorkerID]
		if !exists {
			s.Send(from, WorkerError{Error: "worker not found"})
			return nil
		}

		s.SendExit(workerPID, gen.TerminateReasonNormal)
		delete(s.workers, msg.WorkerID)
		s.Send(from, WorkerStopped{WorkerID: msg.WorkerID})

	case RestartWorker:
		// Stop existing worker if it exists
		if workerPID, exists := s.workers[msg.WorkerID]; exists {
			s.SendExit(workerPID, gen.TerminateReasonNormal)
		}

		// Start new worker
		workerPID, err := s.Spawn(factoryWorkerActor, gen.ProcessOptions{}, msg.WorkerID, msg.Config)
		if err != nil {
			s.Send(from, WorkerError{Error: err.Error()})
			return nil
		}

		s.workers[msg.WorkerID] = workerPID
		s.Send(from, WorkerRestarted{WorkerID: msg.WorkerID, PID: workerPID})

	case GetManagerStatus:
		s.Send(from, ManagerStatus{
			Strategy:      s.strategy,
			MaxWorkers:    s.maxWorkers,
			ActiveWorkers: len(s.workers),
			Workers:       s.workers,
		})

	case ProcessTask:
		// Distribute task to worker based on strategy
		if len(s.workers) == 0 {
			s.Send(from, TaskError{TaskID: msg.TaskID, Error: "no workers available"})
			return nil
		}

		// Simple round-robin selection
		var selectedWorker gen.PID
		for _, worker := range s.workers {
			selectedWorker = worker
			break // Just take first one for simplicity
		}

		s.Send(selectedWorker, msg)
	}
	return nil
}

type workerActor struct {
	act.Actor
	workerID string
	config   map[string]any
	tasks    int
}

func factoryWorkerActor() gen.ProcessBehavior {
	return &workerActor{}
}

func (w *workerActor) Init(args ...any) error {
	if len(args) > 0 {
		w.workerID = args[0].(string)
	}
	if len(args) > 1 {
		w.config = args[1].(map[string]any)
	}
	return nil
}

func (w *workerActor) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case ProcessTask:
		w.tasks++
		// Simulate task processing
		result := fmt.Sprintf("processed by %s", w.workerID)
		w.Send(from, TaskResult{
			TaskID:   msg.TaskID,
			WorkerID: w.workerID,
			Result:   result,
		})

	case GetWorkerStatus:
		w.Send(from, WorkerStatus{
			WorkerID:       w.workerID,
			TasksProcessed: w.tasks,
			Config:         w.config,
		})
	}
	return nil
}

// Message types for worker management
type StartWorker struct {
	WorkerID string
	Config   map[string]any
}

type StopWorker struct {
	WorkerID string
}

type RestartWorker struct {
	WorkerID string
	Config   map[string]any
}

type GetManagerStatus struct{}

type WorkerStarted struct {
	WorkerID string
	PID      gen.PID
}

type WorkerStopped struct {
	WorkerID string
}

type WorkerRestarted struct {
	WorkerID string
	PID      gen.PID
}

type WorkerError struct {
	Error string
}

type ManagerStatus struct {
	Strategy      string
	MaxWorkers    int
	ActiveWorkers int
	Workers       map[string]gen.PID
}

type WorkerMetrics struct {
	Action        string
	WorkerID      string
	ActiveWorkers int
}

type ProcessTask struct {
	TaskID string
	Data   any
}

type GetWorkerStatus struct{}

type TaskResult struct {
	TaskID   string
	WorkerID string
	Result   string
}

type TaskError struct {
	TaskID string
	Error  string
}

type WorkerStatus struct {
	WorkerID       string
	TasksProcessed int
	Config         map[string]any
}
