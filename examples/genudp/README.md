## UDP demo scenario ##

This example implements a simple application that starts the child process with UDP server

Here is output of this example:
```
❯❯❯❯ go run .
Start node udp@127.0.0.1
Application started!
send string "b65574c95fbdcd8e" to "[::1]:5533"
[UDP handler] got message from "[::1]:49634": "b65574c95fbdcd8e"
send string "64c203bbf65e121c" to "[::1]:5533"
[UDP handler] got message from "[::1]:49634": "64c203bbf65e121c"
send string "2f2d280a8ad76bd4" to "[::1]:5533"
[UDP handler] got message from "[::1]:49634": "2f2d280a8ad76bd4"
send string "7c125d26809ec335" to "[::1]:5533"
[UDP handler] got message from "[::1]:49634": "7c125d26809ec335"
send string "01317dae8aa231b2" to "[::1]:5533"
[UDP handler] got message from "[::1]:49634": "01317dae8aa231b2"
stop node udp@127.0.0.1
```
