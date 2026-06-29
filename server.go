package main

import (
	"embed"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

//go:embed ui/index.html
var uiFiles embed.FS

type Server struct {
	R2Client *R2Client
	Token    string
}

func NewServer(r2Client *R2Client) *Server {
	token := os.Getenv("AUTH_TOKEN")
	if token == "" {
		log.Println("WARNING: AUTH_TOKEN is not set, API will be unauthenticated!")
	}
	return &Server{
		R2Client: r2Client,
		Token:    token,
	}
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Token != "" {
			authHeader := r.Header.Get("Authorization")
			expected := "Bearer " + s.Token
			if authHeader != expected {
				// Allow passing token in query string for downloads
				tokenQuery := r.URL.Query().Get("token")
				if tokenQuery != s.Token {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
		}
		next(w, r)
	}
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	files, err := s.R2Client.ListFiles(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		// try to get it from multipart
		err := r.ParseMultipartForm(32 << 20)
		if err == nil {
			file, header, err := r.FormFile("file")
			if err == nil {
				defer file.Close()
				err = s.R2Client.UploadFile(r.Context(), header.Filename, file)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		http.Error(w, "filename is required either via query param ?filename=... or multipart form", http.StatusBadRequest)
		return
	}

	err := s.R2Client.UploadFile(r.Context(), filename, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimPrefix(r.URL.Path, "/api/files/")
	if filename == "" {
		http.Error(w, "filename is required", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodGet {
		body, size, err := s.R2Client.DownloadFile(r.Context(), filename)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		defer body.Close()

		w.Header().Set("Content-Disposition", "attachment; filename=\""+path.Base(filename)+"\"")
		w.Header().Set("Content-Type", "application/octet-stream")
		if size != nil {
			w.Header().Set("Content-Length", strconv.FormatInt(*size, 10))
		}
		io.Copy(w, body)
		return
	}

	if r.Method == http.MethodDelete {
		err := s.R2Client.DeleteFile(r.Context(), filename)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method == http.MethodPatch {
		action := r.URL.Query().Get("action")
		if action == "rename" {
			newName := r.URL.Query().Get("newname")
			if newName == "" {
				http.Error(w, "newname is required", http.StatusBadRequest)
				return
			}
			err := s.R2Client.RenameFile(r.Context(), filename, newName)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/files", s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.handleUploadFile(w, r)
		} else {
			s.handleListFiles(w, r)
		}
	}))

	mux.HandleFunc("/api/files/", s.authMiddleware(s.handleFile))

	// Serve UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		data, err := uiFiles.ReadFile("ui/index.html")
		if err != nil {
			http.Error(w, "UI not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	return mux
}
