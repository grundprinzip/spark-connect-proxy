package control

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/grundprinzip/spark-connect-proxy/internal/proxy"
)

func handleDeleteSesssion(state *proxy.ProxyState, l *slog.Logger, w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		http.Error(w, "No session ID provided", http.StatusBadRequest)
	}
	l.Debug("Deleting session", "id", id)
	if err := state.StopSession(id); err != nil {
		l.Error("Could not stop session", "id", id, "error", err)
		http.Error(w, "Could not stop session", http.StatusInternalServerError)
	}
}

func handleNewSession(state *proxy.ProxyState, l *slog.Logger, w http.ResponseWriter, r *http.Request) {
	id, err := state.StartSession()
	if err != nil {
		http.Error(w, "Could not create session", http.StatusInternalServerError)
	}
	l.Debug("Created session", "id", id)
	if _, err = w.Write([]byte(id)); err != nil {
		l.Error("Could not write session ID", "error", err)
		http.Error(w, "Could not write session ID", http.StatusInternalServerError)
	}
}
