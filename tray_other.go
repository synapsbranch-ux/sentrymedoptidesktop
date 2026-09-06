//go:build !windows

package main

func startTray(*DesktopBridge) {}
func stopTray()                {}
func trayAvailable() bool      { return false }
