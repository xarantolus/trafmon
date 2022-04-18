package web

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/xarantolus/trafmon/app/web/query"

	"github.com/gorilla/mux"
)

func (s *Server) handleReposAPI(w http.ResponseWriter, r *http.Request) (err error) {
	repos, err := query.AllRepos(s.Manager.Database)
	if err != nil {
		return
	}
	return serveJSON(w, r, repos)
}

func (s *Server) handleRepoStatsAPI(w http.ResponseWriter, r *http.Request) (err error) {
	vars := mux.Vars(r)
	username, reponame := vars["username"], vars["reponame"]

	repo, err := query.Repository(s.Manager.Database, username, reponame)
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	} else if err != nil {
		return err
	}

	clonesChart, err := query.ClonesChart(s.Manager.Database, repo.ID)
	if err != nil {
		return
	}

	return serveJSON(w, r, map[string]any{
		"repository": repo,
		"clones":     clonesChart,
	})
}
