package unit

import (
	"reflect"
	"strings"
	"time"

	"ergo.services/ergo/gen"
)

// SendAssertion provides fluent assertions for Send operations
type SendAssertion struct {
	actor     *TestActor
	expected  bool
	to        any
	message   any
	count     int
	matchers  []func(any) bool
	captureAs string
}

func (sa *SendAssertion) To(target any) *SendAssertion {
	sa.to = target
	return sa
}

func (sa *SendAssertion) Message(msg any) *SendAssertion {
	sa.message = msg
	return sa
}

func (sa *SendAssertion) MessageMatching(matcher func(any) bool) *SendAssertion {
	sa.matchers = append(sa.matchers, matcher)
	return sa
}

func (sa *SendAssertion) Times(n int) *SendAssertion {
	sa.count = n
	return sa
}

func (sa *SendAssertion) Once() *SendAssertion {
	return sa.Times(1)
}

func (sa *SendAssertion) CaptureAs(name string) *SendAssertion {
	sa.captureAs = name
	return sa
}

func (sa *SendAssertion) Assert() {
	sa.actor.t.Helper()

	events := sa.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		sendEvent, ok := event.(SendEvent)
		if !ok {
			continue
		}

		// Check if this send event matches our criteria
		matches := true

		if sa.to != nil && !reflect.DeepEqual(sendEvent.To, sa.to) {
			matches = false
		}

		if sa.message != nil && !reflect.DeepEqual(sendEvent.Message, sa.message) {
			matches = false
		}

		// Apply custom matchers
		for _, matcher := range sa.matchers {
			if !matcher(sendEvent.Message) {
				matches = false
				break
			}
		}

		if matches {
			matchingEvents++
			if sa.captureAs != "" {
				sa.actor.Capture(sa.captureAs, sendEvent)
			}
		}
	}

	if sa.expected {
		if matchingEvents != sa.count {
			sa.actor.t.Errorf("Expected %d Send events, but found %d", sa.count, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			sa.actor.t.Errorf("Expected no Send events, but found %d", matchingEvents)
		}
	}
}

// SpawnAssertion provides fluent assertions for Spawn operations
type SpawnAssertion struct {
	actor    *TestActor
	expected bool
	factory  gen.ProcessFactory
	count    int
	captured *SpawnEvent
}

func (sa *SpawnAssertion) Factory(factory gen.ProcessFactory) *SpawnAssertion {
	sa.factory = factory
	return sa
}

func (sa *SpawnAssertion) Times(n int) *SpawnAssertion {
	sa.count = n
	return sa
}

func (sa *SpawnAssertion) Once() *SpawnAssertion {
	return sa.Times(1)
}

func (sa *SpawnAssertion) Capture() *SpawnResult {
	sa.actor.t.Helper()

	events := sa.actor.Events()
	for _, event := range events {
		if spawnEvent, ok := event.(SpawnEvent); ok {
			if sa.factory == nil || reflect.ValueOf(spawnEvent.Factory).Pointer() == reflect.ValueOf(sa.factory).Pointer() {
				sa.captured = &spawnEvent
				return &SpawnResult{
					PID:     spawnEvent.Result,
					Factory: spawnEvent.Factory,
					Options: spawnEvent.Options,
					Args:    spawnEvent.Args,
				}
			}
		}
	}

	sa.actor.t.Errorf("No matching Spawn event found to capture")
	return nil
}

func (sa *SpawnAssertion) Assert() {
	sa.actor.t.Helper()

	events := sa.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		spawnEvent, ok := event.(SpawnEvent)
		if !ok {
			continue
		}

		if sa.factory == nil || reflect.ValueOf(spawnEvent.Factory).Pointer() == reflect.ValueOf(sa.factory).Pointer() {
			matchingEvents++
		}
	}

	if sa.expected {
		if matchingEvents != sa.count {
			sa.actor.t.Errorf("Expected %d Spawn events, but found %d", sa.count, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			sa.actor.t.Errorf("Expected no Spawn events, but found %d", matchingEvents)
		}
	}
}

// SpawnResult holds information about a spawned process
type SpawnResult struct {
	PID     gen.PID
	Factory gen.ProcessFactory
	Options gen.ProcessOptions
	Args    []any
}

// SpawnMetaAssertion provides fluent assertions for SpawnMeta operations
type SpawnMetaAssertion struct {
	actor    *TestActor
	expected bool
	behavior gen.MetaBehavior
	count    int
}

func (sma *SpawnMetaAssertion) Behavior(behavior gen.MetaBehavior) *SpawnMetaAssertion {
	sma.behavior = behavior
	return sma
}

func (sma *SpawnMetaAssertion) Times(n int) *SpawnMetaAssertion {
	sma.count = n
	return sma
}

func (sma *SpawnMetaAssertion) Once() *SpawnMetaAssertion {
	return sma.Times(1)
}

func (sma *SpawnMetaAssertion) Capture() *SpawnMetaResult {
	sma.actor.t.Helper()

	events := sma.actor.Events()
	for _, event := range events {
		if spawnEvent, ok := event.(SpawnMetaEvent); ok {
			if sma.behavior == nil || reflect.ValueOf(spawnEvent.Behavior).Pointer() == reflect.ValueOf(sma.behavior).Pointer() {
				return &SpawnMetaResult{
					ID:       spawnEvent.Result,
					Behavior: spawnEvent.Behavior,
					Options:  spawnEvent.Options,
				}
			}
		}
	}

	sma.actor.t.Errorf("No matching SpawnMeta event found to capture")
	return nil
}

func (sma *SpawnMetaAssertion) Assert() {
	sma.actor.t.Helper()

	events := sma.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		spawnEvent, ok := event.(SpawnMetaEvent)
		if !ok {
			continue
		}

		if sma.behavior == nil || reflect.ValueOf(spawnEvent.Behavior).Pointer() == reflect.ValueOf(sma.behavior).Pointer() {
			matchingEvents++
		}
	}

	if sma.expected {
		if matchingEvents != sma.count {
			sma.actor.t.Errorf("Expected %d SpawnMeta events, but found %d", sma.count, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			sma.actor.t.Errorf("Expected no SpawnMeta events, but found %d", matchingEvents)
		}
	}
}

// SpawnMetaResult holds information about a spawned meta process
type SpawnMetaResult struct {
	ID       gen.Alias
	Behavior gen.MetaBehavior
	Options  gen.MetaOptions
}

// LogAssertion provides fluent assertions for Log operations
type LogAssertion struct {
	actor    *TestActor
	expected bool
	level    gen.LogLevel
	pattern  string
	count    int
}

func (la *LogAssertion) Level(level gen.LogLevel) *LogAssertion {
	la.level = level
	return la
}

func (la *LogAssertion) Containing(pattern string) *LogAssertion {
	la.pattern = pattern
	return la
}

func (la *LogAssertion) Times(n int) *LogAssertion {
	la.count = n
	return la
}

func (la *LogAssertion) Once() *LogAssertion {
	return la.Times(1)
}

func (la *LogAssertion) Assert() {
	la.actor.t.Helper()

	events := la.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		logEvent, ok := event.(LogEvent)
		if !ok {
			continue
		}

		matches := true

		if la.level != 0 && logEvent.Level != la.level {
			matches = false
		}

		if la.pattern != "" && !strings.Contains(logEvent.Message, la.pattern) {
			matches = false
		}

		if matches {
			matchingEvents++
		}
	}

	if la.expected {
		if matchingEvents != la.count {
			la.actor.t.Errorf("Expected %d Log events, but found %d", la.count, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			la.actor.t.Errorf("Expected no Log events, but found %d", matchingEvents)
		}
	}
}

// CallAssertion provides fluent assertions for Call operations
type CallAssertion struct {
	actor    *TestActor
	expected bool
	to       any
	request  any
	count    int
}

func (ca *CallAssertion) To(target any) *CallAssertion {
	ca.to = target
	return ca
}

func (ca *CallAssertion) Request(req any) *CallAssertion {
	ca.request = req
	return ca
}

func (ca *CallAssertion) Times(n int) *CallAssertion {
	ca.count = n
	return ca
}

func (ca *CallAssertion) Once() *CallAssertion {
	return ca.Times(1)
}

func (ca *CallAssertion) Assert() {
	ca.actor.t.Helper()

	events := ca.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		callEvent, ok := event.(CallEvent)
		if !ok {
			continue
		}

		matches := true

		if ca.to != nil && !reflect.DeepEqual(callEvent.To, ca.to) {
			matches = false
		}

		if ca.request != nil && !reflect.DeepEqual(callEvent.Request, ca.request) {
			matches = false
		}

		if matches {
			matchingEvents++
		}
	}

	if ca.expected {
		if matchingEvents != ca.count {
			ca.actor.t.Errorf("Expected %d Call events, but found %d", ca.count, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			ca.actor.t.Errorf("Expected no Call events, but found %d", matchingEvents)
		}
	}
}

// Matcher types for advanced assertions
type Matcher func(any) bool

func IsValidPID() Matcher {
	return func(v any) bool {
		if pid, ok := v.(gen.PID); ok {
			return pid.Node != "" && pid.ID > 0
		}
		return false
	}
}

func IsValidAlias() Matcher {
	return func(v any) bool {
		if alias, ok := v.(gen.Alias); ok {
			return alias.Node != "" && alias.ID[0] > 0
		}
		return false
	}
}

func IsTypeGeneric[T any]() Matcher {
	return func(v any) bool {
		_, ok := v.(T)
		return ok
	}
}

func HasField(fieldName string, fieldMatcher Matcher) Matcher {
	return func(v any) bool {
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
		if rv.Kind() != reflect.Struct {
			return false
		}
		field := rv.FieldByName(fieldName)
		if !field.IsValid() {
			return false
		}
		return fieldMatcher(field.Interface())
	}
}

func Equals(expected any) Matcher {
	return func(v any) bool {
		return reflect.DeepEqual(v, expected)
	}
}

// Helper for structure matching with ignored fields
func StructureMatching(template any, fieldMatchers map[string]Matcher) Matcher {
	return func(v any) bool {
		templateVal := reflect.ValueOf(template)
		actualVal := reflect.ValueOf(v)

		if templateVal.Type() != actualVal.Type() {
			return false
		}

		if templateVal.Kind() == reflect.Ptr {
			templateVal = templateVal.Elem()
			actualVal = actualVal.Elem()
		}

		if templateVal.Kind() != reflect.Struct {
			return reflect.DeepEqual(template, v)
		}

		for i := 0; i < templateVal.NumField(); i++ {
			fieldName := templateVal.Type().Field(i).Name
			templateFieldVal := templateVal.Field(i)
			actualFieldVal := actualVal.Field(i)

			// Check if we have a custom matcher for this field
			if matcher, exists := fieldMatchers[fieldName]; exists {
				if !matcher(actualFieldVal.Interface()) {
					return false
				}
				continue
			}

			// Default comparison
			if !reflect.DeepEqual(templateFieldVal.Interface(), actualFieldVal.Interface()) {
				return false
			}
		}

		return true
	}
}

// WithTimeout adds timeout support to assertions
func WithTimeout(assertion func(), timeout time.Duration) func() bool {
	return func() bool {
		done := make(chan bool, 1)
		go func() {
			assertion()
			done <- true
		}()

		select {
		case <-done:
			return true
		case <-time.After(timeout):
			return false
		}
	}
}

// RemoteSpawnAssertion for testing remote spawn operations
type RemoteSpawnAssertion struct {
	actor    *TestActor
	expected bool
	count    int
	node     gen.Atom
	name     gen.Atom
	register gen.Atom
	options  *gen.ProcessOptions
}

// ShouldRemoteSpawn starts a remote spawn assertion
func (ta *TestActor) ShouldRemoteSpawn() *RemoteSpawnAssertion {
	return &RemoteSpawnAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

// ShouldNotRemoteSpawn starts a negative remote spawn assertion
func (ta *TestActor) ShouldNotRemoteSpawn() *RemoteSpawnAssertion {
	return &RemoteSpawnAssertion{
		actor:    ta,
		expected: false,
	}
}

func (rsa *RemoteSpawnAssertion) ToNode(node gen.Atom) *RemoteSpawnAssertion {
	rsa.node = node
	return rsa
}

func (rsa *RemoteSpawnAssertion) WithName(name gen.Atom) *RemoteSpawnAssertion {
	rsa.name = name
	return rsa
}

func (rsa *RemoteSpawnAssertion) WithRegister(register gen.Atom) *RemoteSpawnAssertion {
	rsa.register = register
	return rsa
}

func (rsa *RemoteSpawnAssertion) WithOptions(options gen.ProcessOptions) *RemoteSpawnAssertion {
	rsa.options = &options
	return rsa
}

func (rsa *RemoteSpawnAssertion) Times(count int) *RemoteSpawnAssertion {
	rsa.count = count
	return rsa
}

func (rsa *RemoteSpawnAssertion) Once() *RemoteSpawnAssertion {
	rsa.count = 1
	return rsa
}

func (rsa *RemoteSpawnAssertion) Assert() {
	events := rsa.actor.Events()
	found := 0

	for _, event := range events {
		if rsEvent, ok := event.(RemoteSpawnEvent); ok {
			if rsa.matches(rsEvent) {
				found++
			}
		}
	}

	if rsa.expected {
		if found != rsa.count {
			rsa.actor.t.Errorf("Expected %d RemoteSpawn events, but found %d", rsa.count, found)
		}
	} else {
		if found > 0 {
			rsa.actor.t.Errorf("Expected no RemoteSpawn events, but found %d", found)
		}
	}
}

func (rsa *RemoteSpawnAssertion) matches(event RemoteSpawnEvent) bool {
	if rsa.node != "" && event.Node != rsa.node {
		return false
	}
	if rsa.name != "" && event.Name != rsa.name {
		return false
	}
	if rsa.register != "" && event.Register != rsa.register {
		return false
	}
	// Add more matching logic as needed
	return true
}

// RemoteApplicationStartAssertion for testing remote application start operations
type RemoteApplicationStartAssertion struct {
	actor    *TestActor
	expected bool
	count    int
	node     gen.Atom
	appName  gen.Atom
	mode     *gen.ApplicationMode
}

// ShouldRemoteApplicationStart starts a remote application start assertion
func (ta *TestActor) ShouldRemoteApplicationStart() *RemoteApplicationStartAssertion {
	return &RemoteApplicationStartAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

// ShouldNotRemoteApplicationStart starts a negative remote application start assertion
func (ta *TestActor) ShouldNotRemoteApplicationStart() *RemoteApplicationStartAssertion {
	return &RemoteApplicationStartAssertion{
		actor:    ta,
		expected: false,
	}
}

func (rasa *RemoteApplicationStartAssertion) OnNode(node gen.Atom) *RemoteApplicationStartAssertion {
	rasa.node = node
	return rasa
}

func (rasa *RemoteApplicationStartAssertion) Application(name gen.Atom) *RemoteApplicationStartAssertion {
	rasa.appName = name
	return rasa
}

func (rasa *RemoteApplicationStartAssertion) WithMode(mode gen.ApplicationMode) *RemoteApplicationStartAssertion {
	rasa.mode = &mode
	return rasa
}

func (rasa *RemoteApplicationStartAssertion) Times(count int) *RemoteApplicationStartAssertion {
	rasa.count = count
	return rasa
}

func (rasa *RemoteApplicationStartAssertion) Once() *RemoteApplicationStartAssertion {
	rasa.count = 1
	return rasa
}

func (rasa *RemoteApplicationStartAssertion) Assert() {
	events := rasa.actor.Events()
	found := 0

	for _, event := range events {
		if raEvent, ok := event.(RemoteApplicationStartEvent); ok {
			if rasa.matches(raEvent) {
				found++
			}
		}
	}

	if rasa.expected {
		if found != rasa.count {
			rasa.actor.t.Errorf("Expected %d RemoteApplicationStart events, but found %d", rasa.count, found)
		}
	} else {
		if found > 0 {
			rasa.actor.t.Errorf("Expected no RemoteApplicationStart events, but found %d", found)
		}
	}
}

func (rasa *RemoteApplicationStartAssertion) matches(event RemoteApplicationStartEvent) bool {
	if rasa.node != "" && event.Node != rasa.node {
		return false
	}
	if rasa.appName != "" && event.AppName != rasa.appName {
		return false
	}
	if rasa.mode != nil && event.Mode != *rasa.mode {
		return false
	}
	return true
}

// NodeConnectionAssertion for testing node connection events
type NodeConnectionAssertion struct {
	actor     *TestActor
	expected  bool
	count     int
	node      gen.Atom
	connected *bool
	action    string
}

// ShouldConnect starts a node connection assertion
func (ta *TestActor) ShouldConnect() *NodeConnectionAssertion {
	return &NodeConnectionAssertion{
		actor:    ta,
		expected: true,
		count:    1,
		action:   "connect",
	}
}

// ShouldDisconnect starts a node disconnection assertion
func (ta *TestActor) ShouldDisconnect() *NodeConnectionAssertion {
	return &NodeConnectionAssertion{
		actor:    ta,
		expected: true,
		count:    1,
		action:   "disconnect",
	}
}

func (nca *NodeConnectionAssertion) ToNode(node gen.Atom) *NodeConnectionAssertion {
	nca.node = node
	return nca
}

func (nca *NodeConnectionAssertion) Times(count int) *NodeConnectionAssertion {
	nca.count = count
	return nca
}

func (nca *NodeConnectionAssertion) Once() *NodeConnectionAssertion {
	nca.count = 1
	return nca
}

func (nca *NodeConnectionAssertion) Assert() {
	events := nca.actor.Events()
	found := 0

	for _, event := range events {
		if ncEvent, ok := event.(NodeConnectionEvent); ok {
			if nca.matches(ncEvent) {
				found++
			}
		}
	}

	if nca.expected {
		if found != nca.count {
			nca.actor.t.Errorf("Expected %d NodeConnection events, but found %d", nca.count, found)
		}
	} else {
		if found > 0 {
			nca.actor.t.Errorf("Expected no NodeConnection events, but found %d", found)
		}
	}
}

func (nca *NodeConnectionAssertion) matches(event NodeConnectionEvent) bool {
	if nca.node != "" && event.Node != nca.node {
		return false
	}
	if nca.action != "" && event.Action != nca.action {
		return false
	}
	if nca.connected != nil && event.Connected != *nca.connected {
		return false
	}
	return true
}
