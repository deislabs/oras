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

package text

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"oras.land/oras/cmd/oras/internal/output"
)

// TestNewRestoreHandler tests the constructor for RestoreHandler
func TestNewRestoreHandler(t *testing.T) {
	tests := []struct {
		name   string
		dryRun bool
	}{
		{
			name:   "with dryRun false",
			dryRun: false,
		},
		{
			name:   "with dryRun true",
			dryRun: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(&bytes.Buffer{}, &bytes.Buffer{})
			handler := NewRestoreHandler(printer, tt.dryRun)
			if handler.printer != printer {
				t.Errorf("expected handler.printer to be %v, got %v", printer, handler.printer)
			}
			if handler.dryRun != tt.dryRun {
				t.Errorf("expected handler.dryRun to be %v, got %v", tt.dryRun, handler.dryRun)
			}
		})
	}
}

func TestRestoreHandler_OnTarLoaded(t *testing.T) {
	path := "backup.tar"
	tests := []struct {
		name    string
		out     io.Writer
		size    int64
		wantErr bool
		want    string
	}{
		{
			name:    "good path with 1MiB size",
			out:     &bytes.Buffer{},
			size:    int64(1024 * 1024),
			wantErr: false,
			want:    fmt.Sprintf("Loaded backup archive: %s (1 MB)\n", path),
		},
		{
			name:    "good path with 0 size",
			out:     &bytes.Buffer{},
			size:    0,
			wantErr: false,
			want:    fmt.Sprintf("Loaded backup archive: %s (0  B)\n", path),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			handler := NewRestoreHandler(printer, false)
			if err := handler.OnTarLoaded(path, tt.size); (err != nil) != tt.wantErr {
				t.Errorf("OnTarLoaded() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				got := tt.out.(*bytes.Buffer).String()
				if got != tt.want {
					t.Errorf("OnTarLoaded() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestRestoreHandler_OnTagsFound(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		out     io.Writer
		wantErr bool
		want    string
	}{
		{
			name:    "good path with a few tags",
			tags:    []string{"v1.0", "latest", "stable"},
			out:     &bytes.Buffer{},
			wantErr: false,
			want:    "Found 3 tag(s) in the backup: v1.0, latest, stable\n",
		},
		{
			name:    "good path with exactly 5 tags",
			tags:    []string{"v1.0", "v2.0", "latest", "stable", "beta"},
			out:     &bytes.Buffer{},
			wantErr: false,
			want:    "Found 5 tag(s) in the backup: v1.0, v2.0, latest, stable, beta\n",
		},
		{
			name:    "good path with more than 5 tags",
			tags:    []string{"v1.0", "v2.0", "v3.0", "latest", "stable", "beta", "dev"},
			out:     &bytes.Buffer{},
			wantErr: false,
			want:    "Found 7 tag(s) in the backup:\nv1.0\nv2.0\nv3.0\nlatest\nstable\nbeta\ndev\n",
		},
		{
			name:    "error writing heading",
			tags:    []string{"v1", "v2", "v3", "v4", "v5", "v6"},
			out:     &errorWriter{},
			wantErr: true,
		},
		{
			name:    "error writing tag",
			tags:    []string{"v1", "v2", "v3", "v4", "v5", "v6"},
			out:     &failAfterWriter{failAfter: 1},
			wantErr: true,
		},
		{
			name:    "good path with one tag",
			tags:    []string{"latest"},
			out:     &bytes.Buffer{},
			wantErr: false,
			want:    "Found 1 tag(s) in the backup: latest\n",
		},
		{
			name:    "good path with no tags",
			tags:    []string{},
			out:     &bytes.Buffer{},
			wantErr: false,
			want:    "No tags found in the backup\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			handler := NewRestoreHandler(printer, false)
			if err := handler.OnTagsFound(tt.tags); (err != nil) != tt.wantErr {
				t.Fatalf("OnTagsFound() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				got := tt.out.(*bytes.Buffer).String()
				if got != tt.want {
					t.Errorf("OnTagsFound() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestRestoreHandler_OnArtifactPushed(t *testing.T) {
	tag := "latest"
	referrerCount := 2
	tests := []struct {
		name    string
		dryRun  bool
		out     io.Writer
		wantErr bool
		want    string
	}{
		{
			name:    "normal push",
			dryRun:  false,
			out:     &bytes.Buffer{},
			wantErr: false,
			want:    fmt.Sprintf("Pushed tag %s with %d referrer(s)\n", tag, referrerCount),
		},
		{
			name:    "dry run",
			dryRun:  true,
			out:     &bytes.Buffer{},
			wantErr: false,
			want:    fmt.Sprintf("Dry run: would push tag %s with %d referrer(s)\n", tag, referrerCount),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			handler := NewRestoreHandler(printer, tt.dryRun)
			if err := handler.OnArtifactPushed(tag, referrerCount); (err != nil) != tt.wantErr {
				t.Errorf("OnArtifactPushed() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				got := tt.out.(*bytes.Buffer).String()
				if got != tt.want {
					t.Errorf("OnArtifactPushed() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestRestoreHandler_OnRestoreCompleted(t *testing.T) {
	tagsCount := 5
	repo := "example.com/myrepo"
	duration := time.Second * 3
	tests := []struct {
		name    string
		dryRun  bool
		out     io.Writer
		wantErr bool
		want    string
	}{
		{
			name:    "normal completion",
			dryRun:  false,
			out:     &bytes.Buffer{},
			wantErr: false,
			want:    fmt.Sprintf("Successfully restored %d tag(s) to %q in %s\n", tagsCount, repo, "3s"),
		},
		{
			name:    "dry run completion",
			dryRun:  true,
			out:     &bytes.Buffer{},
			wantErr: false,
			want:    fmt.Sprintf("Dry run complete: %d tag(s) would be restored to %q (no data pushed)\n", tagsCount, repo),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			handler := NewRestoreHandler(printer, tt.dryRun)
			if err := handler.OnRestoreCompleted(tagsCount, repo, duration); (err != nil) != tt.wantErr {
				t.Errorf("OnRestoreCompleted() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				got := tt.out.(*bytes.Buffer).String()
				if got != tt.want {
					t.Errorf("OnRestoreCompleted() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestRestoreHandler_Render(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "good path",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &RestoreHandler{
				printer: output.NewPrinter(&bytes.Buffer{}, &bytes.Buffer{}),
				dryRun:  false,
			}
			if err := handler.Render(); (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
