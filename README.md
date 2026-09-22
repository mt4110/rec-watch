# RecWatch

**RecWatch** は、macOSの画面収録などで作成された動画ファイルを、自動で監視・変換するCLIツールです。
フォルダに動画を保存するだけで、自動的に1080pのMP4（H.264）に変換し、完了を通知します。

> [!NOTE]
> 本ツールは **macOS** での利用を前提としています。

## 特徴

-   **監視モード (Watch Mode)**: 指定したフォルダを常時監視。録画を停止するだけで勝手にMP4化されます。
-   **デスクトップ通知**: 変換完了時にMacの通知センターでお知らせします（通知音付き）。
    -   **クリックで再生**: 通知をクリックすると、変換されたMP4ファイルがデフォルトのプレイヤー（QuickTime Playerなど）で即座に開きます。
-   **スマートな変換**:
    -   アスペクト比を維持しつつ1080pにリサイズ＆黒帯追加（パディング）。
    -   変換元のファイルは `--source-policy` で `keep` / `trash` / `ask` から選択できます。
    -   既定では元ファイルを残し、必要な場合だけ `--source-policy trash` / `ask` でゴミ箱移動を選べます。
    -   変換後ファイルが存在しない、0バイト、または元ファイルより大きい場合は、元ファイルをゴミ箱へ移動しません。
    -   **ファイル名自動整理**: 録画日時と元ファイル名（`YYYY-MM-DD_HH-MM-SS_name.mp4`）に自動リネームし、同名衝突を避けます。
-   **安定化待ち**: 監視モードでは、作成直後のファイルにすぐ触らず、サイズと更新日時が連続して安定するまで待ってから変換します。
-   **高速処理**: CPUコア数に応じた並列処理で、大量のファイルもサクサク変換。

## セキュリティとプライバシー

RecWatchは、ユーザーのプライバシーとセキュリティを第一に設計されています。

-   **完全ローカル動作**: すべての処理はご自身のMac内（ローカル）で完結します。動画データやログが外部サーバーに送信されることは一切ありません。
-   **安全な既定値**: 変換後も元ファイルを残す `--source-policy keep` が既定です。ゴミ箱への移動は明示的に選んだ場合だけ行います。
-   **削除前の安全判定**: 変換が成功し、出力ファイルが存在し、変換後サイズが元ファイルより小さい場合だけ、元ファイルをゴミ箱へ移動できます。
-   **オープンソース**: ソースコードは全て公開されており、不審な挙動がないことを誰でも確認できます。

## インストール

### 1. FFmpegのインストール
このツールは動画変換に `ffmpeg` を必須とします。RecWatchはFFmpegを自動更新しないため、Homebrewなどで管理してください。
```bash
brew install ffmpeg
brew upgrade ffmpeg
```

### 2. terminal-notifierのインストール (推奨)
変換完了通知をクリックしてファイルを開くために必要です。
```bash
brew install terminal-notifier
```

### 3. ツールのインストール
Go環境がある場合:
```bash
go install github.com/mt4110/rec-watch@latest
```
または、リポジトリをクローンしてビルドします。開発時のGoは `mise.toml` で `1.26.5` に固定しています。
```bash
git clone https://github.com/mt4110/rec-watch.git
cd rec-watch
mise install
mise exec -- go test ./...
mise exec -- go build -o rec-watch main.go
sudo mv rec-watch /usr/local/bin/
```

## 使い方

### 監視モード (おすすめ)
指定したディレクトリを常時監視し、新しいファイルが追加されると自動で変換します。
```bash
mkdir -p ~/Desktop/ScreenRecordings-out ~/Desktop/ScreenRecordings
./rec-watch --watch ~/Desktop/ScreenRecordings --notify --dest ~/Desktop/ScreenRecordings-out
```
![DEMO](./docs/demo.gif)

監視モードでは、録画アプリやESETなどのリアルタイム保護、クラウド同期が作成直後のファイルを触っている可能性があります。RecWatchは固定秒数で待つのではなく、ファイルサイズと更新日時が安定したことを確認してから変換します。

```bash
rec-watch --watch ~/Desktop/ScreenRecordings \
  --stable-timeout 120s \
  --stable-interval 1s \
  --stable-samples 3
```

### 変換元ファイルの扱い

デフォルトは `--source-policy keep` です。`trash` または `ask` を選んだ場合でも、変換後ファイルが0バイト、元ファイル以上のサイズ、または存在しない場合は、元ファイルを残します。

```bash
# 元ファイルを必ず残す (default)
rec-watch ~/Movies/ScreenRecordings --source-policy keep

# 安全条件を満たした場合だけゴミ箱へ移動
rec-watch ~/Movies/ScreenRecordings --source-policy trash

# 変換後に削減量を見て確認する (macOS ではダイアログ、その他のOSでは keep)
rec-watch ~/Movies/ScreenRecordings --source-policy ask

# 後方互換: --source-policy keep と同じ
rec-watch ~/Movies/ScreenRecordings --no-trash
```

### 一括変換モード
カレントディレクトリ、または指定したディレクトリ以下の動画ファイルを一括変換します。
```bash
# カレントディレクトリ
rec-watch

# 指定ディレクトリ
rec-watch ~/Movies/ScreenRecordings
```

### オプション一覧
```bash
Flags:
      --batch-stamp               出力先ディレクトリをタイムスタンプ付きで作成する (default true)
      --concurrent int            並列実行数 (default CPUコア数-1)
      --crf int                   CRF値 (品質) (default 22)
      --dest string               出力先ディレクトリ (default "./out")
      --dry-run                   実行せずにコマンドを表示する
      --ffmpeg-bin string         ffmpegのバイナリパスを明示的に指定する
      --fps int                   フレームレート (0で無効)
      --gpu                       GPU(VideoToolbox)を使用して変換する（超爆速・画質/圧縮率はCPUに劣る）
  -h, --help                      help for rec-watch
      --ignore-keywords strings   ファイル名に含まれるキーワード 除外
      --keywords strings          ファイル名に含まれるキーワードでフィルタ
      --mute                      音声をミュートする
      --no-pad                    1080pにリサイズする際に黒帯を追加しない
      --no-trash                  変換元のファイルをゴミ箱に移動しない
      --notify                    変換完了時にデスクトップ通知を送る (default true)
      --parallel-split            動画を分割して並列変換する（大容量ファイル向け・爆速）
      --preset string             エンコードプリセット (default "faster")
      --profile string            使用するプロファイル名
      --source-policy string      変換元ファイルの扱い (keep, trash, ask)
      --stamp-per-file            個別のファイル名にタイムスタンプを追加する
      --stable-interval duration  監視時にファイルサイズと更新日時を確認する間隔 (default 1s)
      --stable-samples int        安定判定に必要な連続サンプル数 (default 3)
      --stable-timeout duration   監視時にファイル安定化を待つ最大時間 (default 120s)
      --watch                     指定したディレクトリを監視して自動変換する
```

変換履歴は成功時にOS標準の設定ディレクトリ配下の `RecWatch/history.jsonl` へJSONL形式で保存されます。`rec-watch stats` はこの履歴と従来のログを両方集計します。

### 実行前チェック

通常変換、監視モード、TUI、LaunchAgentの `install` では、開始前に最低限の実行条件を確認します。FFmpegが見つからない、FFmpegを実行できない、出力先に書き込めない、監視対象ディレクトリが存在しない場合は、変換や常駐化を開始しません。

```bash
rec-watch doctor
```

`doctor` は詳細診断用です。FFmpeg、通知、ログ、LaunchAgent状態をまとめて確認できます。

---

## 自動実行（常駐化） on macOS

PC起動時に自動で `RecWatch` を立ち上げるには、インストール済みの `rec-watch` バイナリからLaunchAgentを登録します。`go run . install` のような一時バイナリは登録しません。

```bash
rec-watch install --watch ~/Desktop/ScreenRecordings --dest ~/Desktop/ScreenRecordings-out
rec-watch status
```

`install` は `~/Library/LaunchAgents/com.user.recwatch.plist` を生成し、`launchctl bootstrap` と `launchctl kickstart` で登録・起動します。plistには監視対象、出力先、作業ディレクトリ、ログ出力先が明示されます。

```bash
rec-watch start
rec-watch stop
rec-watch uninstall
```

手元でビルドしたバイナリを登録する場合は、絶対パスを指定できます。

```bash
rec-watch install --bin /usr/local/bin/rec-watch --watch ~/Desktop/ScreenRecordings --dest ~/Desktop/ScreenRecordings-out
```

## トラブルシューティング

### 通知が表示されない場合

`terminal-notifier` をインストールしても通知が表示されない場合は、以下を確認してください。

1.  **通知設定の確認**:
    - macOSの「システム設定」>「通知」を開きます。
    - アプリケーション一覧から `terminal-notifier` (または `rec-watch`) を探し、通知が許可されているか確認してください。
    - **「集中モード」（おやすみモードなど）がオンになっていないか確認してください。** オンになっていると通知が届かない場合があります。

2.  **通知のテスト**:
    ターミナルで以下のコマンドを実行して、通知が表示されるか確認できます。
    ```bash
    terminal-notifier -title "テスト" -message "これはテストです" -sound default
    ```
    これで表示されない場合は、`terminal-notifier` 自体の問題か、macOSの設定の問題です。

3.  **古い通知の削除**:
    通知センターに古い通知が溜まっていると、新しい通知が表示されない（隠れている）場合があります。通知センターを開いて確認してみてください。


## 📖 詳細マニュアル
TUIモードの操作方法や、GPU/並列変換モードの詳しい仕様については、以下のドキュメントを参照してください。

👉 [詳細マニュアル (docs/USAGE.md)](./docs/USAGE.md)

## 関連、親和性があるリポジトリ
 [readme-gif-crafter](https://github.com/mt4110#:~:text=1-,readme%2Dgif%2Dcrafter,-Public)
