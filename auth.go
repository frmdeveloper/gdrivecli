package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	credentialsFile = "credentials.json"
	tokenFile       = "token.json"
	redirectURL     = "http://127.0.0.1:8765/callback"
)

// getDriveService menyiapkan client Drive API. Kalau belum ada token
// tersimpan, akan menjalankan OAuth flow (buka browser sekali).
func getDriveService() (*drive.Service, error) {
	ctx := context.Background()

	b, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf(
			"tidak bisa baca %s: %w\n→ buat OAuth client ID (tipe \"Desktop app\") di Google Cloud Console, "+
				"download JSON-nya dan simpan sebagai %s (lihat README.md)",
			credentialsFile, err, credentialsFile,
		)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("gagal parse credentials: %w", err)
	}
	config.RedirectURL = redirectURL

	client := getClient(config)

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("gagal buat Drive service: %w", err)
	}
	return srv, nil
}

func getClient(config *oauth2.Config) *http.Client {
	tok, err := tokenFromFile(tokenFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tok)
	}

	ts := config.TokenSource(context.Background(), tok)

	// Kalau access token sudah expired, TokenSource otomatis refresh
	// pakai refresh token. Simpan ulang hasilnya supaya tidak perlu
	// login lagi lain kali.
	if newTok, err := ts.Token(); err == nil && newTok.AccessToken != tok.AccessToken {
		saveToken(newTok)
	}

	return oauth2.NewClient(context.Background(), ts)
}

// getTokenFromWeb menjalankan local HTTP server untuk menangkap redirect
// OAuth (loopback flow), lalu menukar authorization code dengan token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	codeCh := make(chan string, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			fmt.Fprintf(w, "Login gagal: %s. Tutup tab ini dan jalankan ulang perintahnya.", errMsg)
			return
		}
		fmt.Fprintln(w, "Login berhasil! Tab ini bisa ditutup, kembali ke terminal.")
		codeCh <- r.URL.Query().Get("code")
	})

	server := &http.Server{Addr: "127.0.0.1:8765", Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("local auth server error: %v", err)
		}
	}()
	defer server.Close()

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	fmt.Println("Buka link berikut di browser (HP/PC yang sama) untuk login akun Google:")
	fmt.Println(authURL)
	fmt.Println()
	fmt.Println("Menunggu login...")

	code := <-codeCh

	tok, err := config.Exchange(context.Background(), code)
	if err != nil {
		log.Fatalf("gagal tukar authorization code dengan token: %v", err)
	}
	return tok
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tok := &oauth2.Token{}
	if err := json.NewDecoder(f).Decode(tok); err != nil {
		return nil, err
	}
	return tok, nil
}

func saveToken(token *oauth2.Token) {
	f, err := os.OpenFile(tokenFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Printf("gagal simpan token: %v", err)
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}
