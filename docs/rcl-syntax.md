# RCL (Route Change Intent Language) Syntax Reference

> Version: hoyan/v1

## Document Structure

```yaml
version: hoyan/v1          # 必須
vars: { ... }              # 変数定義（オプション）
networks: { ... }          # ネットワーク状態定義
scenarios: { ... }         # 検証シナリオ定義
intents: [ ... ]           # 検証ルール（1つ以上）
```

### networks

ネットワークのスナップショット（ある時点の状態）を定義します。

```yaml
networks:
  current:
    lab: labs/base-wan       # ラボディレクトリパス
  pre:
    lab: labs/change-before  # 変更前
  post:
    lab: labs/change-after   # 変更後
```

### scenarios

検証シナリオを定義します。使用するネットワーク状態と障害条件を指定します。

```yaml
scenarios:
  normal:                    # シナリオ名
    network: current         # 使用するネットワーク
    # 障害なし（failures省略可）

  one-link-failure:
    network: current
    failures:
      max: 1                 # 同時障害数の上限
      include_link_roles: [core]     # 対象リンクロール（フィルタ）
      exclude_link_roles: [leaf]     # 除外リンクロール
      include_links: [...]           # 対象リンクID（個別指定）
      exclude_links: [...]           # 除外リンクID
      include_node_roles: [core]     # 対象ノードロール
      exclude_node_roles: [edge]     # 除外ノードロール
      include_nodes: [...]           # 対象ノード名
      exclude_nodes: [...]           # 除外ノード名
```

## Intent 式 (RCLExpr)

1つのintentは以下の式タイプのいずれか1つを持ちます。

### rib_eval — RIBクエリと集約比較

単一ネットワークのRIBをクエリし、集約値と期待値を比較します。

```yaml
rcl:
  rib_eval:
    network: current          # 使用するネットワーク（省略時はscenarioのnetwork）
    where: { ... }            # ルートフィルタ（オプション）
    aggregate: count()        # 集約関数（必須）
    gte: 1                    # 比較演算子 + 期待値
```

**集約関数:**

| 関数 | 説明 | 戻り値の型 | 例 |
|------|------|-----------|-----|
| `count()` | マッチした経路数 | int | `count()` |
| `distCnt(field)` | フィールドの異なる値の数 | int | `distCnt(nexthop)` |
| `distVals(field)` | フィールドの異なる値のリスト | []any | `distVals(local_pref)` |
| `max(field)` | フィールドの最大値 | int | `max(local_pref)` |
| `min(field)` | フィールドの最小値 | int | `min(cost)` |
| `avg(field)` | フィールドの平均値 | float | `avg(cost)` |
| `sum(field)` | フィールドの合計値 | int | `sum(weight)` |

**比較演算子:**

| 演算子 | 例 | 意味 |
|--------|-----|------|
| `eq` | `eq: [10]` | 一致 |
| `ne` | `ne: [0]` | 不一致 |
| `gt` | `gt: 5` | より大きい |
| `gte` | `gte: 1` | 以上 |
| `lt` | `lt: 10` | 未満 |
| `lte` | `lte: 100` | 以下 |

### guard — 条件付き検証 (p ⇒ g)

前提条件を満たす経路についてのみ、内部intentを検証します。前提を満たす経路がない場合は自動でパス（vacuous pass）になります。

```yaml
rcl:
  guard:
    where:                    # 前提条件（ルートフィルタ）
      device: bj-edge1
      prefix: 10.4.0.0/16
    intent:                   # 結論（ネストしたRCLExpr）
      rib_eval:
        aggregate: count()
        gte: 1
```

### packet_reachable — パケット到達性検証

パケット forwarding の到達性を検証します。障害シナリオと組み合わせて耐障害性の確認も可能です。

```yaml
rcl:
  packet_reachable:
    from: cust-bj             # 送信元ノード
    to: 10.4.1.10             # 宛先IPアドレス
    protocol: tcp             # プロトコル（tcp/udp/icmp）
    dst_port: 443             # 宛先ポート（tcp/udpの場合）
    vrf: default              # VRF（オプション）
    expect: true              # 期待する到達性（true=到達可能/false=到達不可）
```

### rib_eq — 2ネットワーク間のRIB比較

変更前後のRIBを比較し、差分がないことを検証します。

```yaml
rcl:
  rib_eq:
    left: pre                 # 比較元ネットワーク
    right: post               # 比較先ネットワーク
    where:                    # 比較対象フィルタ（オプション）
      not:
        prefix: ${changed_prefix}
```

### forall — 全称量化

複数の値に対して同じintentを繰り返し評価します。

```yaml
rcl:
  forall:
    var: edge                 # 変数名
    in: [bj-edge1, sh-edge1]  # 値のリスト（省略時は全値自動列挙）
    intent:                   # 各値に対して評価するintent
      rib_eval:
        where:
          device: ${edge}     # 変数参照
          prefix: 10.4.0.0/16
        aggregate: count()
        gte: 1
```

`in` 省略時の自動列挙対応フィールド: `device`, `node`, `vrf`, `protocol`, `route_type`, `area`, `origin`, `connected_class`

### 論理合成

複数のintentを論理演算で合成します。

```yaml
# AND: 全ての子intentがパスすればパス
rcl:
  and:
    - rib_eval: { ... }
    - guard: { ... }

# OR: いずれかの子intentがパスすればパス
rcl:
  or:
    - rib_eval: { ... }
    - rib_eval: { ... }

# NOT: 子intentがパスならfail、failならパス
rcl:
  not:
    rib_eval:
      aggregate: count()
      gte: 1

# IMPLY: 前提が真なら結論を検証、前提が偽なら自動パス
rcl:
  imply:
    - rib_eval: { aggregate: count(), gte: 1 }
    - rib_eval: { aggregate: count(), gte: 5 }
```

## Where 述語

`rib_eval`, `guard`, `rib_eq` の `where` フィールドで使用可能なルートフィルタです。複数指定した場合は AND 結合されます。

### 基本述語

| 述語 | 型 | 例 | 説明 |
|------|-----|-----|------|
| `device` | string | `device: bj-edge1` | ルータ名の完全一致 |
| `device_in` | [string] | `device_in: [bj-edge1, sh-edge1]` | 複数ルータのいずれかに一致 |
| `vrf` | string | `vrf: default` | VRF名の完全一致 |
| `prefix` | string (CIDR) | `prefix: 10.4.0.0/16` | プレフィックス一致（サブネット包含も可） |
| `prefix_within` | string (CIDR) | `prefix_within: 10.0.0.0/8` | 指定された範囲内のプレフィックス |
| `protocol` | string | `protocol: bgp` | ルーティングプロトコル |
| `selected` | bool | `selected: true` | ベスト経路のみ |

### BGP述語

| 述語 | 型 | 例 | 説明 |
|------|-----|-----|------|
| `local_pref` | int | `local_pref: 100` | BGP local preference |
| `as_path` | string | `as_path: "65100 65004"` | ASパス文字列（完全一致） |
| `as_path` + `matches` | regex | `as_path: { matches: ".*65004.*" }` | ASパス正規表現マッチ |
| `as_path_len` | int | `as_path_len: 3` | ASパス長 |
| `communities` + `contains` | string | `communities: { contains: "65000:100" }` | BGPコミュニティ包含 |
| `large_communities` + `contains` | string | `large_communities: { contains: "65000:100:1" }` | Large Community包含 |
| `weight` | int | `weight: 100` | BGP weight |
| `origin` | string | `origin: igp` | BGP origin code (igp/egp/incomplete) |
| `med` | int | `med: 50` | BGP multi-exit discriminator |
| `peer` | string | `peer: 10.0.0.1` | BGPピアアドレス |
| `peer_as` | int | `peer_as: 65001` | BGPピアAS番号 |

### OSPF述語

| 述語 | 型 | 例 | 説明 |
|------|-----|-----|------|
| `route_type` | string | `route_type: intra_area` | OSPF経路タイプ（intra_area/inter_area/external_type_1/external_type_2） |
| `area` | string | `area: "0.0.0.0"` | OSPFエリアID |
| `cost` | int | `cost: 10` | OSPFコスト |

### Connected経路述語

| 述語 | 型 | 例 | 説明 |
|------|-----|-----|------|
| `connected_class` | string | `connected_class: loopback` | Connected経路クラス（loopback/link/service/host） |
| `nexthop` | string | `nexthop: 10.0.0.1` | ネクストホップアドレス（部分一致） |

### 論理述語

where 内でも論理演算が可能です。

```yaml
where:
  and:
    - device_in: [bj-edge1, sh-edge1]
    - protocol: bgp

where:
  or:
    - protocol: static
    - protocol: connected

where:
  not:
    prefix: 10.0.0.0/8

where:
  imply:
    - device_in: [bj-edge1, sh-edge1]   # 前提
    - prefix: 10.4.0.0/16               # 結論
```

## 変数

`vars` で定義した変数を `${var_name}` で参照できます。

```yaml
vars:
  service_prefix: 10.4.0.0/16
  edges: [bj-edge1, sh-edge1, gz-edge1]

intents:
  - name: example
    rcl:
      rib_eval:
        where:
          device: ${edges}       # 変数参照
        aggregate: count()
        gte: 1
```

`forall` と組み合わせて展開も可能です:

```yaml
intents:
  - name: check-edge
    forall:
      edge: ${edges}             # edgesの各値で展開
    rcl:
      rib_eval:
        where:
          device: ${edge}
        aggregate: count()
        gte: 1
```

## 完全な例

```yaml
version: hoyan/v1

vars:
  service_prefix: 10.4.0.0/16
  edges: [bj-edge1, sh-edge1, gz-edge1]

networks:
  current:
    lab: labs/base-wan

scenarios:
  normal:
    network: current
  one-core-failure:
    network: current
    failures:
      max: 1
      include_link_roles: [core]

intents:
  # 全エッジルータにサービスプレフィックスが存在すること
  - name: service-prefix-visible
    scenario: normal
    rcl:
      forall:
        var: edge
        in: ${edges}
        intent:
          rib_eval:
            where:
              device: ${edge}
              prefix: ${service_prefix}
            aggregate: count()
            gte: 1

  # HTTPSは1本のコアリンク障害に耐えられること
  - name: https-survives-failure
    scenario: one-core-failure
    rcl:
      packet_reachable:
        from: cust-bj
        to: 10.4.1.10
        protocol: tcp
        dst_port: 443
        expect: true

  # 変更前後で関係ない経路は変わっていないこと
  - name: unrelated-routes-unchanged
    rcl:
      rib_eq:
        left: pre
        right: post
        where:
          not:
            prefix: ${service_prefix}
```
