## Supervisor demo scenario ##

```
    demoSup
       |
        - demoServer01
        - demoServer02
        - demoServer03
```

Here is output of this example:
```
❯❯❯❯ go run .

to stop press Ctrl-C

Started new process
        Pid: <32747620.0.1012>
        Name: "demoServer01"
        Parent: <32747620.0.1011>
        Args:[]etf.Term(nil)
Started new process
        Pid: <32747620.0.1013>
        Name: "demoServer02"
        Parent: <32747620.0.1011>
        Args:[]etf.Term{12345}
Started new process
        Pid: <32747620.0.1014>
        Name: "demoServer03"
        Parent: <32747620.0.1011>
        Args:[]etf.Term{"abc", 67890}
Started supervisor process <32747620.0.1011>

```
