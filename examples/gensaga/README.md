## Saga demo scenario ##

1. Start Tx on Saga1. Saga1 sends it to Saga2 with enabled TrapCancel, Saga2 sends Tx to Saga3.

```
  Saga1 -> Tx -> Saga2 -> Tx -> Saga3
```

2. Saga2 terminates, and Saga1 handle it, sending this Tx to Saga4 and Saga4

```
         --------> Tx ------> Saga4 -------- Tx ------->
       /                                                 \
  Saga1 <--- signal DOWN <--- Saga2 ---> signal DOWN ---> Saga3

```

3. Saga1 commits Tx when its done

```
         --> signal COMMIT --> Saga4 --> signal COMMIT -->
       /                                                   \
  Saga1 ............... Saga2 (terminated) ................ Saga3

```
