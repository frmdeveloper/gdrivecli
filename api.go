package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

// ── LIST ─────────────────────────────────────────────────────────────────────

func (c *DriveClient) ListFiles(parent string, pageToken string) (*DriveFileList, error) {
	if parent == "" {
		parent = "root"
	}
	params := url.Values{}
	params.Set("q", fmt.Sprintf("'%s' in parents and trashed = false", parent))
	params.Set("pageSize", "100")
	params.Set("orderBy", "folder,name")
	params.Set("fields", "nextPageToken,files("+driveFields+")")
	if pageToken != "" {
		params.Set("pageToken", pageToken)
	}

	resp, err := c.http.Get(driveBase + "/files?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result DriveFileList
	if err := checkAndDecode(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ── SEARCH ───────────────────────────────────────────────────────────────────

func (c *DriveClient) SearchFiles(keyword string) (*DriveFileList, error) {
	params := url.Values{}
	params.Set("q", fmt.Sprintf("name contains '%s' and trashed = false",
		strings.ReplaceAll(keyword, "'", "\\'")))
	params.Set("pageSize", "50")
	params.Set("orderBy", "folder,name")
	params.Set("fields", "nextPageToken,files("+driveFields+")")

	resp, err := c.http.Get(driveBase + "/files?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result DriveFileList
	if err := checkAndDecode(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ── GET FILE META ─────────────────────────────────────────────────────────────

func (c *DriveClient) GetFile(fileID string) (*DriveFile, error) {
	params := url.Values{}
	params.Set("fields", driveFields)

	resp, err := c.http.Get(driveBase + "/files/" + fileID + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var f DriveFile
	if err := checkAndDecode(resp, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// ── UPLOAD (multipart) ────────────────────────────────────────────────────────

func (c *DriveClient) CreateFile(name string, parents []string, content io.Reader, fields string) (*DriveFile, error) {
	if fields == "" {
		fields = driveFields
	}

	meta := map[string]any{"name": name}
	if len(parents) > 0 {
		meta["parents"] = parents
	}
	metaJSON, _ := json.Marshal(meta)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	// Part 1: metadata JSON
	metaHeader := textproto.MIMEHeader{}
	metaHeader.Set("Content-Type", "application/json; charset=UTF-8")
	metaPart, _ := mw.CreatePart(metaHeader)
	metaPart.Write(metaJSON)

	// Part 2: file content
	contentHeader := textproto.MIMEHeader{}
	contentHeader.Set("Content-Type", "application/octet-stream")
	contentPart, _ := mw.CreatePart(contentHeader)
	io.Copy(contentPart, content)

	mw.Close()

	req, err := http.NewRequest("POST",
		uploadBase+"/files?uploadType=multipart&fields="+url.QueryEscape(fields),
		&body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "multipart/related; boundary="+mw.Boundary())

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var f DriveFile
	if err := checkAndDecode(resp, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// ── UPDATE (upload konten baru ke file yang sudah ada) ────────────────────────

func (c *DriveClient) UpdateFile(fileID string, content io.Reader) error {
	req, err := http.NewRequest("PATCH",
		uploadBase+"/files/"+fileID+"?uploadType=media",
		content)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp)
}

// ── MKDIR ─────────────────────────────────────────────────────────────────────

func (c *DriveClient) CreateFolder(name string, parents []string) (*DriveFile, error) {
	meta := map[string]any{
		"name":     name,
		"mimeType": folderMimeType,
	}
	if len(parents) > 0 {
		meta["parents"] = parents
	}
	metaJSON, _ := json.Marshal(meta)

	req, err := http.NewRequest("POST",
		driveBase+"/files?fields="+url.QueryEscape(driveFields),
		bytes.NewReader(metaJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var f DriveFile
	if err := checkAndDecode(resp, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// ── DELETE ────────────────────────────────────────────────────────────────────

func (c *DriveClient) DeleteFile(fileID string) error {
	req, err := http.NewRequest("DELETE", driveBase+"/files/"+fileID, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp)
}

// ── DOWNLOAD ──────────────────────────────────────────────────────────────────

func (c *DriveClient) DownloadFile(fileID string) (*http.Response, error) {
	params := url.Values{}
	params.Set("alt", "media")
	resp, err := c.http.Get(driveBase + "/files/" + fileID + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, driveAPIError(resp)
	}
	return resp, nil
}

func (c *DriveClient) ExportFile(fileID, mimeType string) (*http.Response, error) {
	params := url.Values{}
	params.Set("mimeType", mimeType)
	resp, err := c.http.Get(driveBase + "/files/" + fileID + "/export?" + params.Encode())
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, driveAPIError(resp)
	}
	return resp, nil
}

// ── HELPER ────────────────────────────────────────────────────────────────────

type driveError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func driveAPIError(resp *http.Response) error {
	var e driveError
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &e)
	if e.Error.Message != "" {
		return fmt.Errorf("drive API %d: %s", e.Error.Code, e.Error.Message)
	}
	return fmt.Errorf("drive API error: status %d", resp.StatusCode)
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 300 {
		return driveAPIError(resp)
	}
	return nil
}

func checkAndDecode(resp *http.Response, v any) error {
	if resp.StatusCode >= 300 {
		return driveAPIError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// ListFilesRaw untuk query custom (dipakai watch.go dan folderCache).
func (c *DriveClient) ListFilesRaw(params url.Values) (*DriveFileList, error) {
	resp, err := c.http.Get(driveBase + "/files?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result DriveFileList
	if err := checkAndDecode(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
