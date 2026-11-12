package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type Snapshot struct {
	Revision     uint64
	Active       string // Empty means application catalog.
	Status       string
	Applications []Manifest
	Application  AppSnapshot
}
type Host struct {
	mu        sync.Mutex
	root      context.Context
	registry  *Registry
	instances map[string]Application
	active    string
	cancel    context.CancelFunc
	revision  uint64
	status    string
	closed    bool
}

func NewHost(ctx context.Context, r *Registry) *Host {
	return &Host{root: ctx, registry: r, instances: make(map[string]Application), revision: 1}
}
func (h *Host) leave() {
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	if app := h.instances[h.active]; app != nil {
		app.Stop()
	}
	h.active = ""
}
func (h *Host) Open(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ErrClosed
	}
	if err := h.root.Err(); err != nil {
		return err
	}
	index, ok := h.registry.byID[id]
	if !ok {
		return fmt.Errorf("未注册的应用: %s", id)
	}
	if h.active == id {
		return nil
	}
	h.leave()
	h.revision++
	h.status = ""
	entry := h.registry.entries[index]
	app := h.instances[id]
	var err error
	if app == nil {
		app, err = entry.New()
	}
	if err == nil && app == nil {
		err = errors.New("应用工厂返回空实例")
	}
	ctx, cancel := context.WithCancel(h.root)
	if err == nil {
		err = app.Start(ctx)
	}
	if err != nil {
		cancel()
		if app != nil {
			app.Stop()
			_ = app.Close()
		}
		delete(h.instances, id)
		h.status = "打开" + entry.Manifest.Title + "失败：" + err.Error()
		return errors.New(h.status)
	}
	h.instances[id] = app
	h.active = id
	h.cancel = cancel
	return nil
}
func (h *Host) Back() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.leave()
	h.status = ""
	h.revision++
}
func (h *Host) Dispatch(e Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ErrClosed
	}
	if h.active == "" {
		return ErrInactive
	}
	if e.AppID != h.active {
		return ErrStaleEvent
	}
	err := h.instances[h.active].Handle(e)
	h.status = ""
	if err != nil {
		h.status = err.Error()
	}
	h.revision++
	return err
}
func (h *Host) Snapshot() Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := Snapshot{Revision: h.revision, Active: h.active, Status: h.status, Applications: h.registry.Manifests()}
	if app := h.instances[h.active]; app != nil {
		s.Application = app.Snapshot()
		if h.status != "" {
			s.Application.Status = h.status
		}
	}
	return s
}
func (h *Host) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	h.closed = true
	h.leave()
	var result error
	for _, e := range h.registry.entries {
		if app := h.instances[e.Manifest.ID]; app != nil {
			result = errors.Join(result, app.Close())
		}
	}
	return result
}
