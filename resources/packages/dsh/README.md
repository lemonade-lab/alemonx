# ALemonX DSH Runtime

This directory pins the only Agent runtime used by ALemonX. It is installed as
an application dependency, not as a user-managed global package. The Go host
starts it with an application-owned `DSH_HOME` and communicates exclusively
through the `sdk` JSON-RPC profile.

Custom ALemonX DSH plugins belong under this directory. They must expose robot
and PM2 capabilities through the loopback bridge; browser clients never talk
to DSH directly.
