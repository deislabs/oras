/*
Copyright The ORAS Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package status

import (
	"strconv"
	"testing"

	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content/memory"
	"oras.land/oras/internal/testutils"
)

type testGraphTarget struct {
	oras.GraphTarget
}

func TestTTYPushHandler_TrackTarget(t *testing.T) {
	// prepare pty
	_, child, err := testutils.NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Close() }()
	ph := NewTTYPushHandler(child, mockFetcher.Fetcher)
	store := memory.New()
	// test
	_, fn, err := ph.TrackTarget(store)
	if err != nil {
		t.Fatal("TrackTarget() should not return an error")
	}
	defer func() {
		if err := fn(); err != nil {
			t.Fatal(err)
		}
	}()
	if ttyPushHandler, ok := ph.(*TTYPushHandler); !ok {
		t.Fatalf("TrackTarget() should return a *TTYPushHandler, got %T", ttyPushHandler)
	}
}

func Test_TTYPullHandler_TrackTarget(t *testing.T) {
	src := memory.New()
	t.Run("has TTY", func(t *testing.T) {
		_, device, err := testutils.NewPipe()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = device.Close() }()
		ph := NewTTYPullHandler(device)
		got, fn, err := ph.TrackTarget(src)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := fn(); err != nil {
				t.Fatal(err)
			}
		}()
		if got == src {
			t.Fatal("GraphTarget not be modified on TTY")
		}
	})

	t.Run("invalid TTY", func(t *testing.T) {
		ph := NewTTYPullHandler(nil)

		if _, _, err := ph.TrackTarget(src); err == nil {
			t.Fatal("expected error for no tty but got nil")
		}
	})
}

func TestTTYCopyHandler_OnMounted(t *testing.T) {
	reader, child, err := testutils.NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Close() }()
	ch := NewTTYCopyHandler(child, nil)
	_, err = ch.StartTracking(&testGraphTarget{memory.New()})
	if err != nil {
		t.Fatal(err)
	}

	if err = ch.OnMounted(ctx, mockFetcher.OciImage); err != nil {
		t.Fatalf("OnMounted() should not return an error: %v", err)
	}

	if err = ch.StopTracking(); err != nil {
		t.Fatalf("StopTracking() should not return an error: %v", err)
	}

	if err = testutils.MatchPipe(reader, child, "✓", "Mounted", strconv.FormatInt(mockFetcher.OciImage.Size, 10), "100.00%", mockFetcher.OciImage.Digest.String()); err != nil {
		t.Fatal(err)
	}
}

func TestTTYCopyHandler_OnCopySkipped(t *testing.T) {
	reader, child, err := testutils.NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Close() }()
	ch := NewTTYCopyHandler(child, nil)
	_, err = ch.StartTracking(&testGraphTarget{memory.New()})
	if err != nil {
		t.Fatal(err)
	}

	if err = ch.OnCopySkipped(ctx, mockFetcher.OciImage); err != nil {
		t.Errorf("OnCopySkipped() should not return an error: %v", err)
	}

	if err = ch.StopTracking(); err != nil {
		t.Errorf("StopTracking() should not return an error: %v", err)
	}
	if err = testutils.MatchPipe(reader, child, "Exists", "oci-image", strconv.FormatInt(mockFetcher.OciImage.Size, 10), "100.00%"); err != nil {
		t.Fatal(err)
	}
}

func TestTTYCopyHandler_PostCopy(t *testing.T) {
	reader, child, err := testutils.NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Close() }()
	store := &testGraphTarget{memory.New()}
	ch := NewTTYCopyHandler(child, store)
	_, err = ch.StartTracking(store)
	if err != nil {
		t.Fatal(err)
	}

	if ch.PostCopy(ctx, bogus) == nil {
		t.Error("PostCopy() should return an error")
	}

	if err = ch.StopTracking(); err != nil {
		t.Errorf("StopTracking() should not return an error: %v", err)
	}
	if err = testutils.MatchPipe(reader, child, "\x1b[?25l\x1b7\x1b[0m"); err != nil {
		t.Fatal(err)
	}
}

func TestTTYCopyHandler_PreCopy(t *testing.T) {
	reader, child, err := testutils.NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Close() }()
	ch := NewTTYCopyHandler(child, nil)
	_, err = ch.StartTracking(&testGraphTarget{memory.New()})
	if err != nil {
		t.Fatal(err)
	}

	if err = ch.PreCopy(ctx, mockFetcher.OciImage); err != nil {
		t.Errorf("PreCopy() should not return an error: %v", err)
	}

	if err = ch.StopTracking(); err != nil {
		t.Errorf("StopTracking() should not return an error: %v", err)
	}
	if err = testutils.MatchPipe(reader, child, "\x1b[?25l\x1b7\x1b[0m"); err != nil {
		t.Fatal(err)
	}
}

func TestTTYBlobPushHandler_StartStopTracking(t *testing.T) {
	_, child, err := testutils.NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Close() }()
	bh := NewTTYBlobPushHandler(child, mockFetcher.OciImage)
	if _, err := bh.StartTracking(memory.New()); err != nil {
		t.Fatalf("StartTracking() error = %v", err)
	}
	if err := bh.StopTracking(); err != nil {
		t.Fatalf("StopTracking() error = %v", err)
	}
}
