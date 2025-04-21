package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath" // filepath パッケージをインポート
	"runtime"       // runtime パッケージをインポート (OS判定用)
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage" // 保存ダイアログのフィルター用
	"fyne.io/fyne/v2/widget"
	"github.com/skip2/go-qrcode" // QRコード生成ライブラリ
)

// --- クリップボード関連 ---

// copyImageToClipboardWindows は Windows 環境で画像をクリップボードにコピーします。
// PowerShell を使用して System.Windows.Forms の機能を利用します。
func copyImageToClipboardWindows(img image.Image) error {
	// 一時ディレクトリを取得
	tmpDir := os.TempDir()
	if tmpDir == "" {
		return fmt.Errorf("一時ディレクトリが見つかりません")
	}

	// 一時ファイル名をユニークにする (プロセスIDとナノ秒タイムスタンプ)
	tmpFileName := fmt.Sprintf("temp_qrcode_%d_%d.png", os.Getpid(), time.Now().UnixNano())
	tmpFilePath := filepath.Join(tmpDir, tmpFileName)

	// 一時ファイルを作成
	file, err := os.Create(tmpFilePath)
	if err != nil {
		return fmt.Errorf("一時ファイル作成失敗 (%s): %w", tmpFilePath, err)
	}

	// この関数が終了する際に、ファイルを閉じてから削除する
	// エラーが発生しても確実にクリーンアップを試みる
	defer func() {
		// ファイルを閉じる (エラーはログに出力するだけにする)
		if errClose := file.Close(); errClose != nil {
			log.Printf("一時ファイルクローズエラー (%s): %v", tmpFilePath, errClose)
		}
		// 一時ファイルを削除 (存在しないエラーは無視)
		if errRemove := os.Remove(tmpFilePath); errRemove != nil && !os.IsNotExist(errRemove) {
			log.Printf("一時ファイル削除失敗 (%s): %v", tmpFilePath, errRemove)
		} else if errRemove == nil {
			log.Printf("一時ファイル削除成功 (%s)", tmpFilePath)
		}
	}()

	// QRコード画像をPNG形式で一時ファイルにエンコード
	err = png.Encode(file, img)
	if err != nil {
		// エンコード失敗時は defer がクローズと削除を処理する
		return fmt.Errorf("PNGエンコード失敗: %w", err)
	}

	// ファイルへの書き込みを完了させるために一度閉じる（deferでも閉じるが、ここでエラーを確認）
	if err := file.Close(); err != nil {
		// クローズ失敗時も defer が削除を試みる
		return fmt.Errorf("一時ファイルへの書き込み完了(クローズ)失敗 (%s): %w", tmpFilePath, err)
	}

	// --- PowerShell コマンドの準備 ---
	// ファイルパス中のバックスラッシュをスラッシュに置換 (PowerShellはどちらも解釈できることが多い)
	psEscapedPath := filepath.ToSlash(tmpFilePath)

	// PowerShell スクリプトブロック
	// - エラー発生時にスクリプトを停止 ($ErrorActionPreference = 'Stop')
	// - try-catch でエラーを捕捉し、標準エラーに出力して終了コード1で終了
	// - System.Drawing.Image を使用してファイルを読み込み、クリップボードに設定
	// - 使用後、画像リソースを解放 ($img.Dispose())
	psCmd := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms;
$ErrorActionPreference = 'Stop';
try {
    $img = [System.Drawing.Image]::FromFile('%s');
    [System.Windows.Forms.Clipboard]::SetImage($img);
    $img.Dispose();
} catch {
    Write-Error "クリップボードへの画像設定中にエラーが発生しました: $($_.Exception.Message)";
    exit 1;
}
`, psEscapedPath)

	// --- PowerShell コマンド実行 ---
	cmd := exec.Command("powershell", "-Command", psCmd)
	output, err := cmd.CombinedOutput() // 標準出力と標準エラーを結合して取得

	// 実行時のログ出力 (デバッグ用)
	log.Printf("実行したPowerShellコマンド: powershell -Command \"%s\"", psCmd)
	log.Printf("PowerShellからの出力:\n%s", string(output))

	// PowerShell コマンドの実行結果を確認
	if err != nil {
		// PowerShellが非ゼロの終了コードで終了した場合や、コマンドが見つからない場合など
		// output には PowerShell のエラーメッセージが含まれている可能性が高い
		return fmt.Errorf("PowerShell実行失敗: %w\n出力: %s", err, string(output))
	}
	// PowerShellスクリプト内で exit 1 などを実行した場合も err != nil となる

	log.Println("PowerShellによるクリップボードへの画像コピー成功")
	return nil
}

// copyImageToClipboardOther は Windows 以外の OS 用のプレースホルダー関数です。
// 必要に応じて各 OS (macOS, Linux) 用の実装を追加します。
func copyImageToClipboardOther(img image.Image) error {
	// TODO: macOS ('osascript' or 'pbcopy') や Linux ('xclip') 用の実装を追加
	// 例: クロスプラットフォームライブラリ (golang.design/x/clipboard など) の利用を検討
	return fmt.Errorf("クリップボードへの画像コピーはこのOSでは現在サポートされていません (%s)", runtime.GOOS)
}

// copyImageToClipboard は OS を判定し、適切なコピー関数を呼び出すラッパーです。
func copyImageToClipboard(img image.Image) error {
	switch runtime.GOOS {
	case "windows":
		return copyImageToClipboardWindows(img)
	// case "darwin":
	//  return copyImageToClipboardDarwin(img) // macOS用の関数 (未実装)
	// case "linux":
	//  return copyImageToClipboardLinux(img) // Linux用の関数 (未実装)
	default:
		return copyImageToClipboardOther(img)
	}
}

// --- メイン処理 ---

func main() {
	// Fyne アプリケーションの初期化
	a := app.New()

	// アプリケーションアイコンの設定 ("icon.png" が存在する場合)
	iconResource, err := fyne.LoadResourceFromPath("icon.png")
	if err != nil {
		// アイコンが見つからない場合はログ出力のみ（アプリは続行）
		log.Println("アイコンファイル(icon.png)の読み込みに失敗しました:", err)
	} else {
		a.SetIcon(iconResource)
		log.Println("アプリケーションアイコンを設定しました。")
	}

	// メインウィンドウの作成
	w := a.NewWindow("QRコード生成ツール")
	w.Resize(fyne.NewSize(450, 550)) // ウィンドウサイズ調整
	w.SetMaster()                    // このウィンドウが閉じられたらアプリを終了する
	w.CenterOnScreen()               // ウィンドウを画面中央に表示

	// --- UI コンポーネントの作成 ---

	// 入力用テキストエリア (複数行対応)
	inputEntry := widget.NewMultiLineEntry()
	inputEntry.SetPlaceHolder("QRコードにしたいテキストを入力してください...\n(例: https://example.com)")
	inputEntry.Wrapping = fyne.TextWrapWord // 自動折り返し
	inputEntry.SetMinRowsVisible(4)         // 最小表示行数

	// QRコード画像表示用キャンバス
	qrImageCanvas := canvas.NewImageFromImage(nil)   // 初期状態は画像なし
	qrImageCanvas.FillMode = canvas.ImageFillContain // アスペクト比を維持して表示エリアに収める
	qrImageCanvas.SetMinSize(fyne.NewSize(256, 256)) // 画像表示エリアの最小サイズ確保

	// ステータス表示用ラベル
	statusLabel := widget.NewLabel("テキストを入力して「生成」ボタンを押してください。")
	statusLabel.Wrapping = fyne.TextWrapWord     // 長いメッセージは折り返す
	statusLabel.Alignment = fyne.TextAlignCenter // 中央揃え

	// 生成された image.Image を保持する変数 (コピー/保存用)
	var currentImage image.Image

	// --- ボタンとそのアクション ---

	// QRコード生成ボタンのアクション
	generateAction := func() {
		inputText := inputEntry.Text
		if inputText == "" {
			currentImage = nil // 画像データをクリア
			qrImageCanvas.Image = nil
			qrImageCanvas.Refresh()
			statusLabel.SetText("エラー: QRコードにする内容が入力されていません。")
			dialog.ShowError(fmt.Errorf("QRコードにする内容を入力してください。"), w)
			return
		}

		// QRコード生成 (go-qrcode ライブラリ使用)
		// qrcode.Medium: 誤り訂正レベル (L, M, Q, H)
		// 256: 生成するPNG画像のサイズ (ピクセル)
		pngData, err := qrcode.Encode(inputText, qrcode.Medium, 256)
		if err != nil {
			log.Printf("QRコードのエンコードに失敗: %v", err)
			statusLabel.SetText(fmt.Sprintf("エラー: QRコード生成失敗 (エンコード): %v", err))
			currentImage = nil
			qrImageCanvas.Image = nil
			qrImageCanvas.Refresh()
			dialog.ShowError(fmt.Errorf("QRコードの生成に失敗しました。\n入力内容を確認してください。\n詳細: %w", err), w)
			return
		}

		// PNGデータを image.Image にデコード
		imgData, format, err := image.Decode(bytes.NewReader(pngData))
		if err != nil {
			log.Printf("生成されたQRコード(PNG)のデコードに失敗: %v", err)
			statusLabel.SetText(fmt.Sprintf("エラー: QRコード生成失敗 (デコード): %v", err))
			currentImage = nil
			qrImageCanvas.Image = nil
			qrImageCanvas.Refresh()
			dialog.ShowError(fmt.Errorf("内部エラー: 生成された画像の処理に失敗しました。\n詳細: %w", err), w)
			return
		}
		log.Printf("QRコード生成成功 (フォーマット: %s)", format)

		// 生成された画像を保持し、キャンバスに表示
		currentImage = imgData
		qrImageCanvas.Image = currentImage
		qrImageCanvas.Refresh()
		statusLabel.SetText("QRコードを生成しました。")
	}

	// 生成ボタン
	generateBtn := widget.NewButton("QRコード生成", generateAction)

	// 画像をクリップボードにコピーボタンのアクション
	copyAction := func() {
		if currentImage == nil {
			statusLabel.SetText("エラー: コピーするQRコードが生成されていません。")
			dialog.ShowInformation("情報", "先にQRコードを生成してください。", w)
			return
		}

		err := copyImageToClipboard(currentImage)
		if err != nil {
			errMsg := fmt.Sprintf("クリップボードへのコピーに失敗しました。\n詳細: %v", err)
			statusLabel.SetText("コピーに失敗しました。")
			log.Printf("画像コピー失敗: %v", err)
			dialog.ShowError(fmt.Errorf(errMsg), w) // エラー詳細をダイアログで表示
		} else {
			statusLabel.SetText("画像をクリップボードにコピーしました。")
			log.Println("QRコード画像をクリップボードにコピーしました。")
			// 成功した場合の通知 (少し控えめに)
			// dialog.ShowInformation("成功", "画像をクリップボードにコピーしました。", w)
		}
	}

	// コピーボタン
	copyBtn := widget.NewButton("画像をコピー", copyAction)

	// 画像をファイルに保存ボタンのアクション
	saveAction := func() {
		if currentImage == nil {
			statusLabel.SetText("エラー: 保存するQRコードが生成されていません。")
			dialog.ShowInformation("情報", "先にQRコードを生成してください。", w)
			return
		}

		// デフォルトのファイル名を生成 (タイムスタンプ付き)
		timestamp := time.Now().Format("20060102_150405") // YYYYMMDD_HHMMSS形式
		defaultFilename := "qrcode_" + timestamp + ".png"

		// ファイル保存ダイアログを作成
		saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			// ダイアログ自体でエラーが発生した場合 (例: 権限なし)
			if err != nil {
				statusLabel.SetText("エラー: 保存場所の選択中にエラーが発生しました。")
				log.Printf("ファイル保存ダイアログエラー: %v", err)
				dialog.ShowError(fmt.Errorf("保存場所の選択エラー: %w", err), w)
				return
			}
			// ユーザーがキャンセルした場合、writer は nil になる
			if writer == nil {
				statusLabel.SetText("保存がキャンセルされました。")
				log.Println("ファイル保存がキャンセルされました。")
				return
			}

			// writer を必ず閉じる (defer を使用)
			defer func() {
				if errClose := writer.Close(); errClose != nil {
					// クローズエラーはログに残す程度で良い場合が多い
					log.Printf("保存ファイル(%s)のクローズエラー: %v", writer.URI().Path(), errClose)
				}
			}()

			// PNG形式で画像をエンコードしてファイルに書き込む
			errEncode := png.Encode(writer, currentImage)
			if errEncode != nil {
				errMsg := fmt.Sprintf("画像の保存(エンコード)に失敗しました。\n詳細: %v", errEncode)
				statusLabel.SetText("エラー: 画像の保存に失敗しました。")
				log.Printf("PNGエンコード失敗 (%s): %v", writer.URI().Path(), errEncode)
				dialog.ShowError(fmt.Errorf(errMsg), w)
				return
			}

			// 保存成功
			savedPath := writer.URI().Path()          // 保存されたファイルのフルパス
			savedFilename := filepath.Base(savedPath) // ファイル名のみ取得
			statusLabel.SetText(fmt.Sprintf("画像を保存しました: %s", savedFilename))
			log.Printf("QRコードを %s に保存しました", savedPath)
			// 成功通知 (任意)
			// dialog.ShowInformation("保存完了", fmt.Sprintf("画像を保存しました:\n%s", savedPath), w)

		}, w) // 親ウィンドウを指定

		// ダイアログの初期設定
		saveDialog.SetFileName(defaultFilename) // デフォルトファイル名
		// 保存可能なファイル形式フィルター (PNGのみに限定)
		saveDialog.SetFilter(storage.NewExtensionFileFilter([]string{".png"}))
		// // 初期表示ディレクトリを設定 (例: ユーザーのピクチャフォルダ)
		// picturesDir, err := os.UserPicturesDir()
		// if err == nil {
		// 	picturesURI := storage.NewFileURI(picturesDir)
		// 	listableURI, err := storage.ListerForURI(picturesURI)
		// 	if err == nil {
		// 		saveDialog.SetLocation(listableURI)
		// 	} else {
		//      log.Printf("ピクチャフォルダのURI取得エラー: %v", err)
		//  }
		// } else {
		//  log.Printf("ピクチャフォルダパス取得エラー: %v", err)
		// }

		// 保存ダイアログを表示
		saveDialog.Show()
	}

	// 保存ボタン
	saveBtn := widget.NewButton("画像を保存", saveAction)

	// --- UI レイアウト ---

	// 入力エリアと生成ボタンを縦に配置
	inputArea := container.NewVBox(widget.NewLabel("入力テキスト:"), inputEntry, generateBtn)

	// 画像表示エリア (中央寄せ)
	imageArea := container.NewCenter(qrImageCanvas)

	// コピーボタンと保存ボタンを横 (Gridで均等) に配置
	actionButtons := container.NewGridWithColumns(2, copyBtn, saveBtn)

	// 下部の操作ボタンとステータスラベルを縦に配置
	bottomArea := container.NewVBox(actionButtons, statusLabel)

	// 全体のレイアウトを Border コンテナで構成
	// Top: 入力エリア
	// Center: 画像エリア
	// Bottom: 操作ボタンとステータスエリア
	content := container.NewBorder(
		inputArea,  // Top
		bottomArea, // Bottom
		nil,        // Left
		nil,        // Right
		imageArea,  // Center
	)

	// ウィンドウにコンテンツを設定
	w.SetContent(content)

	// アプリケーションを実行
	w.ShowAndRun()
}
