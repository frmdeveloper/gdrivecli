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
| `gdrive watch <dir> <folder-id>` | 👁️ Sinkron otomatis (event-driven) |

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

---

## ♻️ `restore` — download seluruh folder

```bash
./gdrive restore 1AbCdEfGhIjKlMnOp ./restore-data
```

- 🔁 Berjalan **rekursif**, struktur sub-folder di Drive direplikasi di lokal
- ✏️ **Selalu menimpa** file lokal yang sudah ada
- 📄 Google Docs / Sheets / Slides otomatis di-export ke `.pdf`

---

## 👁️ `watch` — sinkron otomatis

```bash
./gdrive watch ./data 1AbCdEfGhIjKlMnOp
```

| Event | Aksi |
|---|---|
| ⬆️ File dibuat / ditulis | Upload / update ke Drive (debounce 300ms) |
| 🗂️ Folder baru dibuat | Daftarkan ke watcher + buat di Drive |
| 🗑️ File dihapus / di-rename | Hapus permanen di Drive seketika |
| 🙈 File/folder berawalan `.` | Diabaikan (hidden/temp) |

> ⚠️ **Hapus lokal = hapus permanen di Drive.**

---

## 🟨 Penggunaan dari Node.js

Setelah build `gdrive.node`, semua fungsi langsung tersedia tanpa `create()`:

```js
const gdrive = require('./gdrive.node')

// List folder (default: root)
const files = gdrive.list()
console.log(files) // DriveFile[]

// List folder tertentu
const folder = gdrive.list('1AbCdEfGhIjKlMnOp')

// Upload
const uploaded = gdrive.upload('./laporan.pdf', '1AbCdEfGhIjKlMnOp')
console.log(uploaded.id)

// Search
const results = gdrive.search('laporan')

// Download
const bytes = gdrive.download('1AbCd...', './output.pdf')

// Mkdir
const newFolder = gdrive.mkdir('Backup', '1AbCd...')

// Hapus permanen
gdrive.remove('1AbCd...')

// Info detail file
const detail = gdrive.info('1AbCd...')

// Restore seluruh folder Drive ke lokal
const { ok, failed } = gdrive.restore('1AbCd...', './restore-data')

// Watch — jalan di goroutine, tidak memblokir
const watcher = gdrive.watch('./data', '1AbCd...')
// ... lakukan hal lain ...
watcher.stop()
```

### TypeScript

```ts
interface DriveFile {
  id: string
  name: string
  mimeType: string
  size: number
  isDir: boolean
  createdTime?: string
  modifiedTime?: string
  webViewLink?: string
  parents?: string[]
}

interface GDrive {
  list(folderId?: string): DriveFile[]
  search(keyword: string): DriveFile[]
  upload(localPath: string, folderId?: string): DriveFile
  download(fileId: string, outputPath?: string): number
  mkdir(name: string, parentId?: string): DriveFile
  remove(fileId: string): void
  info(fileId: string): DriveFile
  restore(folderId: string, localDir: string): { ok: number; failed: number }
  watch(localDir: string, folderId: string): { stop(): void }
}

const gdrive: GDrive = require('./gdrive.node')
```

> Karena semua fungsi **sinkron**, untuk production gunakan `worker_threads`:

```js
const { Worker, isMainThread, parentPort, workerData } = require('worker_threads')

if (isMainThread) {
  const w = new Worker(__filename, { workerData: { folderId: '1AbCd...' } })
  w.on('message', files => console.log(files))
} else {
  const gdrive = require('./gdrive.node')
  parentPort.postMessage(gdrive.list(workerData.folderId))
}
```

---

## 📝 Catatan

- 🚫 `credentials.json` dan `token.json` jangan di-commit ke git
- ☠️ `delete` dan `watch` (saat file lokal dihapus) bersifat **permanen**, bukan ke trash
- 🔧 Scope: `drive` (full access). Untuk akses terbatas ganti scope di `auth.go` → `https://www.googleapis.com/auth/drive.file`, lalu hapus `token.json`
- 📦 Tidak ada dependency `google.golang.org/api` — semua komunikasi Drive pakai pure `net/http`
