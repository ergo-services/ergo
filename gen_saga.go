package ergo

import (
	"fmt"

	"github.com/halturin/ergo/etf"
)

type GenSaga struct {
	GenServer
}

type GenSagaOptions struct{}

type GenSagaState struct {
	Options GenSagaOptions
	State   interface{}
}

type GenSagaTransaction struct {
	Name string
	Pid  etf.Pid
	Ref  etf.Ref
}

type GenSagaBehaviour interface {
	InitSaga(process *Process, args ...interface{}) (GenSagaOptions, interface{})

	HandleNext(tx GenSagaTransaction, state GenSagaState) error
	HandleCanceled(tx GenSagaTransaction, reason string, state GenSagaState) error
	HandleInterim(tx GenSagaTransaction, interim interface{}, state GenSagaState) error
	HandleDone(tx GenSagaTransaction, result interface{}, state GenSagaState) error

	HandleGenSagaCall(from etf.Tuple, message etf.Term, state GenSagaState) (string, etf.Term)
	HandleGenSageCast(message etf.Term, state GenSagaState) string
	HandleGenSagaInfo(message etf.Term, state GenSagaState) string
}

// default GenSaga callbacks

func (gs *GenSaga) InitSaga(process *Process, args ...interface{}) (GenSagaOptions, error) {
	opts := GenSagaOptions{}
	return opts, nil
}

func (gs *GenSaga) HandleGenSagaCall(from etf.Tuple, message etf.Term, state GenSagaState) (string, etf.Term) {
	// default callback if it wasn't implemented
	fmt.Printf("HandleGenSagaCall: unhandled message (from %#v) %#v\n", from, message)
	return "reply", etf.Atom("ok")
}

func (gs *GenSaga) HandleGenSagaCast(message etf.Term, state GenSagaState) string {
	// default callback if it wasn't implemented
	fmt.Printf("HandleGenSagaCast: unhandled message %#v\n", message)
	return "noreply"
}
func (gs *GenSaga) HandleGenSagaInfo(message etf.Term, state GenSagaState) string {
	// default callback if it wasn't implemnted
	fmt.Printf("HandleGenSagaInfo: unhandled message %#v\n", message)
	return "noreply"
}

func (gs *GenSaga) HandleNext(tx GenSagaTransaction, state GenSagaState) error {
	return nil
}
func (gs *GenSaga) HandleCanceled(tx GenSagaTransaction, reason string, state GenSagaState) error {
	return nil
}
func (gs *GenSaga) HandleInterim(tx GenSagaTransaction, interim interface{}, state GenSagaState) error {
	return nil
}
func (gs *GenSaga) HandleDone(tx GenSagaTransaction, result interface{}, state GenSagaState) error {
	return nil
}
