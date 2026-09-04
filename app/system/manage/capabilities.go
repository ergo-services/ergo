package manage

// Capability names of the mutating plane. One vocabulary serves three purposes:
// feature detection by the caller, the caller's own ceiling, and the audit record.
const (
	CapSend         = "manage.send"
	CapSendMeta     = "manage.send_meta"
	CapSendExit     = "manage.send_exit"
	CapSendExitMeta = "manage.send_exit_meta"
	CapKill         = "manage.kill"

	CapSetLogLevel        = "manage.set_log_level"
	CapSetProcessLogLevel = "manage.set_process_log_level"
	CapSetMetaLogLevel    = "manage.set_meta_log_level"

	CapSetNodeTracingSampler    = "manage.set_node_tracing_sampler"
	CapSetProcessTracingSampler = "manage.set_process_tracing_sampler"

	CapSetProcessSendPriority         = "manage.set_process_send_priority"
	CapSetProcessCompression          = "manage.set_process_compression"
	CapSetProcessCompressionType      = "manage.set_process_compression_type"
	CapSetProcessCompressionLevel     = "manage.set_process_compression_level"
	CapSetProcessCompressionThreshold = "manage.set_process_compression_threshold"
	CapSetProcessKeepNetworkOrder     = "manage.set_process_keep_network_order"
	CapSetProcessImportantDelivery    = "manage.set_process_important_delivery"

	CapSetMetaSendPriority = "manage.set_meta_send_priority"

	CapAppStart  = "manage.app_start"
	CapAppStop   = "manage.app_stop"
	CapAppUnload = "manage.app_unload"
)

// Capabilities returns every capability of the mutating plane. The system
// application passes it to the inspector, which reports it to the callers.
func Capabilities() []string {
	return []string{
		CapSend,
		CapSendMeta,
		CapSendExit,
		CapSendExitMeta,
		CapKill,
		CapSetLogLevel,
		CapSetProcessLogLevel,
		CapSetMetaLogLevel,
		CapSetNodeTracingSampler,
		CapSetProcessTracingSampler,
		CapSetProcessSendPriority,
		CapSetProcessCompression,
		CapSetProcessCompressionType,
		CapSetProcessCompressionLevel,
		CapSetProcessCompressionThreshold,
		CapSetProcessKeepNetworkOrder,
		CapSetProcessImportantDelivery,
		CapSetMetaSendPriority,
		CapAppStart,
		CapAppStop,
		CapAppUnload,
	}
}
