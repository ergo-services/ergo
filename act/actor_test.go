package act

import "testing"

// TODO: split-handler coverage (migrated intent of the old testing/tests/001_local
// t008_actor_split_test.go). To be implemented with the testing/unit harness once
// that work is the focus, then this stub removed.
//
// Cover, via testing/unit:
//   - SplitHandle() defaults to false; SetSplitHandle(true)/(false) toggles it and
//     SplitHandle() reflects the current value.
//   - With split disabled, every incoming message/call goes to HandleMessage /
//     HandleCall regardless of how the process was addressed.
//   - With split enabled, dispatch is by addressing:
//       by registered name  -> HandleMessageName / HandleCallName (name carried in)
//       by alias            -> HandleMessageAlias / HandleCallAlias (alias carried in)
//       by PID              -> HandleMessage / HandleCall
//   - Toggling split at runtime re-routes subsequent messages/calls accordingly.
func TestActorSplitHandle(t *testing.T) {
	t.Skip("TODO: implement split-handler coverage with the testing/unit harness")
}
