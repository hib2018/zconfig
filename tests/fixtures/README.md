# テストフィクスチャ

- `configs/app.json` / `schemas/app.schema.json` / `proposals/app.proposal.json`: add・replace・remove を含む標準シナリオ
- `configs/duplicate-key.json`: 必ず拒否する重複キー
- `configs/unicode.json`: UTF-8、escape、絵文字
- `configs/crlf.json`: CRLF を保持する入力（バイト列はテスト内でも生成する）
- `usability/`: 20項目の固定ユーザビリティ試験と回答表
- `generate_scale.go`: 10 MiB、最大100,000ノード、1,000変更の負荷用データを一時ディレクトリへ生成する

機密値はすべてテスト専用です。実在する資格情報を追加しないでください。
