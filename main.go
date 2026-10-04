package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"wami-auto-fisher/internal/fisher"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[fisher.Status]("status")
}

func main() {
	svc := NewService()

	app := application.New(application.Options{
		Name:        "WAMI Auto Fisher",
		Description: "Automates the WAMI active fishing minigame",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
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
