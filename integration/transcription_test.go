//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
)

// A voice note in integration/testdata/audio, made with ElevenLabs by
// scripts/integration-audio.py, and the words its transcript must have.
type audioCase struct {
	File     string   `json:"file"`
	Language string   `json:"language"`
	Text     string   `json:"text"`
	Expect   []string `json:"expect"` // each may list alternatives: "quinze|15"
}

// testTranscription sends each voice note to WMCP_TO, transcribes it on the
// test Mac (whisper.cpp, installed in Configurações › Transcrição de áudio)
// and checks the transcript has the words said, allowing one miss per note.
// The messages are deleted for everyone at the end.
func testTranscription(t *testing.T, a *app) {
	to := os.Getenv("WMCP_TO")
	if to == "" {
		t.Skip("WMCP_TO is not set")
	}
	raw, err := os.ReadFile("testdata/audio/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []audioCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, c := range cases {
		if _, err := os.Stat(filepath.Join("testdata/audio", c.File)); err == nil {
			files = append(files, c.File)
		}
	}
	if len(files) == 0 {
		t.Skip("no voice notes in testdata/audio: make them with scripts/integration-audio.py")
	}
	st := a.call("whatsapp_status", nil)
	if get(st, "transcription.local.ready") != true {
		t.Fatalf("transcription is not installed on the test Mac (Configurações › Transcrição de áudio): %s", clipAny(get(st, "transcription")))
	}

	// Copy the notes to the test Mac.
	dir := media + "-audio"
	tarCmd := exec.Command("tar", append([]string{"-C", "testdata/audio", "-cf", "-"}, files...)...)
	tarball, err := tarCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if out, err := remote(t, a.e, "rm -rf "+dir+" && mkdir -p "+dir+" && tar -C "+dir+" -xf -", tarball); err != nil {
		t.Fatalf("copying the voice notes: %v %s", err, out)
	}
	t.Cleanup(func() { remote(t, a.e, "rm -rf "+dir, nil) })

	chat := str(a.call("send_text_message", obj{"to": to, "text": "x", "dry_run": true}), "recipient.jid")
	var sent []string
	t.Cleanup(func() {
		for _, id := range sent {
			a.tryCall("delete_message", obj{"message_id": id, "chat_jid": chat, "confirm": true})
		}
	})

	for _, c := range cases {
		if !contains(files, c.File) {
			continue
		}
		t.Run(c.File, func(t *testing.T) {
			v := a.call("send_media_message", obj{"to": to, "type": "audio", "url": dir + "/" + c.File})
			id := sentID(v)
			if id == "" {
				t.Fatalf("send returned no id: %s", clipAny(v))
			}
			sent = append(sent, id)
			var res any
			eventually(t, 60*time.Second, "the voice note in the index", func() bool {
				r, isErr := a.tryCall("transcribe_audio", obj{"message_id": id, "chat_jid": chat, "language": c.Language})
				res = r
				return !isErr || !strings.Contains(mustJSON(r), "no indexed message")
			})
			text := ""
			for _, p := range []string{"transcript.text", "text", "transcript"} {
				if text = str(res, p); text != "" {
					break
				}
			}
			if text == "" {
				t.Fatalf("transcribe_audio = %s", clipAny(res))
			}
			got := fold(text)
			var missing []string
			for _, want := range c.Expect {
				found := false
				for _, alt := range strings.Split(want, "|") {
					if strings.Contains(got, fold(alt)) {
						found = true
					}
				}
				if !found {
					missing = append(missing, want)
				}
			}
			t.Logf("said:  %s\nheard: %s", c.Text, text)
			if len(missing) > 1 {
				t.Errorf("transcript misses %v", missing)
			}
			// Asking again returns the kept transcript at once.
			again := a.call("transcribe_audio", obj{"message_id": id, "chat_jid": chat})
			if !strings.Contains(fold(mustJSON(again)), fold(text)) {
				t.Errorf("the second call did not return the kept transcript: %s", clipAny(again))
			}
		})
	}
}

var accents = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "é", "e", "ê", "e", "è", "e",
	"í", "i", "î", "i", "ó", "o", "ô", "o", "õ", "o", "ö", "o", "ú", "u", "ü", "u", "ç", "c", "ñ", "n")

// fold lowercases and strips accents and punctuation, for comparing words.
func fold(s string) string {
	var b strings.Builder
	for _, r := range accents.Replace(strings.ToLower(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return " " + strings.Join(strings.Fields(b.String()), " ") + " "
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
