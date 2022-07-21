## TCP demo scenario ##

This example implements a simple application that starts the child process with TCP server

Here is output of this example:
```
❯❯❯❯ go run . -tls
Start node tcp@127.0.0.1
TLS enabled. Generated self signed certificate. You may check it with command below:
   $ openssl s_client -connect :8383
Application started!
[TCP handler] got new connection from "127.0.0.1:35962"
send string "d1f5d8f197f0a4c8" to "127.0.0.1:8383"
[TCP handler] got message from "127.0.0.1:35962": "d1f5d8f197f0a4c8"
send string "4abff0c11d36ed56" to "127.0.0.1:8383"
[TCP handler] got message from "127.0.0.1:35962": "4abff0c11d36ed56"
send string "d2c5e85c04c0b946" to "127.0.0.1:8383"
[TCP handler] got message from "127.0.0.1:35962": "d2c5e85c04c0b946"
send string "782559e4d6ec170c" to "127.0.0.1:8383"
[TCP handler] got message from "127.0.0.1:35962": "782559e4d6ec170c"
send string "08d2eced5329143c" to "127.0.0.1:8383"
[TCP handler] got message from "127.0.0.1:35962": "08d2eced5329143c"
stop node tcp@127.0.0.1
```
