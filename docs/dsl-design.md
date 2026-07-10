# Hoyan Intent DSL Design

## 目標

- YAML の深いネスト（`rcl` → `guard` → `where` / `intent`）を平坦化
- 自然言語に近いキーワード（`when`、`forall`、`rib`、`packet`）
- 変数参照を `${var}` → `$var` に短縮
- 既存の `intent.Document` AST にコンパイルし、評価エンジンは変更ゼロ
- 手書き再帰下降パーサーで実装可能な文法

---

## 型とリテラル

| 型 | リテラル例 |
|---|---|
| string | `"hello"` |
| int | `42` |
| float | `3.14` |
| bool | `true`, `false` |
| array | `["a", "b", "c"]` |
| var ref | `$var_name` |

変数名: `[A-Za-z_][A-Za-z0-9_]*`

---

## ファイル構造

```
// コメント（行コメントのみ）

version = "hoyan/v1"

// 変数定義
let edges = ["bj-edge1", "sh-edge1", "gz-edge1"]
let changed_prefix = "10.4.0.0/16"
let service_ip = "10.4.1.10"

// スナップショット
snapshot "current" {
  lab = "labs/base-wan"
}

snapshot "pre" {
  lab = "labs/intent-change-before"
}

snapshot "post" {
  lab = "labs/intent-change-after"
}

// シナリオ
scenario "normal" {
  snapshot = "current"
}

scenario "one-core-link-failure" {
  snapshot = "current"
  failures {
    max = 1
    include_link_roles = ["core"]
  }
}

// intent
intent "service-prefix-visible-on-edges" {
  scenario = "normal"

  forall edge in $edges {
    rib where device = $edge, prefix = $changed_prefix {
      count() >= 1
    }
  }
}
```

---

## 文法（EBNF風）

```ebnf
Document    = VersionDecl VarDecl* SnapshotDecl* ScenarioDecl* IntentDecl*

VersionDecl = "version" "=" string

VarDecl     = "let" ident "=" Value

SnapshotDecl = "snapshot" string "{" SnapshotBody "}"
SnapshotBody = "lab" "=" string

ScenarioDecl = "scenario" string "{" ScenarioBody "}"
ScenarioBody = "snapshot" "=" string (","? FailureBlock)?
FailureBlock = "failures" "{" FailureFields "}"
FailureFields = (FailureField (","? FailureField)*)?
FailureField = "max" "=" int
             | "include_link_roles" "=" Array
             | "exclude_link_roles" "=" Array
             | "include_links" "=" Array
             | "exclude_links" "=" Array
             | "include_node_roles" "=" Array
             | "exclude_node_roles" "=" Array
             | "include_nodes" "=" Array
             | "exclude_nodes" "=" Array

IntentDecl  = "intent" string "{" IntentBody "}"
IntentBody  = ("scenario" "=" string)? TopLevelExpr

TopLevelExpr = Expr

Expr        = GuardExpr
            | ForallExpr
            | RibEvalExpr
            | PacketExpr
            | RibEqExpr
            | AndExpr
            | OrExpr
            | NotExpr
            | ImplyExpr
            | BlockExpr

(* ------- Guard (when) ------- *)
GuardExpr   = "when" "where" WherePredicates Block
(* Semantics: if no rows match → vacuous pass; otherwise eval block *)

(* ------- Forall (RCL-level) ------- *)
ForallExpr  = "forall" ident "in" Array ("," ident "in" Array)* Block
(* Semantics: iterate over values (cartesian product for multiple vars); all must pass *)

(* ------- RIB Eval ------- *)
RibEvalExpr = "rib" WhereClause? Block
(* Block must contain exactly one AggregateExpr *)
(* WhereClause on "rib" sets the row filter *)

AggregateExpr = "count()" ComparisonOp
              | "distCnt(" ident ")" ComparisonOp
              | "distVals(" ident ")" ComparisonOp

ComparisonOp = ">=" int
             | ">"  int
             | "<=" int
             | "<"  int
             | "==" Value    (* scalar for count(), array for distVals *)
             | "!=" Value

(* ------- Packet Reachability ------- *)
PacketExpr  = "packet" "from" Value "to" Value ProtocolPort
              ("vrf" Value)? "expect" bool

ProtocolPort = "icmp"
             | "tcp" "/" int
             | "udp" "/" int

(* ------- RIB Equality ------- *)
RibEqExpr   = "rib_eq" "left" "=" string "right" "=" string WhereClause?

(* ------- Logical Combinators ------- *)
AndExpr     = "and" "{" Expr+ "}"
OrExpr      = "or"  "{" Expr+ "}"
NotExpr     = "not" Expr
ImplyExpr   = "if" Expr "then" Expr

(* ------- Block (bare rib_eval without 'rib' keyword) ------- *)
BlockExpr   = Block   (* auto-detect: if contains AggregateExpr → rib_eval *)

(* ------- Where Clause ------- *)
WhereClause = "where" WherePredicates

WherePredicates = WherePredicate ("," WherePredicate)*

WherePredicate = ident "=" Value          (* exact match *)
               | ident "!=" Value         (* not equal *)
               | ident "contains" Value   (* string/array contains *)
               | ident "matches" string   (* regex match *)
               | ident "within" Value     (* prefix within CIDR *)
               | "not" "{" WherePredicate+ "}"  (* negation; block required *)
               | "and" "{" WherePredicate+ "}"  (* compound AND *)
               | "or"  "{" WherePredicate+ "}"  (* compound OR *)
               | "if" "{" WherePredicate "then" WherePredicate "}"

(* ------- Common ------- *)
Block       = "{" Expr* "}"
Array       = "[" Value ("," Value)* "]"
Value       = string | int | float | bool | Array | VarRef
VarRef      = "$" ident
ident       = [A-Za-z_][A-Za-z0-9_]*
```

---

## Where述語の予約フィールド一覧

where句で使用できるフィールド名のうち、一部は評価エンジンが特別な意味を持つものとして処理する。
パーサーはこれらを単なる識別子として扱う（特別扱いしない）。

| フィールド | 型 | 意味 |
|---|---|---|
| `device`, `node` | string | デバイス名でフィルタ |
| `device_in` | `[]string` | 複数デバイスをOR条件でフィルタ（糖衣構文） |
| `prefix` | string (CIDR) | プレフィックスでフィルタ（指定範囲内のサブネットも含む） |
| `prefix_within` | string (CIDR) | 指定したCIDR範囲内のプレフィックスのみにマッチ（`within` 演算子で生成） |
| `protocol` | string | 経路プロトコル（ospf, bgp, static, connected 等） |
| `vrf` | string | VRF名 |
| `selected` | bool | best経路のみにフィルタ（`selected = true`） |
| `nexthop` | string | ネクストホップ（`contains` / `matches` 演算子と併用） |
| `communities` | `[]string` | BGPコミュニティ（`contains` 演算子と併用） |
| `large_communities` | `[]string` | BGP Large Community（`contains` 演算子と併用） |
| `as_path` | string | AS_PATH（`matches` 演算子と併用、または直接比較） |
| `as_path_len` / `aspath_len` | int | AS_PATHの長さ |
| `weight` | int | BGP weight |
| `origin` | string | BGP origin（igp, egp, incomplete） |
| `med` | int | BGP MED（マルチエグジット識別子） |
| `local_pref` / `localPref` | int | BGP local preference |
| `peer` | string | BGPピアアドレス |
| `peer_as` | int | BGPピアAS番号 |
| `route_type` | string | OSPF経路タイプ（intra_area, inter_area 等） |
| `area` | string | OSPFエリア |
| `cost` | int | OSPFコスト |
| `connected_class` | string | 直結経路の種別（`"loopback"` / `"link"`） |

---

## キーワード対応表（YAML → DSL）

### 構造

| YAML | DSL |
|---|---|
| `rcl:` | **削除**（自動推論） |
| `guard: { where: ..., intent: ... }` | `when ... { ... }` |
| `forall: { var: ..., in: [...], intent: ... }` | `forall var in [...] { ... }` |
| `and: [...]` | `and { ... }` |
| `or: [...]` | `or { ... }` |
| `not: ...` | `not ...` |
| `imply: [...]` | `if ... then ...` |

### 述語（where）

| YAML | DSL |
|---|---|
| `prefix: 10.0.0.0/8` | `prefix = "10.0.0.0/8"` |
| `device: r1` | `device = "r1"` |
| `protocol: ospf` | `protocol = "ospf"` |
| `device_in: [r1, r2]` | `device_in = ["r1", "r2"]` |
| `communities: { contains: "65001:100" }` | `communities contains "65001:100"` |
| `as_path: { matches: "65001" }` | `as_path matches "65001"` |
| `not: { prefix: ... }` | `not { prefix = "..." }` |
| `and: [{...}, {...}]` | `and { ... ... }` |
| `or: [{...}, {...}]` | `or { ... ... }` |
| `imply: [{...}, {...}]` | `if { ... then ... }` |
| `prefix_within: 10.0.0.0/8` | `prefix within "10.0.0.0/8"` |
| `nexthop: { contains: "10.0.0.1" }` | `nexthop contains "10.0.0.1"` |

### RIB評価

| YAML | DSL |
|---|---|
| `rib_eval: { aggregate: count(), gte: 4 }` | `count() >= 4` |
| `rib_eval: { aggregate: distCnt(nexthop), gte: 2 }` | `distCnt(nexthop) >= 2` |
| `rib_eval: { aggregate: distVals(route_type), eq: [[intra_area]] }` | `distVals(route_type) == ["intra_area"]` |

### パケット

| YAML | DSL |
|---|---|
| `packet_reachable: { from: ..., to: ..., protocol: tcp, dst_port: 80, expect: true }` | `packet from ... to ... tcp/80 expect true` |

### RIB比較

| YAML | DSL |
|---|---|
| `rib_eq: { left: pre, right: post, where: ... }` | `rib_eq left = "pre" right = "post" where ...` |

---

## 実例: YAML → DSL 変換

### 例1: シンプルなRIB評価 + guard

**YAML:**
```yaml
- name: ospf-routes-on-all-routers
  scenario: normal
  rcl:
    guard:
      where:
        protocol: ospf
        prefix: 10.255.1.1/32
      intent:
        rib_eval:
          aggregate: count()
          gte: 4
```

**DSL:**
```
intent "ospf-routes-on-all-routers" {
  scenario = "normal"

  when where protocol = "ospf", prefix = "10.255.1.1/32" {
    count() >= 4
  }
}
```

### 例2: forall + 複合条件

**YAML:**
```yaml
- name: core-routers-have-ospf-to-loopbacks
  scenario: normal
  rcl:
    forall:
      var: device
      in: [core1, core2]
      intent:
        and:
          - guard:
              where:
                protocol: ospf
                prefix: 10.255.1.1/32
              intent:
                rib_eval:
                  aggregate: count()
                  gte: 1
          - guard:
              where:
                protocol: ospf
                prefix: 10.255.4.4/32
              intent:
                rib_eval:
                  aggregate: count()
                  gte: 1
```

**DSL:**
```
intent "core-routers-have-ospf-to-loopbacks" {
  scenario = "normal"

  forall device in ["core1", "core2"] {
    and {
      when where protocol = "ospf", prefix = "10.255.1.1/32" {
        count() >= 1
      }
      when where protocol = "ospf", prefix = "10.255.4.4/32" {
        count() >= 1
      }
    }
  }
}
```

### 例3: パケット到達性 + forall

**YAML:**
```yaml
- name: customers-http-denied
  scenario: normal
  forall:
    src: ${customers}
  rcl:
    packet_reachable:
      from: ${src}
      to: ${service_ip}
      protocol: tcp
      dst_port: 80
      expect: false
```

**DSL:**
```
intent "customers-http-denied" {
  scenario = "normal"
  forall src in $customers {
    packet from $src to $service_ip tcp/80 expect false
  }
}
```

### 例4: 障害シナリオ

**YAML:**
```yaml
- name: service-prefix-survives-single-core-failure
  scenario: one-core-link-failure
  rcl:
    rib_eval:
      where:
        prefix: 10.4.0.0/16
      aggregate: count()
      gte: 3
```

**DSL:**
```
scenario "one-core-link-failure" {
  snapshot = "current"
  failures {
    max = 1
    include_link_roles = ["core"]
  }
}

intent "service-prefix-survives-single-core-failure" {
  scenario = "one-core-link-failure"

  rib where prefix = "10.4.0.0/16" {
    count() >= 3
  }
}
```

### 例5: RIB比較

**YAML:**
```yaml
- name: unrelated-routes-unchanged
  rcl:
    rib_eq:
      left: pre
      right: post
      where:
        not:
          prefix: ${changed_prefix}
```

**DSL:**
```
intent "unrelated-routes-unchanged" {
  rib_eq left = "pre" right = "post" where not { prefix = $changed_prefix }
}
```

### 例6: 複雑なwhere（AND/OR）

**YAML:**
```yaml
- name: logical-or-selects-static-and-connected
  scenario: post
  rcl:
    rib_eval:
      where:
        and:
          - device: bj-edge1
          - or:
              - protocol: static
              - protocol: connected
      aggregate: count()
      gte: 1
```

**DSL:**
```
intent "logical-or-selects-static-and-connected" {
  scenario = "post"

  rib where and {
    device = "bj-edge1"
    or {
      protocol = "static"
      protocol = "connected"
    }
  } {
    count() >= 1
  }
}
```

### 例7: not + imply

**YAML:**
```yaml
rcl:
  not:
    imply:
      - rib_eval:
          where:
            device: r1
          aggregate: count()
          gte: 999
      - rib_eval:
          where:
            device: r2
          aggregate: count()
          gte: 1
```

**DSL:**
```
not {
  if {
    rib where device = "r1" { count() >= 999 }
  } then {
    rib where device = "r2" { count() >= 1 }
  }
}
```

---

## 実装方針

### ファイル構成

```
internal/adapter/intentdsl/
  parse.go          # Load(path) → *intent.Document (公開API)
  lexer.go          # トークナイザ (text/scanner ベース)
  parser.go         # 再帰下降パーサー
  parse_test.go     # YAMLテストデータと同等のDSLテスト
```

### レキサー

Goの `text/scanner` をそのまま使う。トークン種別:

- `IDENT` — 識別子、キーワード
- `STRING` — ダブルクォート文字列
- `INT` / `FLOAT` — 数値
- `$` + IDENT — 変数参照（専用トークン `VARREF`）
- 演算子: `=`, `==`, `!=`, `>=`, `<=`, `>`, `<`
- 区切り: `{`, `}`, `[`, `]`, `(`, `)`, `,`, `/`
- `//` コメント

### パーサー

再帰下降。各 `parse*` 関数がASTノードを返す:

```
parseDocument()    → *intent.Document
parseVar()         → (string, any)
parseSnapshot()    → (string, intent.Snapshot)
parseScenario()    → (string, intent.Scenario)
parseIntent()      → intent.Intent
parseExpr()        → *intent.RCLExpr
parseGuard()       → *intent.RCLExpr
parseForall()      → *intent.RCLExpr
parseRibEval()     → *intent.RCLExpr
parsePacket()      → *intent.RCLExpr
parseRibEq()       → *intent.RCLExpr
parseAnd()         → *intent.RCLExpr
parseOr()          → *intent.RCLExpr
parseNot()         → *intent.RCLExpr
parseImply()       → *intent.RCLExpr
parseWhere()       → map[string]any
parseValue()       → any
```

### エラーハンドリング

行番号・列番号を付与したエラーメッセージ:

```
Error: intent "foo":3:12: unexpected token '}', expected comparison operator
Error: intent "bar":5:8: undefined variable $unknown_var
```

### CLI統合

`internal/adapter/cli/intent.go` の `loadIntentFile()` は DSL パーサーのみを呼び出す。YAML サポートは削除し、`internal/adapter/intentfile/` パッケージも削除する。

```go
func loadIntentFile(path string) (*domainintent.Document, error) {
    if path == "" {
        return nil, fmt.Errorf("--file is required")
    }
    return intentdsl.Load(path)
}
```

lab の intent ファイルパスも `intent/hoyan.hoyan` に統一する。

### テスト戦略

既存の `testdata/intent/*.yml` は `testdata/intentdsl/*.hoyan` に移行し、旧 `.yml` ファイルは削除する。既存の評価テストは `.hoyan` ファイルを直接ロードして同じ評価結果になることを確認する。

---

## 未解決のDesign Decision

1. **`forall` の位置による意味の違い**: ドキュメントレベル `forall` と RCLレベル `forall` は実装上は別パスだが、文法上は同じキーワードにする。意図的に。

2. **ブロック内の暗黙的rib_eval検出**: `when where ... { count() >= 4 }` のブロックは暗黙的に rib_eval。明示的に `rib` キーワードも書けるが省略可能。

3. **`=` と `==` の使い分け**: 代入（`lab = "..."`）には `=`、等値比較（where内）には `=` を使う（`==` も許容）。`!=` は否定等値。

4. **trailing comma**: 許容する。`where device = "r1", protocol = "ospf",` もOK。

5. **ファイル拡張子**: `.hoyan` に統一する。既存 `.yml` intent 定義との共存はしない。
