package tests

import "testing"

// this test implements cases below
// 1. Single saga cancels Tx
// 2. Saga1 -> Tx -> Saga2 -> Tx -> Saga3
//    a) Saga1 cancels Tx
//       Saga1 -> cancel -> Saga2 -> cancel -> Saga3
//    b) Saga2 cancels Tx
//       Saga1 <- cancel <- Saga2 -> cancel -> Saga3
//    c) Saga3 cancels Tx
//       Saga1 <- cancel <- Saga2 <- cancel <- Saga3
//    d) Saga1 sets TrapCancel, Saga2 process/node is going down, Saga1 sends Tx to the Saga4
//              -> Tx -> Saga4 -> Tx -> Saga3
//            /
//       Saga1 <- signal Down <- Saga2 (terminates) -> signal Down -> Saga3

func TestSagaCancel(t *testing.T) {

}
