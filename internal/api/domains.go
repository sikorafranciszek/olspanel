package api

import (
	"net/http"

	"olspanel/internal/domains"
)

func (s *Server) listDomains(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListUserDomains(r.Context(), ident(r).User.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createDomain(w http.ResponseWriter, r *http.Request) {
	var req domains.CreateRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	dm, err := s.Domains.Create(r.Context(), ident(r).User, req)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "domain.create", dm.Name, dm.Type)
	writeJSON(w, http.StatusCreated, dm)
}

func (s *Server) updateDomain(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req domains.UpdateRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	dm, err := s.Domains.Update(r.Context(), ident(r).User, id, req)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "domain.update", dm.Name, "")
	writeJSON(w, http.StatusOK, dm)
}

func (s *Server) deleteDomain(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Domains.Delete(r.Context(), ident(r).User, id); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "domain.delete", "", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) issueSSL(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	u := ident(r).User
	dm, err := s.DB.DomainByID(r.Context(), id)
	if err != nil || dm.UserID != u.ID {
		writeErr(w, http.StatusNotFound, "not_found", "nie znaleziono")
		return
	}
	if dm.Type == "alias" {
		writeErr(w, http.StatusBadRequest, "invalid", "certyfikat wystaw dla domeny nadrzędnej (alias zostanie dołączony)")
		return
	}
	if err := s.ACME.Issue(r.Context(), dm); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "ssl.issue", dm.Name, "")
	out, _ := s.DB.DomainByID(r.Context(), id)
	writeJSON(w, http.StatusOK, out)
}
