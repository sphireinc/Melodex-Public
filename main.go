package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist logo.png logo_cropped.png
var assets embed.FS

func init() {
	application.RegisterEvent[AppState]("melodex:state")
	application.RegisterEvent[PlaybackState]("melodex:playback")
	application.RegisterEvent[JobProgressEvent]("melodex:job-progress")
}

func main() {
	configureLogging()
	defer closeLogging()
	log.Printf("launch: Melodex %s", buildSummary())

	wailsApp := application.New(application.Options{
		Name:        "Melodex",
		Description: "A local-first music library manager",
		Assets: application.AssetOptions{
			Handler: newAssetHandler(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	service := NewApp()
	service.wailsApp = wailsApp
	service.window = wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Melodex",
		Width:            1480,
		Height:           960,
		MinWidth:         1200,
		MinHeight:        760,
		BackgroundColour: application.NewRGB(9, 14, 25),
		URL:              "/",
	})
	wailsApp.RegisterService(application.NewService(service))

	err := wailsApp.Run()
	if err != nil {
		log.Fatal(err)
	}
}
