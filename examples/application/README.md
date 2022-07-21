## Application demo scenario ##

This example implements a simple application that starts one server and one supervisor. Here is supervision tree of this application

```
  demoApp
    |
     - demoSup
    |    |
    |     - demoServer01
    |     - demoServer02
    |     - demoServer03
    |
     - demoServer
```

Here is output of this example:
```
❯❯❯❯ go run .

to stop press Ctrl-C

Started new process
        Pid: <1A1F7F53.0.1013>
        Name: "demoServer01"
        Parent: <1A1F7F53.0.1012>
        Args:[]etf.Term(nil)
Started new process
        Pid: <1A1F7F53.0.1014>
        Name: "demoServer02"
        Parent: <1A1F7F53.0.1012>
        Args:[]etf.Term{12345}
Started new process
        Pid: <1A1F7F53.0.1015>
        Name: "demoServer03"
        Parent: <1A1F7F53.0.1012>
        Args:[]etf.Term{"abc", 67890}
Started new process
        Pid: <1A1F7F53.0.1016>
        Name: "demoServer"
        Parent: <1A1F7F53.0.1011>
        Args:[]etf.Term(nil)
Application started with Pid <1A1F7F53.0.1011>!

```
