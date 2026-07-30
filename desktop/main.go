package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:              "Oberth",
		Width:              1440,
		Height:             900,
		MinWidth:           840,
		MinHeight:          640,
		AssetServer:        &assetserver.Options{Assets: assets, Middleware: app.assetMiddleware},
		BackgroundColour:   &options.RGBA{R: 10, G: 13, B: 18, A: 1},
		OnStartup:          app.startup,
		OnShutdown:         app.shutdown,
		Bind:               []interface{}{app},
		Windows:            &windows.Options{WebviewIsTransparent: false, WindowIsTranslucent: false},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "oberth-public-alpha"},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
