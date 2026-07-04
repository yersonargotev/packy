package dashboard

import (
	"errors"
	"net/http"
)

var errUnauthorized = errors.New("dashboard: unauthorized")

func (h *handlers) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.cfg.RequireSession != nil {
			if err := h.cfg.RequireSession(r); err != nil {
				loginPath := dashboardLoginPathWithNext(r.URL.RequestURI())
				if isHTMXRequest(r) {
					w.Header().Set("HX-Redirect", loginPath)
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				http.Redirect(w, r, loginPath, http.StatusSeeOther)
				return
			}
		}
		next(w, r)
	}
}
