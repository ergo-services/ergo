package manage

import (
	"reflect"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

func TestRollbackRestoresThePreviousValue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request any
		arm     func(node *unit.MockNode, applied *[]any)
		want    []any
	}{
		{
			name:    "meta log level",
			request: RequestDoSetMetaLogLevel{Meta: targetMet, Level: gen.LogLevelWarning},
			arm: func(node *unit.MockNode, applied *[]any) {
				node.OnSetMetaLogLevel(func(meta gen.Alias, level gen.LogLevel) error {
					*applied = append(*applied, level)
					return nil
				})
			},
			want: []any{gen.LogLevelWarning, gen.LogLevelInfo},
		},
		{
			name:    "process send priority",
			request: RequestDoSetProcessSendPriority{PID: targetPID, Priority: gen.MessagePriorityMax},
			arm: func(node *unit.MockNode, applied *[]any) {
				node.OnSetProcessSendPriority(func(pid gen.PID, priority gen.MessagePriority) error {
					*applied = append(*applied, priority)
					return nil
				})
			},
			want: []any{gen.MessagePriorityMax, gen.MessagePriorityNormal},
		},
		{
			name:    "process compression",
			request: RequestDoSetProcessCompression{PID: targetPID, Enabled: true},
			arm: func(node *unit.MockNode, applied *[]any) {
				node.OnSetProcessCompression(func(pid gen.PID, enabled bool) error {
					*applied = append(*applied, enabled)
					return nil
				})
			},
			want: []any{true, false},
		},
		{
			name:    "process compression type",
			request: RequestDoSetProcessCompressionType{PID: targetPID, Type: gen.CompressionTypeLZW},
			arm: func(node *unit.MockNode, applied *[]any) {
				node.OnSetProcessCompressionType(func(pid gen.PID, ctype gen.CompressionType) error {
					*applied = append(*applied, ctype)
					return nil
				})
			},
			want: []any{gen.CompressionTypeLZW, gen.CompressionTypeGZIP},
		},
		{
			name:    "process compression level",
			request: RequestDoSetProcessCompressionLevel{PID: targetPID, Level: gen.CompressionBestSize},
			arm: func(node *unit.MockNode, applied *[]any) {
				node.OnSetProcessCompressionLevel(func(pid gen.PID, level gen.CompressionLevel) error {
					*applied = append(*applied, level)
					return nil
				})
			},
			want: []any{gen.CompressionBestSize, gen.CompressionDefault},
		},
		{
			name:    "process compression threshold",
			request: RequestDoSetProcessCompressionThreshold{PID: targetPID, Threshold: 4096},
			arm: func(node *unit.MockNode, applied *[]any) {
				node.OnSetProcessCompressionThreshold(func(pid gen.PID, threshold int) error {
					*applied = append(*applied, threshold)
					return nil
				})
			},
			want: []any{4096, 1024},
		},
		{
			name:    "process keep network order",
			request: RequestDoSetProcessKeepNetworkOrder{PID: targetPID, Order: false},
			arm: func(node *unit.MockNode, applied *[]any) {
				node.OnSetProcessKeepNetworkOrder(func(pid gen.PID, order bool) error {
					*applied = append(*applied, order)
					return nil
				})
			},
			want: []any{false, true},
		},
		{
			name:    "process important delivery",
			request: RequestDoSetProcessImportantDelivery{PID: targetPID, Important: true},
			arm: func(node *unit.MockNode, applied *[]any) {
				node.OnSetProcessImportantDelivery(func(pid gen.PID, important bool) error {
					*applied = append(*applied, important)
					return nil
				})
			},
			want: []any{true, false},
		},
		{
			name:    "meta send priority",
			request: RequestDoSetMetaSendPriority{Meta: targetMet, Priority: gen.MessagePriorityHigh},
			arm: func(node *unit.MockNode, applied *[]any) {
				node.OnSetMetaSendPriority(func(meta gen.Alias, priority gen.MessagePriority) error {
					*applied = append(*applied, priority)
					return nil
				})
			},
			want: []any{gen.MessagePriorityHigh, gen.MessagePriorityNormal},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := settingNode(t)
			node.OnMetaInfo(func(meta gen.Alias) (gen.MetaInfo, error) {
				return gen.MetaInfo{LogLevel: gen.LogLevelInfo, MessagePriority: gen.MessagePriorityNormal}, nil
			})

			applied := []any{}
			tc.arm(node, &applied)

			sub := spawnManage(t, node)
			sub.OnSendResponse(callerPID).Fail(gen.ErrResponseIgnored)
			if _, err := sub.Call(callerPID, tc.request); err != nil {
				t.Fatalf("call: %s", err)
			}

			if reflect.DeepEqual(applied, tc.want) == false {
				t.Fatalf("the setting moved through %v; an ignored response must leave it at %v", applied, tc.want[1])
			}
		})
	}
}

func TestRollbackRestoresTheNodeLogLevel(t *testing.T) {
	node := manageNode(t)
	before := node.Log().Level()

	sub := spawnManage(t, node)
	sub.OnSendResponse(callerPID).Fail(gen.ErrResponseIgnored)
	if _, err := sub.Call(callerPID, RequestDoSetLogLevel{Level: gen.LogLevelError}); err != nil {
		t.Fatalf("call: %s", err)
	}

	if got := node.Log().Level(); got != before {
		t.Fatalf("the node log level stands at %s; an ignored response must leave it at %s", got, before)
	}
}

func TestRollbackRestoresTheNodeTracingSampler(t *testing.T) {
	node := manageNode(t)
	node.OnTracingSampler(func() gen.TracingSampler { return gen.TracingSamplerAlways })
	applied := []string{}
	node.OnSetTracingSampler(func(sampler gen.TracingSampler) error {
		applied = append(applied, sampler.String())
		return nil
	})

	sub := spawnManage(t, node)
	sub.OnSendResponse(callerPID).Fail(gen.ErrResponseIgnored)
	if _, err := sub.Call(callerPID, RequestDoSetNodeTracingSampler{Type: "disable"}); err != nil {
		t.Fatalf("call: %s", err)
	}

	want := []string{"disable", "always"}
	if reflect.DeepEqual(applied, want) == false {
		t.Fatalf("the node sampler moved through %v instead of %v", applied, want)
	}
}

func TestRollbackIsSkippedWithoutAPreviousValue(t *testing.T) {
	node := manageNode(t)
	applied := []gen.MessagePriority{}
	node.OnSetProcessSendPriority(func(pid gen.PID, priority gen.MessagePriority) error {
		applied = append(applied, priority)
		return nil
	})

	sub := spawnManage(t, node)
	sub.OnSendResponse(callerPID).Fail(gen.ErrResponseIgnored)
	request := RequestDoSetProcessSendPriority{PID: targetPID, Priority: gen.MessagePriorityMax}
	if _, err := sub.Call(callerPID, request); err != nil {
		t.Fatalf("call: %s", err)
	}

	if len(applied) != 1 || applied[0] != gen.MessagePriorityMax {
		t.Fatalf("the priority moved through %v; an unreadable process has nothing to restore", applied)
	}
}
