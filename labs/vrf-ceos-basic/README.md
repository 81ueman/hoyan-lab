# cEOS VRF Basic

cEOS lab for static and connected routing across two isolated VRFs.

- `tenant-a` and `tenant-b` use overlapping link and service prefixes.
- `r1` has per-VRF static routes to the matching remote service loopback.
- The modeled RIB/FIB and live route-table/FIB collectors should report the same per-VRF routes.
