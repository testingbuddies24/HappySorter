package httpserver

import "net/http"

func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request) {
	s.watcher.Rescan()
	redirectFlash(w, r, "/", "Rescan triggered.", false)
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	s.watcher.Pause()
	redirectFlash(w, r, "/", "Watcher paused.", false)
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	s.watcher.Resume()
	redirectFlash(w, r, "/", "Watcher resumed.", false)
}
