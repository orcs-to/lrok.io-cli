// Package selfupdate replaces the running lrok binary with the latest
// release from GitHub. Cross-platform notes:
//
//   - Linux/macOS: os.Rename onto the running binary works because the OS
//     keeps the inode of the running process. The path now points at the
//     new file; the old inode is freed when the process exits.
//
//   - Windows: you can't delete or os.Rename-overwrite a running .exe, but
//     you CAN move the running .exe out of the way. Standard pattern is
//     rename current → current+".old", then move new → current. The .old
//     file lingers until the next process start, when we best-effort remove
//     it (see CleanupStaleOld).
//
// SHA-256 verification is mandatory: we pull `checksums.txt` from the same
// release and refuse to replace if the entry is missing or doesn't match.
// Without this, a compromised mirror could ship a backdoored binary.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CheckSumsURL builds the URL for the checksums.txt file at a given tag.
// Mirrors the layout in scripts/install.sh / install.ps1.
func CheckSumsURL(tag string) string {
	return fmt.Sprintf(
		"https://github.com/orcs-to/lrok.io-cli/releases/download/%s/checksums.txt",
		tag,
	)
}

// AssetName returns the bare filename of the current platform's binary
// asset (e.g. "lrok-linux-amd64", "lrok-windows-arm64.exe"). It must
// match the line in checksums.txt.
func AssetName() string {
	name := fmt.Sprintf("lrok-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// Apply downloads the release asset for `tag`, verifies its SHA-256 against
// the release's checksums.txt, and atomically replaces the running binary.
// On Windows it uses a rename-then-move dance and leaves a `.old` file that
// CleanupStaleOld can remove on next launch.
//
// Returns the resolved binary path on success.
func Apply(ctx context.Context, tag string, assetURL string) (string, error) {
	if assetURL == "" {
		return "", errors.New("no asset URL for this platform")
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate self: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks: %w", err)
	}

	// Sanity: refuse to replace something whose parent dir we can't write.
	dir := filepath.Dir(exePath)
	if err := canWriteDir(dir); err != nil {
		return "", fmt.Errorf("install dir not writable (%s) — re-run with sudo / admin: %w", dir, err)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Download checksums.txt and parse the line for our asset.
	expectedHex, err := fetchExpectedSum(ctx, tag, AssetName())
	if err != nil {
		return "", fmt.Errorf("checksums: %w", err)
	}

	// Download the binary into a temp file alongside the target so the
	// final rename is on the same filesystem (cheap + atomic on Unix).
	tmpFile, err := os.CreateTemp(dir, ".lrok-update-*")
	if err != nil {
		return "", fmt.Errorf("temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op if rename succeeded

	gotHex, err := downloadAndHash(ctx, assetURL, tmpFile)
	closeErr := tmpFile.Close()
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close temp: %w", closeErr)
	}

	if !strings.EqualFold(gotHex, expectedHex) {
		return "", fmt.Errorf("checksum mismatch (expected %s, got %s)", expectedHex, gotHex)
	}

	if runtime.GOOS != "windows" {
		// Make it executable. Mirror typical 0755.
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			return "", fmt.Errorf("chmod: %w", err)
		}
	}

	if err := atomicReplace(exePath, tmpPath); err != nil {
		return "", fmt.Errorf("replace: %w", err)
	}
	return exePath, nil
}

// CleanupStaleOld best-effort removes a `<exe>.old` left by a prior
// Windows update. Call this on every CLI startup; it's a no-op when
// nothing's there or the file is still in use.
func CleanupStaleOld() {
	if runtime.GOOS != "windows" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = os.Remove(exe + ".old")
}

func canWriteDir(dir string) error {
	probe, err := os.CreateTemp(dir, ".lrok-write-probe-*")
	if err != nil {
		return err
	}
	probePath := probe.Name()
	_ = probe.Close()
	_ = os.Remove(probePath)
	return nil
}

func fetchExpectedSum(ctx context.Context, tag, asset string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, CheckSumsURL(tag), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "lrok-cli")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MiB cap
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Format: "<sha256-hex>  <filename>" (two spaces per coreutils)
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[len(parts)-1] == asset {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("no entry for %q", asset)
}

func downloadAndHash(ctx context.Context, url string, dst io.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "lrok-cli")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dst, h), resp.Body); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// atomicReplace swaps `tmp` into `target`. On Unix, os.Rename is atomic
// and works on a running binary. On Windows, we move the running .exe to
// `<exe>.old` first (legal for a running .exe), then rename the new file
// into place. Best-effort revert on the second-step failure.
func atomicReplace(target, tmp string) error {
	if runtime.GOOS == "windows" {
		old := target + ".old"
		// If a previous update left `.old` in place (process still alive
		// then), os.Remove can fail — try, ignore the error, proceed.
		_ = os.Remove(old)
		if err := os.Rename(target, old); err != nil {
			return fmt.Errorf("move old aside: %w", err)
		}
		if err := os.Rename(tmp, target); err != nil {
			// Try to revert; don't mask the original error.
			_ = os.Rename(old, target)
			return fmt.Errorf("install new: %w", err)
		}
		return nil
	}
	return os.Rename(tmp, target)
}
