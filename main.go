package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"wami-auto-fisher/internal/fisher"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func init() {
	application.RegisterEvent[fisher.Status]("status")
}

func main() {
	svc := NewService()

	var app *application.App
	app = application.New(application.Options{
		Name:        "WAMI Auto Fisher",
		Description: "Automates the WAMI active fishing minigame",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		// Refuse a second instance; bring the running window forward instead.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "io.github.wamiautofisher.WamiAutoFisher",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if app == nil {
					return
				}
				if w := app.Window.Current(); w != nil {
					w.Show()
					w.Focus()
				}
			},
		},
	})
	svc.SetApp(app)

	if err := svc.Startup(); err != nil {
		log.Println("startup:", err)
	}

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "WAMI Auto Fisher",
		Width:            1100,
		Height:           720,
		BackgroundColour: application.NewRGB(10, 10, 12),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
