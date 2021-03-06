package store

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/go-github/v43/github"
)

func (m *Manager) StartBackgroundTasks() (err error) {

	go m.runBackgroundTasks()

	return
}

func (m *Manager) runBackgroundTasks() {
	for {
		log.Printf("[Background] Start running background tasks")

		err := m.runReposUpdate()
		if err != nil {
			log.Printf("[Warning] Running repo data update: %s\n", err.Error())
		}

		nextTime := getNextDay()
		waitDur := time.Until(nextTime)
		if waitDur < 0 {
			panic("invalid nextTime when running background tasks: got " + nextTime.String())
		}

		log.Printf("[Background] Running next background tasks on %s", nextTime.String())
		time.Sleep(waitDur)
	}
}

// getNextDay returns a time after midnight
func getNextDay() time.Time {
	now := time.Now().UTC()

	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 5, 0, 0, time.UTC)
}

func getOwnerName(repo *github.Repository) string {
	owner := "unknown"
	if repo.Owner != nil {
		owner = repo.Owner.GetLogin()
	}
	return owner
}

func (m *Manager) runReposUpdate() (err error) {
	ctx := context.Background()

	repos, err := m.fetchAllRepos(ctx)
	if err != nil {
		return
	}

	for _, repo := range repos {
		rerr := m.processRepo(ctx, repo)
		if rerr != nil {
			log.Printf("[Warning] Updating repo data for %s/%s: %s\n", getOwnerName(repo), repo.GetName(), rerr.Error())
		}
	}

	log.Printf("[Background] Finished working on %d repos", len(repos))

	return
}

func (m *Manager) fetchAllRepos(ctx context.Context) (repos []*github.Repository, err error) {
	var repoPage = 1

	for {
		log.Printf("[Background] Fetching page %d of repositories", repoPage)

		// Get Repos for the currently logged in user
		fetched, resp, err := m.GitHub.Repositories.List(ctx, "", &github.RepositoryListOptions{
			Visibility:  "all",
			Affiliation: "owner,collaborator,organization_member",

			ListOptions: github.ListOptions{
				Page:    repoPage,
				PerPage: 100,
			},
		})
		if err != nil || len(fetched) == 0 {
			return repos, err
		}

		repos = append(repos, fetched...)
		log.Printf("[Background] Got %d repos, now have %d", len(fetched), len(repos))

		repoPage = resp.NextPage
		if repoPage == 0 {
			break
		}
	}

	return
}

func (m *Manager) processRepo(ctx context.Context, repo *github.Repository) (err error) {
	var (
		repoUser = getOwnerName(repo)
		repoName = repo.GetName()
	)
	log.Printf("[Background] Working on %s/%s", repoUser, repoName)

	// Insert or update repository info on every run (e.g. if a repo was renamed or the description changed)
	_, err = m.Database.Exec(`insert into Repositories(id, username, name, description, is_fork) values ($1, $2, $3, $4, $5)
			on conflict (id) do update set username=EXCLUDED.username, name=EXCLUDED.name, description=EXCLUDED.description, is_fork=EXCLUDED.is_fork`,
		repo.ID, repoUser, repoName, repo.GetDescription(), repo.GetFork())
	if err != nil {
		return fmt.Errorf("inserting basic repo: %s", err.Error())
	}

	// Then keep a log of all repo stats
	// Important: repo.GetWatchersCount() just returns the stargazers count, we need subscribers
	_, err = m.Database.Exec(`insert into RepoStats(repo_id, stars, forks, size, watchers, date_time)
								 values ($1, $2, $3, $4, $5, $6)
  							on conflict (repo_id, date_time) do update set stars=EXCLUDED.stars, forks=EXCLUDED.forks, size=EXCLUDED.size, watchers=EXCLUDED.watchers, date_time=EXCLUDED.date_time`,
		repo.ID, repo.GetStargazersCount(), repo.GetForksCount(), repo.GetSize(), repo.GetSubscribersCount(), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("inserting basic repo info: %s", err.Error())
	}

	views, _, err := m.GitHub.Repositories.ListTrafficViews(ctx, repoUser, repoName, &github.TrafficBreakdownOptions{
		Per: "day",
	})
	if err != nil {
		return fmt.Errorf("fetching traffic views: %s", err.Error())
	}
	for _, day := range views.Views {
		_, err = m.Database.Exec(`insert into RepoTrafficViews(repo_id, date, count, uniques)
								 values ($1, $2, $3, $4)
  							on conflict (repo_id, date) do update set count=EXCLUDED.count, uniques=EXCLUDED.uniques`,
			repo.ID, day.GetTimestamp().Time.UTC(), day.GetCount(), day.GetUniques(),
		)
		if err != nil {
			return fmt.Errorf("inserting traffic view data: %s", err.Error())
		}
	}

	paths, _, err := m.GitHub.Repositories.ListTrafficPaths(ctx, repoUser, repoName)
	if err != nil {
		return fmt.Errorf("fetching traffic paths: %s", err.Error())
	}
	for _, path := range paths {
		_, err = m.Database.Exec(`insert into RepoTrafficPaths(repo_id, path, title, count, uniques, date_time)
								 values ($1, $2, $3, $4, $5, $6)
  							on conflict (repo_id, date_time, path) do update set count=EXCLUDED.count, uniques=EXCLUDED.uniques, date_time=EXCLUDED.date_time`,
			repo.ID, path.GetPath(), path.GetTitle(), path.GetCount(), path.GetUniques(), time.Now().UTC(),
		)
		if err != nil {
			return fmt.Errorf("inserting traffic path data: %s", err.Error())
		}
	}

	refs, _, err := m.GitHub.Repositories.ListTrafficReferrers(ctx, repoUser, repoName)
	if err != nil {
		return fmt.Errorf("fetching traffic referrers: %s", err.Error())
	}
	for _, ref := range refs {
		_, err = m.Database.Exec(`insert into RepoTrafficReferrers(repo_id, referrer, count, uniques, date_time)
								 values ($1, $2, $3, $4, $5)
  							on conflict (repo_id, date_time, referrer) do update set count=EXCLUDED.count, uniques=EXCLUDED.uniques`,
			repo.ID, ref.GetReferrer(), ref.GetCount(), ref.GetUniques(), time.Now().UTC(),
		)
		if err != nil {
			return fmt.Errorf("inserting traffic referrer data: %s", err.Error())
		}
	}

	clones, _, err := m.GitHub.Repositories.ListTrafficClones(ctx, repoUser, repoName, &github.TrafficBreakdownOptions{
		Per: "day",
	})
	if err != nil {
		return fmt.Errorf("fetching traffic clones: %s", err.Error())
	}
	for _, clone := range clones.Clones {
		_, err = m.Database.Exec(`insert into RepoTrafficClones(repo_id, date, count, uniques)
								 values ($1, $2, $3, $4)
  							on conflict (repo_id, date) do update set count=EXCLUDED.count, uniques=EXCLUDED.uniques`,
			repo.ID, clone.GetTimestamp().Time.UTC(), clone.GetCount(), clone.GetUniques(),
		)
		if err != nil {
			return fmt.Errorf("inserting traffic referrer data: %s", err.Error())
		}
	}

	releases, err := fetchAllPages(func(opts github.ListOptions) ([]*github.RepositoryRelease, *github.Response, error) {
		return m.GitHub.Repositories.ListReleases(ctx, getOwnerName(repo), repo.GetName(), &opts)
	}, 100)
	if err != nil {
		return fmt.Errorf("fetching releases: %s", err.Error())
	}

	for _, release := range releases {
		_, err = m.Database.Exec(`insert into Releases(id, repo_id, tag_name, created, name, body)
								 values ($1, $2, $3, $4, $5, $6)
			on conflict (id) do update set repo_id=EXCLUDED.repo_id, tag_name=EXCLUDED.tag_name, created=EXCLUDED.created, name=EXCLUDED.name, body=EXCLUDED.body`,
			release.ID, repo.ID, release.GetTagName(), release.GetCreatedAt().Time.UTC(), release.GetName(), release.GetBody(),
		)
		if err != nil {
			return fmt.Errorf("inserting basic release data: %s", err.Error())
		}

		for _, asset := range release.Assets {
			_, err = m.Database.Exec(`insert into ReleaseAssets(id, release_id, filename, download_count, updated_at, size, date_time)
								 values ($1, $2, $3, $4, $5, $6, $7)
			on conflict (id, release_id) do update set filename=EXCLUDED.filename, download_count=EXCLUDED.download_count, updated_at=EXCLUDED.updated_at, size=EXCLUDED.size, date_time=EXCLUDED.date_time`,
				asset.ID, release.ID, asset.GetName(), asset.GetDownloadCount(), asset.GetUpdatedAt().Time, asset.GetSize(), time.Now().UTC(),
			)
			if err != nil {
				return fmt.Errorf("inserting release asset data: %s", err.Error())
			}
		}
	}

	return
}
