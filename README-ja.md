[English](README.md) · 日本語 · [Deutsch](README-de.md) · [Français](README-fr.md)

# dead-mans-ping (`mip`)

[![CI](https://github.com/didvc/dead-mans-ping/actions/workflows/ci.yml/badge.svg)](https://github.com/didvc/dead-mans-ping/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/didvc/dead-mans-ping)](https://goreportcard.com/report/github.com/didvc/dead-mans-ping)

マウスの動きに応じて、1つ以上のエンドポイントに HTTP `GET` リクエストを送る、小さなクロスプラットフォーム（Linux、Windows）の Go 製 CLI です。たとえば、マシンが数日間放置されたら URL に ping を送るデッドマンスイッチとして使えます。

![didvc/dead-mans-ping](assets/social-preview.png)

一定間隔でカーソルの絶対位置を読み取る仕組みです。管理者権限は不要で、グローバルな入力フックも仕掛けないので、一般ユーザーとして安全に実行できます。

- Linux：X11 セッション（`$DISPLAY`）が必要です。XWayland のウィンドウでは動作しますが、ネイティブの Wayland は設計上、ポインタの位置をグローバルに公開しません。
- Windows：標準ライブラリ経由で `user32!GetCursorPos` を使います（cgo 不要）。

![動作中の dead-mans-ping](assets/demo-run.png)

## インストール

```sh
# From source (Go 1.25+); installs the `mip` binary:
go install github.com/didvc/dead-mans-ping/cmd/mip@latest
```

または、[Releases](https://github.com/didvc/dead-mans-ping/releases) ページから、お使いのプラットフォーム向けのビルド済みバイナリを入手してください。

## ビルド

```sh
make            # test + build ./bin/mip for the host
make release    # cross-compile ./bin/mip-linux-amd64 and mip-windows-amd64.exe
make test
```

## 使い方

```sh
mip --endpoint https://example.com/ping [--endpoint https://backup/ping ...] [options]
```

リクエストはすべて HTTP `GET` です。`--endpoint` は少なくとも1つ必要です。エンドポイントはホストを含む `http`/`https` である必要があり、リクエストにはタイムアウト、リダイレクト回数の上限、読み込むボディのサイズ制限があります。

### 動作は3つの独立した選択で決まる

いつ発火するか（「フラグ」）:

| フラグ              | 意味                                                                |
| ------------------- | ------------------------------------------------------------------- |
| `--inactive-ping`   | *（デフォルト）* マウスが `--inactive-period` 以上動かないとフラグが立つ|
| `--active-ping`     | 動きを検出した瞬間にフラグが立つ（`--inactive-period` は無視） |

フラグが立っている間の ping の送り方:

| フラグ              | 意味                                                                |
| ------------------- | ------------------------------------------------------------------- |
| `--ping-once`       | *（デフォルト）* フラグが立つごとに1回だけ ping                     |
| `--ping-continuous` | フラグが立っている間 `--ping-interval` ごとに ping。フラグが下りると停止 |

ライフサイクル:

| フラグ              | 意味                                                                |
| ------------------- | ------------------------------------------------------------------- |
| `--onetime`         | *（デフォルト）* 最初の ping の後、または連続 ping が止まった後に終了 |
| `--cold-period D`   | 動作を続け、ping の間隔を最低 `D` 空ける（`--onetime` より優先） |

### すべてのオプション

| フラグ                | デフォルト | 説明                                             |
| --------------------- | ------- | -------------------------------------------------- |
| `--endpoint URL`         | -                 | 動きに応じた ping の送信先。複数指定するには繰り返す |
| `--inactive-period D`    | `3d`              | `--inactive-ping` の無操作しきい値           |
| `--ping-interval D`      | `30s`             | `--ping-continuous` の繰り返し間隔           |
| `--cold-period D`        | unset             | ping の最小間隔（`--onetime` ではない動作になる） |
| `--heartbeat-endpoint URL` | unset           | 生存確認用の URL。一定間隔で GET（下記参照） |
| `--heartbeat-interval D` | `60s`             | `--heartbeat-endpoint` の間隔                |
| `--server`               | off               | 制御用 HTTP サーバーを起動（下記参照）        |
| `--server-addr HOST:PORT`| `127.0.0.1:8080`  | `--server` の待ち受けアドレス                 |
| `--poll-interval D`      | `1s`              | カーソルを読み取る間隔                        |
| `--move-threshold N`     | `1.0`             | 動きとみなす最小のピクセル距離                |
| `--timeout D`            | `10s`             | リクエストごとの HTTP タイムアウト            |
| `--no-log`               | off               | ステータス行とイベントログを無効化            |

## ハートビート（生存確認）

`--heartbeat-endpoint` は、動きに応じた ping とは別のループです。マウスの動きに関係なく、`--heartbeat-interval` ごとに（起動直後から）その URL に GET を送るので、外部の監視からこのプロセスがまだ動いていることがわかります。動きに応じた ping とエンドポイントやタイミングを共有することはありません。

```sh
mip --endpoint https://example.com/idle \
    --heartbeat-endpoint https://hc-ping.com/alive --heartbeat-interval 5m
```

## 制御サーバー

`--server` を付けると、HTTP の制御インターフェースが起動します（デフォルトでは `127.0.0.1:8080` で待ち受け）:

| エンドポイント              | 効果                                                          |
| --------------------------- | ------------------------------------------------------------- |
| `GET /extend?seconds=<N>`   | 無操作の期限を N 秒先へ延ばす（加算される）                    |
| `GET /extend?until=<unix>`  | 無操作の期限を絶対的な Unix タイムスタンプまで延ばす            |
| `GET /help`                 | 使い方のテキスト                                              |

`/extend` は `--inactive-ping` が発火するタイミングを先送りします。マウスに触れずに使える、遠隔からの「まだここにいるよ」です。期限は先へ進むだけで、`seconds` と `until` のどちらか一方だけを指定します。これはデッドマンスイッチを無効にできてしまうため、サーバーはデフォルトで localhost にだけバインドされます。公開する場合は、必ず自前の認証やプロキシの後ろに置いてください。

```sh
mip --endpoint https://example.com/idle --inactive-period 1h --cold-period 1h --server
# from elsewhere on the box:
curl 'http://127.0.0.1:8080/extend?seconds=3600'   # hold off for another hour
```

![制御サーバーとハートビート](assets/demo-server.png)

時間の指定には `s`、`m`、`h` に加えて `d`（日）と `w`（週）が使えます。例：`3d`、`1w`、`1d12h`。

### ステータス行

`--no-log` を付けない限り、現在のモード、無操作の時間、直近 1時間／1日／1週間の動きの概要（ピクセル距離の合計と移動回数）を示すステータス行が表示されます:

```
[14:22:07] mode=inactive flag=false idle=1m3s | 1h 4821.5px/142 1d 4821.5px/142 1w 4821.5px/142
```

動きの指標は1分単位の区切りに集計され、1週間分のリングバッファに保持されるので、プロセスをどれだけ長く動かしてもメモリ使用量は一定です。

## 使用例

```sh
# Dead-man's switch: ping once after 3 days idle, then exit (all defaults).
mip --endpoint https://hc-ping.com/UUID

# Heartbeat: while idle ≥ 1h, ping every 5 minutes; keep running,
# no more than one ping per 5 minutes.
mip --inactive-period 1h --ping-continuous --ping-interval 5m \
    --cold-period 5m --endpoint https://example.com/idle

# Presence beacon: ping the instant the mouse moves, at most every 30s.
mip --active-ping --cold-period 30s --endpoint https://example.com/active
```

## CLI リファレンス

すべてのフラグ（`mip --help`）:

![mip --help](assets/demo-help.png)

## プライバシー

このツールが読み取るのは、動きを検出するためのカーソルの画面座標だけで、メモリ上でのみ扱います。キー入力、ウィンドウのタイトル、画面の内容は読み取りません。ディスクには何も保存されず、テレメトリもありません。ネットワーク通信は、`--endpoint` と `--heartbeat-endpoint` で設定した GET リクエストだけです。

## ライセンス

[Apache License 2.0](LICENSE) のもとで公開しています。

<!-- BEGIN gh-mutual-linking -->

---

### Related projects

- [chatnote](https://github.com/didvc/chatnote): Self-hosted note-to-self chatrooms. Privacy-first by design, infinite rooms, Markdown, ephemeral/incognito room types, image uploads, tags, JSON import/export. Astro SSR + SQLite.
- [visited](https://github.com/didvc/visited): Securely collect browsing history over browsers.
<!-- END gh-mutual-linking -->