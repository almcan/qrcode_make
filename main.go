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
		return fmt.Errorf("PNGエンコード失敗: %w", err)
	}

	// ファイルへの書き込みを完了させるために一度閉じる（deferでも閉じるが、ここでエラーを確認）
	if err := file.Close(); err != nil {
		return fmt.Errorf("一時ファイルへの書き込み完了(クローズ)失敗 (%s): %w", tmpFilePath, err)
	}

	// --- PowerShell コマンドの準備 ---
	psEscapedPath := filepath.ToSlash(tmpFilePath)
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
	log.Printf("実行したPowerShellコマンド: powershell -Command \"%s\"", psCmd)
	log.Printf("PowerShellからの出力:\n%s", string(output))
	if err != nil {
		return fmt.Errorf("PowerShell実行失敗: %w\n出力: %s", err, string(output))
	}
	log.Println("PowerShellによるクリップボードへの画像コピー成功")
	return nil
}

// copyImageToClipboardOther は Windows 以外の OS 用のプレースホルダー関数です。
func copyImageToClipboardOther(img image.Image) error {
	return fmt.Errorf("クリップボードへの画像コピーはこのOSでは現在サポートされていません (%s)", runtime.GOOS)
}

// copyImageToClipboard は OS を判定し、適切なコピー関数を呼び出すラッパーです。
func copyImageToClipboard(img image.Image) error {
	switch runtime.GOOS {
	case "windows":
		return copyImageToClipboardWindows(img)
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

	// --- UI コンポーネントと関連変数 ---

	// 入力用テキストエリア
	inputEntry := widget.NewMultiLineEntry()
	// プレースホルダーにショートカットキー情報を追加
	inputEntry.SetPlaceHolder("QRコードにしたいテキストを入力してください...\n(例: https://example.com)")
	inputEntry.Wrapping = fyne.TextWrapWord // 自動折り返し
	inputEntry.SetMinRowsVisible(4)         // 最小表示行数

	// QRコード画像表示用キャンバス
	qrImageCanvas := canvas.NewImageFromImage(nil)   // 初期状態は画像なし
	qrImageCanvas.FillMode = canvas.ImageFillContain // アスペクト比を維持して表示エリアに収める
	qrImageCanvas.SetMinSize(fyne.NewSize(256, 256)) // 画像表示エリアの最小サイズ確保

	// ステータス表示用ラベル
	// 初期メッセージを更新
	statusLabel := widget.NewLabel("テキストを入力して「生成」ボタンを押すか、Ctrl+Enterを押してください。")
	statusLabel.Wrapping = fyne.TextWrapWord     // 長いメッセージは折り返す
	statusLabel.Alignment = fyne.TextAlignCenter // 中央揃え

	// 生成された image.Image を保持する変数 (コピー/保存用)
	var currentImage image.Image

	// --- QRコード生成処理 (共通関数化) ---
	// このクロージャは inputEntry, currentImage, qrImageCanvas, statusLabel, w をキャプチャする
	generateQRCode := func() {
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

	// --- イベントハンドラの設定 ---

	// ★★★ 入力エリアで Submit (通常 Ctrl+Enter) された時の処理を設定 ★★★
	inputEntry.OnSubmitted = func(text string) {
		log.Println("入力エリア OnSubmitted 発火")
		generateQRCode() // 共通の生成関数を呼び出す
	}

	// --- ボタンの作成 ---

	// QRコード生成ボタン (共通の生成関数を呼び出すように変更)
	generateBtn := widget.NewButton("QRコード生成", generateQRCode)

	// 画像をクリップボードにコピーボタン
	copyBtn := widget.NewButton("画像をコピー", func() {
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
			dialog.ShowError(fmt.Errorf(errMsg), w)
		} else {
			statusLabel.SetText("画像をクリップボードにコピーしました。")
			log.Println("QRコード画像をクリップボードにコピーしました。")
		}
	})

	// 画像をファイルに保存ボタン
	saveBtn := widget.NewButton("画像を保存", func() {
		if currentImage == nil {
			statusLabel.SetText("エラー: 保存するQRコードが生成されていません。")
			dialog.ShowInformation("情報", "先にQRコードを生成してください。", w)
			return
		}
		timestamp := time.Now().Format("20060102_150405") // YYYYMMDD_HHMMSS形式
		defaultFilename := "qrcode_" + timestamp + ".png"
		saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil {
				statusLabel.SetText("エラー: 保存場所の選択中にエラーが発生しました。")
				log.Printf("ファイル保存ダイアログエラー: %v", err)
				dialog.ShowError(fmt.Errorf("保存場所の選択エラー: %w", err), w)
				return
			}
			if writer == nil {
				statusLabel.SetText("保存がキャンセルされました。")
				log.Println("ファイル保存がキャンセルされました。")
				return
			}
			defer func() {
				if errClose := writer.Close(); errClose != nil {
					log.Printf("保存ファイル(%s)のクローズエラー: %v", writer.URI().Path(), errClose)
				}
			}()
			errEncode := png.Encode(writer, currentImage)
			if errEncode != nil {
				errMsg := fmt.Sprintf("画像の保存(エンコード)に失敗しました。\n詳細: %v", errEncode)
				statusLabel.SetText("エラー: 画像の保存に失敗しました。")
				log.Printf("PNGエンコード失敗 (%s): %v", writer.URI().Path(), errEncode)
				dialog.ShowError(fmt.Errorf(errMsg), w)
				return
			}
			savedPath := writer.URI().Path()
			savedFilename := filepath.Base(savedPath)
			statusLabel.SetText(fmt.Sprintf("画像を保存しました: %s", savedFilename))
			log.Printf("QRコードを %s に保存しました", savedPath)
		}, w)
		saveDialog.SetFileName(defaultFilename)
		saveDialog.SetFilter(storage.NewExtensionFileFilter([]string{".png"}))
		saveDialog.Show()
	})

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
