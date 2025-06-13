package unit

import (
	"fmt"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// Test call operations
func TestCallOperations(t *testing.T) {
	actor, err := Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	actor.ClearEvents()

	// Trigger the "increase" message which contains a call operation
	actor.SendMessage(gen.PID{}, "increase")

	// Verify call was made (the ExampleActor makes a call to gen.PID{} with 12345)
	actor.ShouldCall().To(gen.PID{}).Request(12345).Once().Assert()

	// Verify other expected behavior from "increase" message
	actor.ShouldSend().To(gen.Atom("abc")).Message(9).Once().Assert()
	actor.ShouldSend().To(gen.Atom("def")).Once().Assert() // Response from call
}

// advancedCallActor for testing complex async call patterns
type advancedCallActor struct {
	act.Actor
	pendingRequests map[string]PendingRequest
}

type PendingRequest struct {
	From   gen.PID
	Ref    gen.Ref
	Data   any
	Status string
}

func factoryAdvancedCallActor() gen.ProcessBehavior {
	return &advancedCallActor{
		pendingRequests: make(map[string]PendingRequest),
	}
}

func (a *advancedCallActor) Init(args ...any) error {
	return nil
}

func (a *advancedCallActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch req := request.(type) {
	case map[string]any:
		command, ok := req["command"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid command")
		}

		switch command {
		case "start_task":
			taskID := req["task_id"].(string)
			data := req["data"]
			a.pendingRequests[taskID] = PendingRequest{
				From:   from,
				Ref:    ref,
				Data:   data,
				Status: "pending",
			}
			// Return nil to indicate async response
			return nil, nil

		case "complete_task":
			taskID := req["task_id"].(string)
			if pending, exists := a.pendingRequests[taskID]; exists {
				// Send async response to original caller
				response := map[string]any{
					"task_id": taskID,
					"result":  "completed",
					"data":    pending.Data,
				}
				a.SendResponse(pending.From, pending.Ref, response)
				delete(a.pendingRequests, taskID)
				// Return immediate confirmation
				return map[string]any{"status": "completion_triggered"}, nil
			}
			return nil, fmt.Errorf("task not found")

		case "list_tasks":
			tasks := make([]string, 0, len(a.pendingRequests))
			for taskID := range a.pendingRequests {
				tasks = append(tasks, taskID)
			}
			return map[string]any{"tasks": tasks}, nil
		}
	case string:
		if req == "process_async" {
			// Store ref for later response
			a.pendingRequests["async_task"] = PendingRequest{
				From:   from,
				Ref:    ref,
				Status: "processing",
			}
			// Send async response immediately for demo
			a.SendResponse(from, ref, "async_complete")
			return nil, nil
		}
	}
	return nil, fmt.Errorf("unknown request: %v", request)
}

func (a *advancedCallActor) HandleMessage(from gen.PID, message any) error {
	// Handle regular messages if needed
	return nil
}

// callResponseActor for testing SendResponse functionality
type callResponseActor struct {
	act.Actor
	pendingCalls map[gen.Ref]gen.PID
}

func factoryCallResponseActor() gen.ProcessBehavior {
	return &callResponseActor{
		pendingCalls: make(map[gen.Ref]gen.PID),
	}
}

func (c *callResponseActor) Init(args ...any) error {
	c.Log().Info("CallResponse actor initialized")
	return nil
}

func (c *callResponseActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "complete_async":
		// Complete the stored async request
		for ref, caller := range c.pendingCalls {
			err := c.SendResponse(caller, ref, map[string]any{
				"status": "completed",
				"data":   "async_response_data",
			})
			if err != nil {
				return err
			}
			delete(c.pendingCalls, ref)
		}
		return nil
	}
	return nil
}

// HandleCall handles async call requests with refs
func (c *callResponseActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "sync_request":
		// Respond immediately with a simple response using SendResponse
		err := c.SendResponse(from, ref, "immediate_response")
		return nil, err // Don't return the response directly - use SendResponse
	case "async_request":
		// Store the request for later response (don't respond immediately)
		c.pendingCalls[ref] = from
		c.Log().Debug("Stored async request from %s with ref %s", from, ref)
		// Return nil to indicate we'll respond later via SendResponse
		return nil, nil
	case "error_request":
		// Send error response asynchronously
		err := c.SendResponseError(from, ref, fmt.Errorf("something went wrong"))
		return nil, err
	case "batch_responses":
		// Send multiple responses asynchronously
		for i := 1; i <= 3; i++ {
			batchRef := gen.Ref{Node: c.Node().Name(), ID: [3]uint64{uint64(i), 0, 0}, Creation: 1}
			err := c.SendResponse(from, batchRef, fmt.Sprintf("batch_response_%d", i))
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unknown request: %v", request)
}

// Async Call Tests

func TestAsyncCall_BasicWorkflow(t *testing.T) {
	actor, err := Spawn(t, factoryAdvancedCallActor)
	if err != nil {
		t.Fatal(err)
	}

	clientPID := gen.PID{Node: "client", ID: 123}
	actor.ClearEvents()

	// Test async task creation
	task1 := actor.Call(clientPID, map[string]any{
		"command": "start_task",
		"task_id": "task_1",
		"data":    "test_data",
	})

	// Should return nil for async
	Nil(t, task1.Response, "Async task should not have immediate response")
	Nil(t, task1.Error, "Should not have error")

	// Test task completion
	completeResult := actor.Call(clientPID, map[string]any{
		"command": "complete_task",
		"task_id": "task_1",
	})

	Equal(t, map[string]any{"status": "completion_triggered"}, completeResult.Response, "Should return completion status")
	Nil(t, completeResult.Error, "Should not have error")

	// Verify async response was sent
	response, err := task1.GetAsyncResponse()
	Nil(t, err, "Should not have error getting async response")
	if resp, ok := response.(map[string]any); ok {
		Equal(t, "task_1", resp["task_id"], "Task ID should match")
		Equal(t, "completed", resp["result"], "Result should be completed")
	} else {
		t.Fatal("Response should be a map")
	}

	// Test error handling
	errorResult := actor.Call(clientPID, map[string]any{
		"command": "complete_task",
		"task_id": "nonexistent",
	})

	Nil(t, errorResult.Response, "Error call should not have response")
	Equal(t, fmt.Errorf("task not found"), errorResult.Error, "Should have task not found error")
}

func TestAsyncCall_ConcurrentRequests(t *testing.T) {
	actor, err := Spawn(t, factoryAdvancedCallActor)
	if err != nil {
		t.Fatal(err)
	}

	client1 := gen.PID{Node: "client1", ID: 100}
	client2 := gen.PID{Node: "client2", ID: 200}
	actor.ClearEvents()

	// Start multiple tasks
	task1 := actor.Call(client1, map[string]any{
		"command": "start_task",
		"task_id": "task_1",
		"data":    "data_1",
	})

	task2 := actor.Call(client2, map[string]any{
		"command": "start_task",
		"task_id": "task_2",
		"data":    "data_2",
	})

	// Check task list
	listResult := actor.Call(client1, map[string]any{"command": "list_tasks"})
	// Fix: Don't check exact order since map iteration is non-deterministic
	if listResp, ok := listResult.Response.(map[string]any); ok {
		if tasks, ok := listResp["tasks"].([]string); ok {
			Equal(t, 2, len(tasks), "Should have 2 tasks")
			// Check that both tasks exist without caring about order
			taskMap := make(map[string]bool)
			for _, task := range tasks {
				taskMap[task] = true
			}
			True(t, taskMap["task_1"], "Should contain task_1")
			True(t, taskMap["task_2"], "Should contain task_2")
		} else {
			t.Fatal("List response should contain tasks array")
		}
	} else {
		t.Fatal("List response should be a map")
	}
	Nil(t, listResult.Error, "Should not have error")

	// Complete all tasks
	actor.Call(client1, map[string]any{
		"command": "complete_task",
		"task_id": "task_1",
	})

	actor.Call(client1, map[string]any{
		"command": "complete_task",
		"task_id": "task_2",
	})

	// Verify responses
	response1, err1 := task1.GetAsyncResponse()
	Nil(t, err1, "Task1 should not have error")
	if resp1, ok := response1.(map[string]any); ok {
		Equal(t, "task_1", resp1["task_id"], "Task1 ID should match")
		Equal(t, "completed", resp1["result"], "Task1 result should be completed")
		Equal(t, "data_1", resp1["data"], "Task1 data should match")
	} else {
		t.Fatal("Task1 response should be a map")
	}

	response2, err2 := task2.GetAsyncResponse()
	Nil(t, err2, "Task2 should not have error")
	if resp2, ok := response2.(map[string]any); ok {
		Equal(t, "task_2", resp2["task_id"], "Task2 ID should match")
		Equal(t, "completed", resp2["result"], "Task2 result should be completed")
		Equal(t, "data_2", resp2["data"], "Task2 data should match")
	} else {
		t.Fatal("Task2 response should be a map")
	}
}

func TestAsyncCall_ResponseRetrieval(t *testing.T) {
	actor, err := Spawn(t, factoryAdvancedCallActor)
	if err != nil {
		t.Fatal(err)
	}

	clientPID := gen.PID{Node: "client", ID: 123}
	actor.ClearEvents()

	// Start async task
	task := actor.Call(clientPID, map[string]any{
		"command": "start_task",
		"task_id": "retrieve_test",
		"data":    "retrieve_data",
	})

	// Complete task
	actor.Call(clientPID, map[string]any{
		"command": "complete_task",
		"task_id": "retrieve_test",
	})

	// Get response directly
	response, err := task.GetAsyncResponse()
	Nil(t, err, "Should not have error")

	if resp, ok := response.(map[string]any); ok {
		Equal(t, "retrieve_test", resp["task_id"], "Task ID should match")
		Equal(t, "completed", resp["result"], "Result should be completed")
		Equal(t, "retrieve_data", resp["data"], "Data should match")
	} else {
		t.Fatal("Response should be a map")
	}
}

func TestAsyncCall_ComplexWorkflow(t *testing.T) {
	actor, err := Spawn(t, factoryAdvancedCallActor)
	if err != nil {
		t.Fatal(err)
	}

	clientPID := gen.PID{Node: "client", ID: 100}
	managerPID := gen.PID{Node: "manager", ID: 200}

	// Clear initialization events
	actor.ClearEvents()

	// Client starts multiple async tasks
	task1 := actor.Call(clientPID, map[string]any{
		"command": "start_task",
		"task_id": "task_1",
		"data":    "important_data_1",
	})

	task2 := actor.Call(clientPID, map[string]any{
		"command": "start_task",
		"task_id": "task_2",
		"data":    "important_data_2",
	})

	// Manager checks pending tasks
	listResult := actor.Call(managerPID, map[string]any{
		"command": "list_tasks",
	})

	// Check the response directly (list_tasks returns immediate response)
	Nil(t, listResult.Error, "List should not have error")
	if listResp, ok := listResult.Response.(map[string]any); ok {
		if tasks, ok := listResp["tasks"].([]string); ok {
			Equal(t, 2, len(tasks), "Should have 2 pending tasks")
			// Verify both tasks exist without caring about order
			taskMap := make(map[string]bool)
			for _, task := range tasks {
				taskMap[task] = true
			}
			True(t, taskMap["task_1"], "Should contain task_1")
			True(t, taskMap["task_2"], "Should contain task_2")
		} else {
			t.Fatal("List response should contain tasks array")
		}
	} else {
		t.Fatal("List response should be a map")
	}

	// Manager completes first task
	completeResult := actor.Call(managerPID, map[string]any{
		"command": "complete_task",
		"task_id": "task_1",
	})

	// Check completion result (complete_task returns immediate response)
	Nil(t, completeResult.Error, "Complete should not have error")
	if completeResp, ok := completeResult.Response.(map[string]any); ok {
		Equal(t, "completion_triggered", completeResp["status"], "Status should be completion_triggered")
	} else {
		t.Fatal("Complete response should be a map")
	}

	// Client should receive async response for task_1
	task1Response, task1Err := task1.GetAsyncResponse()
	Nil(t, task1Err, "Task1 should not have error")
	if task1Resp, ok := task1Response.(map[string]any); ok {
		Equal(t, "task_1", task1Resp["task_id"], "Task1 ID should match")
		Equal(t, "completed", task1Resp["result"], "Task1 result should be completed")
		Equal(t, "important_data_1", task1Resp["data"], "Task1 data should match")
	} else {
		t.Fatal("Task1 response should be a map")
	}

	// Complete second task
	actor.Call(managerPID, map[string]any{
		"command": "complete_task",
		"task_id": "task_2",
	})

	// Client should receive async response for task_2
	task2Response, task2Err := task2.GetAsyncResponse()
	Nil(t, task2Err, "Task2 should not have error")
	if task2Resp, ok := task2Response.(map[string]any); ok {
		Equal(t, "task_2", task2Resp["task_id"], "Task2 ID should match")
		Equal(t, "completed", task2Resp["result"], "Task2 result should be completed")
		Equal(t, "important_data_2", task2Resp["data"], "Task2 data should match")
	} else {
		t.Fatal("Task2 response should be a map")
	}

	// Verify all SendResponse events were captured correctly
	events := actor.Events()
	responseCount := 0
	for _, event := range events {
		if _, ok := event.(SendResponseEvent); ok {
			responseCount++
		}
	}

	True(t, responseCount >= 4, "Should have at least 4 SendResponse events")
}

// Simple demo of realistic async Call -> SendResponse workflow
func TestCall_AsyncResponseDemo(t *testing.T) {
	actor, err := Spawn(t, factoryCallResponseActor)
	if err != nil {
		t.Fatal(err)
	}

	callerPID := gen.PID{Node: "client", ID: 123}
	actor.ClearEvents()

	// 1. Make a normal Call - actor receives HandleCall(from, ref, request)
	result := actor.Call(callerPID, "async_request")

	// 2. Actor returns (nil, nil) from HandleCall - no immediate response
	// 3. Actor stores the ref for later and responds asynchronously via SendResponse

	// 4. Trigger the async completion
	actor.SendMessage(callerPID, "complete_async")

	// 5. Assert on the SendResponse event that was generated
	asyncResponse, asyncErr := result.GetAsyncResponse()
	Nil(t, asyncErr, "Should not have error getting async response")
	if asyncResp, ok := asyncResponse.(map[string]any); ok {
		Equal(t, "completed", asyncResp["status"], "Status should be completed")
		Equal(t, "async_response_data", asyncResp["data"], "Data should match")
	} else {
		t.Fatal("Async response should be a map")
	}

	// Alternative: Check the response directly
	response, err := result.GetAsyncResponse()
	Nil(t, err, "Should not have error")
	if resp, ok := response.(map[string]any); ok {
		Equal(t, "completed", resp["status"], "Status should be completed")
		Equal(t, "async_response_data", resp["data"], "Data should match")
	}
}
