package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func cmdList(srv *DriveClient, args []string) {
	parent := "root"
	long := false
	for _, a := range args {
		if a == "-l" || a == "--long" {
			long = true
			continue
		}
		parent = a
	}

	r, err := srv.ListFiles(parent, "")
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

func cmdSearch(srv *DriveClient, args []string) {
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

	r, err := srv.SearchFiles(keyword)
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

func printFileList(files []*DriveFile, long bool) {
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

func cmdUpload(srv *DriveClient, args []string) {
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

	var parents []string
	if len(args) > 1 {
		parents = []string{args[1]}
	}

	uploaded, err := srv.CreateFile(filepath.Base(path), parents, f, "id,name,size")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error upload:", err)
		os.Exit(1)
	}

	fmt.Printf("Upload berhasil: %s (%d bytes)\nID: %s\n", uploaded.Name, uploaded.Size, uploaded.Id)
}

func cmdDownload(srv *DriveClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Penggunaan: gdrive download <file-id> [output-path]")
		os.Exit(1)
	}
	fileID := args[0]

	meta, err := srv.GetFile(fileID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error ambil metadata:", err)
		os.Exit(1)
	}

	destPath := meta.Name
	appendPdfExt := strings.HasPrefix(meta.MimeType, "application/vnd.google-apps.")
	if len(args) > 1 {
		destPath = args[1]
		appendPdfExt = false
	}

	finalPath, written, err := downloadToPath(srv, fileID, meta.MimeType, destPath, appendPdfExt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error download:", err)
		os.Exit(1)
	}

	fmt.Printf("Download berhasil: %s (%d bytes)\n", finalPath, written)
}

// downloadToPath men-download (atau export ke PDF kalau Google Docs/Sheets/Slides)
// sebuah file Drive ke destPath.
func downloadToPath(srv *DriveClient, fileID, mimeType, destPath string, appendPdfExt bool) (string, int64, error) {
	var (
		resp interface{ Body() io.ReadCloser }
		body io.ReadCloser
		err  error
	)

	if strings.HasPrefix(mimeType, "application/vnd.google-apps.") {
		if appendPdfExt {
			destPath += ".pdf"
		}
		r, e := srv.ExportFile(fileID, "application/pdf")
		if e != nil {
			return destPath, 0, e
		}
		body = r.Body
		_ = resp
	} else {
		r, e := srv.DownloadFile(fileID)
		if e != nil {
			return destPath, 0, e
		}
		body = r.Body
	}
	defer body.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return destPath, 0, err
	}
	defer out.Close()

	written, err := io.Copy(out, body)
	return destPath, written, err
}

func cmdMkdir(srv *DriveClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Penggunaan: gdrive mkdir <nama-folder> [parent-id]")
		os.Exit(1)
	}

	var parents []string
	if len(args) > 1 {
		parents = []string{args[1]}
	}

	created, err := srv.CreateFolder(args[0], parents)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fmt.Printf("Folder dibuat: %s\nID: %s\n", created.Name, created.Id)
}

func cmdDelete(srv *DriveClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Penggunaan: gdrive delete <file-id>")
		os.Exit(1)
	}

	if err := srv.DeleteFile(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fmt.Println("Berhasil dihapus permanen.")
}

func cmdInfo(srv *DriveClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Penggunaan: gdrive info <file-id>")
		os.Exit(1)
	}

	f, err := srv.GetFile(args[0])
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
