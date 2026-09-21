# 検証結果

最終更新: 2026-09-21

## 自動検証

| ゲート | 結果 |
|---|---|
| Zig test | PASS |
| Zig ReleaseSafe build | PASS |
| Go test | PASS |
| Go race test | PASS |
| Go vet | PASS |
| Zig／Go golden contract compatibility | PASS（64文書） |
| 10 MiB級／100,000ノード、1,000変更、1,000項目UI benchmark | PASS |
| 通常ファイル以外の fail-closed | macOS で directory、symlink、FIFO、device を PASS。他OSはネイティブCI結果待ち |
| 最終確認の分離 | PASS（事前プレビューの状態結合トークンが必須。偽造・期限切れ・一回消費・コメント状態変更を拒否） |
| 完成候補のスキーマ再検証 | PASS（プレビュー時と反映直前。スキーマ変更後の不適合も拒否） |
| セッション・監査接続 | PASS（判断・コメントを再読込し、監査追記不能時は権限操作を開始しない） |
| バリデーターパス | PASS（候補を絶対パスで渡し、異なる作業ディレクトリから検証） |

承認サブセット、再開、古い元ファイル、復旧、nonce 一回消費、機密値の表示・プロセス・保存境界は自動統合試験で確認した。例外承認はない。

## Constitution gate

- 人間の最終権限: PASS。公開 CLI は別実行のプレビューで得た短命・一回限り・状態結合トークンを要求する。TUI内の操作導線は今後のUI拡張として未接続。
- 構造化・検証可能な変更: PASS。
- lossless な限定編集: JSON の自動試験は PASS。TOML/YAML は対応済みとして表示していない。
- portable core: Zig コアはネットワーク不要。Linux/Windows のネイティブ結果待ち。
- incremental simplicity: 外部バリデーターは raw JSON 1往復として契約を明記。
- secret redaction: PASS。最終 diff はスキーマを再読込し、通常値の実バイトを保持しながらスキーマ指定・名前検出の機密値を変更前後ともマスクする。
- final validation: PASS。Zigコアは完成候補全体をスキーマで再検証し、反映時にも同じ検証を繰り返す。
- audit gate: PASS。CLIの判断、プレビュー、確認、反映開始、結果を値なしで追記し、反映後の成功記録に失敗した場合は復旧版へ戻す。

## 未完了のリリースゲート

- Linux と Windows のネイティブ置換検証（2026-09-20のプロジェクト判断により現段階では意図的にskip）
- quickstart 全シナリオの手動実行と初回利用者5名以上のユーザビリティ試験（2026-09-20のプロジェクト判断により現段階では意図的にskip）

skipした項目はPASSと推定せず、将来リリース基準を引き上げる際に再開する。
