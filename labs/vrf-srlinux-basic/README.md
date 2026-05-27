# SR Linux VRF Basic

SR Linux lab for static and connected routing across two isolated network instances.

- `tenant-a` and `tenant-b` use overlapping link and service prefixes.
- `r1` has per-network-instance static routes to the matching remote service loopback.
- The modeled RIB/FIB and live route-table/FIB collectors should report the same per-network-instance routes.
