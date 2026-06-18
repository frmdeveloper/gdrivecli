package main

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounceDelay = 300 * time.Millisecond

func cmdWatch(srv *DriveClient, args []string) {
	if len(args) < 2 {
		fmt.Println("Penggunaan: gdrive watch <local-dir> <drive-folder-id>")
		os.Exit(1)
	}

	fmt.Printf("👁️  Memantau %s -> Drive %s\n", args[0], args[1])
	fmt.Println("Tekan Ctrl+C untuk berhenti.")

	if err := startWatch(srv, args[0], args[1], nil); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func startWatch(srv *DriveClient, localDir, rootFolderID string, stopCh <-chan struct{}) error {
	if info, err := os.Stat(localDir); err != nil || !info.IsDir() {
		return fmt.Errorf("bukan folder yang valid: %s", localDir)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("gagal buat watcher: %w", err)
	}
	defer watcher.Close()

	_ = filepath.WalkDir(localDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || !d.IsDir() {
			return nil
		}
		return watcher.Add(path)
	})

	cache := newFolderCache(srv, rootFolderID)

	var mu sync.Mutex
	timers := make(map[string]*time.Timer)

	for {
		select {
		case <-stopCh:
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			if strings.HasPrefix(filepath.Base(event.Name), ".") {
				continue
			}

			switch {
			case event.Has(fsnotify.Create):
				if fi, statErr := os.Stat(event.Name); statErr == nil && fi.IsDir() {
					_ = watcher.Add(event.Name)
					continue
				}
				fallthrough

			case event.Has(fsnotify.Write):
				path := event.Name
				rel, relErr := filepath.Rel(localDir, path)
				if relErr != nil {
					continue
				}
				rel = filepath.ToSlash(rel)

				mu.Lock()
				if t, exists := timers[path]; exists {
					t.Stop()
				}
				timers[path] = time.AfterFunc(debounceDelay, func() {
					if fi, statErr := os.Stat(path); statErr == nil && !fi.IsDir() {
						uploadFile(srv, cache, path, rel)
					}
					mu.Lock()
					delete(timers, path)
					mu.Unlock()
				})
				mu.Unlock()

			case event.Has(fsnotify.Remove), event.Has(fsnotify.Rename):
				rel, relErr := filepath.Rel(localDir, event.Name)
				if relErr != nil {
					continue
				}
				go deleteFromDrive(srv, cache, filepath.ToSlash(rel))
			}

		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintln(os.Stderr, "watcher error:", watchErr)
		}
	}
}

func uploadFile(srv *DriveClient, cache *folderCache, localPath, relPath string) {
	relPath = filepath.ToSlash(relPath)
	name := filepath.Base(relPath)
	relDir := filepath.Dir(relPath)

	parentID, err := cache.ensureFolder(relDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] gagal siapkan folder Drive: %v\n", relPath, err)
		return
	}

	// Cek apakah file sudah ada di Drive
	params := url.Values{}
	params.Set("q", fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false",
		escapeQueryValue(name), parentID))
	params.Set("fields", "files(id)")
	params.Set("pageSize", "1")

	existing, err := srv.ListFilesRaw(params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] gagal cek Drive: %v\n", relPath, err)
		return
	}

	f, err := os.Open(localPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] gagal buka file: %v\n", relPath, err)
		return
	}
	defer f.Close()

	if len(existing.Files) > 0 {
		if err := srv.UpdateFile(existing.Files[0].Id, f); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] gagal update: %v\n", relPath, err)
			return
		}
		fmt.Printf("[update] %s\n", relPath)
		return
	}

	if _, err := srv.CreateFile(name, []string{parentID}, f, "id"); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] gagal upload: %v\n", relPath, err)
		return
	}
	fmt.Printf("[upload] %s\n", relPath)
}

func deleteFromDrive(srv *DriveClient, cache *folderCache, relPath string) {
	relPath = filepath.ToSlash(relPath)
	name := filepath.Base(relPath)
	relDir := filepath.Dir(relPath)

	parentID, ok := cache.lookupFolder(relDir)
	if !ok {
		return
	}

	params := url.Values{}
	params.Set("q", fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false",
		escapeQueryValue(name), parentID))
	params.Set("fields", "files(id)")
	params.Set("pageSize", "1")

	r, err := srv.ListFilesRaw(params)
	if err != nil || len(r.Files) == 0 {
		return
	}

	if err := srv.DeleteFile(r.Files[0].Id); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] gagal hapus di Drive: %v\n", relPath, err)
		return
	}
	fmt.Printf("[delete] %s\n", relPath)
}

func escapeQueryValue(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}

// folderCache memetakan path relatif lokal → Drive folder ID, lazy + cached.
type folderCache struct {
	mu    sync.Mutex
	srv   *DriveClient
	cache map[string]string
}

func newFolderCache(srv *DriveClient, rootID string) *folderCache {
	return &folderCache{srv: srv, cache: map[string]string{".": rootID}}
}

func (c *folderCache) lookupFolder(relPath string) (string, bool) {
	relPath = filepath.ToSlash(relPath)
	if relPath == "" {
		relPath = "."
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.cache[relPath]
	return id, ok
}

func (c *folderCache) ensureFolder(relPath string) (string, error) {
	relPath = filepath.ToSlash(relPath)
	if relPath == "." || relPath == "" {
		return c.cache["."], nil
	}

	c.mu.Lock()
	if id, ok := c.cache[relPath]; ok {
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	parentID, err := c.ensureFolder(filepath.Dir(relPath))
	if err != nil {
		return "", err
	}

	name := filepath.Base(relPath)

	c.mu.Lock()
	defer c.mu.Unlock()
	if id, ok := c.cache[relPath]; ok {
		return id, nil
	}

	params := url.Values{}
	params.Set("q", fmt.Sprintf("name = '%s' and '%s' in parents and mimeType = '%s' and trashed = false",
		escapeQueryValue(name), parentID, folderMimeType))
	params.Set("fields", "files(id)")
	params.Set("pageSize", "1")

	r, err := c.srv.ListFilesRaw(params)
	if err != nil {
		return "", err
	}

	var id string
	if len(r.Files) > 0 {
		id = r.Files[0].Id
	} else {
		created, createErr := c.srv.CreateFolder(name, []string{parentID})
		if createErr != nil {
			return "", createErr
		}
		id = created.Id
		fmt.Printf("[mkdir] %s\n", relPath)
	}

	c.cache[relPath] = id
	return id, nil
}
