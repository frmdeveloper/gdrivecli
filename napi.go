//go:build napi

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	_ "unsafe"

	napi "sirherobrine23.com.br/Sirherobrine23/napi-go"
	_ "sirherobrine23.com.br/Sirherobrine23/napi-go/module"
)

func main() {}

// initDrive menjalankan getDriveService() di goroutine baru (luar CGo stack)
// agar encoding/json tidak crash saat stack growth di CGo thread.
func initDrive() (*DriveClient, error) {
	type result struct {
		srv *DriveClient
		err error
	}
	ch := make(chan result, 1)
	go func() {
		srv, err := getDriveService()
		ch <- result{srv, err}
	}()
	r := <-ch
	return r.srv, r.err
}

//go:linkname RegisterNapi sirherobrine23.com.br/Sirherobrine23/napi-go/module.Register
func RegisterNapi(env napi.EnvType, export *napi.Object) {
	srv, err := initDrive()
	if err != nil {
		panic(fmt.Sprintf("gdrive: %v", err))
	}

	bind := func(name string, fn any) {
		v, err := napi.GoFuncOf(env, fn)
		if err != nil {
			panic(fmt.Sprintf("gdrive: GoFuncOf %q: %v", name, err))
		}
		if err := export.Set(name, v); err != nil {
			panic(fmt.Sprintf("gdrive: export.Set %q: %v", name, err))
		}
	}

	bind("list", func(args ...string) ([]*DriveFile, error) {
		parent := "root"
		if len(args) > 0 && args[0] != "" {
			parent = args[0]
		}
		r, err := srv.ListFiles(parent, "")
		if err != nil {
			return nil, err
		}
		for _, f := range r.Files {
			f.IsDir = f.MimeType == folderMimeType
		}
		return r.Files, nil
	})

	bind("search", func(keyword string) ([]*DriveFile, error) {
		r, err := srv.SearchFiles(keyword)
		if err != nil {
			return nil, err
		}
		for _, f := range r.Files {
			f.IsDir = f.MimeType == folderMimeType
		}
		return r.Files, nil
	})

	bind("upload", func(localPath string, args ...string) (*DriveFile, error) {
		f, err := os.Open(localPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		var parents []string
		if len(args) > 0 && args[0] != "" {
			parents = []string{args[0]}
		}
		return srv.CreateFile(filepath.Base(localPath), parents, f, driveFields)
	})

	bind("download", func(fileID string, args ...string) (int64, error) {
		meta, err := srv.GetFile(fileID)
		if err != nil {
			return 0, err
		}
		destPath := meta.Name
		appendPdf := true
		if len(args) > 0 && args[0] != "" {
			destPath = args[0]
			appendPdf = false
		}
		_, written, err := downloadToPath(srv, fileID, meta.MimeType, destPath, appendPdf)
		return written, err
	})

	bind("mkdir", func(name string, args ...string) (*DriveFile, error) {
		var parents []string
		if len(args) > 0 && args[0] != "" {
			parents = []string{args[0]}
		}
		return srv.CreateFolder(name, parents)
	})

	bind("remove", func(fileID string) error {
		return srv.DeleteFile(fileID)
	})

	bind("info", func(fileID string) (*DriveFile, error) {
		return srv.GetFile(fileID)
	})

	bind("restore", func(folderID, localDir string) (map[string]int, error) {
		if err := os.MkdirAll(localDir, 0755); err != nil {
			return nil, err
		}
		ok, failed, err := restoreFolder(srv, folderID, localDir)
		return map[string]int{"ok": ok, "failed": failed}, err
	})

	bind("watch", func(localDir, folderID string) (*napi.Object, error) {
		stopCh := make(chan struct{})
		var once sync.Once

		go func() {
			if err := startWatch(srv, localDir, folderID, stopCh); err != nil {
				fmt.Fprintf(os.Stderr, "[gdrive/watch] %v\n", err)
			}
		}()

		stopFn, err := napi.GoFuncOf(env, func() {
			once.Do(func() { close(stopCh) })
		})
		if err != nil {
			once.Do(func() { close(stopCh) })
			return nil, err
		}

		wObj, err := napi.CreateObject(env)
		if err != nil {
			once.Do(func() { close(stopCh) })
			return nil, err
		}
		return wObj, wObj.Set("stop", stopFn)
	})
}
