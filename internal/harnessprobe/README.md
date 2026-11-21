# Closed release probe

The staged runner accepts only the five compile-time probe tuples. It keeps the
exact 5 s action context and inherits the caller environment with `Cmd.Env == nil`.
A fixed, nonconfigurable `Cmd.WaitDelay` of 100 ms bounds pipe cleanup after
context completion or an observed exit. OS scheduling is not a real-time guarantee.

Cancellation, deadline, every non-nil Wait error (including `ErrWaitDelay` after
a zero exit), and incomplete or oversized output return a typed failure. Partial
output never becomes a successful observation. This bounds the caller; it does
not reap descendants that retain output handles.

Tests use inert modes of the test executable and stop their recorded helper
children by PID. They pair open-pipe cancellation and zero-exit failures with a
clean-output positive and a known zero-delay violation. No vendor is executed.

```sh
go test -count=1 ./internal/harnessprobe
go test -race ./internal/harnessprobe
```

Use isolated HOME/TMP and caches; keep local evidence under ignored `.temp/`.
Native vendor release compatibility and parsing remain unported.
