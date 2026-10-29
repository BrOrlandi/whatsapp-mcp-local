package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/transfer"
)

// WacliVersionHeader carries the sending side's wacli version: both sides
// must run the same one.
const WacliVersionHeader = "X-WhatsApp-MCP-Wacli"

// SizeHeader is about how many bytes of files a transfer carries, for the
// progress bar.
const SizeHeader = "X-WhatsApp-MCP-Transfer-Size"

// TransferInfo is what a server tells the app before a transfer.
type TransferInfo struct {
	Available bool   `json:"available"`
	Wacli     string `json:"wacli"`
	Paired    bool   `json:"paired"`
	Phone     string `json:"phone,omitempty"`
}

// transfers moves the main number's data to and from the app of a paired
// computer (internal/transfer). Only that app's own requests reach it, over
// its session, never the loopback port. Either way, the gateway restarts at
// the end (Config.Restart), with the data put in place while it is closed.
type transfers struct {
	d *Daemon

	once  sync.Once
	wacli string

	mu   sync.Mutex
	busy bool
	// hold, while the data is going to the app, keeps sync stopped until
	// the app says the data arrived (true) or the transfer failed (false).
	hold chan bool
}

func (t *transfers) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/transferencia", t.info)
	mux.HandleFunc("POST /api/transferencia/receber", t.receive)
	mux.HandleFunc("POST /api/transferencia/enviar", t.send)
	mux.HandleFunc("POST /api/transferencia/concluir", t.conclude)
	mux.HandleFunc("POST /api/transferencia/cancelar", t.cancel)
	return mux
}

func (t *transfers) wacliVersion() string {
	t.once.Do(func() { t.wacli = transfer.WacliVersion(context.Background(), t.d.cfg.WacliBin) })
	return t.wacli
}

func (t *transfers) state(ctx context.Context) TransferInfo {
	a := t.d.primary
	info := TransferInfo{Available: t.d.cfg.Restart != nil, Wacli: t.wacliVersion()}
	var status struct {
		Authenticated bool `json:"authenticated"`
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if a.cli.Decode(cctx, &status, "--read-only", "auth", "status") == nil {
		info.Paired = status.Authenticated
	}
	if info.Paired {
		info.Phone = a.Info(ctx).Phone
	}
	return info
}

func (t *transfers) info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, t.state(r.Context()))
}

// start claims the one transfer that may run at a time.
func (t *transfers) start(w http.ResponseWriter, r *http.Request) bool {
	if t.d.cfg.Restart == nil {
		writeError(w, http.StatusNotImplemented, i18n.T("este WhatsApp MCP não sabe reiniciar sozinho: a transferência precisa do serviço ou do Docker"))
		return false
	}
	if v := r.Header.Get(WacliVersionHeader); v != t.wacliVersion() {
		writeError(w, http.StatusConflict, i18n.Tf("o app e o servidor usam versões diferentes do wacli (%s e %s): atualize os dois para a mesma versão antes de transferir", v, t.wacliVersion()))
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.busy {
		writeError(w, http.StatusConflict, i18n.T("uma transferência já está em andamento"))
		return false
	}
	t.busy = true
	return true
}

func (t *transfers) done() {
	t.mu.Lock()
	t.busy, t.hold = false, nil
	t.mu.Unlock()
}

// restart answers, then closes the gateway, which runs what it must after
// closing, and starts again.
func (t *transfers) restart(after func()) {
	t.d.mu.Lock()
	t.d.afterStop = append(t.d.afterStop, after)
	t.d.mu.Unlock()
	go func() {
		time.Sleep(time.Second)
		t.d.logger.Info("restarting after a transfer")
		t.d.cfg.Restart()
	}()
}

// receive takes the app's data for a server with no WhatsApp yet.
func (t *transfers) receive(w http.ResponseWriter, r *http.Request) {
	if !t.start(w, r) {
		return
	}
	if t.state(r.Context()).Paired {
		t.done()
		writeError(w, http.StatusConflict, i18n.T("o servidor já tem um WhatsApp conectado: desconecte-o no servidor antes de trazer os dados deste computador"))
		return
	}
	a := t.d.primary
	staging, err := transfer.NewStaging(a.StoreDir, a.DataDir)
	if err == nil {
		err = transfer.Unpack(r.Body, staging, nil)
	}
	if err != nil {
		staging.Remove()
		t.done()
		t.d.logger.Warn("a transfer from the app failed", "error", err)
		writeError(w, http.StatusBadRequest, i18n.T("os dados não chegaram inteiros; nada mudou no servidor")+": "+err.Error())
		return
	}
	t.d.logger.Info("received a number's data from the app; installing it on restart")
	t.restart(func() {
		aside, err := staging.Install(a.StoreDir, a.DataDir, transfer.KeepRemoteClients)
		if err != nil {
			t.d.logger.Error("could not install the data from the app", "error", err)
			return
		}
		t.d.logger.Info("installed the data from the app", "previous", aside)
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// send streams the number's data to the app. Sync stays stopped from then
// on: until conclude, which sets the session aside for good, or cancel, or
// 15 minutes without either.
func (t *transfers) send(w http.ResponseWriter, r *http.Request) {
	if !t.start(w, r) {
		return
	}
	if !t.state(r.Context()).Paired {
		t.done()
		writeError(w, http.StatusConflict, i18n.T("o servidor não tem um WhatsApp conectado para transferir"))
		return
	}
	a := t.d.primary
	hold, stopped := make(chan bool, 1), make(chan struct{})
	t.mu.Lock()
	t.hold = hold
	t.mu.Unlock()
	go func() {
		_ = a.sup.Exclusive(t.d.ctx, "transfer", func(ctx context.Context) error {
			close(stopped)
			select {
			case moved := <-hold:
				if moved {
					// The session is the app's now: it never runs here again.
					// The gateway restarts before sync can.
					<-ctx.Done()
				}
			case <-time.After(15 * time.Minute):
				t.d.logger.Warn("a transfer to the app was never concluded; sync resumes")
			case <-ctx.Done():
			}
			return nil
		})
		t.mu.Lock()
		if t.hold == hold {
			t.busy, t.hold = false, nil
		}
		t.mu.Unlock()
	}()
	<-stopped
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set(WacliVersionHeader, t.wacliVersion())
	w.Header().Set(SizeHeader, strconv.FormatInt(transfer.Size(a.StoreDir, a.DataDir), 10))
	if err := transfer.Pack(r.Context(), w, a.StoreDir, a.DataDir, nil); err != nil {
		t.d.logger.Warn("a transfer to the app failed", "error", err)
		hold <- false
		return
	}
	t.d.logger.Info("sent the number's data to the app; sync stays stopped until it confirms")
}

func (t *transfers) conclude(w http.ResponseWriter, r *http.Request) {
	t.mu.Lock()
	hold := t.hold
	t.mu.Unlock()
	if hold == nil {
		writeError(w, http.StatusConflict, i18n.T("não há transferência em andamento"))
		return
	}
	a := t.d.primary
	t.restart(func() {
		aside := a.StoreDir + ".levado-" + time.Now().Format("20060102-150405")
		if err := os.Rename(a.StoreDir, aside); err != nil {
			t.d.logger.Error("could not set the session aside after the transfer", "error", err)
			return
		}
		t.d.logger.Info("the WhatsApp session moved to the app; set aside here", "to", aside)
	})
	hold <- true
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (t *transfers) cancel(w http.ResponseWriter, r *http.Request) {
	t.mu.Lock()
	hold := t.hold
	t.mu.Unlock()
	if hold != nil {
		select {
		case hold <- false:
		default:
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// isTransfer reports whether a request of the app's window is a transfer's.
func isTransfer(path string) bool {
	return path == "/api/transferencia" || strings.HasPrefix(path, "/api/transferencia/")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
