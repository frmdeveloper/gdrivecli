package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cmdRestore(srv *DriveClient, args []string) {
	if len(args) < 2 {
		fmt.Println("Penggunaan: gdrive restore <drive-folder-id> <local-dir>")
		os.Exit(1)
	}

	folderID := args[0]
	localDir := args[1]

	if err := os.MkdirAll(localDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "Gagal buat direktori:", err)
		os.Exit(1)
	}

	ok, failed, err := restoreFolder(srv, folderID, localDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error restore:", err)
		os.Exit(1)
	}

	fmt.Printf("Restore selesai: %d berhasil, %d gagal\n", ok, failed)
}

func restoreFolder(srv *DriveClient, folderID, localDir string) (ok, failed int, err error) {
	pageToken := ""
	for {
		r, e := srv.ListFiles(folderID, pageToken)
		if e != nil {
			return ok, failed, e
		}

		for _, f := range r.Files {
			localPath := filepath.Join(localDir, f.Name)

			if f.MimeType == folderMimeType {
				if mkErr := os.MkdirAll(localPath, 0755); mkErr != nil {
					fmt.Fprintf(os.Stderr, "[skip] %s: %v\n", f.Name, mkErr)
					failed++
					continue
				}
				subOk, subFailed, subErr := restoreFolder(srv, f.Id, localPath)
				ok += subOk
				failed += subFailed
				if subErr != nil {
					return ok, failed, subErr
				}
				continue
			}

			if strings.HasPrefix(f.MimeType, "application/vnd.google-apps.") {
				localPath += ".pdf"
			}

			_, written, dlErr := downloadToPath(srv, f.Id, f.MimeType, localPath, false)
			if dlErr != nil {
				fmt.Fprintf(os.Stderr, "[fail] %s: %v\n", f.Name, dlErr)
				failed++
				continue
			}

			fmt.Printf("[restore] %s (%d bytes)\n", localPath, written)
			ok++
		}

		if r.NextPageToken == "" {
			break
		}
		pageToken = r.NextPageToken
	}

	return ok, failed, nil
}
