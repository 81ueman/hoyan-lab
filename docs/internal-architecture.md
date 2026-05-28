# Internal Architecture

Hoyan keeps package dependencies flowing from adapters toward pure domain code:

```txt
internal/adapter -> internal/usecase -> internal/engine -> internal/domain
```

Packages should not import in the opposite direction. Domain packages model routing, topology, failure, symbolic expressions, and other pure rules. Engine packages orchestrate simulation and convergence over domain concepts. Usecase packages coordinate application workflows such as verification, live checks, snapshots, and topology building. Adapter packages own external dependencies and boundary formats such as Cobra, YAML/containerlab files, vendor config parsing, Z3/cgo, containerlab execution, and live device IO.

## Package Roles

- `internal/domain/model`: topology, packet, prefix, protocol, and route model types.
- `internal/domain/failure`: failure-domain conditions and failure-set helpers.
- `internal/domain/symbolic`: pure symbolic expression types.
- `internal/domain/solver`: solver-facing problem and answer types.
- `internal/domain/query`: offline verification query schema.
- `internal/domain/intent`: intent document and report schema.
- `internal/domain/facts`: modeled fact row and canonical comparison types.
- `internal/domain/routing/route`: shared simulated route/RIB entry attributes used by protocol rules and engines.
- `internal/domain/routing/bgp`: BGP route decision, import/export behavior, vendor best-path differences, and BGP path attribute helpers.
- `internal/domain/routing/ospf`: OSPF route type constants, interface/advertisement/path/SPF types, route ranking, and path helpers.
- `internal/domain/routing/policy`: route-policy match/set logic and prefix-list, AS-path-list, and community-list evaluation.
- `internal/engine`: control-plane and data-plane simulation orchestration. Engines decide when to apply domain protocol rules, mutate simulated RIB/FIB state, and run convergence loops; protocol law should stay in `internal/domain/routing`.
- `internal/usecase`: application workflows that assemble engines, adapters, and reports, including verify, topology build, intent evaluation, live checks, snapshots, and RIB/FIB comparison.
- `internal/adapter`: CLI, file formats, config parsers, concrete solver backends, SR Linux JSON command execution, and other boundary IO.

## Routing Logic

BGP and OSPF protocol decisions belong in `internal/domain/routing/*` when they can be expressed without simulation state. Examples include BGP best-path ordering, route-policy rule matching, AS-path/community/prefix-list evaluation, OSPF route-type ranking, and SPF path utility logic.

Control-plane convergence remains in `internal/engine/controlplane`: iterative message propagation, RIB orchestration, route advertisement scheduling, and conversion between protocol decisions and simulated RIB entries.

## Migration Order

1. Move pure model, failure, and symbolic packages into `internal/domain`.
2. Split solver interfaces into `internal/domain/solver` and concrete backends into `internal/adapter/solver`.
3. Move containerlab parsing into `internal/adapter/labfile` and topology assembly into `internal/usecase/topology`.
4. Move Cobra commands under `internal/adapter/cli` and keep application orchestration in `internal/usecase`.
5. Extract pure BGP and OSPF decision logic into `internal/domain/routing`.
6. Move query, intent, and facts schemas into `internal/domain`, with YAML loading in `internal/adapter/*file` and evaluation/build workflows in `internal/usecase`.
7. Move RIB/FIB comparison workflows into `internal/usecase/ribcompare` and `internal/usecase/fibcompare`; keep SR Linux command execution under `internal/adapter/srlinuxjson`.
