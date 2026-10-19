package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/skip2/go-qrcode"
)

// pollEvery is how often pair asks the daemon how the pairing is going.
// WhatsApp replaces the QR code about every 20 seconds.
var pollEvery = time.Second

// pairEvent is a line of `pair --json`: one per QR code, code or ending.
type pairEvent struct {
	Event     string `json:"event"` // qr, code, paired, already_paired
	QR        string `json:"qr,omitempty"`
	QRPNG     string `json:"qr_png,omitempty"`
	QRVersion int    `json:"qr_version,omitempty"`
	Code      string `json:"code,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Name      string `json:"name,omitempty"`
}

// pairCmd connects WhatsApp from the terminal: a QR code drawn in it, kept
// current as WhatsApp replaces it, or with --phone a code to type on the
// phone. It ends once the phone has linked.
func pairCmd(cfg config, args []string) error {
	f := newFlags("pair")
	phone := f.String("phone", "", "")
	noWait := f.Bool("no-wait", false, "")
	timeout := f.Duration("timeout", 0, "")
	invert := f.Bool("invert", false, "")
	qrFile := f.String("qr-file", "", "")
	rest, err := f.parse(args)
	if err != nil {
		return err
	}
	if *phone == "" {
		*phone = strings.Join(rest, " ")
	} else if len(rest) > 0 {
		return fail(exitUsage, "pair: %s", i18n.T("informe o número uma vez só"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}
	a := newAPI(cfg)
	st, _, err := a.status(ctx)
	if err != nil {
		return err
	}
	if st.WhatsApp.Paired {
		if jsonOutput {
			printJSON(pairEvent{Event: "already_paired", Phone: st.WhatsApp.Phone, Name: st.WhatsApp.Name})
		} else {
			say("%s", i18n.Tf("Este computador já está conectado ao WhatsApp de %s. Para conectar outro número, desconecte antes: whatsapp-mcp logout", who(st)))
		}
		return nil
	}
	if err := a.call(ctx, http.MethodPost, "/api/pair", map[string]string{"phone": *phone}, nil); err != nil {
		return err
	}

	// The pairing stops with the command: a QR code nobody is looking at
	// should not stay up, nor a code on its way to a phone.
	finished := false
	defer func() {
		if !finished {
			cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = a.call(cctx, http.MethodPost, "/api/pair/cancel", nil, nil)
		}
	}()

	colour := isTerminal(os.Stdout) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	shown := ""
	for {
		st, _, err := a.status(ctx)
		if err != nil {
			return pairStopped(ctx, *timeout, err)
		}
		p := st.Pairing
		switch p.State {
		case "qr":
			key := fmt.Sprint("qr", p.QRVersion)
			if key != shown && p.QR != "" {
				shown = key
				if err := showQR(st, colour, *invert, *qrFile); err != nil {
					return err
				}
			}
		case "code":
			if key := "code" + p.Code; key != shown && p.Code != "" {
				shown = key
				showCode(p.Code, *phone)
			}
		case "syncing", "done":
			finished = true
			return paired(st)
		case "error":
			finished = true
			return fail(exitNotReady, "%s", i18n.Tf("a conexão não deu certo: %s", p.Error))
		case "cancelled":
			finished = true
			return fail(exitNotReady, "%s", i18n.T("a conexão foi cancelada"))
		case "idle":
			// The daemon restarted, or another program ended the pairing.
			if st.WhatsApp.Paired {
				finished = true
				return paired(st)
			}
			finished = true
			return fail(exitNotReady, "%s", i18n.T("a conexão parou antes de o celular confirmar; tente de novo"))
		}
		if *noWait && shown != "" {
			// Another program takes it from here, following /api/status.
			finished = true
			return nil
		}
		select {
		case <-ctx.Done():
			return pairStopped(ctx, *timeout, ctx.Err())
		case <-time.After(pollEvery):
		}
	}
}

// pairStopped is the error of a pairing that ran out of time or was
// interrupted, which the deferred cancel then ends on the daemon too.
func pairStopped(ctx context.Context, timeout time.Duration, err error) error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fail(exitTimeout, "%s", i18n.Tf("o celular não confirmou em %s; a conexão foi cancelada", timeout))
	case ctx.Err() != nil:
		return fail(exitCancelled, "%s", i18n.T("a conexão foi cancelada"))
	}
	return err
}

func showQR(st status, colour, invert bool, file string) error {
	p := st.Pairing
	if file != "" {
		if err := qrcode.WriteFile(p.QR, qrcode.Medium, 512, file); err != nil {
			return err
		}
	}
	if jsonOutput {
		printJSON(pairEvent{Event: "qr", QR: p.QR, QRPNG: p.QRPNG, QRVersion: p.QRVersion})
		return nil
	}
	drawn, err := renderQR(p.QR, colour, invert)
	if err != nil {
		return err
	}
	if isTerminal(os.Stdout) {
		fmt.Fprint(stdout, "\x1b[H\x1b[2J") // the new code in place of the old one
	} else if p.QRVersion > 1 {
		fmt.Fprintln(stdout, "\n"+strings.Repeat("─", 40)+"\n")
	}
	say("%s\n", i18n.T("No celular, abra o WhatsApp › Configurações › Dispositivos conectados › Conectar dispositivo e leia este QR code:"))
	fmt.Fprint(stdout, drawn)
	say("\n%s\n%s", i18n.T("O conteúdo do QR code, para gerar a imagem em outro lugar (por exemplo, qrencode -t ansiutf8 '<conteúdo>'):"), p.QR)
	say("%s %s", i18n.T("A imagem, neste computador:"), p.QRPNG)
	if file != "" {
		say("%s %s", i18n.T("A imagem, salva em:"), file)
	}
	say("\n%s", i18n.T("O WhatsApp troca o QR code a cada 20 segundos, mais ou menos; esta tela acompanha. Ctrl+C cancela."))
	say("%s", i18n.T("Prefere digitar um código no celular? whatsapp-mcp pair --phone '+55 11 91234-5678'"))
	return nil
}

func showCode(code, phone string) {
	if jsonOutput {
		printJSON(pairEvent{Event: "code", Code: code, Phone: phone})
		return
	}
	say("%s\n\n    %s\n", i18n.T("No celular, abra o WhatsApp › Configurações › Dispositivos conectados › Conectar dispositivo › Conectar com número de telefone, e digite:"), code)
	say("%s", i18n.T("Esperando o celular. Ctrl+C cancela."))
}

func paired(st status) error {
	if jsonOutput {
		printJSON(pairEvent{Event: "paired", Phone: st.WhatsApp.Phone, Name: st.WhatsApp.Name})
		return nil
	}
	say("\n%s", i18n.Tf("WhatsApp conectado: %s.", who(st)))
	say("%s", i18n.T("O histórico das conversas continua chegando em segundo plano, o que pode levar alguns minutos; whatsapp-mcp status mostra quando terminar."))
	return nil
}

// who names the connected account: its name and number, as far as known.
func who(st status) string {
	parts := []string{}
	for _, s := range []string{st.WhatsApp.Name, st.WhatsApp.Phone} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return i18n.T("um número")
	}
	return strings.Join(parts, ", ")
}
