// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package uploadertest

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const envInertDir = "UPLOADERTEST_INERT_DIR"

// TestCrashChildOutsideTestBinary hits a crash point the way a non-test
// binary would. A Go build tool is not available under Bazel, so the child is
// still a test binary and the seam stands in for testing.Testing.
func TestCrashChildOutsideTestBinary(t *testing.T) {
	if !CrashChild() {
		t.Skip("runs only as a crash child")
	}
	crashAt("armed", false)
	if err := os.WriteFile(filepath.Join(os.Getenv(envInertDir), "after"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCrashPointIsInertOutsideATestBinary(t *testing.T) {
	dir := t.TempDir()
	result := RunCrash(t, "TestCrashChildOutsideTestBinary", "armed", envInertDir+"="+dir)

	if result.Killed {
		t.Fatalf("a non-test binary was killed at an armed crash point\n%s", result.Output)
	}
	if _, err := os.Stat(filepath.Join(dir, "after")); err != nil {
		t.Errorf("work after the crash point did not run: %v", err)
	}
}

const envTempReport = "UPLOADERTEST_TEMP_REPORT"

// TestCrashChildTempDir is a crash child that makes a temporary directory
// and then dies, so its own cleanup never runs.
func TestCrashChildTempDir(t *testing.T) {
	if !CrashChild() {
		t.Skip("runs only as a crash child")
	}
	if err := os.WriteFile(os.Getenv(envTempReport), []byte(t.TempDir()), 0o600); err != nil {
		t.Fatal(err)
	}
	CrashPoint("made-temp")
}

func TestCrashChildTempDirIsInsideTheParentsTempDir(t *testing.T) {
	var childTemp string
	report := filepath.Join(t.TempDir(), "report")
	t.Run("run", func(t *testing.T) {
		result := RunCrash(t, "TestCrashChildTempDir", "made-temp", envTempReport+"="+report)
		if !result.Killed {
			t.Fatalf("child was not killed\n%s", result.Output)
		}
		data, err := os.ReadFile(report)
		if err != nil {
			t.Fatal(err)
		}
		childTemp = string(data)
		if _, err := os.Stat(childTemp); err != nil {
			t.Fatalf("child temp dir: %v", err)
		}
	})
	// The subtest's cleanup removed its temporary directories, which must
	// have held the child's.
	if _, err := os.Stat(childTemp); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("child temp dir %s after the parent cleanup: %v, want it removed", childTemp, err)
	}
}

// fatalRecorder stands in for a test so a Fatalf from RunCrash can be read.
type fatalRecorder struct {
	testing.TB
	message string
}

func (f *fatalRecorder) Helper() {}

func (f *fatalRecorder) Fatalf(format string, args ...any) {
	f.message = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

func TestRunCrashReportsAChildThatCannotStart(t *testing.T) {
	original := childBinary
	t.Cleanup(func() { childBinary = original })
	childBinary = filepath.Join(t.TempDir(), "missing-binary")

	recorder := &fatalRecorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunCrash(recorder, "TestCrashChildTempDir", "made-temp")
	}()
	<-done
	if !strings.Contains(recorder.message, "start crash child") {
		t.Fatalf("RunCrash failure = %q, want the start error", recorder.message)
	}
}
