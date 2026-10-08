// Package acme issues and renews Let's Encrypt certificates with lego (HTTP-01
// via a panel-owned webroot that OLS maps on every vhost).
package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	legoacme "github.com/go-acme/lego/v5/acme"
	"github.com/go-acme/lego/v5/certcrypto"
	"github.com/go-acme/lego/v5/certificate"
	"github.com/go-acme/lego/v5/lego"
	"github.com/go-acme/lego/v5/providers/http/webroot"
	"github.com/go-acme/lego/v5/registration"

	"olspanel/internal/db"
)

// Applier re-renders OLS after a certificate changes.
type Applier interface {
	Apply(ctx context.Context) error
}

// Service manages ACME accounts and certificates.
type Service struct {
	DB      *db.DB
	DataDir string // /var/lib/olspanel
	OLS     Applier
	// OnPanelCert is called after the panel's own certificate is renewed.
	OnPanelCert func(certPEM, keyPEM []byte)
	NoSystem    bool

	mu sync.Mutex
}

type account struct {
	Email        string                    `json:"email"`
	Registration *legoacme.ExtendedAccount `json:"registration"`
	key          crypto.Signer
}

func (a *account) GetEmail() string                           { return a.Email }
func (a *account) GetRegistration() *legoacme.ExtendedAccount { return a.Registration }
func (a *account) GetPrivateKey() crypto.Signer               { return a.key }

func (s *Service) staging(ctx context.Context) bool {
	return s.DB.Setting(ctx, "acme_staging", "0") == "1"
}

func (s *Service) caDir(ctx context.Context) (string, string) {
	if s.staging(ctx) {
		return lego.DirectoryURLLetsEncryptStaging, "staging"
	}
	return lego.DirectoryURLLetsEncrypt, "production"
}

func (s *Service) loadOrCreateAccount(ctx context.Context, caName, email string) (*account, error) {
	dir := filepath.Join(s.DataDir, "acme", caName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dir, "account.key")
	acc := &account{Email: email}
	if b, err := os.ReadFile(keyPath); err == nil {
		blk, _ := pem.Decode(b)
		if blk == nil {
			return nil, errors.New("uszkodzony klucz konta ACME")
		}
		k, err := x509.ParseECPrivateKey(blk.Bytes)
		if err != nil {
			return nil, err
		}
		acc.key = k
	} else {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		der, _ := x509.MarshalECPrivateKey(k)
		if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			return nil, err
		}
		acc.key = k
	}
	if b, err := os.ReadFile(filepath.Join(dir, "account.json")); err == nil {
		var saved account
		if json.Unmarshal(b, &saved) == nil && saved.Email == email {
			acc.Registration = saved.Registration
		}
	}
	return acc, nil
}

func (s *Service) saveAccount(caName string, acc *account) error {
	b, err := json.MarshalIndent(acc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.DataDir, "acme", caName, "account.json"), b, 0o600)
}

func (s *Service) client(ctx context.Context, webrootDir string) (*lego.Client, error) {
	email := s.DB.Setting(ctx, "acme_email", "")
	if email == "" {
		return nil, errors.New("ustaw adres e-mail dla Let's Encrypt w ustawieniach panelu")
	}
	caURL, caName := s.caDir(ctx)
	acc, err := s.loadOrCreateAccount(ctx, caName, email)
	if err != nil {
		return nil, err
	}
	cfg := lego.NewConfig(acc)
	cfg.CADirURL = caURL
	cfg.UserAgent = "olspanel"
	cl, err := lego.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	prov, err := webroot.NewHTTPProvider(webrootDir)
	if err != nil {
		return nil, err
	}
	if err := cl.Challenge.SetHTTP01Provider(prov); err != nil {
		return nil, err
	}
	if acc.Registration == nil {
		reg, err := cl.Registration.Register(ctx, registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return nil, fmt.Errorf("rejestracja konta ACME: %w", err)
		}
		acc.Registration = reg
		if err := s.saveAccount(caName, acc); err != nil {
			return nil, err
		}
	}
	return cl, nil
}

// certDir returns where a domain's cert lives.
func (s *Service) certDir(name string) string { return filepath.Join(s.DataDir, "ssl", name) }

func (s *Service) store(name string, res *certificate.Resource) error {
	dir := s.certDir(name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "privkey.pem.tmp"), res.PrivateKey, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "fullchain.pem.tmp"), res.Certificate, 0o600); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(dir, "privkey.pem.tmp"), filepath.Join(dir, "privkey.pem")); err != nil {
		return err
	}
	return os.Rename(filepath.Join(dir, "fullchain.pem.tmp"), filepath.Join(dir, "fullchain.pem"))
}

// Expiry parses the stored certificate's NotAfter.
func (s *Service) Expiry(name string) (time.Time, error) {
	b, err := os.ReadFile(filepath.Join(s.certDir(name), "fullchain.pem"))
	if err != nil {
		return time.Time{}, err
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return time.Time{}, errors.New("uszkodzony certyfikat")
	}
	c, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return time.Time{}, err
	}
	return c.NotAfter, nil
}

// hostsFor returns the SAN list for a domain row: name, www.name and aliases (+www).
func (s *Service) hostsFor(ctx context.Context, dm *db.Domain) []string {
	hosts := []string{dm.Name, "www." + dm.Name}
	all, _ := s.DB.ListUserDomains(ctx, dm.UserID)
	for _, d := range all {
		if d.Type == "alias" && d.ParentID != nil && *d.ParentID == dm.ID {
			hosts = append(hosts, d.Name, "www."+d.Name)
		}
	}
	return hosts
}

// Issue obtains a certificate for a domain and re-renders OLS.
func (s *Service) Issue(ctx context.Context, dm *db.Domain) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.NoSystem {
		return errors.New("wystawianie certyfikatów nie jest dostępne w trybie deweloperskim")
	}
	webrootDir := filepath.Join(s.DataDir, "acme-challenge", dm.Name)
	if err := os.MkdirAll(filepath.Join(webrootDir, ".well-known", "acme-challenge"), 0o755); err != nil {
		return err
	}
	cl, err := s.client(ctx, webrootDir)
	if err != nil {
		return err
	}
	hosts := s.hostsFor(ctx, dm)
	res, err := s.obtainBestEffort(ctx, cl, hosts)
	if err != nil {
		dm.SSLStatus = "error"
		_ = s.DB.UpdateDomain(ctx, dm)
		return fmt.Errorf("Let's Encrypt: %w", err)
	}
	if err := s.store(dm.Name, res); err != nil {
		return err
	}
	exp, _ := s.Expiry(dm.Name)
	es := exp.UTC().Format(time.RFC3339)
	dm.SSLStatus = "active"
	dm.SSLExpiresAt = &es
	if err := s.DB.UpdateDomain(ctx, dm); err != nil {
		return err
	}
	return s.OLS.Apply(ctx)
}

// obtainBestEffort tries all hosts first, then falls back to the bare domain
// alone (www/aliases may lack DNS records).
func (s *Service) obtainBestEffort(ctx context.Context, cl *lego.Client, hosts []string) (*certificate.Resource, error) {
	res, err := cl.Certificate.Obtain(ctx, certificate.ObtainRequest{Domains: hosts, Bundle: true, KeyType: certcrypto.EC256})
	if err == nil {
		return res, nil
	}
	if len(hosts) == 1 {
		return nil, err
	}
	slog.Warn("acme: full SAN set failed, retrying with bare domain", "hosts", hosts, "err", err)
	return cl.Certificate.Obtain(ctx, certificate.ObtainRequest{Domains: hosts[:1], Bundle: true, KeyType: certcrypto.EC256})
}

// RenewDue renews every active certificate that expires within 30 days.
func (s *Service) RenewDue(ctx context.Context) {
	if s.NoSystem {
		return
	}
	all, err := s.DB.ListDomains(ctx)
	if err != nil {
		return
	}
	for i := range all {
		dm := &all[i]
		if dm.SSLStatus != "active" || dm.Type == "alias" {
			continue
		}
		exp, err := s.Expiry(dm.Name)
		if err != nil || time.Until(exp) > 30*24*time.Hour {
			continue
		}
		slog.Info("acme: renewing", "domain", dm.Name, "expires", exp)
		if err := s.Issue(ctx, dm); err != nil {
			slog.Error("acme: renew failed", "domain", dm.Name, "err", err)
		}
	}
	s.renewPanel(ctx)
}

// IssuePanel obtains a certificate for the panel hostname (served on :2222
// and used as the OLS fallback cert and FTP cert).
func (s *Service) IssuePanel(ctx context.Context, hostname string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.NoSystem {
		return errors.New("niedostępne w trybie deweloperskim")
	}
	webrootDir := filepath.Join(s.DataDir, "acme-challenge", "_default")
	if err := os.MkdirAll(filepath.Join(webrootDir, ".well-known", "acme-challenge"), 0o755); err != nil {
		return err
	}
	cl, err := s.client(ctx, webrootDir)
	if err != nil {
		return err
	}
	res, err := cl.Certificate.Obtain(ctx, certificate.ObtainRequest{Domains: []string{hostname}, Bundle: true, KeyType: certcrypto.EC256})
	if err != nil {
		return fmt.Errorf("Let's Encrypt: %w", err)
	}
	if err := s.store("_panel", res); err != nil {
		return err
	}
	if s.OnPanelCert != nil {
		s.OnPanelCert(res.Certificate, res.PrivateKey)
	}
	return nil
}

func (s *Service) renewPanel(ctx context.Context) {
	host := s.DB.Setting(ctx, "panel_hostname", "")
	if host == "" {
		return
	}
	exp, err := s.Expiry("_panel")
	if err != nil || time.Until(exp) > 30*24*time.Hour {
		return
	}
	if err := s.IssuePanel(ctx, host); err != nil {
		slog.Error("acme: panel renew failed", "err", err)
	}
}
