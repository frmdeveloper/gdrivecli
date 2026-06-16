# 🚀 gdrive-cli

> 📦 CLI ringan untuk **Google Drive**, ditulis dengan Go.  
> ✅ Jalan di Termux (Android), Linux, dan Mac.

---

## ⚙️ Setup

### 🔐 1. Buat OAuth credentials

1. 🌐 Buka [Google Cloud Console](https://console.cloud.google.com/)
2. 📁 Buat/pilih project, lalu aktifkan **Google Drive API**  
   (*APIs & Services* → *Library* → cari "Google Drive API" → *Enable*)
3. 🔑 Ke *Credentials* → *Create Credentials* → *OAuth client ID*
4. 🖥️ Pilih tipe **Desktop app**
5. 💾 Download JSON-nya → rename jadi `credentials.json` → taruh sefolder dengan binary `gdrive`

### 🔨 2. Build

```bash
go mod tidy
go build -o gdrive .
```

### 🔓 3. Login pertama kali

```bash
./gdrive list
```

🔗 Link OAuth akan muncul di terminal — buka di browser, login, izinkan akses.  
💾 Token disimpan otomatis ke `token.json` dan di-refresh sendiri → **login hanya sekali**.

> ⚠️ Port `127.0.0.1:8765` dipakai saat login — pastikan bebas.

---

## 📋 Daftar Perintah

| Perintah | Keterangan |
|---|---|
| `gdrive list [-l] [folder-id]` | 📂 List isi folder (default: root) |
| `gdrive upload <file> [parent-id]` | ⬆️ Upload file ke Drive |
| `gdrive download <file-id> [output]` | ⬇️ Download file dari Drive |
| `gdrive mkdir <nama> [parent-id]` | 📁 Buat folder baru |
| `gdrive delete <file-id>` | 🗑️ Hapus permanen |
| `gdrive search [-l] <kata-kunci>` | 🔍 Cari file berdasarkan nama |
| `gdrive info <file-id>` | ℹ️ Detail file (ukuran, tipe, link, dll) |
| `gdrive restore <folder-id> <dir>` | ♻️ Download seluruh folder rekursif |
| `gdrive watch <dir> <folder-id> [interval]` | 👁️ Sinkron 2 arah otomatis |

---

## 📂 `list` & `search`

Format output: `tipe | nama | ukuran`

```
d | Proyek |  -
- | catatan.txt | 1.2 KiB
- | foto.jpg | 3.4 MiB
```

Tambah `-l` untuk tampilkan **Drive ID** (dibutuhkan buat perintah lain):

```
d | Proyek | - | 1BxYz...
- | foto.jpg | 3.4 MiB | 1AbCd...
```

```bash
./gdrive list -l                # 📋 list root dengan ID
./gdrive search -l catatan      # 🔍 cari + tampilkan ID
```

---

## ♻️ `restore` — download & timpa seluruh folder

```bash
./gdrive restore 1AbCdEfGhIjKlMnOp ./restore-data
```

- 🔁 Berjalan **rekursif**, struktur sub-folder di Drive direplikasi di lokal
- ✏️ **Selalu menimpa** file lokal yang sudah ada
- 📄 Google Docs / Sheets / Slides otomatis di-export ke `.pdf`

---

## 👁️ `watch` — sinkron 2 arah otomatis

```bash
./gdrive watch ./data 1AbCdEfGhIjKlMnOp          # ⏱️ cek tiap 1s (default)
./gdrive watch ./data 1AbCdEfGhIjKlMnOp 200ms    # ⚡ lebih responsif
./gdrive watch ./data 1AbCdEfGhIjKlMnOp 5s       # 🔋 lebih hemat baterai
```

| Event | Aksi |
|---|---|
| ⬆️ File baru / berubah (mtime atau ukuran beda) | Upload / update ke Drive |
| 🗂️ Sub-folder baru | Buat folder di Drive (struktur di-mirror) |
| 🗑️ File dihapus lokal | Hapus permanen di Drive pada scan berikutnya |
| 🙈 File/folder berawalan `.` | Diabaikan (hidden/temp) |

> ⚠️ **Hapus lokal = hapus permanen di Drive.** Jangan `watch` di folder yang dibersihkan otomatis proses lain kalau Drive ini satu-satunya backup-mu.

> 💡 Pakai **polling** bukan inotify/fsnotify karena folder shared storage Termux (`~/storage/...`) memakai FUSE dan tidak meneruskan event inotify. Polling jalan di semua filesystem.

---

## 🔒 Keamanan & Catatan

- 🚫 `credentials.json` dan `token.json` jangan di-commit ke git → sudah masuk `.gitignore`
- ☠️ `gdrive delete` dan `watch` (saat file lokal dihapus) bersifat **permanen**, bukan ke trash
- 🔧 Scope: `drive` (full access). Untuk akses terbatas, ganti `drive.DriveScope` → `drive.DriveFileScope` di `auth.go`, lalu hapus `token.json` agar re-auth
