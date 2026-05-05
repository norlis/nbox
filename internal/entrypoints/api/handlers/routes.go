package handlers

import "net/http"

type Route interface {
	Register(mux *http.ServeMux)
}
