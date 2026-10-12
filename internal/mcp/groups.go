package mcp

import (
	"context"
	"fmt"
	"strings"
)

// Group changes need the store lock, so each one pauses sync for the seconds
// it takes. They are rare enough for that to be the right trade.

// needGroupsCommands refuses, before changing anything, when the wacli inside
// the app is older than the group commands a tool needs.
func (s *Server) needGroupsCommands(ctx context.Context, names ...string) map[string]any {
	var missing []string
	for _, n := range names {
		if !s.cli.HasGroupsCommand(ctx, n) {
			missing = append(missing, "groups "+n)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return toolError("the wacli inside this app is too old for this (missing %s); nothing was changed", strings.Join(missing, ", "))
}

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

func (s *Server) setGroupPhoto(ctx context.Context, a arguments) map[string]any {
	jid, fail := groupJID(a)
	if fail != nil {
		return fail
	}
	source := strings.TrimSpace(a.URL)
	if a.Remove == (source != "") {
		return toolError("give either url (the new picture) or remove true")
	}
	if fail := s.needGroupsCommands(ctx, "photo"); fail != nil {
		return fail
	}
	args := []string{"groups", "photo", "remove", "--jid", jid}
	if !a.Remove {
		path, cleanup, err := s.fetchFile(ctx, source, a.Filename)
		if err != nil {
			return toolError("%v", err)
		}
		defer cleanup()
		args = []string{"groups", "photo", "set", "--jid", jid, "--file", path}
	}
	var out map[string]any
	if err := s.exclusive(ctx, "group photo", &out, args...); err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"done": true, "group_jid": jid, "group": s.index.Names.Name(ctx, jid),
		"removed": a.Remove, "picture_id": out["picture_id"]}, false)
}

func (s *Server) updateGroupSettings(ctx context.Context, a arguments) map[string]any {
	jid, fail := groupJID(a)
	if fail != nil {
		return fail
	}
	settings := []struct {
		key, command string
		value        *bool
	}{
		{"join_approval", "join-approval", a.JoinApproval},
		{"announce_only", "announce-only", a.AnnounceOnly},
		{"locked", "locked", a.Locked},
		{"admins_add", "admin-add", a.AdminsAdd},
	}
	result := map[string]any{"done": true, "group_jid": jid, "group": s.index.Names.Name(ctx, jid)}
	var todo [][]string
	for _, st := range settings {
		if st.value == nil {
			continue
		}
		mode := "--off"
		if *st.value {
			mode = "--on"
		}
		todo = append(todo, []string{"groups", st.command, "--jid", jid, mode})
		result[st.key] = *st.value
	}
	if len(todo) == 0 {
		return toolError("give at least one of join_approval, announce_only, locked or admins_add")
	}
	for _, args := range todo {
		if fail := s.needGroupsCommands(ctx, args[1]); fail != nil {
			return fail
		}
	}
	ctx, cancel := context.WithTimeout(ctx, liveTimeout)
	defer cancel()
	err := s.supervisor.Exclusive(ctx, "group settings", func(ctx context.Context) error {
		for _, args := range todo {
			if _, err := s.cli.Run(ctx, args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return toolError("%v", err)
	}
	return textResult(result, false)
}

func (s *Server) groupJoinRequests(ctx context.Context, a arguments) map[string]any {
	jid, fail := groupJID(a)
	if fail != nil {
		return fail
	}
	switch a.Action {
	case "list":
		var out any
		if err := s.exclusive(ctx, "group join requests", &out, "groups", "requests", "list", "--jid", jid); err != nil {
			return toolError("%v", err)
		}
		return textResult(map[string]any{"group_jid": jid, "group": s.index.Names.Name(ctx, jid), "requests": out}, false)
	case "approve", "reject":
		if len(a.Participants) == 0 || len(a.Participants) > 50 {
			return toolError("participants must list one to fifty phone numbers or JIDs")
		}
		args := []string{"groups", "requests", a.Action, "--jid", jid}
		for _, p := range a.Participants {
			args = append(args, "--user", strings.TrimSpace(p))
		}
		var out any
		if err := s.exclusive(ctx, "group join requests", &out, args...); err != nil {
			return toolError("%v", err)
		}
		return textResult(map[string]any{"done": true, "action": a.Action, "group_jid": jid, "result": out}, false)
	}
	return toolError("action must be list, approve or reject")
}

func (s *Server) createGroup(ctx context.Context, a arguments) map[string]any {
	name := strings.TrimSpace(a.Name)
	if name == "" || len([]rune(name)) > 100 {
		return toolError("name is required, up to 100 characters")
	}
	args := []string{"groups", "create", "--name", name}
	if community := strings.TrimSpace(a.CommunityJID); community != "" {
		if !strings.HasSuffix(community, "@g.us") {
			return toolError("community_jid must be a community JID ending in @g.us")
		}
		args = append(args, "--linked-parent", community)
	}
	for flag, on := range map[string]*bool{"--join-approval": a.JoinApproval, "--announce-only": a.AnnounceOnly, "--locked": a.Locked} {
		if on != nil && *on {
			args = append(args, flag)
		}
	}
	if a.AdminsAdd != nil && *a.AdminsAdd {
		if fail := s.needGroupsCommands(ctx, "admin-add"); fail != nil {
			return fail
		}
	}
	ctx, cancel := context.WithTimeout(ctx, liveTimeout)
	defer cancel()
	var jid string
	err := s.supervisor.Exclusive(ctx, "create group", func(ctx context.Context) error {
		// Exclusive retries the whole callback after a lock error: never create twice.
		if jid == "" {
			var out struct {
				JID string `json:"JID"`
			}
			if err := s.cli.Decode(ctx, &out, args...); err != nil {
				return err
			}
			if !strings.HasSuffix(out.JID, "@g.us") {
				return fmt.Errorf("WhatsApp did not return the new group's JID")
			}
			jid = out.JID
		}
		if a.Description != nil && *a.Description != "" {
			if _, err := s.cli.Run(ctx, "groups", "description", "--jid", jid, "--text", *a.Description); err != nil {
				return fmt.Errorf("group %s created, but setting its description failed: %w", jid, err)
			}
		}
		if a.AdminsAdd != nil && *a.AdminsAdd {
			if _, err := s.cli.Run(ctx, "groups", "admin-add", "--jid", jid, "--on"); err != nil {
				return fmt.Errorf("group %s created, but limiting who adds people to admins failed: %w", jid, err)
			}
		}
		return nil
	})
	if err != nil {
		return toolError("%v", err)
	}
	result := map[string]any{"done": true, "group_jid": jid, "name": name}
	if a.CommunityJID != "" {
		result["community_jid"] = strings.TrimSpace(a.CommunityJID)
	}
	for key, v := range map[string]*bool{"join_approval": a.JoinApproval, "announce_only": a.AnnounceOnly, "locked": a.Locked, "admins_add": a.AdminsAdd} {
		if v != nil {
			result[key] = *v
		}
	}
	return textResult(result, false)
}

func (s *Server) communityGroups(ctx context.Context, a arguments) map[string]any {
	community := strings.TrimSpace(a.CommunityJID)
	if !strings.HasSuffix(community, "@g.us") {
		return toolError("community_jid must be a community JID ending in @g.us")
	}
	if fail := s.needGroupsCommands(ctx, "community"); fail != nil {
		return fail
	}
	if a.Action == "list" {
		var out any
		if err := s.exclusive(ctx, "community groups", &out, "groups", "community", "groups", "--community", community); err != nil {
			return toolError("%v", err)
		}
		return textResult(map[string]any{"community_jid": community, "community": s.index.Names.Name(ctx, community), "groups": out}, false)
	}
	if a.Action != "link" && a.Action != "unlink" {
		return toolError("action must be list, link or unlink")
	}
	jid, fail := groupJID(a)
	if fail != nil {
		return fail
	}
	if a.Action == "unlink" && !a.Confirm {
		return textResult(map[string]any{"preview": true, "community": s.index.Names.Name(ctx, community), "group": s.index.Names.Name(ctx, jid),
			"next": "call community_groups again with confirm true to take the group out of the community; the group itself stays"}, false)
	}
	var out any
	if err := s.exclusive(ctx, "community "+a.Action, &out, "groups", "community", a.Action, "--community", community, "--jid", jid); err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"done": true, "action": a.Action, "community_jid": community, "group_jid": jid, "group": s.index.Names.Name(ctx, jid)}, false)
}
