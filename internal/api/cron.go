package api

import (
	"net/http"

	"olspanel/internal/cron"
)

func (s *Server) listCron(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListUserCron(r.Context(), ident(r).User.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createCron(w http.ResponseWriter, r *http.Request) {
	var req cron.Request
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	j, err := s.Cron.Create(r.Context(), ident(r).User, req)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "cron.create", j.Schedule, "")
	writeJSON(w, http.StatusCreated, j)
}

func (s *Server) updateCron(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req cron.Request
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	j, err := s.Cron.Update(r.Context(), ident(r).User, id, req)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "cron.update", j.Schedule, "")
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) deleteCron(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Cron.Delete(r.Context(), ident(r).User, id); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "cron.delete", "", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
