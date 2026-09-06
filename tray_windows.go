//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"time"

	"github.com/getlantern/systray"
)

//go:embed build/appicon.ico
var trayIcon []byte

func startTray(bridge *DesktopBridge) {
	go systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("SentryMed Opti")
		systray.SetTooltip("SentryMed Opti clinic server")
		open := systray.AddMenuItem("Open SentryMed", "Open the desktop window")
		status := systray.AddMenuItem("Server: Running", "")
		status.Disable()
		clients := systray.AddMenuItem("Connected users: 0", "")
		clients.Disable()
		systray.AddSeparator()
		mobile := systray.AddMenuItem("Open mobile access", "Open the local PWA address")
		backup := systray.AddMenuItem("Backup now", "Create a verified database snapshot")
		systray.AddSeparator()
		stop := systray.AddMenuItem("Stop server", "Stop LAN access without exiting")
		exit := systray.AddMenuItem("Exit", "Stop server and close SentryMed")
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-open.ClickedCh:
					bridge.Show()
				case <-mobile.ClickedCh:
					bridge.OpenMobileAccess()
				case <-backup.ClickedCh:
					_ = bridge.BackupNow()
				case <-stop.ClickedCh:
					_ = bridge.StopServer()
					status.SetTitle("Server: Stopped")
					stop.Disable()
				case <-exit.ClickedCh:
					bridge.Exit()
					return
				case <-ticker.C:
					clients.SetTitle(fmt.Sprintf("Connected users: %d", bridge.ConnectedClients()))
				}
			}
		}()
	}, func() {})
}

func stopTray() { systray.Quit() }
