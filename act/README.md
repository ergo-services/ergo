## Standard Actors Library ##

### Actor
  Implements generic Actor behavior.

Doc: https://docs.ergo.services/actors/actor

### Supervisor
  Implements generic Supervisor behavior.

A supervisor is responsible for starting, stopping, and monitoring its child processes. The basic idea of a supervisor is that it is to keep its child processes alive by restarting them when necessary.

Doc: https://docs.ergo.services/actors/supervisor

### Pool
  Implements generic pool of workers.

  This behavior implements a basic design pattern with a pool of workers. All messages/requests received by the pool process are forwarded to the workers using the "Round Robin" algorithm. The worker process is automatically restarting on termination.

Doc: https://docs.ergo.services/actors/pool

### Web
  Web API Gateway behavior.

  The Web API Gateway pattern is also sometimes known as the "Backend For Frontend" (BFF)  because you build it while thinking about the needs of the client app. Therefore, BFF sits between the client apps and the microservices. It acts as a reverse proxy, routing requests from clients to services.

Doc: https://docs.ergo.services/actors/web

