package api

import (
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

func (s *Server) filesList(w http.ResponseWriter, r *http.Request) {
	list, err := s.Files.List(ident(r).User, r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) filesRead(w http.ResponseWriter, r *http.Request) {
	data, err := s.Files.Read(ident(r).User, r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": string(data)})
}

func (s *Server) filesWrite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := decodeLarge(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.Files.Write(ident(r).User, req.Path, []byte(req.Content)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) filesDownload(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	f, st, err := s.Files.Open(ident(r).User, p)
	if err != nil {
		fail(w, err)
		return
	}
	defer f.Close()
	name := path.Base(p)
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	http.ServeContent(w, r, name, st.ModTime(), f)
}

func (s *Server) filesUpload(w http.ResponseWriter, r *http.Request) {
	maxMB := s.Files.MaxUploadMB
	if maxMB <= 0 {
		maxMB = 512
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMB<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", "oczekiwano multipart/form-data")
		return
	}
	dir := r.URL.Query().Get("path")
	n := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			fail(w, err)
			return
		}
		if part.FormName() == "path" {
			b, _ := io.ReadAll(io.LimitReader(part, 4096))
			dir = string(b)
			continue
		}
		if part.FileName() == "" {
			continue
		}
		if err := s.Files.Upload(ident(r).User, dir, path.Base(part.FileName()), part); err != nil {
			fail(w, err)
			return
		}
		n++
	}
	writeJSON(w, http.StatusOK, map[string]int{"uploaded": n})
}

func (s *Server) filesMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.Files.Mkdir(ident(r).User, req.Path); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) filesRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.Files.Rename(ident(r).User, req.From, req.To); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) filesCopy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.Files.Copy(ident(r).User, req.From, req.To); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) filesDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.Files.Delete(ident(r).User, req.Paths); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) filesChmod(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Mode string `json:"mode"` // octal string e.g. "0755"
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	mode, err := strconv.ParseUint(req.Mode, 8, 32)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", "nieprawidłowe uprawnienia")
		return
	}
	if err := s.Files.Chmod(ident(r).User, req.Path, uint32(mode)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) filesCompress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
		Dest  string   `json:"dest"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.Files.Compress(ident(r).User, req.Paths, req.Dest); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) filesExtract(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Dest string `json:"dest"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.Files.Extract(ident(r).User, req.Path, req.Dest); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
