# Oberth for VS Code

Delegate coding work to a local Oberth runtime without giving an agent direct
access to the main checkout. The extension starts tasks, reports runtime
status, opens the latest isolated diff, and links to the local Control Room.

## Requirements

- A running Oberth local service.
- The `oberth` CLI available on `PATH`, installed in its standard per-user
  location, or configured through `oberth.cliPath`.

Oberth is a review boundary, not an operating-system sandbox. Review recorded
commands, evidence, and diffs before approving a task.
