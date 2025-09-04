package gen

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestDefaultLoggerJSON(t *testing.T) {
	var buf bytes.Buffer

	// Test JSON output
	jsonLogger := CreateDefaultLogger(DefaultLoggerOptions{
		EnableJSON:      true,
		IncludeBehavior: true,
		IncludeName:     true,
		IncludeFields:   true,
		Output:          &buf,
		TimeFormat:      "2006-01-02T15:04:05",
	})

	// Create test message
	testTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	testPID := PID{Node: "testnode@localhost", ID: 123}

	msg := MessageLog{
		Time:   testTime,
		Level:  LogLevelInfo,
		Format: "Test message with %s",
		Args:   []any{"arguments"},
		Source: MessageLogProcess{
			PID:      testPID,
			Name:     "test_process",
			Behavior: "TestBehavior",
		},
		Fields: []LogField{
			{Name: "key1", Value: "value1"},
			{Name: "key2", Value: 42},
		},
	}

	jsonLogger.Log(msg)

	output := buf.String()

	// Verify JSON structure
	if !strings.Contains(output, `"time":"2023-01-01T12:00:00"`) {
		t.Errorf("Expected formatted time in JSON output, got: %s", output)
	}

	if !strings.Contains(output, `"level":"info"`) {
		t.Errorf("Expected level in JSON output, got: %s", output)
	}

	if !strings.Contains(output, `"message":"Test message with arguments"`) {
		t.Errorf("Expected formatted message in JSON output, got: %s", output)
	}

	if !strings.Contains(output, `"type":"process"`) {
		t.Errorf("Expected process type in source, got: %s", output)
	}

	if !strings.Contains(output, `"behavior":"TestBehavior"`) {
		t.Errorf("Expected behavior in JSON output, got: %s", output)
	}

	if !strings.Contains(output, `"name":"'test_process'"`) {
		t.Errorf("Expected name in JSON output, got: %s", output)
	}

	if !strings.Contains(output, `"fields":{"key1":"value1","key2":"42"}`) {
		t.Errorf("Expected fields in JSON output, got: %s", output)
	}
}

func TestDefaultLoggerPlainText(t *testing.T) {
	var buf bytes.Buffer

	// Test plain text output (default)
	plainLogger := CreateDefaultLogger(DefaultLoggerOptions{
		EnableJSON:      false,
		IncludeBehavior: true,
		IncludeName:     true,
		IncludeFields:   true,
		Output:          &buf,
		TimeFormat:      "2006-01-02T15:04:05",
	})

	// Create test message
	testTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	testPID := PID{Node: "testnode@localhost", ID: 123}

	msg := MessageLog{
		Time:   testTime,
		Level:  LogLevelInfo,
		Format: "Test message",
		Args:   []any{},
		Source: MessageLogProcess{
			PID:      testPID,
			Name:     "test_process",
			Behavior: "TestBehavior",
		},
		Fields: []LogField{
			{Name: "key1", Value: "value1"},
		},
	}

	plainLogger.Log(msg)

	output := buf.String()

	// Verify plain text format
	if !strings.Contains(output, "2023-01-01T12:00:00") {
		t.Errorf("Expected formatted time in plain text output, got: %s", output)
	}

	if !strings.Contains(output, "[info]") {
		t.Errorf("Expected level in brackets, got: %s", output)
	}

	if !strings.Contains(output, "Test message") {
		t.Errorf("Expected message in plain text output, got: %s", output)
	}

	if !strings.Contains(output, "TestBehavior") {
		t.Errorf("Expected behavior in plain text output, got: %s", output)
	}
}

func TestJSONEscaping(t *testing.T) {
	var buf bytes.Buffer

	jsonLogger := CreateDefaultLogger(DefaultLoggerOptions{
		EnableJSON:      true,
		IncludeBehavior: true,
		Output:          &buf,
	})

	// Test message with special characters that need escaping
	testTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	testPID := PID{Node: "testnode@localhost", ID: 123}

	msg := MessageLog{
		Time:   testTime,
		Level:  LogLevelError,
		Format: "Error with \"quotes\" and \n newlines and \t tabs",
		Args:   []any{},
		Source: MessageLogProcess{
			PID:      testPID,
			Name:     "",
			Behavior: "Test\\Behavior",
		},
		Fields: []LogField{},
	}

	jsonLogger.Log(msg)

	output := buf.String()

	// Verify JSON escaping
	if !strings.Contains(output, `\"quotes\"`) {
		t.Errorf("Expected escaped quotes in JSON output, got: %s", output)
	}

	if !strings.Contains(output, `\n`) {
		t.Errorf("Expected escaped newline in JSON output, got: %s", output)
	}

	if !strings.Contains(output, `\t`) {
		t.Errorf("Expected escaped tab in JSON output, got: %s", output)
	}

	if !strings.Contains(output, `Test\\Behavior`) {
		t.Errorf("Expected escaped backslash in JSON output, got: %s", output)
	}
}
