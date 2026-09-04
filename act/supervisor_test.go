package act

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// helpers shared by supervisor unit tests

func makePID(id uint64) gen.PID {
	return gen.PID{Node: "test@localhost", ID: id, Creation: 1}
}

func dummyFactory() gen.ProcessBehavior {
	return &Actor{}
}

// normSpec applies the same default normalization that act.Supervisor.ProcessInit
// applies before invoking sup.init. Test helpers use it so that unit tests
// exercise supBehavior implementations as the runtime would.
func normSpec(spec SupervisorSpec) SupervisorSpec {
	if spec.Restart.Strategy == SupervisorStrategyInherit {
		spec.Restart.Strategy = SupervisorStrategyTransient
	}
	if spec.Restart.Intensity == 0 {
		spec.Restart.Intensity = defaultRestartIntensity
	}
	if spec.Restart.Period == 0 {
		spec.Restart.Period = defaultRestartPeriod
	}
	return spec
}

//
// SupervisorStrategy.String / OnExceed.String
//

func TestSupervisorStrategyString(t *testing.T) {
	tests := []struct {
		s    SupervisorStrategy
		want string
	}{
		{SupervisorStrategyInherit, "Inherit"},
		{SupervisorStrategyTransient, "Transient"},
		{SupervisorStrategyTemporary, "Temporary"},
		{SupervisorStrategyPermanent, "Permanent"},
		{SupervisorStrategy(99), "Bug: unknown supervisor strategy type"},
	}
	for _, tc := range tests {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("SupervisorStrategy(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestOnExceedString(t *testing.T) {
	tests := []struct {
		v    OnExceed
		want string
	}{
		{OnExceedTerminateSupervisor, "TerminateSupervisor"},
		{OnExceedDisable, "Disable"},
		{OnExceed(99), "Bug: unknown OnExceed"},
	}
	for _, tc := range tests {
		if got := tc.v.String(); got != tc.want {
			t.Errorf("OnExceed(%d).String() = %q, want %q", tc.v, got, tc.want)
		}
	}
}

//
// resolveStrategy
//

func TestResolveStrategy(t *testing.T) {
	tests := []struct {
		name  string
		sup   SupervisorStrategy
		child SupervisorStrategy
		want  SupervisorStrategy
	}{
		{"inherit returns supervisor's", SupervisorStrategyPermanent, SupervisorStrategyInherit, SupervisorStrategyPermanent},
		{"override permanent over transient", SupervisorStrategyTransient, SupervisorStrategyPermanent, SupervisorStrategyPermanent},
		{"override transient over permanent", SupervisorStrategyPermanent, SupervisorStrategyTransient, SupervisorStrategyTransient},
		{"override temporary over transient", SupervisorStrategyTransient, SupervisorStrategyTemporary, SupervisorStrategyTemporary},
		{"both inherit", SupervisorStrategyInherit, SupervisorStrategyInherit, SupervisorStrategyInherit},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveStrategy(tc.sup, tc.child); got != tc.want {
				t.Errorf("resolveStrategy(%s, %s) = %s, want %s", tc.sup, tc.child, got, tc.want)
			}
		})
	}
}

//
// supCheckRestartIntensity
//

func TestCheckRestartIntensityNotExceededAfterFirst(t *testing.T) {
	r, exceeded := supCheckRestartIntensity(nil, 5, 5)
	if exceeded {
		t.Errorf("first restart should not exceed")
	}
	if len(r) != 1 {
		t.Errorf("expected 1 restart logged, got %d", len(r))
	}
}

func TestCheckRestartIntensityNotExceededAtThreshold(t *testing.T) {
	now := time.Now().UnixMilli()
	// 4 prior entries + 1 new = 5 == intensity, not exceeded
	r := []int64{now - 100, now - 80, now - 60, now - 40}
	r2, exceeded := supCheckRestartIntensity(r, 5, 5)
	if exceeded {
		t.Errorf("at exactly intensity should not exceed")
	}
	if len(r2) != 5 {
		t.Errorf("expected 5 entries, got %d", len(r2))
	}
}

func TestCheckRestartIntensityExceededInWindow(t *testing.T) {
	now := time.Now().UnixMilli()
	r := []int64{now - 100, now - 80, now - 60, now - 40, now - 20, now - 10}
	_, exceeded := supCheckRestartIntensity(r, 5, 5)
	if exceeded == false {
		t.Errorf("6 events in 5s window with intensity=5 should exceed")
	}
}

func TestCheckRestartIntensityOldEventsPruned(t *testing.T) {
	now := time.Now().UnixMilli()
	// 5 old events outside window + 1 fresh
	r := []int64{now - 10000, now - 9000, now - 8000, now - 7000, now - 6000}
	r2, exceeded := supCheckRestartIntensity(r, 5, 5)
	if exceeded {
		t.Errorf("old entries should be pruned, no exceed")
	}
	// after prune+append, only the new one stays since old are out of 5s window
	if len(r2) != 1 {
		t.Errorf("expected 1 entry after prune, got %d", len(r2))
	}
}

func TestCheckRestartIntensityIntensityOne(t *testing.T) {
	now := time.Now().UnixMilli()
	r := []int64{now - 100}
	_, exceeded := supCheckRestartIntensity(r, 1, 1)
	if exceeded == false {
		t.Errorf("two events in 1s window with intensity=1 should exceed")
	}
}

//
// validateChildRestart
//

func TestValidateChildRestart(t *testing.T) {
	tests := []struct {
		name    string
		r       SupervisorChildRestart
		t       SupervisorType
		wantErr bool
	}{
		{
			name:    "all defaults OK",
			r:       SupervisorChildRestart{},
			t:       SupervisorTypeOneForOne,
			wantErr: false,
		},
		{
			name:    "Strategy override only OK",
			r:       SupervisorChildRestart{Strategy: SupervisorStrategyPermanent},
			t:       SupervisorTypeOneForOne,
			wantErr: false,
		},
		{
			name:    "OFO with Intensity OK",
			r:       SupervisorChildRestart{Intensity: 5, Period: 10},
			t:       SupervisorTypeOneForOne,
			wantErr: false,
		},
		{
			name:    "SOFO with Intensity OK",
			r:       SupervisorChildRestart{Intensity: 5, Period: 10},
			t:       SupervisorTypeSimpleOneForOne,
			wantErr: false,
		},
		{
			name:    "ARFO with Intensity rejected",
			r:       SupervisorChildRestart{Intensity: 5, Period: 10},
			t:       SupervisorTypeAllForOne,
			wantErr: true,
		},
		{
			name:    "ROFO with Intensity rejected",
			r:       SupervisorChildRestart{Intensity: 5, Period: 10},
			t:       SupervisorTypeRestForOne,
			wantErr: true,
		},
		{
			name:    "Period without Intensity rejected",
			r:       SupervisorChildRestart{Period: 10},
			t:       SupervisorTypeOneForOne,
			wantErr: true,
		},
		{
			name:    "OnExceedDisable without Intensity rejected",
			r:       SupervisorChildRestart{OnExceed: OnExceedDisable},
			t:       SupervisorTypeOneForOne,
			wantErr: true,
		},
		{
			name:    "OnExceedDisable with Intensity OK",
			r:       SupervisorChildRestart{Intensity: 5, OnExceed: OnExceedDisable},
			t:       SupervisorTypeOneForOne,
			wantErr: false,
		},
		{
			name:    "OnExceedDisable on SOFO with Intensity OK",
			r:       SupervisorChildRestart{Intensity: 5, OnExceed: OnExceedDisable},
			t:       SupervisorTypeSimpleOneForOne,
			wantErr: false,
		},
		{
			name:    "unknown Strategy rejected",
			r:       SupervisorChildRestart{Strategy: SupervisorStrategy(99)},
			t:       SupervisorTypeOneForOne,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateChildRestart(tc.r, tc.t)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateChildRestart() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

//
// gen.Error wrapping on supervisor exit
//

func TestGenErrorUnwrap(t *testing.T) {
	inner := errors.New("inner reason")
	outer := &gen.Error{Msg: "wrapper: inner reason", Wrapped: []error{inner}}

	if errors.Is(outer, inner) == false {
		t.Errorf("errors.Is must reach Wrapped element")
	}
	ws := outer.Unwrap()
	if len(ws) != 1 || ws[0] != inner {
		t.Errorf("Unwrap() returned %v, want [inner]", ws)
	}
}

func TestGenErrorMessage(t *testing.T) {
	e := &gen.Error{Msg: "outer: inner", Wrapped: []error{errors.New("inner")}}
	if got := e.Error(); got != "outer: inner" {
		t.Errorf("Error() = %q, want %q", got, "outer: inner")
	}
}

func TestGenErrorMessageEmptyMsgFallback(t *testing.T) {
	inner := errors.New("inner")
	e := &gen.Error{Wrapped: []error{inner}}
	if got := e.Error(); got != "inner" {
		t.Errorf("Error() with empty Msg should fall back to Wrapped[0]; got %q", got)
	}
}

func TestGenErrorMatchesMultipleWrapped(t *testing.T) {
	a := errors.New("a")
	b := errors.New("b")
	e := &gen.Error{Msg: "a: b", Wrapped: []error{a, b}}

	if errors.Is(e, a) == false {
		t.Errorf("errors.Is must match first Wrapped")
	}
	if errors.Is(e, b) == false {
		t.Errorf("errors.Is must match second Wrapped")
	}
}

func TestGenErrorNilReceiver(t *testing.T) {
	var e *gen.Error
	if e.Error() != "" {
		t.Errorf("nil receiver Error() must be empty")
	}
	if e.Unwrap() != nil {
		t.Errorf("nil receiver Unwrap() must be nil")
	}
}

func TestGenErrorf(t *testing.T) {
	marker := errors.New("payment declined")
	err := gen.Errorf("user %d: %w", 42, marker)

	if err.Error() != "user 42: payment declined" {
		t.Errorf("formatted message mismatch: %q", err.Error())
	}
	if errors.Is(err, marker) == false {
		t.Errorf("errors.Is must find the %%w marker")
	}
	var ge *gen.Error
	if errors.As(err, &ge) == false {
		t.Fatalf("errors.As must extract *gen.Error")
	}
	if len(ge.Wrapped) != 1 || ge.Wrapped[0] != marker {
		t.Errorf("Wrapped = %v, want [marker]", ge.Wrapped)
	}
}

func TestGenErrorfMultiWrap(t *testing.T) {
	a := errors.New("a")
	b := errors.New("b")
	err := gen.Errorf("%w and %w", a, b)

	if errors.Is(err, a) == false || errors.Is(err, b) == false {
		t.Errorf("errors.Is must match both wrapped markers")
	}
}

func TestGenErrorfNoWrap(t *testing.T) {
	err := gen.Errorf("just text %d", 7)
	if err.Error() != "just text 7" {
		t.Errorf("formatted text mismatch: %q", err.Error())
	}
	var ge *gen.Error
	if errors.As(err, &ge) == false {
		t.Fatalf("errors.As must extract *gen.Error")
	}
	if ge.Wrapped != nil {
		t.Errorf("Wrapped must be nil when no %%w; got %v", ge.Wrapped)
	}
}
