## Raft demo scenario ##

1. Starting 4 nodes with raft processes. They are connecting to each other to build a quorum. Since we use 4 raft processes, only 3 of them will be chosen for the quorum (gen.RaftQuorumState3). One becomes a follower (it won't receive the leader election result).
2. Quorum is built. All cluster members receive infomartion about the new quorum (invoking HandleQuorum callback on each raft process). Quorum members start leader election.
3. Leader is elected (chose the raft process with the latest 'serial'). Every quorum member receives information about the leader and its 'serial' (invoking HandleLeader callback on them).
4. Raft process checks the missing serials on its storage and makes a Get request to the cluster to receive them.
5. Follower makes an Append request to add a new key/value to the cluster (it sends to the quorum member, and the quorum member forwards it further to the leader)
6. All cluster members receive the new key/value with the increased 'serial' by 1.
7. Raft process checks the missing serials on its storage and makes a Get request to the cluster to receive them (follower had no chance to make this check since it doesn't receive information with the election leader result)

Here is output of this example

```
❯❯❯❯ go run .

to stop press Ctrl-C or wait 10 seconds...

Starting node: node1@localhost and raft1 process - Serial: 1 PID: <1B30E9C5.0.1008> ...OK
Starting node: node2@localhost and raft2 process - Serial: 4 PID: <08151198.0.1008> ...OK
Starting node: node3@localhost and raft3 process - Serial: 4 PID: <9FF54552.0.1008> ...OK
Starting node: node4@localhost and raft4 process - Serial: 0 PID: <2E5EE122.0.1008> ...OK
<08151198.0.1008> Quorum built - State: 3 Quorum member: true
<9FF54552.0.1008> Quorum built - State: 3 Quorum member: true
<2E5EE122.0.1008> Quorum built - State: 3 Quorum member: false
<2E5EE122.0.1008>     since I'm not a quorum member, I won't receive any information about elected leader
<1B30E9C5.0.1008> Quorum built - State: 3 Quorum member: true
<08151198.0.1008> I'm a leader of this quorum
<9FF54552.0.1008> Leader elected: <08151198.0.1008> with serial 4
<1B30E9C5.0.1008> Leader elected: <08151198.0.1008> with serial 4
<1B30E9C5.0.1008> Missing serials: 2 .. 4
<1B30E9C5.0.1008>     requested missing serial to the cluster: 2  id: Ref#<1B30E9C5.138519.23452.0>
<1B30E9C5.0.1008>     requested missing serial to the cluster: 3  id: Ref#<1B30E9C5.138520.23452.0>
<1B30E9C5.0.1008>     requested missing serial to the cluster: 4  id: Ref#<1B30E9C5.138521.23452.0>
<08151198.0.1008> Received request for serial 2
<08151198.0.1008> Received request for serial 3
<08151198.0.1008> Received request for serial 4
<1B30E9C5.0.1008> Received requested serial 2 with key key2 and value value2
<1B30E9C5.0.1008> Received requested serial 3 with key key3 and value value3
<1B30E9C5.0.1008> Received requested serial 4 with key key4 and value value4
<08151198.0.1008> Received append request with serial 5, key "key100" and value "value100"
<2E5EE122.0.1008> Received append request with serial 5, key "key100" and value "value100"
<1B30E9C5.0.1008> Received append request with serial 5, key "key100" and value "value100"
<2E5EE122.0.1008> Missing serial: 1  requested missing serial to the cluster. id Ref#<2E5EE122.40367.23452.0>
<2E5EE122.0.1008> Missing serial: 2  requested missing serial to the cluster. id Ref#<2E5EE122.40368.23452.0>
<2E5EE122.0.1008> Missing serial: 3  requested missing serial to the cluster. id Ref#<2E5EE122.40369.23452.0>
<2E5EE122.0.1008> Missing serial: 4  requested missing serial to the cluster. id Ref#<2E5EE122.40370.23452.0>
<9FF54552.0.1008> Received append request with serial 5, key "key100" and value "value100"
<1B30E9C5.0.1008> Received request for serial 1
<08151198.0.1008> Received request for serial 2
<08151198.0.1008> Received request for serial 3
<08151198.0.1008> Received request for serial 4
<2E5EE122.0.1008> Received requested serial 1 with key key1 and value value1
<2E5EE122.0.1008> Received requested serial 2 with key key2 and value value2
<2E5EE122.0.1008> Received requested serial 3 with key key3 and value value3
<2E5EE122.0.1008> Received requested serial 4 with key key4 and value value4

```
