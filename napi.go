//go:build napi

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	_ "unsafe"

	napi "sirherobrine23.com.br/Sirherobrine23/napi-go"
	"sirherobrine23.com.br/Sirherobrine23/napi-go/js"
	_ "sirherobrine23.com.br/Sirherobrine23/napi-go/module"

	"google.golang.org/api/drive/v3"
)

func main() {}

// DriveFile adalah struktur file/folder yang dikembalikan ke JavaScript.
type DriveFile struct {
	Id           string   `json:"id"`
	Name         string   `json:"name"`
	MimeType     string   `json:"mimeType"`
	Size         int64    `json:"size"`
	IsDir        bool     `json:"isDir"`
	CreatedTime  string   `json:"createdTime,omitempty"`
	ModifiedTime string   `json:"modifiedTime,omitempty"`
	WebViewLink  string   `json:"webViewLink,omitempty"`
	Parents      []string `json:"parents,omitempty"`
}

func toFile(f *drive.File) *DriveFile {
	return &DriveFile{
		Id:           f.Id,
		Name:         f.Name,
		MimeType:     f.MimeType,
		Size:         f.Size,
		IsDir:        f.MimeType == folderMimeType,
		CreatedTime:  f.CreatedTime,
		ModifiedTime: f.ModifiedTime,
		WebViewLink:  f.WebViewLink,
		Parents:      f.Parents,
	}
}

const driveFields = "id, name, mimeType, size, createdTime, modifiedTime, parents, webViewLink"

//go:linkname RegisterNapi sirherobrine23.com.br/Sirherobrine23/napi-go/entry.Register
func RegisterNapi(env napi.EnvType, export *napi.Object) {
	srv, err := getDriveService()
	if err != nil {
		panic(fmt.Sprintf("gdrive: gagal init Drive service: %v", err))
	}

	bind := func(name string, fn any) {
		val, err := js.GoFuncOf(env, fn)
		if err != nil {
			panic(fmt.Sprintf("gdrive: gagal bind %q: %v", name, err))
		}
		if err := export.Set(name, val); err != nil {
			panic(fmt.Sprintf("gdrive: gagal export %q: %v", name, err))
		}
	}

	// list(folderId?: string): DriveFile[]
	bind("list", func(args ...string) ([]*DriveFile, error) {
		parent := "root"
		if len(args) > 0 && args[0] != "" {
			parent = args[0]
		}
		r, err := srv.Files.List().
			Q(fmt.Sprintf("'%s' in parents and trashed = false", parent)).
			PageSize(100).OrderBy("folder,name").
			Fields("files(" + driveFields + ")").Do()
		if err != nil {
			return nil, err
		}
		out := make([]*DriveFile, len(r.Files))
		for i, f := range r.Files {
			out[i] = toFile(f)
		}
		return out, nil
	})

	// search(keyword: string): DriveFile[]
	bind("search", func(keyword string) ([]*DriveFile, error) {
		r, err := srv.Files.List().
			Q(fmt.Sprintf("name contains '%s' and trashed = false",
				strings.ReplaceAll(keyword, "'", "\\'"))).
			PageSize(50).OrderBy("folder,name").
			Fields("files(" + driveFields + ")").Do()
		if err != nil {
			return nil, err
		}
		out := make([]*DriveFile, len(r.Files))
		for i, f := range r.Files {
			out[i] = toFile(f)
		}
		return out, nil
	})

	// upload(localPath: string, folderId?: string): DriveFile
	bind("upload", func(localPath string, args ...string) (*DriveFile, error) {
		f, err := os.Open(localPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		meta := &drive.File{Name: filepath.Base(localPath)}
		if len(args) > 0 && args[0] != "" {
			meta.Parents = []string{args[0]}
		}
		uploaded, err := srv.Files.Create(meta).Media(f).
			Fields(driveFields).Do()
		if err != nil {
			return nil, err
		}
		return toFile(uploaded), nil
	})

	// download(fileId: string, outputPath?: string): number  (bytes written)
	bind("download", func(fileId string, args ...string) (int64, error) {
		meta, err := srv.Files.Get(fileId).Fields("name, mimeType").Do()
		if err != nil {
			return 0, err
		}
		destPath := meta.Name
		appendPdf := strings.HasPrefix(meta.MimeType, "application/vnd.google-apps.")
		if len(args) > 0 && args[0] != "" {
			destPath = args[0]
			appendPdf = false
		}
		_, written, err := downloadToPath(srv, fileId, meta.MimeType, destPath, appendPdf)
		return written, err
	})

	// mkdir(name: string, parentId?: string): DriveFile
	bind("mkdir", func(name string, args ...string) (*DriveFile, error) {
		meta := &drive.File{Name: name, MimeType: folderMimeType}
		if len(args) > 0 && args[0] != "" {
			meta.Parents = []string{args[0]}
		}
		created, err := srv.Files.Create(meta).Fields(driveFields).Do()
		if err != nil {
			return nil, err
		}
		return toFile(created), nil
	})

	// remove(fileId: string): void
	// (tidak pakai nama "delete" karena reserved keyword di JS)
	bind("remove", func(fileId string) error {
		return srv.Files.Delete(fileId).Do()
	})

	// info(fileId: string): DriveFile
	bind("info", func(fileId string) (*DriveFile, error) {
		f, err := srv.Files.Get(fileId).Fields(driveFields).Do()
		if err != nil {
			return nil, err
		}
		return toFile(f), nil
	})

	// restore(folderId: string, localDir: string): { ok: number, failed: number }
	bind("restore", func(folderId, localDir string) (map[string]int, error) {
		if err := os.MkdirAll(localDir, 0755); err != nil {
			return nil, err
		}
		ok, failed, err := restoreFolder(srv, folderId, localDir)
		return map[string]int{"ok": ok, "failed": failed}, err
	})

	// watch(localDir: string, folderId: string): { stop(): void }
	// Jalan di goroutine terpisah; panggil stop() dari JS untuk berhenti.
	bind("watch", func(localDir, folderId string) (map[string]any, error) {
		stopCh := make(chan struct{})
		var once sync.Once

		go func() {
			if err := startWatch(srv, localDir, folderId, stopCh); err != nil {
				fmt.Fprintf(os.Stderr, "[gdrive/watch] %v\n", err)
			}
		}()

		return map[string]any{
			"stop": func() {
				once.Do(func() { close(stopCh) })
			},
		}, nil
	})
}
