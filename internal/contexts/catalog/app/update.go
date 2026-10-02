// Keep a vetted or listed library current: check its release tags cheaply, and ingest it only when they changed.

package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// Update is what one update of a library did.
type Update struct {
	// Ingested reports whether the update ingested the library, because its release tags weren't the ones the
	// catalog stored; Result is then what the ingestion did.
	Ingested bool
	Result   Result
	// ListTime is how long listing the release tags took; 0 when the catalog had no clone URL to list.
	ListTime time.Duration
	// IngestTime is how long looking the repository up and ingesting it took; 0 when the tags were unchanged.
	IngestTime time.Duration
}

// Update makes the catalog's rows for library match its release tags, doing as little as it can. It lists the tags
// at the clone URL the catalog stored, and stops when they're the ones it stored. Otherwise, or when the catalog
// doesn't have the library or its tags yet, it looks the repository up by its ID on its code host and ingests it with
// IngestRepository. Running it again on unchanged tags lists them and changes nothing. It writes nothing when it
// fails; errors name the library.
func (in Ingester) Update(ctx context.Context, library domain.LibraryKey) (Update, error) {
	update, err := in.update(ctx, library)
	if err != nil {
		return Update{}, fmt.Errorf("update library host=%s repository=%s: %v", library.Host, library.RepositoryID, err)
	}
	return update, nil
}

func (in Ingester) update(ctx context.Context, library domain.LibraryKey) (Update, error) {
	if library.Host != domain.GitHub {
		return Update{}, errors.New("github is the only code host Rulemart reads libraries from")
	}
	return in.updateFrom(ctx, library, nil)
}

// updateFrom updates library as Update describes, ingesting repo, when it isn't nil, rather than looking the
// repository up by its ID: a check that just looked it up by name has it.
func (in Ingester) updateFrom(ctx context.Context, library domain.LibraryKey, repo *domain.Repository) (Update, error) {
	current, listTime, err := in.current(ctx, library)
	if err != nil || current {
		return Update{ListTime: listTime}, err
	}
	started := time.Now()
	if repo == nil {
		found, err := in.Repositories.RepositoryByID(ctx, library.RepositoryID)
		if err != nil {
			return Update{ListTime: listTime}, libraryError(err, domain.ErrNoPublicRepository)
		}
		repo = &found
	}
	result, err := in.IngestRepository(ctx, *repo)
	if err != nil {
		return Update{ListTime: listTime}, err
	}
	return Update{Ingested: true, Result: result, ListTime: listTime, IngestTime: time.Since(started)}, nil
}

// current reports whether the release tags at the library's stored clone URL are the ones the catalog stored, and how
// long listing them took. It's false, without listing anything, when the catalog doesn't have the library or its
// clone URL.
func (in Ingester) current(ctx context.Context, library domain.LibraryKey) (bool, time.Duration, error) {
	checkpoint, found, err := in.Store.Checkpoint(ctx, library)
	if err != nil || !found || checkpoint.CloneURL == "" {
		return false, 0, err
	}
	started := time.Now()
	listed, err := in.List(ctx, checkpoint.CloneURL, in.Limits.Fetch)
	listTime := time.Since(started)
	if err != nil {
		return false, listTime, &LibraryError{Err: fmt.Errorf("check release tags url=%q: %v", checkpoint.CloneURL, err)}
	}
	return checkpoint.Current(listed), listTime, nil
}
