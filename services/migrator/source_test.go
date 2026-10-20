package migrator

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFSSource_OrdersNumericallyAndParsesNames(t *testing.T) {
	fsys := fstest.MapFS{
		"m/V10__Ten.sql":              {Data: []byte("SELECT 10;")},
		"m/V2__Two_words.sql":         {Data: []byte("SELECT 2;")},
		"m/V1_1__Point_one.sql":       {Data: []byte("SELECT 1.1;")},
		"m/V21__Old.sql.bak":          {Data: []byte("ignored")},
		"m/README.md":                 {Data: []byte("ignored")},
		"m/sub/V99__Not_recursed.sql": {Data: []byte("SELECT 99;")},
	}
	migs, err := FSSource{FS: fsys, Dir: "m"}.Load()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range migs {
		got = append(got, m.Version.String()+"|"+m.Description)
	}
	want := "1.1|Point one,2|Two words,10|Ten"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v, want %s", got, want)
	}
}

func TestFSSource_RejectsAmbiguousFiles(t *testing.T) {
	cases := map[string]fstest.MapFS{
		"duplicate version":    {"m/V1__A.sql": {Data: []byte("x")}, "m/V1_0__B.sql": {Data: []byte("y")}},
		"repeatable migration": {"m/R__View.sql": {Data: []byte("x")}},
		"missing separator":    {"m/V1_Init.sql": {Data: []byte("x")}},
		"non-numeric version":  {"m/Vabc__X.sql": {Data: []byte("x")}},
		"stray sql file":       {"m/cleanup.sql": {Data: []byte("x")}},
	}
	for name, fsys := range cases {
		if _, err := (FSSource{FS: fsys, Dir: "m"}).Load(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestFSSource_LoadsEveryRealMigration(t *testing.T) {
	migs, err := FSSource{FS: os.DirFS("../../database"), Dir: "migrations"}.Load()
	if err != nil {
		t.Fatalf("real migrations must load cleanly: %v", err)
	}
	if len(migs) < 250 {
		t.Fatalf("loaded only %d migrations", len(migs))
	}
	for i := 1; i < len(migs); i++ {
		if migs[i-1].Version.Compare(migs[i].Version) >= 0 {
			t.Fatalf("not strictly ordered at %s, %s", migs[i-1].Script, migs[i].Script)
		}
	}
	for _, m := range migs {
		if want, ok := realFlywayChecksums[m.Script]; ok && m.Checksum != want {
			t.Errorf("%s checksum %d, want %d", m.Script, m.Checksum, want)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	v := func(s string) Version {
		x, err := ParseVersion(s)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	if v("2").Compare(v("10")) >= 0 || v("1.1").Compare(v("1.10")) >= 0 || v("1").Compare(v("1.0")) != 0 {
		t.Fatal("numeric version comparison is wrong")
	}
	if v("1.0").key() != v("1").key() {
		t.Fatal("1.0 and 1 must be the same version")
	}
}
