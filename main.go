package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joshuarubin/go-sway"
	"github.com/joshuarubin/lifecycle"
)

var exec = []struct {
	Command     string
	AppID       string
	Class       string
	Title       string
	Workspace   string
	Layout      string
	Float       bool
	PostCommand string
	SleepBefore time.Duration
}{{
	Command:   "exec gtk-launch firefox",
	AppID:     "firefox",
	Workspace: "3",
}, {
	Command:   "exec gtk-launch slack",
	Class:     "Slack",
	Workspace: "2",
	Layout:    "tabbed",
}, {
	Command:   "exec gtk-launch discord",
	Class:     "discord",
	Workspace: "2",
}, {
	Command:   "exec gtk-launch signal-desktop",
	Class:     "Signal",
	Workspace: "2",
}, {
	Command:   "exec gtk-launch GridTracker",
	Class:     "GridTracker",
	Workspace: "2",
}, {
	Command:     "exec gtk-launch pavucontrol",
	AppID:       "pavucontrol",
	Workspace:   "5",
	PostCommand: `[workspace="5"] move workspace to DP-3`,
}, {
	Command:   "exec gtk-launch catia",
	Class:     "Catia",
	AppID:     "python3",
	Title:     "Catia",
	Workspace: "5",
}, {
	Command: `exec ray-daemon --session-root "/home/jrubin/Ray Sessions" --session Default`,
}, {
	Command:   "exec gtk-launch raysession",
	AppID:     "raysession",
	Workspace: "5",
}}

var socketPath string

func init() {
	flag.StringVar(&socketPath, "socketpath", "", "Use the specified socket path")
}

func main() {
	if err := run(); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(1)
	}
}

type handler struct {
	sway.EventHandler
	client      sway.Client
	unsubscribe func()
}

func (h *handler) ParentByConID(ctx context.Context, conID int64) *sway.Node {
	n, err := h.client.GetTree(ctx)
	if err != nil {
		log.Printf("error getting tree: %s", err)
		return nil
	}

	type qNode struct {
		Parent *sway.Node
		Node   *sway.Node
	}

	queue := []qNode{{Node: n}}
	for len(queue) > 0 {
		q := queue[0]
		queue = queue[1:]

		if q.Node == nil {
			continue
		}

		if q.Node.ID == conID {
			return q.Parent
		}

		for _, c := range q.Node.Nodes {
			queue = append(queue, qNode{
				Parent: q.Node,
				Node:   c,
			})
		}

		for _, c := range q.Node.FloatingNodes {
			queue = append(queue, qNode{
				Parent: q.Node,
				Node:   c,
			})
		}
	}
	return nil
}

func (h *handler) Window(ctx context.Context, e sway.WindowEvent) {
	if e.Change != "new" && e.Change != "title" {
		return
	}

	for i, s := range exec {
		var found bool
		if s.Class != "" && e.Container.WindowProperties != nil && e.Container.WindowProperties.Class == s.Class {
			found = true
		}

		if s.AppID != "" && e.Container.AppID != nil && *e.Container.AppID == s.AppID {
			if s.Title == "" {
				found = true
			} else if e.Container.Name == s.Title {
				found = true
			}
		}

		if !found {
			continue
		}

		delExecIndex(i)

		if s.Workspace != "" {
			cmd := fmt.Sprintf("[con_id=%d] move to workspace %s", e.Container.ID, s.Workspace)
			if _, err := h.client.RunCommand(ctx, cmd); err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
			}
		}

		if s.Layout != "" {
			if p := h.ParentByConID(ctx, e.Container.ID); p != nil {
				var cmd string
				if p.Type == "workspace" {
					cmd = fmt.Sprintf("[workspace=%q] layout %s", p.Name, s.Layout)
				} else {
					cmd = fmt.Sprintf("[con_id=%d] layout %s", p.ID, s.Layout)
				}
				if _, err := h.client.RunCommand(ctx, cmd); err != nil {
					fmt.Fprintf(os.Stderr, "%s\n", err)
				}
			}
		}

		if s.Float {
			cmd := fmt.Sprintf("[con_id=%d] floating enable", e.Container.ID)
			if _, err := h.client.RunCommand(ctx, cmd); err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
			}
		}

		if s.PostCommand != "" {
			if _, err := h.client.RunCommand(ctx, s.PostCommand); err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
			}
		}

		break
	}

	if len(exec) == 0 {
		h.unsubscribe()
	}
}

func run() error {
	ctx := lifecycle.New(context.Background())

	flag.Parse()

	client, err := sway.New(ctx, sway.WithSocketPath(socketPath))
	if err != nil {
		return err
	}

	sctx, unsubscribe := context.WithCancel(ctx)

	h := handler{
		EventHandler: sway.NoOpEventHandler(),
		client:       client,
		unsubscribe:  unsubscribe,
	}

	lifecycle.GoErr(ctx, func() error {
		return sway.Subscribe(sctx, &h, sway.EventTypeWindow)
	})

	for i, s := range exec {
		if s.Command != "" {
			if s.SleepBefore > 0 {
				time.Sleep(s.SleepBefore)
			}
			client.RunCommand(ctx, s.Command)
		}
		if s.AppID == "" && s.Title == "" && s.Class == "" {
			delExecIndex(i)
		}
	}

	return lifecycle.Wait(ctx)
}

func delExecIndex(i int) {
	if len(exec) <= 1 {
		exec = nil
	} else {
		exec = append(exec[:i], exec[i+1:]...)
	}
}
