package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
)

const defaultPollInterval = 1 * time.Second

// fileState menyimpan info untuk mendeteksi perubahan: kalau mtime ATAU
// ukuran berubah dibanding scan sebelumnya, file dianggap berubah dan
// di-upload ulang.
type fileState struct {
	modTime time.Time
	size    int64
}

func cmdWatch(srv *drive.Service, args []string) {
	if len(args) < 2 {
		fmt.Println("Penggunaan: gdrive watch <local-dir> <drive-folder-id> [interval]")
		fmt.Println("Contoh interval: 1s (default), 500ms, 200ms, 2s")
		os.Exit(1)
	}
	localDir := args[0]
	rootFolderID := args[1]

	interval := defaultPollInterval
	if len(args) > 2 {
		d, err := time.ParseDuration(args[2])
		if err != nil || d <= 0 {
			fmt.Fprintln(os.Stderr, "Error: interval tidak valid, contoh: 1s, 500ms, 200ms")
			os.Exit(1)
		}
		interval = d
	}

	info, err := os.Stat(localDir)
	if err != nil || !info.IsDir() {
		fmt.Fprintln(os.Stderr, "Error: bukan folder yang valid:", localDir)
		os.Exit(1)
	}

	cache := newFolderCache(srv, rootFolderID)
	known := make(map[string]fileState)

	fmt.Printf("Memantau %s -> Drive folder %s (cek tiap %s)\n", localDir, rootFolderID, interval)
	fmt.Println("Tekan Ctrl+C untuk berhenti.")

	for {
		scanOnce(srv, cache, localDir, known)
		time.Sleep(interval)
	}
}

// scanOnce memindai folder lokal sekali: file baru/berubah (mtime atau
// ukuran beda) di-upload/update ke Drive, dan file yang sebelumnya
// terpantau tapi sekarang sudah tidak ada lagi ikut dihapus dari Drive.
//
// Pakai polling (bukan fsnotify) karena di Termux/Android, folder yang
// dipasang lewat shared storage (~/storage/...) memakai FUSE dan biasanya
// tidak meneruskan event inotify -- watcher berbasis event jadi tidak
// mendeteksi perubahan sama sekali. Polling jalan di semua jenis filesystem.
func scanOnce(srv *drive.Service, cache *folderCache, localDir string, known map[string]fileState) {
	current := make(map[string]fileState)

	_ = filepath.WalkDir(localDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // lewati entry yang error dibaca, lanjut scan yang lain
		}
		if path != localDir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}

		rel, relErr := filepath.Rel(localDir, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		state := fileState{modTime: info.ModTime(), size: info.Size()}
		current[rel] = state

		old, seen := known[rel]
		if !seen || !old.modTime.Equal(state.modTime) || old.size != state.size {
			uploadFile(srv, cache, path, rel)
		}
		return nil
	})

	// File yang dulu ada (known) tapi sekarang tidak terlihat lagi di scan
	// -> dianggap dihapus secara lokal, hapus juga di Drive.
	for rel := range known {
		if _, stillExists := current[rel]; !stillExists {
			deleteFromDrive(srv, cache, rel)
		}
	}

	for k := range known {
		delete(known, k)
	}
	for k, v := range current {
		known[k] = v
	}
}

// uploadFile upload (atau update kalau nama sudah ada) satu file ke folder
// Drive yang sesuai relPath, membuat sub-folder Drive sesuai struktur lokal
// kalau perlu.
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

// deleteFromDrive menghapus (permanen) file di Drive yang namanya cocok
// dengan relPath, kalau ada. Dipanggil saat file lokal yang sebelumnya
// terpantau sudah tidak ada lagi.
func deleteFromDrive(srv *drive.Service, cache *folderCache, relPath string) {
	relPath = filepath.ToSlash(relPath)
	name := filepath.Base(relPath)
	relDir := filepath.Dir(relPath)

	parentID, ok := cache.lookupFolder(relDir)
	if !ok {
		// Folder induknya belum pernah dibuat di Drive, berarti file ini
		// juga belum pernah ter-upload -> tidak ada yang perlu dihapus.
		return
	}

	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", escapeQueryValue(name), parentID)
	r, err := srv.Files.List().Q(q).Fields("files(id)").PageSize(1).Do()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] gagal cek Drive sebelum hapus: %v\n", relPath, err)
		return
	}
	if len(r.Files) == 0 {
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

// folderCache memetakan path relatif lokal ("." = root) -> Drive folder ID,
// membuat folder Drive baru (atau memakai yang sudah ada by name) secara
// lazy dan menyimpannya di cache supaya tidak query berulang.
type folderCache struct {
	srv   *drive.Service
	cache map[string]string
}

func newFolderCache(srv *drive.Service, rootID string) *folderCache {
	return &folderCache{srv: srv, cache: map[string]string{".": rootID}}
}

// lookupFolder mengembalikan Drive folder ID untuk relPath HANYA kalau
// sudah ada di cache (tidak membuat folder baru). Dipakai sebelum hapus,
// supaya tidak bikin folder kosong cuma untuk dicek isinya.
func (c *folderCache) lookupFolder(relPath string) (string, bool) {
	relPath = filepath.ToSlash(relPath)
	if relPath == "" {
		relPath = "."
	}
	id, ok := c.cache[relPath]
	return id, ok
}

func (c *folderCache) ensureFolder(relPath string) (string, error) {
	relPath = filepath.ToSlash(relPath)
	if relPath == "." || relPath == "" {
		return c.cache["."], nil
	}

	if id, ok := c.cache[relPath]; ok {
		return id, nil
	}

	parentID, err := c.ensureFolder(filepath.Dir(relPath))
	if err != nil {
		return "", err
	}

	name := filepath.Base(relPath)

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
		fmt.Printf("[mkdir] %s (folder baru di Drive)\n", relPath)
	}

	c.cache[relPath] = id
	return id, nil
}
