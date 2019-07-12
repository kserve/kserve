// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/gotooltest"
	"github.com/rogpeppe/go-internal/internal/os/execpath"
	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"testscript": main1,
	}))
}

func TestScripts(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("need go in PATH for these tests")
	}

	var stderr bytes.Buffer
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to run %v: %v\n%s", strings.Join(cmd.Args, " "), err, stderr.String())
	}
	gomod := string(out)

	if gomod == "" {
		t.Fatalf("apparently we are not running in module mode?")
	}

	p := testscript.Params{
		Dir: "testdata",
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars,
				"GOINTERNALMODPATH="+filepath.Dir(gomod),
			)
			return nil
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"dropgofrompath": dropgofrompath,
			"setfilegoproxy": setfilegoproxy,
		},
	}
	if err := gotooltest.Setup(&p); err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, p)
}

func dropgofrompath(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("unsupported: ! dropgofrompath")
	}
	var newPath []string
	for _, d := range filepath.SplitList(ts.Getenv("PATH")) {
		getenv := func(k string) string {
			if k == "PATH" {
				return d
			}
			return ts.Getenv(k)
		}
		if _, err := execpath.Look("go", getenv); err != nil {
			newPath = append(newPath, d)
		}
	}
	ts.Setenv("PATH", strings.Join(newPath, string(filepath.ListSeparator)))
}

func setfilegoproxy(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("unsupported: ! setfilegoproxy")
	}
	path := args[0]
	path = filepath.ToSlash(path)
	// probably sufficient to just handle spaces
	path = strings.Replace(path, " ", "%20", -1)
	if runtime.GOOS == "windows" {
		path = "/" + path
	}
	ts.Setenv("GOPROXY", "file://"+path)
}
