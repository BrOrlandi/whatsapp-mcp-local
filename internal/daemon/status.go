package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Status is the gateway in one line, for the tray: a colour, what it is
// doing, and a detail.
type Status struct {
	// Tone is ok (connected and receiving), warn (reconnecting, paused,
	// receiving history) or fail (WhatsApp disconnected or sync stopped).
	Tone   string `json:"tone"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	// Sync is the supervisor's state, for notifications to act on.
	Sync string `json:"sync"`
	// MCPDown is set when the MCP is not answering because its port is
	// taken.
	MCPDown bool `json:"mcp_down,omitempty"`
}

// Status reports the gateway now.
func (d *Daemon) Status() Status { return d.status(context.Background()) }

func (d *Daemon) status(ctx context.Context) Status {
	sync := d.sup.Status()
	pairing := d.sup.Pairing()
	st := Status{Sync: sync.State}
	switch sync.State {
	case "connected":
		st.Tone, st.Title = "ok", "Conectado"
	case "paused":
		st.Tone, st.Title = "warn", "Pausado por instantes"
		if sync.PausedFor == "pairing" {
			st.Title = "Conectando o WhatsApp"
		}
	case "starting":
		st.Tone, st.Title = "warn", "Carregando"
	case "reconnecting":
		st.Tone, st.Title = "warn", "Reconectando"
	case "not_paired":
		st.Tone, st.Title = "fail", "WhatsApp não conectado"
	case "logged_out":
		st.Tone, st.Title = "fail", "Desconectado pelo celular"
	default:
		st.Tone, st.Title = "fail", "Sincronização parada"
	}
	arriving := pairing.State == "syncing" || (!sync.History.LastAt.IsZero() && time.Since(sync.History.LastAt) < 90*time.Second)
	switch {
	case arriving && st.Tone == "ok":
		st.Tone, st.Detail = "warn", "Recebendo o histórico"
	case st.Tone == "ok":
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if a, err := d.idx.Activity(ctx, time.Now()); err == nil {
			st.Detail = messagesLastHour(a.LastHour)
		}
	}
	if d.PortProblem() != nil {
		st.MCPDown = true
		if st.Tone == "ok" {
			st.Tone = "warn"
		}
		st.Detail = fmt.Sprintf("MCP fora do ar: a porta %d está ocupada", d.Port())
	}
	return st
}

func messagesLastHour(n int64) string {
	switch n {
	case 0:
		return "Nenhuma mensagem na última hora"
	case 1:
		return "1 mensagem na última hora"
	}
	return fmt.Sprintf("%d mensagens na última hora", n)
}

// Watch delivers the status every time it changes, starting with the current
// one. Call stop when done.
func (d *Daemon) Watch() (updates <-chan Status, stop func()) {
	c := make(chan Status, 1)
	c <- d.Status()
	id := d.watch.add(c)
	return c, func() { d.watch.remove(id) }
}

// watchLoop recomputes the status when sync changes, and every half minute
// for the message count.
func (d *Daemon) watchLoop(ctx context.Context) {
	defer close(d.watchDone)
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	// Downloaded media past its retention goes a minute after start, then
	// every six hours.
	sweep := time.NewTimer(time.Minute)
	defer sweep.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			_, _, _ = d.server.SweepMedia(ctx)
			sweep.Reset(6 * time.Hour)
			continue
		case <-d.sup.Changes():
		case <-tick.C:
		}
		d.watch.publish(d.status(ctx))
	}
}

type watchers struct {
	mu   sync.Mutex
	next int
	subs map[int]chan Status
	last Status
}

func (w *watchers) add(c chan Status) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.subs == nil {
		w.subs = map[int]chan Status{}
	}
	w.next++
	w.subs[w.next] = c
	return w.next
}

func (w *watchers) remove(id int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.subs, id)
}

// publish hands the newest status to every watcher, replacing one it has
// not read yet: a watcher only ever needs the latest.
func (w *watchers) publish(s Status) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if s == w.last {
		return
	}
	w.last = s
	for _, c := range w.subs {
		select {
		case <-c:
		default:
		}
		c <- s
	}
}
