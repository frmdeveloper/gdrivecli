package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"google.golang.org/api/drive/v3"
)

// debounce supaya editor yang tulis file beberapa kali cepat
// (vim, vscode, dll) tidak spam upload.
const debounceDelay = 300 * time.Millisecond

func cmdWatch(srv *drive.Service, args []string) {
	if len(args) < 2 {
		fmt.Println("Penggunaan: gdrive watch <local-dir> <drive-folder-id>")
		os.Exit(1)
	}

	fmt.Printf("👁️  Memantau %s -> Drive %s\n", args[0], args[1])
	fmt.Println("Tekan Ctrl+C untuk berhenti.")

	// stopCh = nil → select case-nya tidak pernah ready → jalan selamanya
	// sampai Ctrl+C mematikan proses (cocok untuk CLI).
	if err := startWatch(srv, args[0], args[1], nil); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// startWatch menjalankan watcher fsnotify hingga stopCh ditutup (atau
// selamanya kalau stopCh nil). Dipakai oleh cmdWatch (CLI) dan oleh
// Node.js addon (watch yang bisa di-stop dari JS).
func startWatch(srv *drive.Service, localDir, rootFolderID string, stopCh <-chan struct{}) error {
	if info, err := os.Stat(localDir); err != nil || !info.IsDir() {
		return fmt.Errorf("bukan folder yang valid: %s", localDir)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("gagal buat watcher: %w", err)
	}
	defer watcher.Close()

	// Daftarkan semua subfolder yang sudah ada ke watcher (fsnotify
	// tidak rekursif secara otomatis).
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

			// Abaikan file/folder hidden dan temp
			if strings.HasPrefix(filepath.Base(event.Name), ".") {
				continue
			}

			switch {
			// ── Folder baru → daftarkan ke watcher ──────────────────
			case event.Has(fsnotify.Create):
				if fi, statErr := os.Stat(event.Name); statErr == nil && fi.IsDir() {
					_ = watcher.Add(event.Name)
					continue
				}
				fallthrough // file baru → upload (lewat debounce Write)

			// ── File ditulis / dibuat → upload ──────────────────────
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

			// ── File dihapus / di-rename → hapus di Drive ───────────
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

// uploadFile upload (atau update kalau nama sudah ada) satu file ke folder
// Drive yang sesuai relPath.
func uploadFile(srv *drive.Service, cache *folderCache, localPath, relPath string) {
	relPath = filepath.ToSlash(relPath)
	name := filepath.Base(relPath)
	relDir := filepath.Dir(relPath)

	parentID, err := cache.ensureFolder(relDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] gagal siapkan folder Drive: %v\n", relPath, err)
		return
	}

	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", escapeQueryValue(name), parentID)
	existing, err := srv.Files.List().Q(q).Fields("files(id)").PageSize(1).Do()
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
		if _, err := srv.Files.Update(existing.Files[0].Id, &drive.File{}).Media(f).Do(); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] gagal update: %v\n", relPath, err)
			return
		}
		fmt.Printf("[update] %s\n", relPath)
		return
	}

	meta := &drive.File{Name: name, Parents: []string{parentID}}
	if _, err := srv.Files.Create(meta).Media(f).Do(); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] gagal upload: %v\n", relPath, err)
		return
	}
	fmt.Printf("[upload] %s\n", relPath)
}

// deleteFromDrive menghapus permanen file di Drive yang cocok dengan relPath.
func deleteFromDrive(srv *drive.Service, cache *folderCache, relPath string) {
	relPath = filepath.ToSlash(relPath)
	name := filepath.Base(relPath)
	relDir := filepath.Dir(relPath)

	parentID, ok := cache.lookupFolder(relDir)
	if !ok {
		return
	}

	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", escapeQueryValue(name), parentID)
	r, err := srv.Files.List().Q(q).Fields("files(id)").PageSize(1).Do()
	if err != nil || len(r.Files) == 0 {
		return
	}

	if err := srv.Files.Delete(r.Files[0].Id).Do(); err != nil {
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
	srv   *drive.Service
	cache map[string]string
}

func newFolderCache(srv *drive.Service, rootID string) *folderCache {
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

	q := fmt.Sprintf("name = '%s' and '%s' in parents and mimeType = '%s' and trashed = false",
		escapeQueryValue(name), parentID, folderMimeType)
	r, err := c.srv.Files.List().Q(q).Fields("files(id)").PageSize(1).Do()
	if err != nil {
		return "", err
	}

	var id string
	if len(r.Files) > 0 {
		id = r.Files[0].Id
	} else {
		created, createErr := c.srv.Files.Create(&drive.File{
			Name:     name,
			MimeType: folderMimeType,
			Parents:  []string{parentID},
		}).Fields("id").Do()
		if createErr != nil {
			return "", createErr
		}
		id = created.Id
		fmt.Printf("[mkdir] %s\n", relPath)
	}

	c.cache[relPath] = id
	return id, nil
}
