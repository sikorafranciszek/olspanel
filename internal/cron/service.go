// Package cron stores per-user cron jobs and renders the user's crontab.
package cron

import (
	"context"
	"errors"
	"fmt"
	"strings"

	robfig "github.com/robfig/cron/v3"

	"olspanel/internal/db"
	"olspanel/internal/system"
	"olspanel/internal/users"
	"olspanel/internal/validate"
)

var parser = robfig.NewParser(robfig.Minute | robfig.Hour | robfig.Dom | robfig.Month | robfig.Dow)

// Service manages cron jobs.
type Service struct {
	DB       *db.DB
	NoSystem bool
}

// Request is the user's input.
type Request struct {
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Enabled  bool   `json:"enabled"`
}

func (r *Request) validate() error {
	r.Schedule = strings.Join(strings.Fields(r.Schedule), " ")
	if _, err := parser.Parse(r.Schedule); err != nil {
		return errors.Join(validate.ErrInvalid, errors.New("nieprawidłowy harmonogram (format: min godz dzień miesiąc dzień_tyg)"))
	}
	r.Command = strings.TrimSpace(r.Command)
	return validate.CronCommand(r.Command)
}

// Create adds a job.
func (s *Service) Create(ctx context.Context, u *db.User, req Request) (*db.CronJob, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	pkg, err := s.DB.PackageByID(ctx, u.PackageID)
	if err != nil {
		return nil, errors.New("konto nie ma przypisanego pakietu")
	}
	n, _ := s.DB.CountUserCron(ctx, u.ID)
	if err := users.CheckLimit(n, pkg.MaxCron, "zadania cron"); err != nil {
		return nil, err
	}
	id, err := s.DB.CreateCron(ctx, &db.CronJob{UserID: u.ID, Schedule: req.Schedule, Command: req.Command, Enabled: req.Enabled})
	if err != nil {
		return nil, err
	}
	if err := s.Sync(ctx, u); err != nil {
		return nil, err
	}
	return s.DB.CronByID(ctx, id)
}

// Update edits a job.
func (s *Service) Update(ctx context.Context, u *db.User, id int64, req Request) (*db.CronJob, error) {
	j, err := s.owned(ctx, u, id)
	if err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}
	j.Schedule, j.Command, j.Enabled = req.Schedule, req.Command, req.Enabled
	if err := s.DB.UpdateCron(ctx, j); err != nil {
		return nil, err
	}
	if err := s.Sync(ctx, u); err != nil {
		return nil, err
	}
	return s.DB.CronByID(ctx, id)
}

// Delete removes a job.
func (s *Service) Delete(ctx context.Context, u *db.User, id int64) error {
	if _, err := s.owned(ctx, u, id); err != nil {
		return err
	}
	if err := s.DB.DeleteCron(ctx, id); err != nil {
		return err
	}
	return s.Sync(ctx, u)
}

func (s *Service) owned(ctx context.Context, u *db.User, id int64) (*db.CronJob, error) {
	j, err := s.DB.CronByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if j.UserID != u.ID {
		return nil, db.ErrNotFound
	}
	return j, nil
}

// Render produces the full crontab text for a list of jobs.
func Render(jobs []db.CronJob) string {
	var b strings.Builder
	b.WriteString("# Zarządzane przez olspanel - zmiany wprowadzaj w panelu.\n")
	b.WriteString("MAILTO=\"\"\n")
	b.WriteString("SHELL=/bin/sh\n")
	b.WriteString("PATH=/usr/local/bin:/usr/bin:/bin\n")
	for _, j := range jobs {
		cmd := strings.ReplaceAll(j.Command, "%", "\\%")
		if !j.Enabled {
			b.WriteString("#DISABLED ")
		}
		fmt.Fprintf(&b, "%s %s\n", j.Schedule, cmd)
	}
	return b.String()
}

// Sync writes the user's crontab from the database.
func (s *Service) Sync(ctx context.Context, u *db.User) error {
	if s.NoSystem || u.Role != "user" {
		return nil
	}
	jobs, err := s.DB.ListUserCron(ctx, u.ID)
	if err != nil {
		return err
	}
	_, err = system.Run(ctx, "crontab", Render(jobs), "-u", u.Username, "-")
	return err
}

// CleanupUser implements users.Cleaner: removes the crontab.
func (s *Service) CleanupUser(ctx context.Context, u *db.User) error {
	if s.NoSystem || u.Role != "user" {
		return nil
	}
	_, _ = system.Run(ctx, "crontab", "", "-u", u.Username, "-r")
	return nil
}
