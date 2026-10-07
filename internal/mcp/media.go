package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The media folder holds what the tools downloaded on request (download_media,
// transcribe_audio), one folder per chat. Every file in it can be
// downloaded again from WhatsApp while WhatsApp still holds it, so clearing it
// frees space without losing a message.

// RetentionSetting is the state key for how many days downloaded media is
// kept; empty or 0 keeps it forever.
const RetentionSetting = "media_retention_days"

type mediaFile struct {
	path    string
	chat    string
	kind    string
	size    int64
	modTime time.Time
}

func mediaKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".heic", ".bmp", ".tif", ".tiff":
		return "image"
	case ".mp4", ".mov", ".3gp", ".mkv", ".webm":
		return "video"
	case ".ogg", ".opus", ".mp3", ".m4a", ".aac", ".wav", ".amr":
		return "audio"
	}
	return "document"
}

// mediaFiles lists the downloaded files, optionally of one chat only.
func (s *Server) mediaFiles(chat string) ([]mediaFile, error) {
	root := s.mediaDir
	if chat != "" {
		root = filepath.Join(s.mediaDir, safeName(chat))
	}
	var files []mediaFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(s.mediaDir, path)
		files = append(files, mediaFile{path: path, chat: strings.SplitN(filepath.ToSlash(rel), "/", 2)[0], kind: mediaKind(path),
			size: info.Size(), modTime: info.ModTime()})
		return nil
	})
	if os.IsNotExist(err) {
		err = nil
	}
	return files, err
}

// retentionDays is how many days downloaded media is kept, 0 for forever.
func (s *Server) retentionDays(ctx context.Context) int {
	v, _ := s.state.Setting(ctx, RetentionSetting)
	n, _ := strconv.Atoi(v)
	return max(n, 0)
}

// MediaInventory is what the media folder holds.
type MediaInventory struct {
	Dir            string              `json:"dir"`
	Files          int                 `json:"files"`
	Bytes          int64               `json:"bytes"`
	ByType         map[string]mediaSum `json:"by_type"`
	ByChat         []chatMediaSum      `json:"by_chat"`
	DuplicateBytes int64               `json:"duplicate_bytes"`
	Oldest         *time.Time          `json:"oldest_download,omitempty"`
	Newest         *time.Time          `json:"newest_download,omitempty"`
	RetentionDays  int                 `json:"retention_days"`
	// Exports are the files export_messages wrote, kept apart: the person
	// asked for them, so retention never deletes them.
	Exports    mediaSum `json:"exports"`
	ExportsDir string   `json:"exports_dir"`
}

type mediaSum struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

type chatMediaSum struct {
	ChatJID string `json:"chat_jid"`
	Name    string `json:"name"`
	mediaSum
}

// Inventory measures the media folder: by type, by chat (largest first), and
// how much is the same file kept twice.
func (s *Server) Inventory(ctx context.Context) (MediaInventory, error) {
	files, err := s.mediaFiles("")
	if err != nil {
		return MediaInventory{}, err
	}
	inv := MediaInventory{Dir: s.mediaDir, ByType: map[string]mediaSum{}, RetentionDays: s.retentionDays(ctx)}
	chats := map[string]*chatMediaSum{}
	bySize := map[int64][]string{}
	for _, f := range files {
		inv.Files++
		inv.Bytes += f.size
		t := inv.ByType[f.kind]
		t.Files++
		t.Bytes += f.size
		inv.ByType[f.kind] = t
		c, ok := chats[f.chat]
		if !ok {
			c = &chatMediaSum{ChatJID: f.chat, Name: s.index.Names.Name(ctx, f.chat)}
			chats[f.chat] = c
		}
		c.Files++
		c.Bytes += f.size
		bySize[f.size] = append(bySize[f.size], f.path)
		mt := f.modTime.UTC()
		if inv.Oldest == nil || mt.Before(*inv.Oldest) {
			inv.Oldest = &mt
		}
		if inv.Newest == nil || mt.After(*inv.Newest) {
			mt2 := mt
			inv.Newest = &mt2
		}
	}
	// Only files of equal size can be equal, so only those are hashed.
	for size, paths := range bySize {
		if len(paths) < 2 || size == 0 {
			continue
		}
		seen := map[string]bool{}
		for _, p := range paths {
			h, err := hashFile(p)
			if err != nil {
				continue
			}
			if seen[h] {
				inv.DuplicateBytes += size
			}
			seen[h] = true
		}
	}
	for _, c := range chats {
		inv.ByChat = append(inv.ByChat, *c)
	}
	sort.Slice(inv.ByChat, func(i, j int) bool { return inv.ByChat[i].Bytes > inv.ByChat[j].Bytes })
	if len(inv.ByChat) > 20 {
		inv.ByChat = inv.ByChat[:20]
	}
	inv.ExportsDir = s.exportDir
	if entries, err := os.ReadDir(s.exportDir); err == nil {
		for _, e := range entries {
			if info, err := e.Info(); err == nil && !e.IsDir() {
				inv.Exports.Files++
				inv.Exports.Bytes += info.Size()
			}
		}
	}
	return inv, nil
}

// PurgeExports deletes the files export_messages wrote.
func (s *Server) PurgeExports() (files int, bytes int64, err error) {
	entries, err := os.ReadDir(s.exportDir)
	if os.IsNotExist(err) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() {
			continue
		}
		if os.Remove(filepath.Join(s.exportDir, e.Name())) == nil {
			files++
			bytes += info.Size()
		}
	}
	return files, bytes, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Server) mediaStats(ctx context.Context, _ arguments) map[string]any {
	inv, err := s.Inventory(ctx)
	if err != nil {
		return toolError("%v", err)
	}
	return textResult(map[string]any{"inventory": inv,
		"note": "the files the tools downloaded on request; each can be downloaded again while WhatsApp holds it, so purge_media frees space without losing messages"}, false)
}

// PurgeMedia deletes downloaded files older than days (0: any age), of one
// chat or all, of at least minBytes, and reports what it removed. With dryRun
// it only reports what it would remove.
func (s *Server) PurgeMedia(chat string, days int, minBytes int64, dryRun bool) (files int, bytes int64, largest []map[string]any, err error) {
	all, err := s.mediaFiles(chat)
	if err != nil {
		return 0, 0, nil, err
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	var picked []mediaFile
	for _, f := range all {
		if days > 0 && f.modTime.After(cutoff) {
			continue
		}
		if f.size < minBytes {
			continue
		}
		picked = append(picked, f)
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].size > picked[j].size })
	for i, f := range picked {
		if i < 10 {
			largest = append(largest, map[string]any{"path": f.path, "bytes": f.size, "downloaded_at": f.modTime.UTC()})
		}
		if !dryRun {
			if err := os.Remove(f.path); err != nil && !os.IsNotExist(err) {
				continue
			}
			_ = os.Remove(filepath.Dir(f.path)) // only succeeds once the chat's folder is empty
		}
		files++
		bytes += f.size
	}
	return files, bytes, largest, nil
}

func (s *Server) purgeMedia(ctx context.Context, a arguments) map[string]any {
	if a.OlderThan < 0 || a.MinMegabyte < 0 {
		return toolError("older_than_days and min_megabytes must be positive")
	}
	files, bytes, largest, err := s.PurgeMedia(strings.TrimSpace(a.ChatJID), a.OlderThan, int64(a.MinMegabyte)<<20, !a.Confirm)
	if err != nil {
		return toolError("%v", err)
	}
	if !a.Confirm {
		return textResult(map[string]any{"preview": true, "would_delete_files": files, "would_free_bytes": bytes, "largest": largest,
			"next": "call purge_media again with the same filters and confirm true to delete them; each file can be downloaded again while WhatsApp holds it"}, false)
	}
	return textResult(map[string]any{"deleted_files": files, "freed_bytes": bytes}, false)
}

// SweepMedia applies the retention setting: files downloaded more days ago
// than it allows are deleted. It does nothing while retention is off.
func (s *Server) SweepMedia(ctx context.Context) (int, int64, error) {
	days := s.retentionDays(ctx)
	if days <= 0 {
		return 0, 0, nil
	}
	files, bytes, _, err := s.PurgeMedia("", days, 0, false)
	if err == nil && files > 0 {
		s.logger.Info("media retention", "deleted_files", files, "freed_bytes", bytes, "days", days)
	}
	return files, bytes, err
}

// SetRetention changes how many days downloaded media is kept; 0 keeps it.
func (s *Server) SetRetention(ctx context.Context, days int) error {
	if days <= 0 {
		return s.state.SetSetting(ctx, RetentionSetting, "")
	}
	return s.state.SetSetting(ctx, RetentionSetting, strconv.Itoa(days))
}
