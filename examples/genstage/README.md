## Stage demo scenario ##

This example demonstrates a simple implementation of publisher/subscriber using gen.Stage behavior.
The basic scenario is sending numbers to the consumers but distinguishing them on odd and even.

It starts with two nodes - `node_abc@localhost` with one producer and `node_def@localhost` with two consumers. In this example, the producer uses a `partition` dispatcher with the hash function to distinguish numbers (there are three types of dispatchers - `demand`, `broadcast`, `partition`).

Here is output of this example

```
❯❯❯❯ go run .

to stop press Ctrl-C

Starting nodes 'node_abc@localhost' and 'node_def@localhost'
Spawn producer on 'node_abc@localhost'
Spawn 2 consumers on 'node_def@localhost'
Subscribe consumer even [ <6E7F6C08.0.1011> ] with min events = 1 and max events 2
Subscribe consumer odd [ <6E7F6C08.0.1012> ] with min events = 2 and max events 4
New subscription from: <6E7F6C08.0.1011> with min: 1 and max: 2
Producer: just got demand for 2 event(s) from <6E7F6C08.0.1011>
Producer. Generate random numbers and send them to consumers... [62 75 36 10 56]
New subscription from: <6E7F6C08.0.1012> with min: 2 and max: 4
Consumer 'even' got events: [62 36]
Producer: just got demand for 2 event(s) from <6E7F6C08.0.1011>
Producer. Generate random numbers and send them to consumers... [74 64 57 60 6]
Consumer 'even' got events: [60 6]
Consumer 'even' got events: [10 56]
Consumer 'even' got events: [74 64]
Producer: just got demand for 2 event(s) from <6E7F6C08.0.1011>
Producer. Generate random numbers and send them to consumers... [58 62 60 53 63]
Consumer 'even' got events: [60]
Consumer 'even' got events: [58 62]
Producer: just got demand for 2 event(s) from <6E7F6C08.0.1011>
Producer. Generate random numbers and send them to consumers... [47 58 25 93 92]
Consumer 'even' got events: [58 92]
```
