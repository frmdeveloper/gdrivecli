export interface DriveFile {
  id: string;
  name: string;
  mimeType: string;
  size: number;
  isDir: boolean;
  createdTime?: string;
  modifiedTime?: string;
  webViewLink?: string;
  parents?: string[];
}

export interface RestoreResult {
  ok: number;
  failed: number;
}

export interface Watcher {
  /** Hentikan watcher. Aman dipanggil lebih dari satu kali. */
  stop(): void;
}

/** List isi folder. Default: root. */
export declare function list(folderId?: string): DriveFile[];

/** Cari file berdasarkan nama (case-insensitive, substring). */
export declare function search(keyword: string): DriveFile[];

/** Upload file lokal ke Drive. */
export declare function upload(localPath: string, folderId?: string): DriveFile;

/**
 * Download file dari Drive.
 * Google Docs/Sheets/Slides otomatis di-export ke PDF.
 * Mengembalikan jumlah byte yang ditulis.
 */
export declare function download(fileId: string, outputPath?: string): number;

/** Buat folder baru di Drive. */
export declare function mkdir(name: string, parentId?: string): DriveFile;

/**
 * Hapus file atau folder permanen (tidak ke trash).
 * Nama `remove` dipakai karena `delete` adalah reserved keyword di JS.
 */
export declare function remove(fileId: string): void;

/** Ambil detail file. */
export declare function info(fileId: string): DriveFile;

/**
 * Download seluruh isi folder Drive secara rekursif ke localDir.
 * Selalu menimpa file lokal yang sudah ada.
 */
export declare function restore(folderId: string, localDir: string): RestoreResult;

/**
 * Mulai watcher dua arah di goroutine terpisah.
 * Panggil `.stop()` pada object yang dikembalikan untuk menghentikannya.
 *
 * ⚠️ Semua fungsi di modul ini bersifat sinkron dan memblokir event loop.
 * Untuk penggunaan non-blocking, jalankan di `worker_threads`.
 */
export declare function watch(localDir: string, folderId: string): Watcher;
