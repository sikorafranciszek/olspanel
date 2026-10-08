package api

import "net/http"

func (s *Server) listDatabases(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListUserDatabases(r.Context(), ident(r).User.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createDatabase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Suffix string `json:"suffix"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	d, err := s.MariaDB.Create(r.Context(), ident(r).User, req.Suffix)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "db.create", d.Name, "")
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) deleteDatabase(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.MariaDB.Delete(r.Context(), ident(r).User, id); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "db.delete", "", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) createDBUser(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req struct {
		Suffix   string `json:"suffix"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	du, err := s.MariaDB.CreateUser(r.Context(), ident(r).User, id, req.Suffix, req.Password)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "dbuser.create", du.Username, "")
	writeJSON(w, http.StatusCreated, du)
}

func (s *Server) updateDBUser(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	uid, err := idParam(r, "uid")
	if err != nil {
		fail(w, err)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.MariaDB.SetUserPassword(r.Context(), ident(r).User, id, uid, req.Password); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "dbuser.password", "", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) deleteDBUser(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	uid, err := idParam(r, "uid")
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.MariaDB.DeleteUser(r.Context(), ident(r).User, id, uid); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "dbuser.delete", "", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
