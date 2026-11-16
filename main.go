package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "Crazy Download Manager",
		Width:     1100,
		Height:    680,
		MinWidth:  760,
		MinHeight: 440,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 243, G: 245, B: 248, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnShutdown:       app.shutdown,
		// A second launch (e.g. `CrazyDownloadManager /d URL`) hands its
		// arguments to the running instance instead of opening a new window.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "6f1c0f3e-crazy-download-manager",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		DragAndDrop: &options.DragAndDrop{DisableWebViewDrop: false},
		Bind:        []interface{}{app},
		Windows: &windows.Options{
			Theme: windows.SystemDefault,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarDefault(),
			About: &mac.AboutInfo{
				Title:   "Crazy Download Manager",
				Message: "Open-source download accelerator. Version " + Version,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
