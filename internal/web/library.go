package web

import "net/http"

// handleRescan starts an incremental library scan and answers at once with 202.
// The state's scan field shows it running and, later, how it ended. A scan that
// is already running is a 409. There is deliberately no force option here: it
// clears the library when the folder looks empty.
func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request, id Identity) {
	if err := s.bot.StartRescan(); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeState(w, http.StatusAccepted, id)
}
