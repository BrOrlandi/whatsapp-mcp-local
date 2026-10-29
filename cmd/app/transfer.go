package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/appconfig"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/daemon"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/remote"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/transfer"
)

// Moving the number between this computer and the server
// (internal/transfer, internal/daemon/transfer.go): to the server, after
// pairing, the data goes there and this computer's copy is deleted once
// the server is connected with it; back here, the data comes, the server
// sets its session aside, and the app returns to local mode.

const (
	toServer = "servidor"
	toLocal  = "local"
)

// transferJob is a transfer under way, or the last one, for the remote
// page to follow.
type transferJob struct {
	Kind  string `json:"kind"`
	Step  string `json:"step"`
	Done  bool   `json:"done"`
	Error string `json:"error,omitempty"`
	// Server is the server's host, for the guidance at the end.
	Server string `json:"server"`
	Total  int64  `json:"total"`
	moved  atomic.Int64
}

func (j *transferJob) view() map[string]any {
	transferMu.Lock()
	defer transferMu.Unlock()
	return map[string]any{"kind": j.Kind, "step": j.Step, "done": j.Done, "error": j.Error, "server": j.Server,
		"total": j.Total, "moved": j.moved.Load()}
}

var transferMu sync.Mutex

func (a *App) currentTransfer() *transferJob {
	transferMu.Lock()
	defer transferMu.Unlock()
	return a.transfer
}

func (j *transferJob) step(s string) {
	transferMu.Lock()
	j.Step = s
	transferMu.Unlock()
}

func (j *transferJob) fail(err error) {
	transferMu.Lock()
	j.Error, j.Done = err.Error(), true
	transferMu.Unlock()
}

func (j *transferJob) finish() {
	transferMu.Lock()
	j.Done = true
	transferMu.Unlock()
}

// running reports whether a transfer is under way.
func (a *App) running() bool {
	j := a.currentTransfer()
	if j == nil {
		return false
	}
	transferMu.Lock()
	defer transferMu.Unlock()
	return !j.Done
}

func (a *App) newTransfer(kind, server string) *transferJob {
	j := &transferJob{Kind: kind, Server: hostOf(server)}
	transferMu.Lock()
	a.transfer = j
	transferMu.Unlock()
	return j
}

func (a *App) transferStatusAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	j := a.currentTransfer()
	if j == nil {
		_, _ = w.Write([]byte("null"))
		return
	}
	_ = json.NewEncoder(w).Encode(j.view())
}

// transferCheck asks the server whether a transfer can go ahead.
func (a *App) transferCheck(ctx context.Context, rm *remoteMode) (daemon.TransferInfo, string, error) {
	var info daemon.TransferInfo
	local := transfer.WacliVersion(ctx, a.wacliBin)
	if err := serverCall(ctx, rm, http.MethodGet, "/api/transferencia", &info); err != nil {
		return info, local, err
	}
	if !info.Available {
		return info, local, errors.New(i18n.T("o servidor não reinicia sozinho (rode-o como serviço ou no Docker) ou é de uma versão sem transferência: atualize o servidor"))
	}
	if info.Wacli != local {
		return info, local, errors.New(i18n.Tf("o app e o servidor usam versões diferentes do wacli (%s e %s): atualize os dois para a mesma versão antes de transferir", local, info.Wacli))
	}
	return info, local, nil
}

// waitLink waits for the session with the server.
func waitLink(ctx context.Context, rm *remoteMode, d time.Duration) error {
	deadline := time.Now().Add(d)
	for rm.client.Status().State != remote.StateConnected {
		if time.Now().After(deadline) || ctx.Err() != nil {
			return errors.New(i18n.T("o app está sem conexão com o servidor agora; tente de novo em instantes"))
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

// moveToServer runs after pairing with move set: this computer's number
// goes to the server, which must have none, and once the server is
// connected with it, this computer's copy is deleted. phone is the number
// this computer had.
func (a *App) moveToServer(rm *remoteMode, phone string) {
	j := a.newTransfer(toServer, rm.server)
	ctx := context.Background()
	j.step(i18n.T("Conferindo o servidor"))
	if err := waitLink(ctx, rm, time.Minute); err != nil {
		j.fail(err)
		return
	}
	info, version, err := a.transferCheck(ctx, rm)
	if err == nil && info.Paired {
		err = errors.New(i18n.T("o servidor já tem um WhatsApp conectado: desconecte-o no servidor antes de levar os dados deste computador"))
	}
	if err != nil {
		j.fail(err)
		return
	}

	j.step(i18n.T("Enviando as conversas para o servidor"))
	transferMu.Lock()
	j.Total = transfer.Size(a.storeDir, a.dataDir)
	transferMu.Unlock()
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(transfer.Pack(ctx, pw, a.storeDir, a.dataDir, func(n int64) { j.moved.Add(n) }))
	}()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://remote/api/transferencia/receber", pr)
	req.Header.Set(remote.WindowHeader, "1")
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set(daemon.WacliVersionHeader, version)
	if err := expectOK(rm.client.RoundTrip(req)); err != nil {
		pr.CloseWithError(err)
		j.fail(fmt.Errorf("%s: %w", i18n.T("a transferência não terminou; as conversas continuam neste computador"), err))
		return
	}

	// The server restarts with the data, and connects to WhatsApp with
	// this computer's session.
	j.step(i18n.T("O servidor está abrindo as conversas"))
	deadline := time.Now().Add(5 * time.Minute)
	time.Sleep(3 * time.Second)
	for {
		if rm.client.Status().State == remote.StateConnected {
			a.pollServer(ctx, rm)
			rm.mu.Lock()
			wa := rm.whatsapp
			rm.mu.Unlock()
			if wa != nil && wa.Paired && (phone == "" || digitsOf(wa.Phone) == phone) {
				break
			}
		}
		if time.Now().After(deadline) {
			j.fail(errors.New(i18n.T("o servidor recebeu as conversas, mas não confirmou a conexão com o WhatsApp em 5 minutos. Os dados continuam também neste computador: confira o servidor antes de apagar algo")))
			return
		}
		time.Sleep(3 * time.Second)
	}

	j.step(i18n.T("Apagando as conversas deste computador"))
	if err := transfer.RemoveData(a.storeDir, a.dataDir); err != nil {
		a.logger.Error("could not delete this computer's data after the transfer", "error", err)
		j.fail(fmt.Errorf("%s: %w", i18n.T("o servidor já usa as conversas, mas não foi possível apagar a cópia deste computador"), err))
		return
	}
	a.logger.Info("the number moved to the server; this computer's copy was deleted", "server", hostOf(rm.server))
	j.step(i18n.T("Pronto"))
	j.finish()
}

// bringToLocal brings the server's number here: the data comes, the
// server sets its session aside and restarts empty, and the app goes back
// to local mode with it.
func (a *App) bringToLocal(rm *remoteMode) {
	j := a.newTransfer(toLocal, rm.server)
	ctx := context.Background()
	cfg := a.config().Remote
	j.step(i18n.T("Conferindo o servidor"))
	info, version, err := a.transferCheck(ctx, rm)
	if err == nil && !info.Paired {
		err = errors.New(i18n.T("o servidor não tem um WhatsApp conectado para transferir"))
	}
	if err != nil {
		j.fail(err)
		return
	}

	j.step(i18n.T("Trazendo as conversas do servidor"))
	staging, err := transfer.NewStaging(a.storeDir, a.dataDir)
	if err != nil {
		j.fail(err)
		return
	}
	cancel := func() {
		staging.Remove()
		_ = serverCall(ctx, rm, http.MethodPost, "/api/transferencia/cancelar", nil)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://remote/api/transferencia/enviar", nil)
	req.Header.Set(remote.WindowHeader, "1")
	req.Header.Set(daemon.WacliVersionHeader, version)
	resp, err := rm.client.RoundTrip(req)
	if err == nil && resp.StatusCode != http.StatusOK {
		err = expectOK(resp, nil)
	}
	if err != nil {
		cancel()
		j.fail(fmt.Errorf("%s: %w", i18n.T("a transferência não começou; nada mudou"), err))
		return
	}
	total, _ := strconv.ParseInt(resp.Header.Get(daemon.SizeHeader), 10, 64)
	transferMu.Lock()
	j.Total = total
	transferMu.Unlock()
	err = transfer.Unpack(resp.Body, staging, func(n int64) { j.moved.Add(n) })
	resp.Body.Close()
	if err != nil {
		cancel()
		j.fail(fmt.Errorf("%s: %w", i18n.T("as conversas não chegaram inteiras; o servidor continua com o WhatsApp"), err))
		return
	}
	if err := serverCall(ctx, rm, http.MethodPost, "/api/transferencia/concluir", nil); err != nil {
		cancel()
		j.fail(fmt.Errorf("%s: %w", i18n.T("o servidor não confirmou a transferência; ele continua com o WhatsApp"), err))
		return
	}

	// The server's session is set aside: the one here is the only one now.
	j.step(i18n.T("Abrindo as conversas neste computador"))
	if err := a.updateConfig(func(c *appconfig.Config) { c.Remote = nil }); err != nil {
		j.fail(err)
		return
	}
	a.stopRemote()
	aside, err := staging.Install(a.storeDir, a.dataDir, transfer.KeepRemoteClients)
	if err != nil {
		a.logger.Error("could not install the data from the server", "error", err)
		j.fail(fmt.Errorf("%s: %w", i18n.T("as conversas chegaram, mas não foi possível colocá-las no lugar"), err))
		return
	}
	if cfg != nil {
		_ = deleteDeviceKey(cfg.DeviceID)
		_ = deleteDeviceKey(nextKeyAccount(cfg.DeviceID))
	}
	a.logger.Info("the number came back from the server; local mode", "previous", aside)
	a.startDaemon()
	j.step(i18n.T("Pronto"))
	j.finish()
}

// expectOK turns an answer other than 200 into its error.
func expectOK(resp *http.Response, err error) error {
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var e struct {
		Error string `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&e) == nil && e.Error != "" {
		return errors.New(e.Error)
	}
	return fmt.Errorf("the server answered %s", resp.Status)
}

// remoteBring starts bringing the number back here, from the remote page.
func (a *App) remoteBring(w http.ResponseWriter, r *http.Request) {
	rm := a.currentRemote()
	if rm == nil {
		back(w, r, "", errors.New(i18n.T("o app não está no modo remoto")))
		return
	}
	if a.running() {
		back(w, r, "", errors.New(i18n.T("uma transferência já está em andamento")))
		return
	}
	go a.bringToLocal(rm)
	http.Redirect(w, r, "/__app/remoto", http.StatusSeeOther)
}
