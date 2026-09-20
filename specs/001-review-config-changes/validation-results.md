# 検証結果

最終更新: 2026-09-20

## 自動検証

| ゲート | 結果 |
|---|---|
| Zig test | PASS |
| Zig ReleaseSafe build | PASS |
| Go test | PASS |
| Go race test | PASS |
| Go vet | PASS |
| 10 MiB級／100,000ノード、1,000変更、1,000項目UI benchmark | PASS |
| 通常ファイル以外の fail-closed | macOS で directory と symlink を PASS。device/FIFO と他OSは未検証 |

承認サブセット、再開、古い元ファイル、復旧、nonce 一回消費、機密値の表示・プロセス・保存境界は自動統合試験で確認した。例外承認はない。

## Constitution gate

- 人間の最終権限: コアは短命でダイジェストに結び付いた確認 capability を要求する。公開 TUI の完全な導線は未接続のためリリース不可。
- 構造化・検証可能な変更: PASS。
- lossless な限定編集: JSON の自動試験は PASS。TOML/YAML は対応済みとして表示していない。
- portable core: Zig コアはネットワーク不要。Linux/Windows のネイティブ結果待ち。
- incremental simplicity: 外部バリデーターは raw JSON 1往復として契約を明記。
- secret redaction: 通常経路は PASS。最終 diff を含む全コア直列化境界の追加監査が残る。

## 未完了のリリースゲート

- Linux と Windows のネイティブ置換検証
- quickstart 全シナリオの手動実行
- 初回利用者5名以上のユーザビリティ試験
- 全 golden document の Zig／Go 同一受理判定
- 公開 CLI からバリデーター、最終確認、反映までの完全な結線

未完了項目は未実施のまま PASS と推定しない。
