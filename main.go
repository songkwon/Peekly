package main

import (
	"context"
	"embed"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"peekly/internal/capture"
	"peekly/internal/systemproxy"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

type systemProxyController interface {
	Status() systemproxy.Status
	Enable(endpoint string) error
	Restore(allowPrompt bool) error
}

type Desktop struct {
	lifecycle   sync.Mutex
	engine      *capture.Engine
	systemProxy systemProxyController
	app         *application.App
	window      *application.WebviewWindow
}

type DesktopState struct {
	capture.State
	SystemProxy systemproxy.Status `json:"systemProxy"`
}

func (d *Desktop) Snapshot(revision uint64) DesktopState {
	return DesktopState{State: d.engine.Snapshot(revision), SystemProxy: d.systemProxy.Status()}
}

func (d *Desktop) Start(port int, automatic bool) (string, error) {
	d.lifecycle.Lock()
	defer d.lifecycle.Unlock()
	if port < 1024 || port > 65535 {
		return "", errors.New("请输入 1024 至 65535 之间的端口")
	}
	if automatic && d.engine.Network().Bind != "127.0.0.1" {
		return "", errors.New("请先将监听地址改回 127.0.0.1，再启用自动系统代理")
	}
	endpoint, err := d.engine.Start(port, automatic)
	if err != nil {
		return "", err
	}
	if !automatic {
		return endpoint, nil
	}
	if err := d.systemProxy.Enable(endpoint); err != nil {
		// Keep serving any traffic still directed here if rollback failed.
		if !d.systemProxy.Status().Pending {
			d.engine.Stop()
		}
		return "", err
	}
	return endpoint, nil
}

func (d *Desktop) Stop() error               { return d.stop(true) }
func (d *Desktop) RestoreSystemProxy() error { return d.stop(true) }

func (d *Desktop) stop(interactive bool) error {
	d.lifecycle.Lock()
	defer d.lifecycle.Unlock()
	if err := d.systemProxy.Restore(interactive); err != nil {
		return err
	}
	d.engine.Stop()
	return nil
}

func (d *Desktop) Clear()                                   { d.engine.Clear() }
func (d *Desktop) Detail(id uint64) (capture.Detail, error) { return d.engine.Detail(id) }
func (d *Desktop) Rules(rules []string) ([]string, error)   { return d.engine.SetRules(rules) }
func (d *Desktop) Platform() string                         { return runtime.GOOS }
func (d *Desktop) Copy(text string) bool                    { return d.app.Clipboard.SetText(text) }

func (d *Desktop) Compose(ctx context.Context, input capture.RequestInput) (capture.Detail, error) {
	return d.engine.Compose(ctx, input)
}
func (d *Desktop) Repeat(ctx context.Context, id uint64) (capture.Detail, error) {
	return d.engine.Repeat(ctx, id)
}
func (d *Desktop) TrafficRules() []capture.Rule { return d.engine.TrafficRules() }
func (d *Desktop) SaveTrafficRules(rules []capture.Rule) error {
	return d.engine.SetTrafficRules(rules)
}
func (d *Desktop) Network() capture.NetworkOptions { return d.engine.Network() }
func (d *Desktop) SaveNetwork(settings capture.NetworkOptions) error {
	return d.engine.SetNetwork(settings)
}
func (d *Desktop) LocalAddresses() []string { return capture.LocalAddresses() }

func (d *Desktop) MatchTraffic(filters []capture.Filter, anyMatch bool) ([]uint64, error) {
	return d.engine.Matches(filters, anyMatch)
}

func (d *Desktop) SaveBody(id uint64, request bool) (bool, error) {
	detail, err := d.engine.Detail(id)
	if err != nil {
		return false, err
	}
	suffix, body := "response", detail.ResponseBody
	if request {
		suffix, body = "request", detail.RequestBody
	}
	if len(body) == 0 {
		return false, errors.New("没有可保存的正文")
	}
	return d.save("peekly-"+suffix+".bin", "Binary", "*.bin", body)
}

func (d *Desktop) ResolveBreakpoint(id uint64, action capture.BreakpointAction) error {
	return d.engine.ResolveBreakpoint(id, action)
}

func (d *Desktop) ExportHAR(ids []uint64, redact bool) (bool, error) {
	data, err := d.engine.ExportHAR(ids, redact)
	if err != nil {
		return false, err
	}
	return d.save("Peekly-"+time.Now().Format("20060102-150405")+".har", "HAR", "*.har", data)
}

func (d *Desktop) ImportHAR() (int, error) {
	path, err := d.app.Dialog.OpenFile().AddFilter("HAR", "*.har").AttachToWindow(d.window).PromptForSingleSelection()
	if path == "" || err != nil {
		return 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	const maxImport = 32 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(file, maxImport+1))
	if err != nil {
		return 0, err
	}
	if len(data) > maxImport {
		return 0, errors.New("HAR 文件超过 32 MiB")
	}
	return d.engine.ImportHAR(data)
}

func (d *Desktop) Certificate() (capture.Certificate, error) { return d.engine.Certificate() }

func (d *Desktop) ExportCertificate() (bool, error) {
	cert, err := d.engine.Certificate()
	if err != nil {
		return false, err
	}
	return d.save("Peekly-Root-CA.cer", "X.509", "*.cer", []byte(cert.PEM))
}

func (d *Desktop) save(name, kind, pattern string, data []byte) (bool, error) {
	path, err := d.app.Dialog.SaveFile().SetFilename(name).AddFilter(kind, pattern).
		AttachToWindow(d.window).PromptForSingleSelection()
	if path == "" || err != nil {
		return false, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return false, err
	}
	_, writeErr := file.Write(data)
	err = errors.Join(writeErr, file.Close())
	return err == nil, err
}

func main() {
	dir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	config := filepath.Join(dir, "Peekly")
	desktop := &Desktop{engine: capture.New(config), systemProxy: systemproxy.New(config)}
	allowClose := func() bool {
		if err := desktop.stop(true); err != nil {
			desktop.app.Dialog.Error().SetTitle("无法恢复系统代理").SetMessage(err.Error()).Show()
			return false
		}
		return true
	}
	app := application.New(application.Options{
		Name: "Peekly", Description: "Peekly HTTP debugging proxy", Icon: icon,
		Services:   []application.Service{application.NewService(desktop)},
		Assets:     application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:        application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		ShouldQuit: allowClose,
		OnShutdown: func() {
			if err := desktop.stop(false); err != nil {
				log.Print(err)
			}
		},
		SingleInstance: &application.SingleInstanceOptions{UniqueID: "app.peekly.desktop", OnSecondInstanceLaunch: func(application.SecondInstanceData) {
			if window := desktop.window; window != nil {
				window.Show()
				window.Focus()
			}
		}},
	})
	desktop.app = app
	desktop.window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "peekly", Title: "Peekly", URL: "/",
		Width: 1400, Height: 865, MinWidth: 950, MinHeight: 600,
		BackgroundColour: application.NewRGB(255, 255, 255),
		Mac:              application.MacWindow{TitleBar: application.MacTitleBar{HideTitle: true}},
	})
	desktop.window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if !allowClose() {
			event.Cancel()
		}
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go func() {
			desktop.lifecycle.Lock()
			defer desktop.lifecycle.Unlock()
			if !desktop.engine.Snapshot(0).Running {
				if err := desktop.systemProxy.Restore(false); err != nil {
					log.Print(err)
				}
			}
		}()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
