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

**CLI binary:**
```bash
go mod tidy
go build -o gdrive .
```

**Node.js addon (`gdrive.node`):**
```bash
# CGO dibutuhkan — pastikan GCC/Clang tersedia
# Termux: pkg install clang
go build -tags napi -buildmode=c-shared -o gdrive.node .
```

> `go mod tidy` cukup sekali, berlaku untuk kedua build.

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
| `gdrive watch <dir> <folder-id>` | 👁️ Sinkron 2 arah otomatis (event-driven) |

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
./gdrive watch ./data 1AbCdEfGhIjKlMnOp
```

| Event | Aksi |
|---|---|
| ⬆️ File dibuat / ditulis | Upload / update ke Drive (debounce 300ms) |
| 🗂️ Folder baru dibuat | Daftarkan ke watcher + buat di Drive |
| 🗑️ File dihapus / di-rename | Hapus permanen di Drive seketika |
| 🙈 File/folder berawalan `.` | Diabaikan (hidden/temp) |

Berbasis **event** (fsnotify/inotify) — bereaksi seketika saat ada perubahan, tanpa polling.

> ⚠️ **Hapus lokal = hapus permanen di Drive.** Jangan `watch` di folder yang dibersihkan otomatis proses lain kalau Drive ini satu-satunya backup-mu.

> 💡 Kalau jalan di folder shared storage Termux (`~/storage/...`) dan perubahan tidak terdeteksi, itu karena FUSE tidak meneruskan event inotify. Solusinya: taruh file di storage internal Termux (`~/`) yang jalan normal.

---

## 🟨 Penggunaan dari Node.js

Setelah build addon, `require` langsung dari Node.js:

```js
const gdrive = require('./gdrive.node')

// List root
const files = gdrive.list()
console.log(files) // DriveFile[]

// Upload
const uploaded = gdrive.upload('./laporan.pdf', '1AbCdEfGhIjKlMnOp')
console.log(uploaded.id)

// Search
const results = gdrive.search('laporan')

// Download
const bytes = gdrive.download('1AbCd...', './output.pdf')

// Mkdir
const folder = gdrive.mkdir('Backup', '1AbCd...')

// Hapus (nama remove, bukan delete — reserved keyword di JS)
gdrive.remove('1AbCd...')

// Info
const detail = gdrive.info('1AbCd...')

// Restore seluruh folder Drive ke lokal
const { ok, failed } = gdrive.restore('1AbCd...', './restore-data')

// Watch — jalan di goroutine, tidak memblokir
const watcher = gdrive.watch('./data', '1AbCd...')
// ... lakukan hal lain ...
watcher.stop() // hentikan saat selesai
```

TypeScript definitions tersedia di `gdrive.d.ts`.

Karena semua fungsi (kecuali `watch`) **sinkron dan memblokir**, untuk penggunaan
production gunakan `worker_threads`:

```js
const { Worker, isMainThread, parentPort, workerData } = require('worker_threads')

if (isMainThread) {
  const w = new Worker(__filename, { workerData: { folderId: '1AbCd...' } })
  w.on('message', files => console.log('files:', files))
} else {
  const gdrive = require('./gdrive.node')
  parentPort.postMessage(gdrive.list(workerData.folderId))
}
```

---



- 🚫 `credentials.json` dan `token.json` jangan di-commit ke git → sudah masuk `.gitignore`
- ☠️ `gdrive delete` dan `watch` (saat file lokal dihapus) bersifat **permanen**, bukan ke trash
- 🔧 Scope: `drive` (full access). Untuk akses terbatas, ganti `drive.DriveScope` → `drive.DriveFileScope` di `auth.go`, lalu hapus `token.json` agar re-auth
