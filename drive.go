package main

// DriveFile adalah struktur file/folder dari Google Drive REST API.
type DriveFile struct {
	Id           string   `json:"id"`
	Name         string   `json:"name"`
	MimeType     string   `json:"mimeType"`
	Size         int64    `json:"size,string,omitempty"`
	IsDir        bool     `json:"isDir,omitempty"`
	CreatedTime  string   `json:"createdTime,omitempty"`
	ModifiedTime string   `json:"modifiedTime,omitempty"`
	WebViewLink  string   `json:"webViewLink,omitempty"`
	Parents      []string `json:"parents,omitempty"`
}

// DriveFileList adalah response dari Files.list API.
type DriveFileList struct {
	NextPageToken string       `json:"nextPageToken"`
	Files         []*DriveFile `json:"files"`
}

const (
	folderMimeType = "application/vnd.google-apps.folder"
	driveBase      = "https://www.googleapis.com/drive/v3"
	uploadBase     = "https://www.googleapis.com/upload/drive/v3"
	driveFields    = "id,name,mimeType,size,createdTime,modifiedTime,parents,webViewLink"
)
