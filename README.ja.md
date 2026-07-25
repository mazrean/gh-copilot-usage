# gh-copilot-usage

[English](README.md)

GitHub Copilot CLI の AI クレジット（AIC）使用量を、積み上げ時系列チャートとしてブラウザ上に可視化する [`gh` CLI](https://cli.github.com/) 拡張機能。GitHub の課金 API ともクロスチェックできる。

> 以下のスクリーンショットは、ドキュメント作成用に生成したダミーデータであり、実際のアカウントの利用状況ではない。

## できること

`gh-copilot-usage` は、Copilot CLI がローカルに保持している SQLite データベース `~/.copilot/session-store.db` を読み取り、インタラクティブなダッシュボードとして表示する。

- **積み上げ使用量チャート** — AIC 使用量を日次・週次・月次で集計し、モデル別またはセッション別に積み上げ表示する。
- **課金額とのクロスチェック** — 既存の `gh` ログインを使って GitHub 課金 API から当月の AIC 合計を取得し、ローカル計測値と並べて月末時点の使用量予測を表示する。
- **セッション詳細** — バーをクリックすると、そのセッションのモデル別内訳、ターンごとの AIU チャート、（Copilot CLI のバージョンが記録していれば）ターンごとの所要時間・トークン数・個々の呼び出し（サブエージェントへの委譲を含む）のトレースを確認できる。
- **モデル詳細** — モデルのセグメントをクリックすると、トークンコストのカテゴリ別内訳（入力 / キャッシュ読み込み / キャッシュ書き込み / 出力）を確認できる。
- **UI の日英切り替え機能** — ターミナルのロケールとは独立して切り替え可能。
- **スクリプト向け JSON 出力モード**（`--json`） — サーバーを起動せずに集計結果を他のツールへパイプできる。

すべてローカルで完結する。ローカル DB の読み取りにはトークンが一切不要で、課金額とのクロスチェックも既存の `gh` 認証を再利用するため、別途の認証情報設定は必要ない。

## スクリーンショット

**モデル別の日次使用量** — 上部にはローカル計測値と課金額クロスチェックのサマリーカードを表示：

![モデル別の日次使用量](docs/screenshots/dashboard-model-daily.png)

**セッション別の週次使用量** — 各セグメントが 1 つの Copilot CLI セッションに対応：

![セッション別の週次使用量](docs/screenshots/dashboard-session-weekly.png)

**セッション詳細** — モデル別合計、ターンごとのチャート、選択したターンのトークン数と呼び出しトレース：

![セッション詳細モーダル](docs/screenshots/session-detail-modal.png)

**モデル詳細** — トークンコストのカテゴリ別内訳：

![モデル詳細モーダル](docs/screenshots/model-detail-modal.png)

UI 自体も日本語に切り替え可能：

![日本語UIでのモデル別日次使用量](docs/screenshots/dashboard-model-daily-ja.png)

## インストール

```bash
gh extension install mazrean/gh-copilot-usage
```

## 使い方

```bash
gh copilot-usage
```

ローカルサーバーを起動し（デフォルトは `127.0.0.1:8765`。使用中の場合はランダムなポートにフォールバック）、ブラウザで開く。

| フラグ | デフォルト | 説明 |
| --- | --- | --- |
| `--db` | `~/.copilot/session-store.db` | 読み取る Copilot CLI のセッションストア DB のパス。 |
| `--addr` | `127.0.0.1:8765` | Web UI を配信するアドレス。 |
| `--no-open` | `false` | ブラウザを自動的に開かない。 |
| `--json` | `false` | 集計結果を JSON で標準出力して終了する（サーバーは起動しない）。 |
| `--dimension` | `model` | `--json` 時の積み上げ軸: `model` または `session`。 |
| `--granularity` | `day` | `--json` 時の時間バケット: `day` / `week` / `month`。 |

例えば、サーバーを起動せずにモデル別の週次サマリーを取得する場合：

```bash
gh copilot-usage --json --dimension model --granularity week
```

### 課金額とのクロスチェック

「今月（課金ベース）」のカードは、既存の `gh` ログインを通じて GitHub の課金 API を呼び出す。トークンに `user` スコープが無い場合でも、このカードのみ「取得不可」として表示され、ページ全体は正常に動作する。有効化するには `gh auth refresh -s user` を実行する。

## 開発

アーキテクチャの詳細やビルド・テスト・lint コマンドについては [AGENTS.md](AGENTS.md) を参照。要約すると：

```bash
mise run build   # フロントエンドと gh-copilot-usage バイナリをビルド
mise run test    # go test ./...
mise run lint    # go vet + staticcheck + stylecheck
```
