package connectivity

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

type bluetoothDiscovery struct {
	cancel context.CancelFunc
	done   chan struct{}
	err    error // Published by closing done.
}

// StopDiscovery is nonblocking so leaving Settings never waits on a child process.
// The last results remain available when the page is opened again.
func (s *Service) StopDiscovery() {
	s.btDiscoveryMu.Lock()
	defer s.btDiscoveryMu.Unlock()
	if s.btDiscovery != nil {
		s.btDiscovery.cancel()
	}
}
func (s *Service) stopDiscovery(ctx context.Context) error {
	s.StopDiscovery()
	s.btDiscoveryMu.Lock()
	previous := s.btDiscovery
	s.btDiscoveryMu.Unlock()
	if previous != nil {
		select {
		case <-previous.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
func (s *Service) Close() error {
	s.btDiscoveryMu.Lock()
	s.btClosed = true
	scan := s.btDiscovery
	if scan != nil {
		scan.cancel()
	}
	s.btDiscoveryMu.Unlock()
	if scan != nil {
		<-scan.done
	}
	return nil
}

// Keep the discovery owner alive after the initial scan window. BlueZ removes
// unpaired Device objects when the last owner disconnects, making cached rows
// impossible to pair. Settings owns and cancels this session on Stop/Close.
func (s *Service) scanBluetooth(ctx context.Context, window time.Duration) error {
	if err := s.stopDiscovery(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	scanCtx, cancel := context.WithCancel(context.Background())
	// Transfer ownership after the initial task completes; cancellation during the
	// initial scan must still tear down the child immediately.
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	cmd := exec.CommandContext(scanCtx, "bluetoothctl")
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TERM=dumb")
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		cancel()
		return err
	}
	cmd.Stderr = cmd.Stdout
	scan := &bluetoothDiscovery{cancel: cancel, done: make(chan struct{})}
	s.btDiscoveryMu.Lock()
	if s.btClosed || ctx.Err() != nil {
		s.btDiscoveryMu.Unlock()
		input.Close()
		output.Close()
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("蓝牙服务已关闭")
	}
	s.btDiscovery = scan
	s.btDiscoveryMu.Unlock()
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		cancel()
		scan.err = errors.New("无法启动蓝牙扫描")
		close(scan.done)
		return scan.err
	}
	s.btMu.Lock()
	for address, d := range s.btDevices {
		if !d.Paired && !d.Trusted {
			delete(s.btDevices, address)
		} else {
			d.Available = false
			s.btDevices[address] = d
		}
	}
	if s.btDevices == nil {
		s.btDevices = map[string]Device{}
	}
	s.btMu.Unlock()
	go func() {
		defer close(scan.done)
		defer cancel()
		defer input.Close()
		defer func() {
			s.btMu.Lock()
			defer s.btMu.Unlock()
			for address, d := range s.btDevices {
				d.Available = false
				s.btDevices[address] = d
			}
		}()
		reader := bufio.NewScanner(output)
		reader.Buffer(make([]byte, 4096), 64<<10)
		failed := false
		discovering := false
		for reader.Scan() {
			line := ansi.ReplaceAllString(reader.Text(), "")
			if strings.Contains(line, "Discovery started") || (strings.Contains(line, "Controller ") && strings.Contains(line, "Discovering: yes")) {
				discovering = true
			}
			if strings.Contains(line, "Failed") || strings.Contains(line, "No default controller") {
				failed = true
			}
			s.btMu.Lock()
			if s.btDevices == nil {
				s.btDevices = map[string]Device{}
			}
			updateScanDevices(s.btDevices, line, discovering)
			s.btMu.Unlock()
		}
		readErr := reader.Err()
		if readErr != nil {
			cancel()
		}
		err := cmd.Wait()
		if scanCtx.Err() != nil {
			scan.err = scanCtx.Err()
		} else if err != nil || readErr != nil || failed {
			scan.err = errors.New("蓝牙扫描失败，请检查蓝牙电源和 BlueZ 服务")
		}
	}()
	// A live stdin keeps bluetoothctl in interactive mode. In BlueZ 5.77,
	// --timeout 0 exits before discovery starts instead of waiting indefinitely.
	if _, err = input.Write([]byte("scan on\n")); err != nil {
		cancel()
		<-scan.done
		return errors.New("无法开始蓝牙扫描")
	}
	timer := time.NewTimer(window)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	case <-scan.done:
		return scan.err
	case <-timer.C:
		return nil
	}
}
