package migrate

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// Migrations run while the previous release still serves the site, and it stores content only on each rule's current
// version. Once every version may keep its content, the rows it stored, and the rows it goes on storing, must still
// fit, while a version with only part of its content, or HTML without its Markdown, still doesn't.
func TestEveryVersionMayKeepItsContentAndRowsWithoutItStillFit(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	if _, err := up(ctx, connString, migrationsThrough(t, 7)); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	current := insertCurrentRule(t, ctx, conn, "Return errors", "When handling failures.", "---\ntitle: Return errors\n---\nWrap errors.")
	// insertVersion stores version major.0.0 of the rule, published by library release major, with columns set to
	// values.
	insertVersion := func(major int, columns, values string) error {
		_, err := conn.Exec(ctx, `WITH release AS (
				INSERT INTO library_releases (library_id, number, commit_id, tagged_at, updates_shared_files)
				SELECT library_id, $2, 'commit', now(), false FROM rule_versions WHERE id = $1 RETURNING id
			)
			INSERT INTO rule_versions (library_id, rule_id, release_id, major, minor, patch, change, summaries`+columns+`)
			SELECT v.library_id, v.rule_id, release.id, $2, 0, 0, 'major', '{Change it.}'`+values+`
			FROM rule_versions v, release WHERE v.id = $1`,
			current, major)
		return err
	}
	// An older version, as the previous release stores it: without content.
	if err := insertVersion(2, "", ""); err != nil {
		t.Fatalf("store a version without content before the migration: %v", err)
	}

	if _, err := Up(ctx, connString); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		major           int
		columns, values string
		refused         string
	}{
		"without content, as the previous release stores it": {major: 3},
		"with its content and no HTML": {major: 4,
			columns: ", title, impact, impact_description, when_to_read, markdown",
			values:  ", 'Return errors', 'HIGH', 'Prevents mistakes.', 'When handling failures.', '---\ntitle: Return errors\n---\nOld.'"},
		"with part of its content": {major: 5, columns: ", title, markdown", values: ", 'Return errors', 'Old.'",
			refused: "rule_versions_content_check"},
		"with HTML but no Markdown": {major: 6, columns: ", html", values: ", '<p>Old.</p>'", refused: "rule_versions_html_check"},
		"retired, with its last body's HTML": {major: 7,
			columns: ", title, impact, impact_description, when_to_read, markdown, retired_html",
			values:  ", 'Return errors', 'HIGH', 'Prevents mistakes.', 'When handling failures.', '---\ntitle: Return errors\n---\nOld.', '<p>Old.</p>'"},
		"both current and retired": {major: 8,
			columns: ", title, impact, impact_description, when_to_read, markdown, html, retired_html",
			values:  ", 'Return errors', 'HIGH', 'Prevents mistakes.', 'When handling failures.', '---\ntitle: Return errors\n---\nOld.', '<p>Old.</p>', '<p>Old.</p>'",
			refused: "rule_versions_retired_html_check"},
	} {
		t.Run(name, func(t *testing.T) {
			err := insertVersion(tc.major, tc.columns, tc.values)

			if tc.refused == "" && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if tc.refused != "" && (err == nil || !strings.Contains(err.Error(), tc.refused)) {
				t.Fatalf("got %v, want a violation of %s", err, tc.refused)
			}
		})
	}
}
