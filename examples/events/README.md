## Events demo scenario ##

This example implements simple publisher and subscriber to demonstrate `gen.Event` feature.

Here is output of this example:

```
❯❯❯❯ go run .
Start node eventsnode@localhost
process <91BC521D.0.1011> registered event simple
Started process <91BC521D.0.1011> with name "producer"
Started process <91BC521D.0.1012> with name "consumer"
... producing event: {EVNT 1}
consumer got event:  {EVNT 1}
... producing event: {EVNT 2}
consumer got event:  {EVNT 2}
... producing event: {EVNT 3}
consumer got event:  {EVNT 3}
... producing event: {EVNT 4}
consumer got event:  {EVNT 4}
... producing event: {EVNT 5}
consumer got event:  {EVNT 5}
producer has terminated
Stop node eventsnode@localhost

```
