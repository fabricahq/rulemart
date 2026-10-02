package migrate

import (
	"context"
	"io/fs"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"

	"github.com/fabricahq/rulemart/db"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// migrationsThrough returns the embedded migrations up to and including version last, as a release that predates the
// others shipped them.
func migrationsThrough(t *testing.T, last int64) fs.FS {
	t.Helper()
	migrations := fstest.MapFS{}
	names, err := fs.Glob(db.Migrations, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		version, err := goose.NumericComponent(path.Base(name))
		if err != nil {
			t.Fatal(err)
		}
		if version > last {
			continue
		}
		content, err := fs.ReadFile(db.Migrations, name)
		if err != nil {
			t.Fatal(err)
		}
		migrations[path.Base(name)] = &fstest.MapFile{Data: content}
	}
	return migrations
}

// insertCurrentRule stores a library holding one current rule, with the given title, reading guidance, and body, as
// a release's ingestion would, and returns the rule version's id.
func insertCurrentRule(t *testing.T, ctx context.Context, conn *pgx.Conn, title, whenToRead, markdown string) int64 {
	t.Helper()
	var library, group, release, rule, version int64
	scan := func(sql string, dest *int64, args ...any) {
		t.Helper()
		if err := conn.QueryRow(ctx, sql, args...).Scan(dest); err != nil {
			t.Fatalf("run %q: %v", sql, err)
		}
	}
	scan(`INSERT INTO libraries (host, host_repository_id, owner, name, description, owner_avatar_url)
		VALUES ('github', '1', 'example', 'rules', '', '') RETURNING id`, &library)
	scan(`INSERT INTO library_groups (library_id, path, name, description, when_to_read)
		VALUES ($1, 'techs/go', 'Go', 'Go rules.', 'When writing Go.') RETURNING id`, &group, library)
	scan(`INSERT INTO library_releases (library_id, number, commit_id, tagged_at, updates_shared_files)
		VALUES ($1, 1, 'commit', now(), false) RETURNING id`, &release, library)
	scan(`INSERT INTO rules (library_id, group_id, path) VALUES ($1, $2, 'techs/go/a') RETURNING id`, &rule, library, group)
	scan(`INSERT INTO rule_versions (library_id, rule_id, release_id, major, minor, patch, change, summaries, title, impact,
		impact_description, when_to_read, markdown, html)
		VALUES ($1, $2, $3, 1, 0, 0, 'new', '{Add the rule.}', $4, 'HIGH', 'Prevents mistakes.', $5, $6, '<p>Rule.</p>')
		RETURNING id`, &version, library, rule, release, title, whenToRead, markdown)
	return version
}

// Migrations run while the previous release still serves the site, so rules it ingested before the search migration
// must be searchable once it has run, and nothing new is asked of the release still writing them.
func TestSearchMigrationMakesRulesStoredBeforeItSearchable(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	if _, err := up(ctx, connString, migrationsThrough(t, 5)); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	version := insertCurrentRule(t, ctx, conn, "Return errors", "When handling failures.", "Wrap every returned error with context.")

	if _, err := Up(ctx, connString); err != nil {
		t.Fatal(err)
	}

	for query, want := range map[string]bool{"return errors": true, "failure": true, "wrapping context": true, "retry": false} {
		var found bool
		err := conn.QueryRow(ctx, `SELECT search_document @@ websearch_to_tsquery('english', $1) FROM rule_versions WHERE id = $2`,
			query, version).Scan(&found)
		if err != nil {
			t.Fatal(err)
		}
		if found != want {
			t.Errorf("searching for %q found the rule: %v, want %v", query, found, want)
		}
	}
}

// A rule file may hold 1 MiB, but a search document only 1 MB, so the document reads a bounded prefix of each part:
// a rule too long to search in full is still stored, rather than failing its library's ingestion.
func TestSearchDocumentOfARuleTooLongToSearchInFullStillStores(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	if _, err := Up(ctx, connString); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	// Distinct two-letter words of four-byte letters make the largest vector per character of text: in full, 1 MiB
	// of them would make a vector of almost 2 MB.
	var words strings.Builder
	for i := 0; words.Len() < 1<<20; i++ {
		words.WriteRune(rune(0x20000 + i/40000))
		words.WriteRune(rune(0x20000 + i%40000))
		words.WriteByte(' ')
	}
	text := words.String()

	version := insertCurrentRule(t, ctx, conn, text, text, "Opening words. "+text)

	var found bool
	err = conn.QueryRow(ctx, `SELECT search_document @@ websearch_to_tsquery('english', 'opening words') FROM rule_versions WHERE id = $1`,
		version).Scan(&found)
	if err != nil || !found {
		t.Fatalf("the start of the rule isn't searchable: %v, %v", found, err)
	}
}
