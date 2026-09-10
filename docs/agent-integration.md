# エージェント連携

## 役割

外部エージェントは変更案を生成します。zconfig は自然言語解釈を信頼してそのまま書き込まず、構造、元ファイルとの対応、変更範囲を Zig コアで検証します。

初回提案は現在、zconfig の起動前に作成してファイルとして渡します。登録済みエージェントへの修正要求を TUI から操作する完全な導線は開発中です。

## コマンド登録

既定の登録ファイルは、OS のユーザー設定ディレクトリ以下の `zconfig/commands.json` です。別の場所を使う場合は `--config` で指定します。

```json
{
  "agents": [
    {
      "name": "project-agent",
      "executable": "/absolute/path/to/agent",
      "args": ["zconfig-adapter"],
      "working_directory": "/absolute/path/to/project",
      "environment_allowlist": ["PATH"],
      "timeout_seconds": 120,
      "protocol_major": 1
    }
  ],
  "validators": [
    {
      "name": "project-check",
      "executable": "/absolute/path/to/validator",
      "args": [],
      "timeout_seconds": 10,
      "protocol_major": 1
    }
  ]
}
```

`executable` と `args` は文字列のコマンドラインとして再解釈されません。実行ファイルと引数配列を分け、シェルを介さず起動します。作業ディレクトリは省略時にプロジェクトディレクトリ、エージェントの既定タイムアウトは 120 秒、バリデーターは 10 秒です。環境変数は許可リストに列挙した名前だけを引き継ぎます。

```sh
zconfig \
  --core /absolute/path/to/zconfig-core \
  --config /absolute/path/to/commands.json \
  --agent project-agent \
  --handshake
```

## 通信と限定修正

エージェントも一回の起動で標準入力の JSON リクエスト一件に応答し、最初に `protocol_info` で互換性を確認します。修正要求には元の提案を識別するダイジェスト、許可された変更項目 ID、その項目へのコメント、各種制限を含めます。

応答は候補であり、受理を意味しません。`validate_revision` は次を確認します。

- 許可された ID 以外を変更していない
- ID、パス、元ファイル識別子などの不変フィールドを変えていない
- 古い提案を基準にしていない
- JSON 契約と規模上限を満たす

一つでも範囲外変更があれば部分採用せず、応答全体を拒否します。追加変更が必要なら、エージェントは `revision.scope_expansion_required` を返し、人間が別の変更項目として判断できるようにします。

## 機密値

機密値は既定でエージェントへ共有しません。将来の完全なフローでも、共有は項目単位・一回限り・明示確認付きとし、終了や再開で失効させます。この機構は設計済み・未実装です。現段階では、実際の秘密を含むファイルを外部エージェントへ渡さないでください。

## zintent との関係

zintent がスキル群で開発プロセスを組み立てる場合、zconfig は終盤工程のレビュー境界として利用する想定です。zintent 固有の概念をコアへ埋め込まず、構造化提案と一回起動プロトコルを接続点にします。
