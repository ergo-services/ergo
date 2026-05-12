package gen

import (
	"time"
)

var (
	DefaultRequestTimeout = 5

	DefaultCompressionType      CompressionType  = CompressionTypeGZIP
	DefaultCompressionLevel     CompressionLevel = CompressionDefault
	DefaultCompressionThreshold int              = 1024

	DefaultLogFilter = []LogLevel{
		LogLevelTrace,
		LogLevelDebug,
		LogLevelInfo,
		LogLevelWarning,
		LogLevelError,
		LogLevelPanic,
	}

	DefaultShutdownTimeout time.Duration = 3 * time.Minute
	DefaultKeepAlivePeriod time.Duration = 15 * time.Second
	DefaultTCPBufferSize   int           = 65535
	DefaultPort            uint16        = 11144

	DefaultApplicationInitTimeout  time.Duration = 15 * time.Second
	DefaultApplicationStartTimeout time.Duration = 15 * time.Second
	DefaultApplicationStopTimeout  time.Duration = 15 * time.Second

	DefaultHandshakeTimeout        time.Duration = 5 * time.Second
	DefaultSoftwareKeepAliveMisses int           = 3
	DefaultFragmentSize            int           = 65000
	DefaultFragmentTimeout         time.Duration = 30 * time.Second
	DefaultMaxFragmentAssemblies   int           = 1000

	DefaultNetworkFlags = NetworkFlags{
		Enable:                       true,
		EnableRemoteSpawn:            true,
		EnableRemoteApplicationStart: true,
		EnableFragmentation:          true,
		EnableProxyTransit:           false,
		EnableProxyAccept:            true,
		EnableImportantDelivery:      true,
		EnableSimultaneousConnect:    true,
		EnableClockSkew:              true,
		EnableTracing:                true,
		EnableWrappedErrors:          true,
		EnableSoftwareKeepAlive:      15, // seconds
	}

	DefaultNetworkProxyFlags = NetworkProxyFlags{
		Enable:                       true,
		EnableRemoteSpawn:            false,
		EnableRemoteApplicationStart: false,
		EnableEncryption:             false,
		EnableImportantDelivery:      true,
	}

	DefaultLogLevels = []LogLevel{
		LogLevelSystem,
		LogLevelTrace,
		LogLevelDebug,
		LogLevelInfo,
		LogLevelWarning,
		LogLevelError,
		LogLevelPanic,
	}
)

const (
	LicenseMIT  string = "MIT"
	LicenseBSL1 string = "Business Source License 1.1"
)
