# RCL Intent サンプル一覧

RCL DSL (`rcl` field) の全機能をカバーするサンプルファイル。

## 機能別対応表

| 機能カテゴリ | サンプルファイル | キーワード |
|---|---|---|
| **基本** | | |
| 最小構成 | `minimal.yml` | `rib_eval`, `count()`, `gte` |
| forall quantifier | `forall.yml` | `forall`, `vars` |
| 変数展開 | `forall.yml`, `rib-fib-basic.yml` | `${var}` |
| scenario指定 | `minimal.yml` | `scenario` |
| **rib_eval: テーブルクエリ** | | |
| count | `rib-fib-basic.yml` | `count()` |
| distinct count | `rcl-rib-positive.yml` | `distCnt(field)` |
| distinct values | `rcl-rib-positive.yml` | `distVals(field)` |
| 比較演算子: eq | `rib-eval-operators.yml` | `eq` |
| 比較演算子: ne | `rib-eval-operators.yml` | `ne` |
| 比較演算子: gt/gte/lt/lte | `rib-eval-operators.yml` | `gt`, `gte`, `lt`, `lte` |
| **rib_eq: スナップショット比較** | | |
| 変更前後比較 (pass) | `rcl-rib-positive.yml` | `rib_eq`, `left`, `right` |
| 変更前後比較 (fail) | `rcl-rib-negative-compare.yml` | `rib_eq`, diff検出 |
| **guard: 条件付き検証** | | |
| guard 基本 | `guard-basic.yml` | `guard`, `where`, `intent` |
| guard 空満パス | `guard-basic.yml` | 前提偽で vacuous pass |
| guard 内部fail | `guard-basic.yml` | 前提真で内部intent fail |
| **packet_reachable: パケット到達性** | | |
| 到達可能/不可 | `packet-basic.yml` | `packet_reachable`, `expect` |
| 否定テスト | `packet-negative.yml` | expect逆でfail確認 |
| 障害シナリオ | `packet-failure-scenario.yml` | `failures`, `include_link_roles` |
| **where 述語** | | |
| and/or/not | `selector-logic.yml` | `and`, `or`, `not` |
| imply | `predicate-extra.yml` | `imply` |
| device_in | `selector-logic.yml` | `device_in` |
| prefix_within | `selector-logic.yml` | `prefix_within` |
| communities contains | `predicate-extra.yml` | `contains` |
| as_path matches | `predicate-extra.yml` | `matches` |
| **intent合成** | | |
| and | `rcl-composition.yml` | intentレベルの `and` |
| or | `rcl-composition.yml` | intentレベルの `or` |
| not | `rcl-composition.yml` | intentレベルの `not` |
| **バリデーションエラー** | | |
| version欠落 | `invalid-missing-version.yml` | |
| 未定義変数 | `invalid-undefined-var.yml` | |

## 見方

- pass/fail 両方のパターンが含まれています
- 実際の評価結果は `go test ./internal/usecase/intent/...` で確認できます
