package main

import (
	"bytes"
	"image/png"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/skip2/go-qrcode"
)

func main() {
	a := app.New()
	w := a.NewWindow("QRコード生成ツール")

	input := widget.NewEntry()
	input.SetPlaceHolder("QRコードの内容を入力してください")

	img := canvas.NewImageFromImage(nil)
	img.FillMode = canvas.ImageFillOriginal

	input.OnSubmitted = func(s string) {
		pngData, err := qrcode.Encode(s, qrcode.Medium, 256)
		if err != nil {
			return
		}
		imgData, err := png.Decode(bytes.NewReader(pngData))
		if err != nil {
			return
		}
		img.Image = imgData
		img.Refresh()
	}

	content := container.NewVBox(input, img)
	w.SetContent(content)

	// ウィンドウの初期サイズを設定します (例: 幅 400, 高さ 400)
	w.Resize(fyne.NewSize(400, 400))

	w.ShowAndRun()
}
