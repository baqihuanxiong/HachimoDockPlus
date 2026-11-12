// Package terminal provides a generic diagnostic frontend. App-specific aliases
// are supplied by the composition root, never hard-coded in the frontend.
package terminal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"hachimodockplus/internal/core"
)

type Commands map[string]map[string]string

func Run(ctx context.Context, h *core.Host, in io.Reader, out io.Writer, commands Commands) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	fmt.Fprintln(out, "HachimoDock\n/apps 列表；/open <id> 打开；/back 返回；/event <action> [text]；/quit 退出")
	printApps := func() {
		for _, m := range h.Snapshot().Applications {
			fmt.Fprintf(out, "%s — %s：%s\n", m.ID, m.Title, m.Description)
		}
	}
	printApps()
	lines := make(chan string)
	go func() {
		defer close(lines)
		scan := bufio.NewScanner(in)
		scan.Buffer(make([]byte, 4096), 64<<10)
		for scan.Scan() {
			select {
			case lines <- scan.Text():
			case <-ctx.Done():
				return
			}
		}
	}()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	var revision, appRevision uint64
	for {
		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			cmd, arg, _ := strings.Cut(strings.TrimSpace(line), " ")
			var err error
			switch cmd {
			case "/quit":
				return nil
			case "/apps":
				printApps()
			case "/open":
				err = h.Open(strings.TrimSpace(arg))
			case "/back":
				h.Back()
			default:
				active := h.Snapshot().Active
				action, text := "", line
				if cmd == "/event" {
					action, text, _ = strings.Cut(arg, " ")
				} else {
					alias := ""
					if strings.HasPrefix(cmd, "/") {
						alias = cmd
					}
					action = commands[active][alias]
				}
				if action == "" {
					fmt.Fprintln(out, "请先打开应用，并使用其支持的操作")
					continue
				}
				err = h.Dispatch(core.Event{AppID: active, Action: action, Text: text})
			}
			if err != nil {
				fmt.Fprintln(out, err)
			}
		case <-tick.C:
			s := h.Snapshot()
			if s.Revision == revision && s.Application.Revision == appRevision {
				continue
			}
			revision, appRevision = s.Revision, s.Application.Revision
			if s.Active == "" {
				if s.Status != "" {
					fmt.Fprintln(out, s.Status)
				}
				continue
			}
			fmt.Fprintln(out, "["+s.Active+"] "+s.Application.Status)
			if s.Application.Text != "" {
				fmt.Fprintln(out, s.Application.Text)
			}
		}
	}
}
