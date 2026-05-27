# base-wan

Standard multi-region Hoyan WAN lab. It covers FRR edge/customer/transit
routers, one cEOS core, one SR Linux core, BGP propagation, ACL dataplane
checks, prefix classes, recursive next-hop modeling, and RIB/FIB comparison.

Examples:

```bash
go run ./cmd/hoyan intent verify --lab labs/base-wan --format json
go run ./cmd/hoyan live check --lab labs/base-wan
go run ./cmd/hoyan compare rib --lab labs/base-wan
go run ./cmd/hoyan compare fib --lab labs/base-wan
go run ./cmd/hoyan compare rib --lab labs/base-wan --snapshot labs/base-wan/snapshots/latest.json
go run ./cmd/hoyan compare fib --lab labs/base-wan --snapshot labs/base-wan/snapshots/latest.json
go run ./cmd/hoyan live check --lab labs/base-wan --snapshot labs/base-wan/snapshots/latest.json --offline
```
