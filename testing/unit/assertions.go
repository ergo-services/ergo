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

		matches := true

		if sa.to != nil && !reflect.DeepEqual(sendEvent.To, sa.to) {
			matches = false
		}

		if sa.message != nil && !reflect.DeepEqual(sendEvent.Message, sa.message) {
			matches = false
		}

		// Check custom matchers
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

// SendResponseErrorAssertion for testing send response error events
type SendResponseErrorAssertion struct {
	actor    *TestActor
	expected bool
	to       gen.PID
	error    error
	ref      gen.Ref
	count    int
	matchers []func(error) bool
}

func (srea *SendResponseErrorAssertion) To(to gen.PID) *SendResponseErrorAssertion {
	srea.to = to
	return srea
}

func (srea *SendResponseErrorAssertion) Error(err error) *SendResponseErrorAssertion {
	srea.error = err
	return srea
}

func (srea *SendResponseErrorAssertion) Ref(ref gen.Ref) *SendResponseErrorAssertion {
	srea.ref = ref
	return srea
}

func (srea *SendResponseErrorAssertion) WithRef(ref gen.Ref) *SendResponseErrorAssertion {
	srea.ref = ref
	return srea
}

func (srea *SendResponseErrorAssertion) ErrorMatching(matcher func(error) bool) *SendResponseErrorAssertion {
	srea.matchers = append(srea.matchers, matcher)
	return srea
}

func (srea *SendResponseErrorAssertion) Times(n int) *SendResponseErrorAssertion {
	srea.count = n
	return srea
}

func (srea *SendResponseErrorAssertion) Once() *SendResponseErrorAssertion {
	return srea.Times(1)
}

func (srea *SendResponseErrorAssertion) Assert() {
	srea.actor.t.Helper()

	events := srea.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		responseErrorEvent, ok := event.(SendResponseErrorEvent)
		if !ok {
			continue
		}

		matches := true

		if srea.to.Node != "" && !reflect.DeepEqual(responseErrorEvent.To, srea.to) {
			matches = false
		}

		if srea.error != nil && !reflect.DeepEqual(responseErrorEvent.Error, srea.error) {
			matches = false
		}

		if srea.ref.Node != "" && !reflect.DeepEqual(responseErrorEvent.Ref, srea.ref) {
			matches = false
		}

		// Check custom matchers
		for _, matcher := range srea.matchers {
			if !matcher(responseErrorEvent.Error) {
				matches = false
				break
			}
		}

		if matches {
			matchingEvents++
		}
	}

	if srea.expected {
		if matchingEvents != srea.count {
			srea.actor.t.Errorf("Expected %d SendResponseError events, but found %d", srea.count, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			srea.actor.t.Errorf("Expected no SendResponseError events, but found %d", matchingEvents)
		}
	}
}

// CronJobAssertion for testing cron job operations
type CronJobAssertion struct {
	actor    *TestActor
	expected bool
	count    int
	action   string // "add", "remove", "enable", "disable"
	name     gen.Atom
	spec     string
	matchers []func(gen.CronJob) bool
}

func (cja *CronJobAssertion) WithName(name gen.Atom) *CronJobAssertion {
	cja.name = name
	return cja
}

func (cja *CronJobAssertion) WithSpec(spec string) *CronJobAssertion {
	cja.spec = spec
	return cja
}

func (cja *CronJobAssertion) JobMatching(matcher func(gen.CronJob) bool) *CronJobAssertion {
	cja.matchers = append(cja.matchers, matcher)
	return cja
}

func (cja *CronJobAssertion) Times(n int) *CronJobAssertion {
	cja.count = n
	return cja
}

func (cja *CronJobAssertion) Once() *CronJobAssertion {
	return cja.Times(1)
}

func (cja *CronJobAssertion) Assert() {
	cja.actor.t.Helper()

	events := cja.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		matches := false

		switch cja.action {
		case "add":
			if addEvent, ok := event.(CronJobAddEvent); ok {
				matches = cja.matchesJob(addEvent.Job)
			}
		case "remove":
			if removeEvent, ok := event.(CronJobRemoveEvent); ok {
				matches = cja.name == "" || removeEvent.Name == cja.name
			}
		case "enable":
			if enableEvent, ok := event.(CronJobEnableEvent); ok {
				matches = cja.name == "" || enableEvent.Name == cja.name
			}
		case "disable":
			if disableEvent, ok := event.(CronJobDisableEvent); ok {
				matches = cja.name == "" || disableEvent.Name == cja.name
			}
		}

		if matches {
			matchingEvents++
		}
	}

	if cja.expected {
		if matchingEvents != cja.count {
			cja.actor.t.Errorf("Expected %d cron job %s events, but found %d", cja.count, cja.action, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			cja.actor.t.Errorf("Expected no cron job %s events, but found %d", cja.action, matchingEvents)
		}
	}
}

func (cja *CronJobAssertion) matchesJob(job gen.CronJob) bool {
	if cja.name != "" && job.Name != cja.name {
		return false
	}
	if cja.spec != "" && job.Spec != cja.spec {
		return false
	}
	for _, matcher := range cja.matchers {
		if !matcher(job) {
			return false
		}
	}
	return true
}

// CronJobExecutionAssertion for testing cron job executions
type CronJobExecutionAssertion struct {
	actor    *TestActor
	expected bool
	count    int
	name     gen.Atom
	hasError *bool
	matchers []func(CronJobExecutionEvent) bool
}

func (cjea *CronJobExecutionAssertion) WithName(name gen.Atom) *CronJobExecutionAssertion {
	cjea.name = name
	return cjea
}

func (cjea *CronJobExecutionAssertion) WithError() *CronJobExecutionAssertion {
	hasError := true
	cjea.hasError = &hasError
	return cjea
}

func (cjea *CronJobExecutionAssertion) WithoutError() *CronJobExecutionAssertion {
	hasError := false
	cjea.hasError = &hasError
	return cjea
}

func (cjea *CronJobExecutionAssertion) ExecutionMatching(matcher func(CronJobExecutionEvent) bool) *CronJobExecutionAssertion {
	cjea.matchers = append(cjea.matchers, matcher)
	return cjea
}

func (cjea *CronJobExecutionAssertion) Times(n int) *CronJobExecutionAssertion {
	cjea.count = n
	return cjea
}

func (cjea *CronJobExecutionAssertion) Once() *CronJobExecutionAssertion {
	return cjea.Times(1)
}

func (cjea *CronJobExecutionAssertion) Assert() {
	cjea.actor.t.Helper()

	events := cjea.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		if execEvent, ok := event.(CronJobExecutionEvent); ok {
			if cjea.matchesExecution(execEvent) {
				matchingEvents++
			}
		}
	}

	if cjea.expected {
		if matchingEvents != cjea.count {
			cjea.actor.t.Errorf("Expected %d cron job execution events, but found %d", cjea.count, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			cjea.actor.t.Errorf("Expected no cron job execution events, but found %d", matchingEvents)
		}
	}
}

func (cjea *CronJobExecutionAssertion) matchesExecution(event CronJobExecutionEvent) bool {
	if cjea.name != "" && event.Name != cjea.name {
		return false
	}
	if cjea.hasError != nil {
		hasError := event.Error != nil
		if *cjea.hasError != hasError {
			return false
		}
	}
	for _, matcher := range cjea.matchers {
		if !matcher(event) {
			return false
		}
	}
	return true
}

// SendResponseAssertion provides fluent assertions for SendResponse operations
type SendResponseAssertion struct {
	actor    *TestActor
	expected bool
	to       gen.PID
	response any
	ref      gen.Ref
	count    int
	matchers []func(any) bool
}

func (sra *SendResponseAssertion) To(target gen.PID) *SendResponseAssertion {
	sra.to = target
	return sra
}

func (sra *SendResponseAssertion) Response(response any) *SendResponseAssertion {
	sra.response = response
	return sra
}

func (sra *SendResponseAssertion) ResponseMatching(matcher func(any) bool) *SendResponseAssertion {
	sra.matchers = append(sra.matchers, matcher)
	return sra
}

func (sra *SendResponseAssertion) WithRef(ref gen.Ref) *SendResponseAssertion {
	sra.ref = ref
	return sra
}

func (sra *SendResponseAssertion) Times(n int) *SendResponseAssertion {
	sra.count = n
	return sra
}

func (sra *SendResponseAssertion) Once() *SendResponseAssertion {
	return sra.Times(1)
}

func (sra *SendResponseAssertion) Assert() {
	sra.actor.t.Helper()

	events := sra.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		responseEvent, ok := event.(SendResponseEvent)
		if !ok {
			continue
		}

		matches := true

		if sra.to.ID != 0 && responseEvent.To != sra.to {
			matches = false
		}

		if sra.response != nil && !reflect.DeepEqual(responseEvent.Response, sra.response) {
			matches = false
		}

		if sra.ref.ID[0] != 0 && responseEvent.Ref != sra.ref {
			matches = false
		}

		// Check custom matchers
		for _, matcher := range sra.matchers {
			if !matcher(responseEvent.Response) {
				matches = false
				break
			}
		}

		if matches {
			matchingEvents++
		}
	}

	if sra.expected {
		if matchingEvents != sra.count {
			sra.actor.t.Errorf("Expected %d SendResponse events, but found %d", sra.count, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			sra.actor.t.Errorf("Expected no SendResponse events, but found %d", matchingEvents)
		}
	}
}

// TerminateAssertion provides fluent assertions for actor termination
type TerminateAssertion struct {
	actor    *TestActor
	expected bool
	count    int
	reason   error
	matchers []func(error) bool
}

func (ta *TerminateAssertion) WithReason(reason error) *TerminateAssertion {
	ta.reason = reason
	return ta
}

func (ta *TerminateAssertion) ReasonMatching(matcher func(error) bool) *TerminateAssertion {
	ta.matchers = append(ta.matchers, matcher)
	return ta
}

func (ta *TerminateAssertion) Times(n int) *TerminateAssertion {
	ta.count = n
	return ta
}

func (ta *TerminateAssertion) Once() *TerminateAssertion {
	return ta.Times(1)
}

func (ta *TerminateAssertion) Assert() {
	ta.actor.t.Helper()

	events := ta.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		terminateEvent, ok := event.(TerminateEvent)
		if !ok {
			continue
		}

		matches := true

		// Check if PIDs match (terminate event should be for this actor)
		if terminateEvent.PID != ta.actor.PID() {
			matches = false
		}

		// Check specific reason if provided
		if ta.reason != nil && terminateEvent.Reason != ta.reason {
			matches = false
		}

		// Check custom matchers
		for _, matcher := range ta.matchers {
			if !matcher(terminateEvent.Reason) {
				matches = false
				break
			}
		}

		if matches {
			matchingEvents++
		}
	}

	if ta.expected {
		if matchingEvents != ta.count {
			ta.actor.t.Errorf("Expected %d Terminate events, but found %d", ta.count, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			ta.actor.t.Errorf("Expected no Terminate events, but found %d", matchingEvents)
		}
	}
}

// ExitAssertion provides fluent assertions for SendExit operations
type ExitAssertion struct {
	actor    *TestActor
	expected bool
	count    int
	to       gen.PID
	reason   error
	matchers []func(error) bool
}

func (ea *ExitAssertion) To(target gen.PID) *ExitAssertion {
	ea.to = target
	return ea
}

func (ea *ExitAssertion) WithReason(reason error) *ExitAssertion {
	ea.reason = reason
	return ea
}

func (ea *ExitAssertion) ReasonMatching(matcher func(error) bool) *ExitAssertion {
	ea.matchers = append(ea.matchers, matcher)
	return ea
}

func (ea *ExitAssertion) Times(n int) *ExitAssertion {
	ea.count = n
	return ea
}

func (ea *ExitAssertion) Once() *ExitAssertion {
	return ea.Times(1)
}

func (ea *ExitAssertion) Assert() {
	ea.actor.t.Helper()

	events := ea.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		exitEvent, ok := event.(ExitEvent)
		if !ok {
			continue
		}

		matches := true

		// Check target PID if specified
		if ea.to.ID != 0 && exitEvent.To != ea.to {
			matches = false
		}

		// Check specific reason if provided
		if ea.reason != nil && exitEvent.Reason != ea.reason {
			matches = false
		}

		// Check custom matchers
		for _, matcher := range ea.matchers {
			if !matcher(exitEvent.Reason) {
				matches = false
				break
			}
		}

		if matches {
			matchingEvents++
		}
	}

	if ea.expected {
		if matchingEvents != ea.count {
			ea.actor.t.Errorf("Expected %d SendExit events, but found %d", ea.count, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			ea.actor.t.Errorf("Expected no SendExit events, but found %d", matchingEvents)
		}
	}
}

// ExitMetaAssertion provides fluent assertions for SendExitMeta operations
type ExitMetaAssertion struct {
	actor    *TestActor
	expected bool
	count    int
	meta     gen.Alias
	reason   error
	matchers []func(error) bool
}

func (ema *ExitMetaAssertion) ToMeta(target gen.Alias) *ExitMetaAssertion {
	ema.meta = target
	return ema
}

func (ema *ExitMetaAssertion) WithReason(reason error) *ExitMetaAssertion {
	ema.reason = reason
	return ema
}

func (ema *ExitMetaAssertion) ReasonMatching(matcher func(error) bool) *ExitMetaAssertion {
	ema.matchers = append(ema.matchers, matcher)
	return ema
}

func (ema *ExitMetaAssertion) Times(n int) *ExitMetaAssertion {
	ema.count = n
	return ema
}

func (ema *ExitMetaAssertion) Once() *ExitMetaAssertion {
	return ema.Times(1)
}

func (ema *ExitMetaAssertion) Assert() {
	ema.actor.t.Helper()

	events := ema.actor.Events()
	matchingEvents := 0

	for _, event := range events {
		exitEvent, ok := event.(ExitMetaEvent)
		if !ok {
			continue
		}

		matches := true

		// Check target meta if specified
		if ema.meta.ID[0] != 0 && exitEvent.Meta != ema.meta {
			matches = false
		}

		// Check specific reason if provided
		if ema.reason != nil && exitEvent.Reason != ema.reason {
			matches = false
		}

		// Check custom matchers
		for _, matcher := range ema.matchers {
			if !matcher(exitEvent.Reason) {
				matches = false
				break
			}
		}

		if matches {
			matchingEvents++
		}
	}

	if ema.expected {
		if matchingEvents != ema.count {
			ema.actor.t.Errorf("Expected %d SendExitMeta events, but found %d", ema.count, matchingEvents)
		}
	} else {
		if matchingEvents > 0 {
			ema.actor.t.Errorf("Expected no SendExitMeta events, but found %d", matchingEvents)
		}
	}
}
