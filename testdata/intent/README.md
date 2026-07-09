# Intent DSL サンプル一覧

Hoyan DSL (`.hoyan`) の全機能をカバーするサンプルファイル。

YAML版は削除され、DSL (.hoyan) に一本化されました。
DSLファイルは `testdata/intentdsl/` ディレクトリにあります。

## 機能別対応表

| 機能カテゴリ | サンプルファイル | キーワード |
|---|---|---|
| **基本** | | |
| 最小構成 | `minimal.hoyan` | `rib`, `count()`, `>=` |
| forall quantifier | `forall.hoyan` | `forall`, `let` |
| 変数展開 | `forall.hoyan`, `rib-fib-basic.hoyan` | `$var` |
| scenario指定 | `minimal.hoyan` | `scenario` |
| **rib_eval: テーブルクエリ** | | |
| count | `rib-fib-basic.hoyan` | `count()` |
| distinct count | `rcl-rib-positive.hoyan` | `distCnt(field)` |
| distinct values | `rcl-rib-positive.hoyan` | `distVals(field)` |
| 比較演算子: == | `rib-eval-operators.hoyan` | `==` |
| 比較演算子: != | `rib-eval-operators.hoyan` | `!=` |
| 比較演算子: >/>=/</<= | `rib-eval-operators.hoyan` | `>`, `>=`, `<`, `<=` |
| **rib_eq: スナップショット比較** | | |
| 変更前後比較 (pass) | `rcl-rib-positive.hoyan` | `rib_eq`, `left`, `right` |
| 変更前後比較 (fail) | `rcl-rib-negative-compare.hoyan` | `rib_eq`, diff検出 |
| **when: 条件付き検証** | | |
| when 基本 | `guard-basic.hoyan` | `when`, `where` |
| when 空満パス | `guard-basic.hoyan` | 前提偽で vacuous pass |
| when 内部fail | `guard-basic.hoyan` | 前提真で内部intent fail |
| **packet: パケット到達性** | | |
| 到達可能/不可 | `packet-basic.hoyan` | `packet`, `expect` |
| 否定テスト | `packet-negative.hoyan` | expect逆でfail確認 |
| 障害シナリオ | `packet-failure-scenario.hoyan` | `failures`, `include_link_roles` |
| **where 述語** | | |
| where and/or/not | `selector-logic.hoyan` | `and`, `or`, `not` |
| where imply | `predicate-extra.hoyan` | `imply` |
| device_in | `selector-logic.hoyan` | `device_in` |
| contains | `predicate-extra.hoyan` | `contains` |
| matches | `predicate-extra.hoyan` | `matches` |
| **intent合成** | | |
| and | `rcl-composition.hoyan` | intentレベルの `and` |
| or | `rcl-composition.hoyan` | intentレベルの `or` |
| not | `rcl-composition.hoyan` | intentレベルの `not` |
| if/then | `rcl-composition.hoyan` | `if`, `then` |
| **バリデーションエラー** | | |
| version欠落 | `invalid-missing-version.hoyan` | |
| 未定義変数 | `invalid-undefined-var.hoyan` | |

## 見方

- pass/fail 両方のパターンが含まれています
- 実際の評価結果は `go test ./internal/usecase/intent/...` で確認できます
- 構文の詳細は [docs/dsl-design.md](../../docs/dsl-design.md) を参照してください
