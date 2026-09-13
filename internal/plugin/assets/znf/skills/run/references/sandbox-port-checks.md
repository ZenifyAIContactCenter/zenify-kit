<!-- Moved verbatim from run/SKILL.md § Checking the port: `lsof`, never `nc` or `curl` (W4 slim-skills). Read when: `nc`/`curl` says a port is closed. -->

Measured against two ports that were genuinely listening: `nc -z` said both were free, `curl` returned
`http 000`, and `lsof` got both right. A check that always answers "free" is worse than no check,
since it launders a wrong port into something that looks verified. `lsof` queries the kernel's
socket table instead of dialling, which is why it survives.

This is the same failure as `herdr agent prompt --wait --until idle` returning at once because
the agent was already idle. Any wait primitive can be satisfied by state that predates the thing
you are waiting for; make the condition impossible to meet before the event.
