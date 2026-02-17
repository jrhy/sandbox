# AGENTS Notes

For tracing `sb exec` sandbox policy problems, prefer this command:

```sh
log stream --style syslog --level debug --predicate 'eventMessage CONTAINS[c] "Sandbox"' |& tee /tmp/foo
```

This streams Sandbox-related logs and captures them to `/tmp/foo` for later review.
