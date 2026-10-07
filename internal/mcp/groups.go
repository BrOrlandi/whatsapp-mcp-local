package mcp

import (
	"context"
	"strings"
)

// Group changes need the store lock, so each one pauses sync for the seconds
// it takes. They are rare enough for that to be the right trade.

func groupJID(a arguments) (string, map[string]any) {
	jid := strings.TrimSpace(a.GroupJID)
	if !strings.HasSuffix(jid, "@g.us") {
		return "", toolError("group_jid must be a group JID ending in @g.us")
	}
	return jid, nil
}

func (s *Server) manageParticipants(ctx context.Context, a arguments) map[string]any {
	jid, fail := groupJID(a)
	if fail != nil {
		return fail
	}
	actions := map[string]bool{"add": true, "remove": true, "promote": true, "demote": true}
	if !actions[a.Action] {
		return toolError("action must be add, remove, promote or demote")
	}
	if len(a.Participants) == 0 || len(a.Participants) > 50 {
		return toolError("participants must list one to fifty phone numbers or JIDs")
	}
	if a.Action == "remove" && !a.Confirm {
		people := make([]map[string]string, 0, len(a.Participants))
		for _, p := range a.Participants {
			target := p
			if d := phoneDigits(p); d != "" {
				target = d + "@s.whatsapp.net"
			}
			people = append(people, map[string]string{"participant": p, "name": s.index.Names.Name(ctx, target)})
		}
		return textResult(map[string]any{"preview": true, "group": s.index.Names.Name(ctx, jid), "would_remove": people,
			"next": "call manage_group_participants again with confirm true to remove them; WhatsApp tells the group"}, false)
	}
	args := []string{"groups", "participants", a.Action, "--jid", jid}
	for _, p := range a.Participants {
		args = append(args, "--user", strings.TrimSpace(p))
	}
	var out any
	if err := s.exclusive(ctx, "group participants", &out, args...); err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"done": true, "action": a.Action, "group_jid": jid, "result": out}, false)
}

func (s *Server) updateGroup(ctx context.Context, a arguments) map[string]any {
	jid, fail := groupJID(a)
	if fail != nil {
		return fail
	}
	name := strings.TrimSpace(a.Name)
	if name == "" && a.Description == nil {
		return toolError("give a new name, a new description, or both")
	}
	ctx, cancel := context.WithTimeout(ctx, liveTimeout)
	defer cancel()
	err := s.supervisor.Exclusive(ctx, "update group", func(ctx context.Context) error {
		if name != "" {
			if _, err := s.cli.Run(ctx, "groups", "rename", "--jid", jid, "--name", name); err != nil {
				return err
			}
		}
		if a.Description != nil {
			if _, err := s.cli.Run(ctx, "groups", "description", "--jid", jid, "--text", *a.Description); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return toolError("%v", err)
	}
	result := map[string]any{"done": true, "group_jid": jid}
	if name != "" {
		result["name"] = name
	}
	if a.Description != nil {
		result["description"] = *a.Description
	}
	return textResult(result, false)
}

func (s *Server) groupInviteLink(ctx context.Context, a arguments) map[string]any {
	jid, fail := groupJID(a)
	if fail != nil {
		return fail
	}
	if a.Reset && !a.Confirm {
		return textResult(map[string]any{"preview": true, "group": s.index.Names.Name(ctx, jid),
			"next": "call get_group_invite_link again with reset and confirm true: the current link stops working for everyone who has it"}, false)
	}
	verb := "get"
	if a.Reset {
		verb = "revoke"
	}
	var out map[string]any
	if err := s.exclusive(ctx, "group invite link", &out, "groups", "invite", "link", verb, "--jid", jid); err != nil {
		return toolError("%v", err)
	}
	result := map[string]any{"group_jid": jid, "group": s.index.Names.Name(ctx, jid), "reset": a.Reset}
	for k, v := range out {
		result[k] = v
	}
	return textResult(result, false)
}

func (s *Server) leaveGroup(ctx context.Context, a arguments) map[string]any {
	jid, fail := groupJID(a)
	if fail != nil {
		return fail
	}
	if !a.Confirm {
		preview := map[string]any{"preview": true, "group_jid": jid, "group": s.index.Names.Name(ctx, jid),
			"next": "call leave_group again with confirm true to leave; getting back in needs an invite"}
		if g, err := s.index.Group(ctx, jid, ""); err == nil {
			preview["participants"] = g.Size
		}
		return textResult(preview, false)
	}
	var out any
	if err := s.exclusive(ctx, "leave group", &out, "groups", "leave", "--jid", jid); err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"left": true, "group_jid": jid}, false)
}
