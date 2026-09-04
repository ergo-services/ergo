package local

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

func spawnMetaFor(t *testing.T, n *stage.Node) (gen.PID, gen.Alias) {
	t.Helper()
	host := n.Spawn(factoryMetaHost, gen.ProcessOptions{})
	result, err := n.Call(host, spawnMetaCmd{})
	check.NoError(t, err)
	alias, ok := result.(gen.Alias)
	if ok == false {
		t.Fatalf("spawning a meta answered %#v", result)
	}
	return host, alias
}

func TestNodeProcessSettings(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()
	pid := n.Spawn(factoryT0, gen.ProcessOptions{})
	unknown := gen.PID{Node: n.Name(), ID: 999999}

	t.Run("log level", func(t *testing.T) {
		check.NoError(t, nd.SetProcessLogLevel(pid, gen.LogLevelError))
		info, err := nd.ProcessInfo(pid)
		check.NoError(t, err)
		check.Equal(t, gen.LogLevelError, info.LogLevel)

		check.ErrorIs(t, nd.SetProcessLogLevel(unknown, gen.LogLevelError), gen.ErrProcessUnknown)
		check.ErrorIs(t, nd.SetProcessLogLevel(pid, gen.LogLevelTrace), gen.ErrIncorrect)
	})

	t.Run("send priority", func(t *testing.T) {
		for _, priority := range []gen.MessagePriority{gen.MessagePriorityHigh, gen.MessagePriorityMax, gen.MessagePriorityNormal} {
			check.NoError(t, nd.SetProcessSendPriority(pid, priority))
			info, err := nd.ProcessInfo(pid)
			check.NoError(t, err)
			check.Equal(t, priority, info.MessagePriority)
		}

		check.ErrorIs(t, nd.SetProcessSendPriority(pid, gen.MessagePriority(9)), gen.ErrIncorrect)
		check.ErrorIs(t, nd.SetProcessSendPriority(unknown, gen.MessagePriorityHigh), gen.ErrProcessUnknown)
	})

	t.Run("compression", func(t *testing.T) {
		check.NoError(t, nd.SetProcessCompression(pid, true))
		check.NoError(t, nd.SetProcessCompressionType(pid, gen.CompressionTypeLZW))
		check.NoError(t, nd.SetProcessCompressionLevel(pid, gen.CompressionBestSize))
		check.NoError(t, nd.SetProcessCompressionThreshold(pid, gen.DefaultCompressionThreshold+512))

		info, err := nd.ProcessInfo(pid)
		check.NoError(t, err)
		check.True(t, info.Compression.Enable)
		check.Equal(t, gen.CompressionTypeLZW, info.Compression.Type)
		check.Equal(t, gen.CompressionBestSize, info.Compression.Level)
		check.Equal(t, gen.DefaultCompressionThreshold+512, info.Compression.Threshold)

		check.ErrorIs(t, nd.SetProcessCompressionType(pid, gen.CompressionType("brotli")), gen.ErrIncorrect)
		check.ErrorIs(t, nd.SetProcessCompressionLevel(pid, gen.CompressionLevel(42)), gen.ErrIncorrect)
		check.ErrorIs(t, nd.SetProcessCompressionThreshold(pid, gen.DefaultCompressionThreshold-1), gen.ErrIncorrect)

		check.ErrorIs(t, nd.SetProcessCompression(unknown, true), gen.ErrProcessUnknown)
		check.ErrorIs(t, nd.SetProcessCompressionType(unknown, gen.CompressionTypeGZIP), gen.ErrProcessUnknown)
		check.ErrorIs(t, nd.SetProcessCompressionLevel(unknown, gen.CompressionDefault), gen.ErrProcessUnknown)
		check.ErrorIs(t, nd.SetProcessCompressionThreshold(unknown, gen.DefaultCompressionThreshold), gen.ErrProcessUnknown)
	})

	t.Run("keep network order", func(t *testing.T) {
		check.NoError(t, nd.SetProcessKeepNetworkOrder(pid, false))
		info, err := nd.ProcessInfo(pid)
		check.NoError(t, err)
		check.True(t, info.KeepNetworkOrder == false)

		check.NoError(t, nd.SetProcessKeepNetworkOrder(pid, true))
		info, err = nd.ProcessInfo(pid)
		check.NoError(t, err)
		check.True(t, info.KeepNetworkOrder)

		check.ErrorIs(t, nd.SetProcessKeepNetworkOrder(unknown, true), gen.ErrProcessUnknown)
	})

	t.Run("important delivery", func(t *testing.T) {
		check.NoError(t, nd.SetProcessImportantDelivery(pid, true))
		info, err := nd.ProcessInfo(pid)
		check.NoError(t, err)
		check.True(t, info.ImportantDelivery)

		check.NoError(t, nd.SetProcessImportantDelivery(pid, false))
		info, err = nd.ProcessInfo(pid)
		check.NoError(t, err)
		check.True(t, info.ImportantDelivery == false)

		check.ErrorIs(t, nd.SetProcessImportantDelivery(unknown, true), gen.ErrProcessUnknown)
	})
}

func TestNodeMetaSettings(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()
	host, alias := spawnMetaFor(t, n)
	unknown := gen.Alias{Node: n.Name(), ID: [3]uint64{999999, 0, 0}}

	info, err := nd.MetaInfo(alias)
	check.NoError(t, err)
	check.Equal(t, alias, info.ID)
	check.Equal(t, host, info.Parent)

	check.NoError(t, nd.SetMetaLogLevel(alias, gen.LogLevelWarning))
	info, err = nd.MetaInfo(alias)
	check.NoError(t, err)
	check.Equal(t, gen.LogLevelWarning, info.LogLevel)

	check.NoError(t, nd.SetMetaSendPriority(alias, gen.MessagePriorityMax))
	info, err = nd.MetaInfo(alias)
	check.NoError(t, err)
	check.Equal(t, gen.MessagePriorityMax, info.MessagePriority)

	check.ErrorIs(t, nd.SetMetaSendPriority(alias, gen.MessagePriority(9)), gen.ErrIncorrect)
	check.ErrorIs(t, nd.SetMetaLogLevel(alias, gen.LogLevelTrace), gen.ErrIncorrect)

	_, err = nd.MetaInfo(unknown)
	check.ErrorIs(t, err, gen.ErrProcessUnknown)
	check.ErrorIs(t, nd.SetMetaLogLevel(unknown, gen.LogLevelWarning), gen.ErrProcessUnknown)
	check.ErrorIs(t, nd.SetMetaSendPriority(unknown, gen.MessagePriorityMax), gen.ErrProcessUnknown)
}

func TestNodeSettingsRefusedOnATerminatedNode(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()
	pid := n.Spawn(factoryT0, gen.ProcessOptions{})
	alias := gen.Alias{Node: n.Name(), ID: [3]uint64{1, 0, 0}}

	nd.Stop()

	check.ErrorIs(t, nd.SetProcessLogLevel(pid, gen.LogLevelError), gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.SetProcessSendPriority(pid, gen.MessagePriorityHigh), gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.SetProcessCompression(pid, true), gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.SetProcessCompressionType(pid, gen.CompressionTypeLZW), gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.SetProcessCompressionLevel(pid, gen.CompressionBestSize), gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.SetProcessCompressionThreshold(pid, gen.DefaultCompressionThreshold), gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.SetProcessKeepNetworkOrder(pid, false), gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.SetProcessImportantDelivery(pid, true), gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.SetMetaLogLevel(alias, gen.LogLevelError), gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.SetMetaSendPriority(alias, gen.MessagePriorityHigh), gen.ErrNodeTerminated)

	_, err := nd.MetaInfo(alias)
	check.ErrorIs(t, err, gen.ErrNodeTerminated)
}
