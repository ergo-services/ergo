## Saga demo scenario ##

1. Start Tx on Saga1 with enabled TwoPhaseCommit option. Saga1 sends this Tx to Saga2 with enabled TrapCancel option, Saga2 sends Tx to Saga3.

```
  Saga1 ---> Tx ---> Saga2 ---> Tx ---> Saga3
```

2. Saga2 terminates, and Saga1 handles it by sending this Tx to Saga4, and Saga4 sends it to Saga3.

```
         --------> Tx ------> Saga4 -------- Tx ------->
       /                                                 \
  Saga1 <--- signal DOWN <--- Saga2 ---> signal DOWN ---> Saga3

```

3. Saga1 commits Tx when its done.

```
         --> signal COMMIT --> Saga4 --> signal COMMIT -->
       /                                                   \
  Saga1 ............... Saga2 (terminated) ................ Saga3

```

Here is output of this example

```
❯❯❯❯ go run .
Starting node: node1@localhost and Saga1 process...OK
Starting node: node2@localhost and Saga2 process...OK
Starting node: node3@localhost and Saga3 process...OK
Starting node: node4@localhost and Saga4 process...OK
Saga1. Start new transaction TX#25333.23216.0
 Worker started on Saga1 with value "Hello "
Saga1. Received result from worker with value "Hello W"
Saga1. ...sent TX#25333.23216.0 further, to the Saga2 on Node2 (Next#25335.23216.0)
Saga2. Received TX#25333.23216.0 with value "Hello Wo"
 Worker started on Saga2 with value "Hello Wo"
Saga2. Received result from worker with value "Hello Wor"
Saga2. ...sent TX#25333.23216.0 further, to the Saga3 on Node3 (Next#18646.23216.0)
Saga2 termination
 Worker terminated on Saga2 with reason: "normal"
Saga3. Received TX#25333.23216.0 with value "Hello Worl"
Saga1. Trapped cancelation TX#25333.23216.0. Reason "next saga {saga2 node2@localhost} is down"
Saga1. ...sent TX#25333.23216.0 further, to the Saga4 on Node4 (Next#25336.23216.0)
Saga4. Received TX#25333.23216.0 with value "Hello Wo"
 Worker started on Saga3 with value "Hello Worl"
Saga3. Received result from worker with value "Hello World"
Saga3. ...sent result "Hello World!" to the parent saga for TX#25333.23216.0
Saga3. Canceled TX#25333.23216.0 with reason: "parent saga <59AA5040.0.1008> is down"
 Worker terminated on Saga3 with reason: "normal"
 Worker started on Saga4 with value "Hello Wo"
Saga4. Received result from worker with value "Hello Wor"
Saga4. ...sent TX#25333.23216.0 further, to the Saga3 on Node3 (Next#179533.23216.0)
Saga3. Received TX#25333.23216.0 with value "Hello Worl"
 Worker started on Saga3 with value "Hello Worl"
Saga3. Received result from worker with value "Hello World"
Saga3. ...sent result "Hello World!" to the parent saga for TX#25333.23216.0
Saga4. Received result for TX#25333.23216.0 from Saga3 (Next#179533.23216.0) with value "Hello World!"
Saga4. ...sent result "Hello World!" to the parent saga for TX#25333.23216.0
Saga1. Received result for TX#25333.23216.0 from Saga4 (Next#25336.23216.0) with value "Hello World!"
Saga1. Final result for TX#25333.23216.0: "Hello World! 🚀"
 Worker on Saga1 received final result for TX#25333.23216.0 with value "Hello World! 🚀"
 Worker terminated on Saga1 with reason: "normal"
Saga4. Final result for TX#25333.23216.0: "Hello World! 🚀"
 Worker on Saga4 received final result for TX#25333.23216.0 with value "Hello World! 🚀"
 Worker terminated on Saga4 with reason: "normal"
Saga3. Final result for TX#25333.23216.0: "Hello World! 🚀"
 Worker on Saga3 received final result for TX#25333.23216.0 with value "Hello World! 🚀"
 Worker terminated on Saga3 with reason: "normal"
```
