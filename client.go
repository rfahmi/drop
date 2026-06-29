package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

type Client struct {
	ServerURL string
	Token     string
	SyncDir   string
}

func NewClient(serverURL, token, syncDir string) *Client {
	return &Client{
		ServerURL: serverURL,
		Token:     token,
		SyncDir:   syncDir,
	}
}

func (c *Client) Sync() error {
	// 1. Ensure sync dir exists
	err := os.MkdirAll(c.SyncDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create sync directory: %w", err)
	}

	// 2. Fetch file list
	req, err := http.NewRequest("GET", c.ServerURL+"/api/files", nil)
	if err != nil {
		return err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch file list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var files []FileInfo
	err = json.NewDecoder(resp.Body).Decode(&files)
	if err != nil {
		// if empty body or null
		if err == io.EOF {
			log.Println("No files found on server.")
			return nil
		}
		return fmt.Errorf("failed to parse file list: %w", err)
	}

	log.Printf("Found %d files on server", len(files))

	// 3. Download missing files
	for _, file := range files {
		localPath := filepath.Join(c.SyncDir, file.Name)
		stat, err := os.Stat(localPath)
		if err == nil {
			if stat.Size() == file.Size {
				log.Printf("Skipping %s (already exists and size matches)", file.Name)
				continue
			} else {
				log.Printf("Updating %s (size differs %d vs %d)", file.Name, stat.Size(), file.Size)
			}
		} else if !os.IsNotExist(err) {
			log.Printf("Error checking local file %s: %v", localPath, err)
			continue
		}

		err = c.downloadFile(file.Name, localPath)
		if err != nil {
			log.Printf("Failed to download %s: %v", file.Name, err)
		}
	}

	log.Println("Sync complete")
	return nil
}

func (c *Client) downloadFile(remoteName, localPath string) error {
	log.Printf("Downloading %s to %s...", remoteName, localPath)

	// Ensure directory structure exists (in case filenames have slashes)
	os.MkdirAll(filepath.Dir(localPath), 0755)

	// Add token to query
	req, err := http.NewRequest("GET", c.ServerURL+"/api/files/"+url.PathEscape(remoteName)+"?token="+url.QueryEscape(c.Token), nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
