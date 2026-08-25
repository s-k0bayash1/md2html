# md2html

MarkdownをシングルバイナリでHTMLに変換するCLI。出力されるHTMLは1ファイルで完結する。

## 特徴

- シングルバイナリで動作(CSS・mermaid.jsはバイナリに同梱)
- 出力HTMLは1ファイルで成立
  - CSSはインライン埋め込み
  - ローカル画像はBase64データURIとして埋め込み
- 閲覧時のJavaScript依存は最小限
  - Mermaidブロックが無いドキュメントはJSゼロ
  - シンタックスハイライトは変換時に焼き込み(chroma)
- テーブルはヘッダ固定(`position: sticky`)がデフォルト
- Mermaid対応(` ```mermaid `フェンス)。図がある時だけmermaid.jsをインライン同梱
- ダークモード自動追従(`prefers-color-scheme`。コードハイライト・Mermaidも追従)
- GFM(テーブル・打ち消し線・タスクリスト・自動リンク)+ 脚注 + 見出しアンカー
- YAMLフロントマター対応(`title`・`lang`を反映)

## インストール

```sh
go install github.com/s-k0bayash1/md2html/cmd/md2html@latest
```

またはGitHub Releasesからバイナリをダウンロード(macOS / Linux / Windows)。

## 使い方

```sh
# README.htmlを隣に生成
md2html README.md

# 出力先を指定
md2html README.md -o docs/readme.html

# パイプで使う
cat README.md | md2html > readme.html

# ディレクトリを渡すと直下の*.mdを一括変換
md2html docs/

# サブディレクトリも再帰的に変換
md2html -r docs/
```

### ディレクトリ変換

変換先(引数)にディレクトリを渡すと、直下の`.md`ファイルを全て変換する。`-r`を付けるとサブディレクトリも再帰的に処理する(`.git`のようなドット始まりのディレクトリは対象外)。

- 各ファイルの出力先は単一ファイル変換と同じく、隣に同名の`.html`を生成する(`-o`との併用はエラー)
- すでに`.html`が存在するファイルは`-force`を付けない限り上書きせず、警告を出してスキップする(スキップ自体は失敗扱いにしない)
- 1ファイルの変換に失敗しても処理は止めず、そのファイルだけエラーを出して他のファイルの変換を続ける。1件でも失敗があれば終了コードは1
- 対象の`.md`が1件も見つからない場合は警告を出すだけで、終了コードは0

### フラグ

| フラグ | 説明 |
|--------|------|
| `-o <path>` | 出力ファイルパス。デフォルトは入力ファイルの拡張子を`.html`に替えたもの(stdin入力時はstdout)。入力がディレクトリのときは指定不可 |
| `-lang <lang>` | `<html lang>`属性。デフォルトは`en`。フロントマターの`lang`が優先 |
| `-no-embed` | ローカル画像のデータURI埋め込みを無効化 |
| `-r` | 入力がディレクトリのとき、サブディレクトリも再帰的に変換する |
| `-force` | 入力がディレクトリのとき、既存の`.html`を上書きする |
| `-version` | バージョンを表示 |

### 画像の扱い

- ローカル画像はデータURIとしてHTMLに埋め込む
- `http(s)://`の外部URL画像はそのまま残す(取得しない)
- 見つからない画像は警告をstderrに出してパスをそのまま残す

### タイトルの決定順

1. フロントマターの`title`
2. 文書最初のh1見出し
3. 入力ファイル名(拡張子抜き)

## 開発

```sh
go test ./...
go build ./cmd/md2html
```

リリースはタグ(`v*`)をpushするとGitHub Actions + goreleaserがバイナリを添付する。

`cmd/md2html/assets/mermaid.min.js`は[mermaid](https://github.com/mermaid-js/mermaid) v11.12.2(MITライセンス)をjsDelivrから取得したもの。更新する場合は新しいバージョンで差し替える。
