package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/drive/v3"
)

const folderMimeType = "application/vnd.google-apps.folder"

func cmdList(srv *drive.Service, args []string) {
	parent := "root"
	long := false
	for _, a := range args {
		if a == "-l" || a == "--long" {
			long = true
			continue
		}
		parent = a
	}

	r, err := srv.Files.List().
		Q(fmt.Sprintf("'%s' in parents and trashed = false", parent)).
		PageSize(100).
		OrderBy("folder,name").
		Fields("files(id, name, mimeType, size)").
		Do()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	if len(r.Files) == 0 {
		fmt.Println("(kosong)")
		return
	}

	printFileList(r.Files, long)
}

func cmdSearch(srv *drive.Service, args []string) {
	var keyword string
	long := false
	for _, a := range args {
		if a == "-l" || a == "--long" {
			long = true
			continue
		}
		keyword = a
	}
	if keyword == "" {
		fmt.Println("Penggunaan: gdrive search [-l] <kata-kunci>")
		os.Exit(1)
	}

	r, err := srv.Files.List().
		Q(fmt.Sprintf("name contains '%s' and trashed = false", keyword)).
		PageSize(50).
		OrderBy("folder,name").
		Fields("files(id, name, mimeType, size)").
		Do()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	if len(r.Files) == 0 {
		fmt.Println("Tidak ada hasil.")
		return
	}

	printFileList(r.Files, long)
}

// printFileList menampilkan daftar file dengan format:
//
//	d | nama-folder | -
//	- | nama-file   | 1.2 MiB | <id>  (hanya kalau long=true)
func printFileList(files []*drive.File, long bool) {
	for _, f := range files {
		typ := "-"
		size := humanSize(f.Size)
		if f.MimeType == folderMimeType {
			typ = "d"
			size = "-"
		}
		if long {
			fmt.Printf("%s | %s | %s | %s\n", typ, f.Name, size, f.Id)
		} else {
			fmt.Printf("%s | %s | %s\n", typ, f.Name, size)
		}
	}
}

// humanSize mengubah ukuran byte ke bentuk ringkas (B/KiB/MiB/GiB/...).
func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func cmdUpload(srv *drive.Service, args []string) {
	if len(args) < 1 {
		fmt.Println("Penggunaan: gdrive upload <path-file> [parent-folder-id]")
		os.Exit(1)
	}

	path := args[0]
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error buka file:", err)
		os.Exit(1)
	}
	defer f.Close()

	meta := &drive.File{Name: filepath.Base(path)}
	if len(args) > 1 {
		meta.Parents = []string{args[1]}
	}

	uploaded, err := srv.Files.Create(meta).Media(f).Fields("id, name, size").Do()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error upload:", err)
		os.Exit(1)
	}

	fmt.Printf("Upload berhasil: %s (%d bytes)\nID: %s\n", uploaded.Name, uploaded.Size, uploaded.Id)
}

func cmdDownload(srv *drive.Service, args []string) {
	if len(args) < 1 {
		fmt.Println("Penggunaan: gdrive download <file-id> [output-path]")
		os.Exit(1)
	}
	fileID := args[0]

	meta, err := srv.Files.Get(fileID).Fields("name, mimeType").Do()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error ambil metadata:", err)
		os.Exit(1)
	}

	destPath := meta.Name
	appendPdfExt := strings.HasPrefix(meta.MimeType, "application/vnd.google-apps.")
	if len(args) > 1 {
		destPath = args[1]
		appendPdfExt = false // nama sudah ditentukan user, hormati apa adanya
	}

	finalPath, written, err := downloadToPath(srv, fileID, meta.MimeType, destPath, appendPdfExt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error download:", err)
		os.Exit(1)
	}

	fmt.Printf("Download berhasil: %s (%d bytes)\n", finalPath, written)
}

// downloadToPath men-download (atau export ke PDF kalau Google Docs/Sheets/
// Slides) sebuah file Drive ke destPath. Mengembalikan path final (bertambah
// ".pdf" kalau appendPdfExt true dan tipenya Google-native) dan jumlah byte
// yang ditulis. Dipakai oleh cmdDownload dan cmdRestore.
func downloadToPath(srv *drive.Service, fileID, mimeType, destPath string, appendPdfExt bool) (string, int64, error) {
	var resp *http.Response
	var err error

	if strings.HasPrefix(mimeType, "application/vnd.google-apps.") {
		// Google Docs/Sheets/Slides tidak punya konten biner langsung,
		// jadi di-export sebagai PDF.
		if appendPdfExt {
			destPath += ".pdf"
		}
		resp, err = srv.Files.Export(fileID, "application/pdf").Download()
	} else {
		resp, err = srv.Files.Get(fileID).Download()
	}
	if err != nil {
		return destPath, 0, err
	}
	defer resp.Body.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return destPath, 0, err
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	return destPath, written, err
}

func cmdMkdir(srv *drive.Service, args []string) {
	if len(args) < 1 {
		fmt.Println("Penggunaan: gdrive mkdir <nama-folder> [parent-id]")
		os.Exit(1)
	}

	meta := &drive.File{
		Name:     args[0],
		MimeType: folderMimeType,
	}
	if len(args) > 1 {
		meta.Parents = []string{args[1]}
	}

	created, err := srv.Files.Create(meta).Fields("id, name").Do()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fmt.Printf("Folder dibuat: %s\nID: %s\n", created.Name, created.Id)
}

func cmdDelete(srv *drive.Service, args []string) {
	if len(args) < 1 {
		fmt.Println("Penggunaan: gdrive delete <file-id>")
		os.Exit(1)
	}

	if err := srv.Files.Delete(args[0]).Do(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fmt.Println("Berhasil dihapus permanen.")
}

func cmdInfo(srv *drive.Service, args []string) {
	if len(args) < 1 {
		fmt.Println("Penggunaan: gdrive info <file-id>")
		os.Exit(1)
	}

	f, err := srv.Files.Get(args[0]).
		Fields("id, name, mimeType, size, createdTime, modifiedTime, parents, webViewLink").
		Do()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fmt.Printf("Nama    : %s\n", f.Name)
	fmt.Printf("ID      : %s\n", f.Id)
	fmt.Printf("Tipe    : %s\n", f.MimeType)
	fmt.Printf("Ukuran  : %d bytes\n", f.Size)
	fmt.Printf("Dibuat  : %s\n", f.CreatedTime)
	fmt.Printf("Diubah  : %s\n", f.ModifiedTime)
	if len(f.Parents) > 0 {
		fmt.Printf("Parent  : %s\n", strings.Join(f.Parents, ", "))
	}
	if f.WebViewLink != "" {
		fmt.Printf("Link    : %s\n", f.WebViewLink)
	}
}
