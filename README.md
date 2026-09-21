# zconfig

zconfig は、開発終盤の細かな設定変更を、人間とエージェントが安全に確認するためのターミナルツールです。

エージェントが作った変更案を「変更項目」の一覧として表示し、人間は各項目の現在値・提案値・理由・検査結果を確認します。必要な項目には自然言語でコメントし、その項目だけをエージェントへ再提案させます。最終的な採否とファイルへの反映は、常に人間が明示的に決定します。

Z Ecosystem 全体の共通方針・横断Skill・Artifact Flow は [`hib2018/zecosystem`](https://github.com/hib2018/zecosystem) が管理します。このリポジトリは zconfig 固有の configuration review Domain、proposal/revision/apply Protocol、Schema、security rule を所有します。

## 前提条件

- zconfig は Human-controlled Artifact Pipeline の終盤に置く設定変更レビュー境界です。曖昧な要求から変更案を作る汎用Plannerではありません。
- 変更案の生成は外部Agentが担い、zconfig は構造、元ファイルとの対応、変更範囲、機密値、最終確認を検証します。
- Core は反映前に source digest、proposal、判断、コメント、外部検査結果、confirmation capability を再検証します。
- 人間による最終確認を省略した自動適用は対象外です。
- zintent や ztasks との横断的な位置づけは zecosystem の Artifact Flow に従いますが、zconfig 固有ProtocolとSchemaはこのリポジトリが正本です。

## Z Ecosystem における境界

| 領域 | 所有者 |
|---|---|
| proposal / revision / final-change / apply Schema | このリポジトリ |
| zconfig process protocol、external validator contract、security/redaction rule | このリポジトリ |
| zconfig をArtifact Flow終盤へ接続する横断方針 | `zecosystem` |
| Pi/Codex等のharness設定、agent登録のmachine-local値 | dotfiles または利用者環境 |

> [!WARNING]
> 現在は開発中です。コアの最終集合生成と安全な反映まで実装されていますが、公開 CLI から最終承認までを通す操作フローと実利用者による検証は未完了です。重要な設定ファイルへ直接適用する用途にはまだ使用しないでください。

## 目指すもの

プロジェクトが完成に近づくと、色、余白、タイムアウト、機能フラグなど、数は多いものの一つずつは小さな設定調整が増えます。この段階で長い設定ファイルを会話へ貼り付け、エージェントの返答と差分を何度も往復する負荷を減らすのが zconfig の目的です。

```text
設定ファイル + エージェントの変更案
                 │
                 ▼
       変更項目を表形式で確認
          │             │
          │問題なし     │要修正
          ▼             ▼
       採否を記録   自然言語コメント
                         │
                         ▼
                  対象項目だけ再提案
                         │
                         └── 再確認

              最終差分を人間が確認
                         │
                         ▼
                    明示的に反映
```

zconfig 自身は、曖昧な依頼から変更案を生成する汎用チャットではありません。変更案の生成は登録済みの外部エージェントが担い、zconfig はその後の検証・レビュー・承認境界を担当します。

## 現在の実装状況

| 状態 | 範囲 |
|---|---|
| 実装済み | 厳格な JSON 読み込み、重複キー拒否、SHA-256 による元ファイル照合、JSON Pointer、提案と修正範囲の検証 |
| 実装済み | Zig コアと Go TUI 間の JSON プロトコル、プロセス制限、エージェント登録とハンドシェイク |
| 実装済み | 変更項目の一覧・詳細・差分・絞り込み、狭い端末とモノクロ表示、コメント／限定修正の状態管理 |
| 実装済み | 項目ごとの承認・却下、セッション保存と再開、最終集合の再計算、短命な確認 capability、安全な原子的反映と復旧 |
| 実装済み | 値を含まない監査ログ、機密値の一時表示・一回限り共有・エージェント出力の再マスク |
| 実装済み | 外部バリデーターの厳格な1往復ランナー、`apply` コマンドの最終差分・明示確認・反映フロー |
| 未実装 | 公開 `review` TUI 内からコメント・承認・反映までを完結させる対話フロー |
| 将来 | JSONC、TOML、YAML、zintent の終盤工程への汎用統合 |

詳細な進捗は [tasks.md](specs/001-review-config-changes/tasks.md) を参照してください。

## アーキテクチャ

- **Zig コア**: ファイル解析、提案検証、変更範囲の強制、最終集合の再計算、安全な反映を担当します。ネットワークや LLM を必要とせず、単独プロセスとして再利用できます。
- **Go TUI**: Bubble Tea を使い、レビュー画面、状態遷移、外部プロセスの調停を担当します。
- **外部エージェント**: 構造化された変更案と、コメント対象に限定した再提案を返します。シェルを介さず登録済みコマンドとして起動されます。

各プロセス呼び出しは、標準入力の JSON リクエスト 1 件と標準出力の JSON レスポンス 1 件で完結します。詳しくは [アーキテクチャ](docs/architecture.md) と [エージェント連携](docs/agent-integration.md) を参照してください。

## 対象範囲

初期バージョンの対象は、ローカルにある単一の標準 JSON ファイルです。

- 最大 10 MiB、100,000 ノード、1,000 変更項目
- 最大 10 回の修正、5,000 監査イベント
- JSON Schema は任意。対応サブセット外の適用可能なキーワードは、成功扱いにせず `unverified` と表示
- 通常ファイルのみを対象とし、古い元ファイル、重複キー、重複・親子競合する変更先は拒否
- 人間による最終確認を省略する自動適用は対象外

## 開発環境とビルド

必要なバージョンは Zig 0.16.0、Go 1.27.1 です。Go 側は Bubble Tea v2、Bubbles v2、Lip Gloss v2 を利用します。

```sh
export PATH=/usr/local/go/bin:$PATH

zig build test
zig build -Doptimize=ReleaseSafe

go test ./tui/... ./tests/...
go test -race ./tui/... ./tests/...
go vet ./tui/... ./tests/...
```

Go ワークスペースは複数モジュール構成のため、リポジトリ直下での `go test ./...` ではなく、上記のモジュールパスを指定します。より詳しい手順は [開発ガイド](docs/development.md) にあります。

試用時は `zig-out/bin/zconfig-core` と `zig-out/bin/zconfig` を同じディレクトリへ置き、そのディレクトリを `PATH` に追加するか、`--core` でコアの絶対パスを指定してください。現段階ではシステム領域へ自動インストールするコマンドは提供していません。

対応する JSON Schema キーワードは `type`、`const`、`enum`、数値の上下限、文字列長と単純な `pattern`、配列長と `items`、オブジェクトの `required` と `properties`、および `x-zconfig-sensitive` です。適用可能だが未対応のキーワードは検証済みにせず `unverified` として扱います。

## 現在試せる操作

```sh
# Zig コア
zig build

# Go TUI
mkdir -p zig-out/bin
go build -o ./zig-out/bin/zconfig ./tui/cmd/zconfig

# コアとの互換性を確認
./zig-out/bin/zconfig \
  --core ./zig-out/bin/zconfig-core \
  --handshake

# 提案を読み取り専用 TUI で確認
./zig-out/bin/zconfig review ./app.proposal.json \
  --source ./app.json \
  --schema ./app.schema.json \
  --core ./zig-out/bin/zconfig-core

# 全項目の判断と登録済みバリデーターを指定して最終差分を表示
./zig-out/bin/zconfig apply ./app.proposal.json \
  --source ./app.json \
  --schema ./app.schema.json \
  --session app-review \
  --approve change-theme \
  --reject change-legacy \
  --validator project-check \
  --config ./commands.json \
  --core ./zig-out/bin/zconfig-core

# 表示された confirmation token を使い、同じセッションを明示確認して反映
./zig-out/bin/zconfig apply ./app.proposal.json \
  --source ./app.json \
  --schema ./app.schema.json \
  --session app-review \
  --approve change-theme \
  --reject change-legacy \
  --validator project-check \
  --config ./commands.json \
  --confirm-token <表示されたトークン> \
  --core ./zig-out/bin/zconfig-core
```

`--schema` は省略できます。色を使えない端末では `--monochrome` を追加してください。提案ファイルの形式と操作の考え方は [レビューワークフロー](docs/workflow.md) を参照してください。

現時点の `review` コマンドは読み取り専用画面までです。反映は独立した `apply` コマンドで、`--session` と、すべての項目に対する `--approve` または `--reject` を指定します。初回実行はレビューセッションを保存し、最終差分と5分間・一回限りの確認トークンを表示して終了コード2で無変更終了します。表示後の別実行で同じセッションと `--confirm-token` を渡した場合だけ反映します。バリデーター失敗は終了コード3、その他のエラーは終了コード1です。

## コマンド登録

エージェントとバリデーターは、シェル文字列ではなく実行ファイルと引数配列としてユーザー設定へ登録します。既定パスは OS のユーザー設定ディレクトリ配下にある `zconfig/commands.json` です。

```json
{
  "agents": [
    {"name": "local-agent", "executable": "/absolute/path/to/agent", "args": [], "timeout_seconds": 120}
  ],
  "validators": [
    {"name": "project-check", "executable": "/absolute/path/to/validator", "args": [], "timeout_seconds": 10}
  ]
}
```

`working_directory` は省略時にプロジェクトディレクトリ、`environment_allowlist` は子プロセスへ渡してよい環境変数名だけを列挙します。バリデーターは候補ファイルのパスとダイジェストを stdin で受け、同じ候補ダイジェストを含む結果を stdout へ1件だけ返します。

## 主なキー

現在の読み取り画面では `j`/`k` または矢印で移動、`Enter` で詳細、`d` で差分、`/` で絞り込み、`?` でヘルプ、`Esc` で一覧、`q` で終了します。承認画面の操作は公開 CLI への接続後に確定します。

## 復旧

反映成功時にも元内容を `<設定ファイル>.zconfig-recovery` として残します。反映結果が不明な中断後は、元・反映後の両ダイジェストと現在のファイルを照合し、どちらでもない場合は自動継続せず手動確認を要求します。`.zconfig/runtime/` の確認記録は値を含まず、短時間・一回限りです。

## ドキュメント

- [レビューワークフロー](docs/workflow.md) — 変更案の受け取りから限定修正、承認、反映まで
- [アーキテクチャ](docs/architecture.md) — Zig と Go の責務、データフロー、設計境界
- [エージェント連携](docs/agent-integration.md) — コマンド登録、プロトコル、修正範囲の規則
- [安全設計](docs/security.md) — 機密値、プロセス、監査、ファイル反映の方針
- [開発ガイド](docs/development.md) — 環境構築、検証コマンド、ディレクトリ構成
- [機能仕様](specs/001-review-config-changes/spec.md) / [実装計画](specs/001-review-config-changes/plan.md) / [タスク](specs/001-review-config-changes/tasks.md)

仕様書は判断の根拠、`docs/` は利用者・開発者向けの説明として扱います。両者が食い違う場合は仕様書を優先します。
