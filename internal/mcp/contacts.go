package mcp

import (
	"context"
	"strings"
)

// saveContact saves or renames a contact in the WhatsApp account through
// `wacli contacts save`, which syncs the name to the phone.
func (s *Server) saveContact(ctx context.Context, a arguments) map[string]any {
	name := strings.TrimSpace(a.Name)
	if name == "" {
		return toolError("name is required")
	}
	raw := strings.TrimSpace(a.Contact)
	digits := phoneDigits(strings.TrimSuffix(raw, "@s.whatsapp.net"))
	if digits == "" {
		return toolError("contact must be a phone number with country code or a JID ending in @s.whatsapp.net")
	}
	jid := digits + "@s.whatsapp.net"
	result := map[string]any{"contact": jid, "current_name": s.index.Names.Known(ctx, jid), "name": name,
		"phonebook": a.Phonebook}
	if a.DryRun {
		result["dry_run"] = true
		result["next"] = "nothing was saved; call again without dry_run once the user approves"
		return textResult(result, false)
	}
	if !s.cli.HasSubcommand(ctx, "contacts", "save") {
		return toolError("the wacli inside this app is too old for this (missing contacts save); nothing was changed")
	}
	args := []string{"contacts", "save", "--jid", digits, "--name", name}
	if first := strings.TrimSpace(a.FirstName); first != "" {
		args = append(args, "--first-name", first)
	}
	if a.Phonebook {
		args = append(args, "--phonebook")
	}
	var out map[string]any
	if err := s.exclusive(ctx, "save contact", &out, args...); err != nil {
		return toolError("%v", err)
	}
	s.index.Names.Forget()
	result["done"] = true
	result["first_name"] = out["first_name"]
	return textResult(result, false)
}
