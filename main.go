//go:build !napi

package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		printUsage()
		return
	}

	srv, err := getDriveService()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	switch cmd {
	case "list", "ls":
		cmdList(srv, args)
	case "upload":
		cmdUpload(srv, args)
	case "download":
		cmdDownload(srv, args)
	case "mkdir":
		cmdMkdir(srv, args)
	case "delete", "rm":
		cmdDelete(srv, args)
	case "search":
		cmdSearch(srv, args)
	case "info":
		cmdInfo(srv, args)
	case "restore":
		cmdRestore(srv, args)
	case "watch":
		cmdWatch(srv, args)
	default:
		fmt.Fprintf(os.Stderr, "Perintah tidak dikenal: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`gdrive-cli - CLI sederhana untuk Google Drive

Penggunaan:
  gdrive list    [-l] [folder-id]       List isi folder (default: root)
  gdrive upload  <file> [folder-id]      Upload file ke Drive
  gdrive download <file-id> [output]    Download file dari Drive
  gdrive mkdir   <nama> [parent-id]      Buat folder baru
  gdrive delete  <file-id>               Hapus file/folder permanen
  gdrive search  [-l] <kata-kunci>       Cari file berdasarkan nama
  gdrive info    <file-id>               Tampilkan detail file
  gdrive restore <folder-id> <local-dir> Download semua isi folder (rekursif, untuk restore)
  gdrive watch   <local-dir> <folder-id> [interval]
                                          Sinkron 2 arah, cek tiap <interval> (default 1s)

Flag -l/--long pada list & search menampilkan kolom ID Drive (disembunyikan
secara default karena terlalu lebar untuk terminal HP).

Setup awal & autentikasi: lihat README.md
`)
}
