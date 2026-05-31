# base-wan

Standard multi-region Hoyan WAN lab. It covers FRR edge/customer/transit
routers, one cEOS core, one SR Linux core, BGP propagation, ACL dataplane
checks, prefix classes, recursive next-hop modeling, and RIB/FIB comparison.

Examples:

```bash
go run ./cmd/hoyan intent verify --lab labs/base-wan --format json
go run ./cmd/hoyan compare labs/base-wan/hoyan.clab.yml labs/base-wan/hoyan.clab.yml --left-type model --right-type clab
go run ./cmd/hoyan compare labs/base-wan/hoyan.clab.yml labs/base-wan/hoyan.clab.yml --left-type model --right-type clab --check rib
go run ./cmd/hoyan compare labs/base-wan/hoyan.clab.yml labs/base-wan/hoyan.clab.yml --left-type model --right-type clab --check fib
go run ./cmd/hoyan compare labs/base-wan/hoyan.clab.yml labs/base-wan/snapshots/actual.json --left-type model --right-type snapshot
```
