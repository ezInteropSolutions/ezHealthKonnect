package storage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A real message ID: the sender's address contains a colon, which Windows rejects in a file name.
const tcpMessageID = "tcp_127.0.0.1:52244_1761552012345678901"

func TestWindowsFileNames_EncodesWhatWindowsRejects(t *testing.T) {
	c := windowsFileNames{}
	cases := map[string]string{
		tcpMessageID + ".raw": "tcp_127.0.0.1%3A52244_1761552012345678901.raw",
		"[::1]:2575":          "[%3A%3A1]%3A2575",
		`a<b>c"d|e?f*g\h`:     "a%3Cb%3Ec%22d%7Ce%3Ff%2Ag%5Ch",
		"100%":                "100%25",
		"ends.":               "ends%2E",
		"ends ":               "ends%20",
		"CON":                 "%43ON",
		"nul.txt":             "%6Eul.txt",
		"COM3":                "%43OM3",
		"COM10":               "COM10", // not a device name
		"console":             "console",
		"plain-name_1.json":   "plain-name_1.json",
		"patiënt-患者":          "patiënt-患者",
		"":                    "",
	}
	for in, want := range cases {
		if got := c.Encode(in); got != want {
			t.Errorf("Encode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWindowsFileNames_ProducesNothingWindowsRejects(t *testing.T) {
	c := windowsFileNames{}
	for _, in := range []string{tcpMessageID, "a:b", `x\y`, "q?", "tail.", "AUX", "tab\there", "ctrl\x01"} {
		out := c.Encode(in)
		if strings.ContainsAny(out, `<>:"|?*\`) || strings.HasSuffix(out, ".") || strings.HasSuffix(out, " ") {
			t.Errorf("Encode(%q) = %q still has a character Windows rejects", in, out)
		}
		for _, r := range out {
			if r < 0x20 {
				t.Errorf("Encode(%q) = %q still has a control character", in, out)
			}
		}
	}
}

func TestWindowsFileNames_DecodeIsTheExactInverse(t *testing.T) {
	c := windowsFileNames{}
	seen := map[string]string{}
	for _, in := range []string{
		tcpMessageID, "a:b", "a%3Ab", "100%", "%", "%%", "%4", "%41", "tail.", "tail ", "CON", "con", "Nul.x", "COM1", "LPT9.log",
		`x\y`, "q?", "tab\there", "ctrl\x01", "patiënt-患者", "plain", "", "a.b.c.", "..", ".hidden",
	} {
		enc := c.Encode(in)
		if got := c.Decode(enc); got != in {
			t.Errorf("round trip of %q: encoded %q, decoded %q", in, enc, got)
		}
		if prev, dup := seen[enc]; dup && prev != in {
			t.Errorf("two keys share the file name %q: %q and %q", enc, prev, in)
		}
		seen[enc] = in
	}
}

func TestIdentityFileNames_AndLinuxKeepEverythingAsIs(t *testing.T) {
	if got := (identityFileNames{}).Encode(tcpMessageID); got != tcpMessageID {
		t.Errorf("identity changed the name: %q", got)
	}
	if _, ok := codecForOS("linux").(identityFileNames); !ok {
		t.Error("linux must keep names as they are so existing stores stay readable")
	}
	if _, ok := codecForOS("darwin").(identityFileNames); !ok {
		t.Error("darwin must keep names as they are")
	}
	if _, ok := codecForOS("windows").(windowsFileNames); !ok {
		t.Error("windows must use the Windows codec")
	}
}

// A message whose ID has a colon is stored, read, listed (under its real key) and deleted when the driver
// uses Windows naming. Runs on any OS because %3A is a valid file name everywhere.
func TestLocalDriver_WindowsNaming_StoresAMessageIDWithAColon(t *testing.T) {
	dir := t.TempDir()
	d := &LocalDriver{baseDir: dir, names: windowsFileNames{}}
	ctx := context.Background()
	key := "0d0200d8-c3f7-44d7-9933-2f862284ea94/2026/10/27/raw/" + tcpMessageID + ".raw"

	uri, err := d.PutObject(ctx, "ehk-messages", key, strings.NewReader("MSH|^~\\&|"), 9, "")
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if !strings.HasSuffix(uri, tcpMessageID+".raw") {
		t.Errorf("the URI must carry the real key, got %q", uri)
	}

	onDisk := filepath.Join(dir, "ehk-messages", "0d0200d8-c3f7-44d7-9933-2f862284ea94", "2026", "10", "27", "raw", "tcp_127.0.0.1%3A52244_1761552012345678901.raw")
	if _, err := os.Stat(onDisk); err != nil {
		t.Fatalf("expected the file at the Windows-safe path: %v", err)
	}

	body, info, err := d.GetObjectBytes(ctx, "ehk-messages", key)
	if err != nil || string(body) != "MSH|^~\\&|" || info.Key != key {
		t.Fatalf("get: body=%q info=%+v err=%v", body, info, err)
	}

	list, err := d.ListObjects(ctx, "ehk-messages", "0d0200d8-c3f7-44d7-9933-2f862284ea94/2026/10/27/")
	if err != nil || len(list) != 1 || list[0].Key != key {
		t.Fatalf("list must return the real key, got %+v err=%v", list, err)
	}

	if _, err := d.AppendObject(ctx, "ehk-messages", key[:len(key)-4]+".ndjson", []byte("line\n")); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := d.DeleteObject(ctx, "ehk-messages", key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, err := d.GetObjectBytes(ctx, "ehk-messages", key); err == nil {
		t.Error("the object should be gone")
	}
}

// Linux and macOS: the colon stays in the file name, exactly as before, so objects written by earlier versions
// are still found.
func TestLocalDriver_OnLinuxKeepsColonNames(t *testing.T) {
	dir := t.TempDir()
	d, err := NewLocalDriver(dir)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("on Windows the colon is encoded (covered by the Windows-naming test above)")
	}
	key := "iface/2026/10/27/raw/" + tcpMessageID + ".raw"
	if _, err := d.PutObject(context.Background(), "b", key, strings.NewReader("x"), 1, ""); err != nil {
		t.Skipf("this OS rejects colons in file names: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b", "iface", "2026", "10", "27", "raw", tcpMessageID+".raw")); err != nil {
		t.Fatalf("expected the colon name on disk: %v", err)
	}
}

func TestLocalDriver_BuiltByHandStillWorks(t *testing.T) {
	d := &LocalDriver{baseDir: t.TempDir()} // no codec set: must not panic
	if _, err := d.PutObject(context.Background(), "b", "k/plain.txt", strings.NewReader("x"), 1, ""); err != nil {
		t.Fatalf("put: %v", err)
	}
}
