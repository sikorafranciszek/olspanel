package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// pmaProxy forwards /phpmyadmin/* to the OLS vhost bound to 127.0.0.1:8081.
// Only authenticated panel users reach it (enforced by the router).
func (s *Server) pmaProxy() http.Handler {
	target, err := url.Parse(s.Cfg.PMAUpstream)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeErr(w, http.StatusServiceUnavailable, "pma", "phpMyAdmin nie jest skonfigurowany")
		})
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/phpmyadmin")
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		req.Host = target.Host
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Forwarded-Prefix", "/phpmyadmin")
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Rewrite absolute redirects back under the prefix.
		if loc := resp.Header.Get("Location"); loc != "" && strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "/phpmyadmin") {
			resp.Header.Set("Location", "/phpmyadmin"+loc)
		}
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		writeErr(w, http.StatusBadGateway, "pma", "phpMyAdmin nie odpowiada")
	}
	return proxy
}
