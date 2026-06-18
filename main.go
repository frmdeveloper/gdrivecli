//go:build !napi

package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	srv, err := getDriveService()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Auth gagal:", err)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "list", "ls":
		cmdList(srv, args)
	case "search":
		cmdSearch(srv, args)
	case "upload":
		cmdUpload(srv, args)
	case "download":
		cmdDownload(srv, args)
	case "mkdir":
		cmdMkdir(srv, args)
	case "delete", "rm":
		cmdDelete(srv, args)
	case "info":
		cmdInfo(srv, args)
	case "restore":
		cmdRestore(srv, args)
	case "watch":
		cmdWatch(srv, args)
	default:
		fmt.Fprintf(os.Stderr, "Perintah tidak dikenal: %s\n\n", cmd)
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`gdrive - Google Drive CLI

Perintah:
  list   [folder-id]              Tampilkan isi folder (default: root)
  search <kata-kunci>              Cari file/folder
  upload <path> [folder-id]       Upload file
  download <file-id> [output]     Download file
  mkdir  <nama> [parent-id]       Buat folder baru
  delete <file-id>                Hapus file permanen
  info   <file-id>                Info detail file
  restore <folder-id> <local-dir> Download seluruh folder rekursif
  watch  <local-dir> <folder-id>  Pantau folder lokal & sync ke Drive

Tambahkan -l atau --long ke list/search untuk tampilkan ID file.`)
}
