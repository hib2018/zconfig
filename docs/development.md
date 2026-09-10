# 開発ガイド

## 必要な環境

- Zig 0.16.0
- Go 1.27.1 または互換性のある 1.27 パッチ版
- macOS、Linux、または Windows Terminal 相当の端末

依存バージョンはリポジトリ内で固定しています。Zig は 1.0 前で互換性変化が大きいため、コンパイラ更新は明示的な互換性変更として扱います。

## ビルドと検証

```sh
export PATH=/usr/local/go/bin:$PATH

zig build
mkdir -p zig-out/bin
go build -o ./zig-out/bin/zconfig ./tui/cmd/zconfig

zig build test
zig build -Doptimize=ReleaseSafe
go test ./tui/... ./tests/...
go test -race ./tui/... ./tests/...
go vet ./tui/... ./tests/...
```

生成物は `zig-out/bin/zconfig-core` と `zig-out/bin/zconfig` です。Go は `tui` と `tests` の複数モジュールを `go.work` で束ねるため、ルートで `go test ./...` は使いません。

## コードの役割

| 場所 | 内容 |
|---|---|
| `core/src/` | Zig コア。安全規則、JSON、ポインター、提案、スキーマ、伏せ字 |
| `core/tests/` | Zig の単体・契約テスト |
| `tui/cmd/zconfig/` | CLI と各プロセスの結線 |
| `tui/internal/review/` | UI に依存しないレビュー状態と状態遷移 |
| `tui/internal/ui/` | Bubble Tea の画面と操作 |
| `tui/internal/protocol/` | Go 側プロトコル型と契約検証 |
| `tui/internal/runner/` | コア、エージェント、バリデーターの安全な起動 |
| `tests/contract/fixtures/` | Zig と Go が共有するゴールデン JSON |
| `tests/integration/` | 実行ファイル間の統合シナリオ |

## 実装順序

[tasks.md](../specs/001-review-config-changes/tasks.md) の依存順に進めます。

1. 基盤とプロトコル — 完了
2. 変更案を理解する読み取り専用レビュー — 完了
3. コメント対象だけを再提案させる限定修正 — 基盤完了
4. 採否、保存・再開、安全な反映 — 未実装
5. 機密値の表示・一回限り共有 — 未実装
6. 性能、実機、ユーザビリティ検証 — 未実施

安全性に関係する変更では、実装だけでなく `contracts/`、Zig と Go の共通フィクスチャ、双方の受理・拒否テストを更新します。

## 仕様と文書

- `spec.md`: 利用者の要求と受け入れ条件
- `plan.md`: 技術選定と構造
- `contracts/`: プロセス間の機械可読契約
- `tasks.md`: 実装状況と依存関係
- `docs/`: 利用者・開発者向けの説明

仕様と実装の間に差がある場合、完成したように文書化せず「設計済み・未実装」または「未決定」と明示します。
