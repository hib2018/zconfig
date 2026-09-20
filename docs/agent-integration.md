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

機密値は既定でエージェントへ共有しません。共有権限は項目とセッション指紋へ結び付け、人間の追加確認後の一回の呼び出しで消費します。終了や再開では権限を破棄し、共有した既知の値がエージェント出力へ現れた場合は表示・保存前に再マスクします。公開 TUI からこの操作へ到達する結線は未完了なので、現段階では実際の秘密を含むファイルを外部エージェントへ渡さないでください。

## 外部バリデーター

バリデーターはエージェント用 envelope ではなく、`external-validator.schema.json` の raw JSON を stdin/stdout で一往復します。登録時の `protocol_major: 1` が契約版の事前確認です。要求には同一ディレクトリに作成した候補ファイルのパス、候補ダイジェスト、元ファイルダイジェストを含めます。応答の候補ダイジェストが一致しない場合、または timeout、非0終了、不正JSON、`failed` の場合は最終反映を止めます。未登録は `passed` ではなく `unverified` です。

## zintent との関係

zintent がスキル群で開発プロセスを組み立てる場合、zconfig は終盤工程のレビュー境界として利用する想定です。zintent 固有の概念をコアへ埋め込まず、構造化提案と一回起動プロトコルを接続点にします。
