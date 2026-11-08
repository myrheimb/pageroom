package main

import (
	"embed"
	"log"
	"net/http"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/myrheimb/pageroom/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	service, err := app.NewApp()
	if err != nil {
		log.Fatal(err)
	}
	files := http.NewServeMux()
	files.Handle("/site-file/", app.SiteFiles(service)) // serves site images to the editor, read-only
	files.Handle("/", application.AssetFileServerFS(assets))
	wails := application.New(application.Options{
		Name:         "Pageroom",
		Description:  "Write, organize and publish your website.",
		Icon:         icon,
		Services:     []application.Service{application.NewService(service)},
		Assets:       application.AssetOptions{Handler: files, Middleware: ownOriginOnly},
		MarshalError: app.FormatError,
		ShouldQuit:   func() bool { return app.ShouldQuit(service) },
		Mac:          application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		Linux:        application.LinuxOptions{ProgramName: "pageroom"},
	})
	window := wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Pageroom",
		Width:            1280,
		Height:           820,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: application.NewRGB(250, 249, 247),
		EnableFileDrop:   true,
		Mac:              application.MacWindow{TitleBar: application.MacTitleBarHiddenInset},
		URL:              "/",
	})
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !app.ShouldQuit(service) {
			e.Cancel()
		}
	})
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		wails.Event.Emit("files:dropped", e.Context().DroppedFiles())
	})
	if err := wails.Run(); err != nil {
		log.Fatal(err)
	}
}

// ownOriginOnly accepts runtime calls only from Pageroom's own page. The
// preview iframe runs theme JavaScript from another origin and must not call
// into the app, but outside server mode Wails accepts runtime calls from any
// origin. Requests with neither Origin nor Sec-Fetch-Site pass, because the
// app's own webview doesn't always send one. Do not widen this.
func ownOriginOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/wails/") && !fromOrigin(r, appOrigin()) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func appOrigin() string {
	if runtime.GOOS == "windows" {
		return "http://wails.localhost"
	}
	return "wails://localhost"
}

// fromOrigin follows Wails' own server-mode check: browsers send Origin on
// every cross-origin POST, and Sec-Fetch-Site covers any that do not.
func fromOrigin(r *http.Request, own string) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin == own
	}
	site := r.Header.Get("Sec-Fetch-Site")
	return site != "cross-site" && site != "same-site"
}
