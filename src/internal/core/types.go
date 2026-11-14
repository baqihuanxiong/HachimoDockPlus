// Package core defines application contracts without application, UI or platform dependencies.
package core

import (
	"context"
	"errors"
)

var (
	ErrClosed        = errors.New("应用框架已关闭")
	ErrInactive      = errors.New("应用尚未进入")
	ErrUnknownAction = errors.New("应用不支持此操作")
	ErrStaleEvent    = errors.New("忽略已离开页面的操作")
)

const (
	ActionOpen = "shell.open"
	ActionBack = "shell.back"
)

type Capability string

const (
	Network    Capability = "network"
	Microphone Capability = "microphone"
	Speaker    Capability = "speaker"
)

// Capabilities document optional services; this is not a permission sandbox.
type Manifest struct {
	ID, Title, Description, Icon string
	IconImage                    string // Optional PNG path relative to the configuration file; Icon remains the fallback.
	Capabilities                 []Capability
}

// AppID scopes events so queued input cannot reach a newly opened application.
type Event struct{ AppID, Action, Text string }

// Data must be an independent immutable snapshot, owned by the application.
// Status/Text allow diagnostic frontends without application-specific imports.
type AppSnapshot struct {
	Revision     uint64
	Status, Text string
	Data         any
}

// Start/Stop may repeat. Stop cancels foreground work promptly; Close joins all
// workers and releases retained resources. Host serializes lifecycle calls.
// Start must not perform network requests on the UI thread: use a worker.
type Application interface {
	Start(context.Context) error
	Stop()
	Handle(Event) error
	Snapshot() AppSnapshot
	Close() error
}

// BackgroundReporter optionally exposes live work owned outside the foreground
// context. It must be nonblocking; cached instances alone do not imply running.
type BackgroundReporter interface{ BackgroundRunning() bool }
