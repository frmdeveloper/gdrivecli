package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/drive/v3"
)

func cmdRestore(srv *drive.Service, args []string) {
	if len(args) < 2 {
		fmt.Println("Penggunaan: gdrive restore <folder-id> <local-dir>")
		os.Exit(1)
	}
	folderID := args[0]
	localDir := args[1]

	if err := os.MkdirAll(localDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "Error buat folder lokal:", err)
		os.Exit(1)
	}

	ok, failed, err := restoreFolder(srv, folderID, localDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fmt.Printf("\nSelesai: %d file ter-restore ke %s", ok, localDir)
	if failed > 0 {
		fmt.Printf(" (%d gagal, lihat log di atas)", failed)
	}
	fmt.Println()
}

// restoreFolder men-download semua isi folderID secara rekursif ke localDir,
// mempertahankan struktur folder Drive. File yang sudah ada di lokal dengan
// ukuran sama akan dilewati (supaya restore bisa dilanjutkan kalau putus).
func restoreFolder(srv *drive.Service, folderID, localDir string) (ok, failed int, err error) {
	pageToken := ""
	for {
		call := srv.Files.List().
			Q(fmt.Sprintf("'%s' in parents and trashed = false", folderID)).
			PageSize(100).
			Fields("nextPageToken, files(id, name, mimeType, size)")
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		r, listErr := call.Do()
		if listErr != nil {
			return ok, failed, listErr
		}

		for _, f := range r.Files {
			localPath := filepath.Join(localDir, sanitizeName(f.Name))

			if f.MimeType == folderMimeType {
				if mkErr := os.MkdirAll(localPath, 0755); mkErr != nil {
					fmt.Fprintf(os.Stderr, "gagal buat folder %s: %v\n", localPath, mkErr)
					failed++
					continue
				}
				fmt.Printf("DIR  %s/\n", localPath)

				subOK, subFailed, subErr := restoreFolder(srv, f.Id, localPath)
				ok += subOK
				failed += subFailed
				if subErr != nil {
					return ok, failed, subErr
				}
				continue
			}

			// File native Google (Docs/Sheets/Slides) tidak punya konten
			// biner langsung dan di-export ke PDF; yang lain di-download as-is.
			// Keduanya selalu menimpa file lokal yang sudah ada.
			finalPath, written, dlErr := downloadToPath(srv, f.Id, f.MimeType, localPath, true)
			if dlErr != nil {
				fmt.Fprintf(os.Stderr, "gagal download %s: %v\n", f.Name, dlErr)
				failed++
				continue
			}
			fmt.Printf("FILE %s (%d bytes)\n", finalPath, written)
			ok++
		}

		if r.NextPageToken == "" {
			break
		}
		pageToken = r.NextPageToken
	}
	return ok, failed, nil
}

// sanitizeName mengganti karakter yang valid di nama file Drive tapi
// bermasalah untuk path lokal (Drive mengizinkan '/' dalam nama).
func sanitizeName(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}
