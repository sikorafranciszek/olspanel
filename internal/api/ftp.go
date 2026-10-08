package api

import (
	"net/http"

	"olspanel/internal/ftp"
)

func (s *Server) listFTP(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListUserFTP(r.Context(), ident(r).User.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createFTP(w http.ResponseWriter, r *http.Request) {
	var req ftp.CreateRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	a, err := s.FTP.Create(r.Context(), ident(r).User, req)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "ftp.create", a.Login, "")
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) updateFTP(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req ftp.UpdateRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	a, err := s.FTP.Update(r.Context(), ident(r).User, id, req)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "ftp.update", a.Login, "")
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) deleteFTP(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.FTP.Delete(r.Context(), ident(r).User, id); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "ftp.delete", "", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
