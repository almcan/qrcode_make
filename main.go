package main

import (
	"bytes"
	"image/png"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/skip2/go-qrcode"
)

func main() {
	a := app.New()

	// "icon.png" ファイルを読み込む
	iconResource, err := fyne.LoadResourceFromPath("icon.png")
	if err != nil {
		// エラーが発生した場合の処理
		log.Println("アイコンファイルの読み込みに失敗しました:", err)
		// ここでデフォルトアイコンを設定することもできます
		// a.SetIcon(theme.FyneLogo()) // 例: Fyneのデフォルトロゴ
	} else {
		// 読み込みが成功したらアイコンを設定
		a.SetIcon(iconResource)
	}

	w := a.NewWindow("QRコード生成ツール")

	input := widget.NewEntry()
	input.SetPlaceHolder("QRコードの内容を入力してください")

	img := canvas.NewImageFromImage(nil)
	img.FillMode = canvas.ImageFillOriginal

	// statusラベルを定義
	status := widget.NewLabel("")

	input.OnSubmitted = func(s string) {
		pngData, err := qrcode.Encode(s, qrcode.Medium, 256)
		if err != nil {
			status.SetText("QRコードの生成に失敗しました")
			return
		}
		imgData, err := png.Decode(bytes.NewReader(pngData))
		if err != nil {
			status.SetText("QRコードの生成に失敗しました")
			return
		}
		img.Image = imgData
		img.Refresh()
		status.SetText("QRコードを生成しました")
	}

	saveBtn := widget.NewButton("QRコードを保存", func() {
		if img.Image == nil {
			status.SetText("QRコードが生成されていません")
			return
		}

		// 現在の時刻を取得してファイル名を生成
		timestamp := time.Now().Format("20060102_150405")
		filename := "qrcode_" + timestamp + ".png"

		saveDialog := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
			if err != nil || uc == nil {
				status.SetText("保存に失敗しました")
				return
			}
			defer uc.Close()
			// PNGエンコード処理
			if err := png.Encode(uc, img.Image); err != nil {
				status.SetText("PNGエンコードに失敗しました")
				return
			}
			status.SetText("QRコードを保存しました")
		}, w)

		saveDialog.SetFileName(filename)
		saveDialog.Show()
	})

	content := container.NewVBox(
		input,
		img,
		saveBtn,
		status,
	)

	w.SetContent(content)
	w.Resize(fyne.NewSize(600, 500))
	w.ShowAndRun()
}
