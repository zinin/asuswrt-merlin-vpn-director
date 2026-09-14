package platform

import (
	"os"
	"path/filepath"
	"testing"
)

// tree builds a fake root: dirs are created, files are created executable.
func tree(t *testing.T, dirs, execs []string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range execs {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDetectAt(t *testing.T) {
	// DetectAt is the file-system probe. An env override must not leak into it.
	t.Setenv(EnvVar, Merlin)

	cases := []struct {
		name  string
		dirs  []string
		execs []string
		want  string
		fails bool
	}{
		{name: "keenetic", dirs: []string{"opt/etc/ndm"}, execs: []string{"bin/ndmc"}, want: Keenetic},
		{name: "merlin", dirs: []string{"jffs"}, execs: []string{"bin/nvram"}, want: Merlin},
		{name: "keenetic wins when both trees exist", dirs: []string{"opt/etc/ndm", "jffs"}, execs: []string{"bin/ndmc", "bin/nvram"}, want: Keenetic},
		{name: "a directory without its binary is nothing", dirs: []string{"opt/etc/ndm", "jffs"}, fails: true},
		{name: "a binary without its directory is nothing", execs: []string{"bin/ndmc", "bin/nvram"}, fails: true},
		{name: "empty root", fails: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DetectAt(tree(t, tc.dirs, tc.execs))
			if tc.fails {
				if err == nil {
					t.Fatalf("DetectAt() = %+v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DetectAt() error: %v", err)
			}
			want, werr := ForName(tc.want)
			if werr != nil {
				t.Fatal(werr)
			}
			if got != want {
				t.Errorf("DetectAt() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestDetectAt_EmptyRootProbesSlashNotCwd(t *testing.T) {
	t.Chdir(tree(t, []string{"opt/etc/ndm"}, []string{"bin/ndmc"}))

	empty, errEmpty := DetectAt("")
	slash, errSlash := DetectAt("/")
	if (errEmpty == nil) != (errSlash == nil) || (errEmpty == nil && empty != slash) {
		t.Fatalf("DetectAt(\"\") = %+v, %v; DetectAt(\"/\") = %+v, %v; empty root must probe /",
			empty, errEmpty, slash, errSlash)
	}
	if errEmpty == nil && empty.Name == Keenetic {
		if _, err := os.Stat("/opt/etc/ndm"); err != nil {
			t.Fatal("DetectAt(\"\") returned keenetic from the working directory")
		}
	}
}

func TestForName_PasswordFiles(t *testing.T) {
	m, err := ForName(Merlin)
	if err != nil || m.Name != Merlin || m.PasswordFile != "/etc/shadow" {
		t.Errorf("ForName(merlin) = %+v, %v; want name merlin and /etc/shadow", m, err)
	}
	k, err := ForName(Keenetic)
	if err != nil || k.Name != Keenetic || k.PasswordFile != "/opt/etc/passwd" {
		t.Errorf("ForName(keenetic) = %+v, %v; want name keenetic and /opt/etc/passwd", k, err)
	}
	if _, err := ForName("amiga"); err == nil {
		t.Error("ForName(amiga) accepted an unknown platform")
	}
}

func TestDetect_HonoursTheEnvironment(t *testing.T) {
	t.Setenv(EnvVar, Keenetic)
	// Empty probe root: on this machine detection would fail, so an answer proves
	// the variable was used.
	t.Setenv("VPD_PROBE_ROOT", t.TempDir())
	p, err := Detect()
	if err != nil || p.Name != Keenetic {
		t.Fatalf("Detect() = %+v, %v; want keenetic from %s", p, err, EnvVar)
	}
	t.Setenv(EnvVar, "amiga")
	if _, err := Detect(); err == nil {
		t.Error("Detect() accepted VPD_PLATFORM=amiga")
	}
}

func TestDetect_ProbesUnderTheProbeRoot(t *testing.T) {
	// Empty is unset: Detect must not treat VPD_PLATFORM="" as a name.
	t.Setenv(EnvVar, "")
	t.Setenv("VPD_PROBE_ROOT", tree(t, []string{"jffs"}, []string{"bin/nvram"}))
	p, err := Detect()
	if err != nil || p.Name != Merlin {
		t.Fatalf("Detect() = %+v, %v; want merlin from the probe root", p, err)
	}
}

func TestResolve(t *testing.T) {
	t.Setenv(EnvVar, "")
	t.Setenv("VPD_PROBE_ROOT", t.TempDir()) // nothing detectable
	if p, err := Resolve(Keenetic, false); err != nil || p.Name != Keenetic {
		t.Errorf("Resolve(flag) = %+v, %v; want keenetic", p, err)
	}
	if p, err := Resolve("", true); err != nil || p.Name != Merlin {
		t.Errorf("Resolve(dev) = %+v, %v; want merlin", p, err)
	}
	if _, err := Resolve("", false); err == nil {
		t.Error("Resolve() detected a platform on an empty root")
	}
	if _, err := Resolve("amiga", true); err == nil {
		t.Error("Resolve(amiga) accepted an unknown platform")
	}
}

// lib/platform.sh takes VPD_PLATFORM as its first rule, and the daemons run
// vpn-director.sh as a child process with the environment they inherited. A
// --platform honoured only on the Go side left /api/platform and every routing
// command on the other firmware's implementation.
func TestExport_PublishesTheDecisionToChildProcesses(t *testing.T) {
	t.Setenv(EnvVar, Merlin) // what the daemon inherited, and must not win

	p, err := Resolve(Keenetic, false)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if p.Name != Keenetic {
		t.Fatalf("Resolve() = %q, want %q", p.Name, Keenetic)
	}
	if err := p.Export(); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if got := os.Getenv(EnvVar); got != Keenetic {
		t.Errorf("%s = %q, want %q so the shell agrees with the daemon", EnvVar, got, Keenetic)
	}
}
