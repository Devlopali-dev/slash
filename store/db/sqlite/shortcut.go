package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	storepb "github.com/devlopali-dev/slash/proto/gen/store"
	"github.com/devlopali-dev/slash/store"
)

func (d *DB) CreateShortcut(ctx context.Context, create *storepb.Shortcut) (*storepb.Shortcut, error) {
	set := []string{"creator_id", "name", "link", "title", "description", "visibility", "tag"}
	tagJSON, err := encodeTags(create.Tags)
	if err != nil {
		return nil, err
	}
	args := []any{create.CreatorId, create.Name, create.Link, create.Title, create.Description, create.Visibility.String(), tagJSON}
	placeholder := []string{"?", "?", "?", "?", "?", "?", "?"}
	if create.OgMetadata != nil {
		set = append(set, "og_metadata")
		openGraphMetadataBytes, err := protojson.Marshal(create.OgMetadata)
		if err != nil {
			return nil, err
		}
		args = append(args, string(openGraphMetadataBytes))
		placeholder = append(placeholder, "?")
	}

	stmt := `
		INSERT INTO shortcut (
			` + strings.Join(set, ", ") + `
		)
		VALUES (` + strings.Join(placeholder, ",") + `)
		RETURNING id, created_ts, updated_ts
	`
	if err := d.db.QueryRowContext(ctx, stmt, args...).Scan(
		&create.Id,
		&create.CreatedTs,
		&create.UpdatedTs,
	); err != nil {
		return nil, err
	}
	shortcut := create
	return shortcut, nil
}

func (d *DB) UpdateShortcut(ctx context.Context, update *store.UpdateShortcut) (*storepb.Shortcut, error) {
	set, args := []string{}, []any{}
	if update.Name != nil {
		set, args = append(set, "name = ?"), append(args, *update.Name)
	}
	if update.Link != nil {
		set, args = append(set, "link = ?"), append(args, *update.Link)
	}
	if update.Title != nil {
		set, args = append(set, "title = ?"), append(args, *update.Title)
	}
	if update.Description != nil {
		set, args = append(set, "description = ?"), append(args, *update.Description)
	}
	if update.Visibility != nil {
		set, args = append(set, "visibility = ?"), append(args, update.Visibility.String())
	}
	if update.Tags != nil {
		tagJSON, err := encodeTags(update.Tags)
		if err != nil {
			return nil, err
		}
		set, args = append(set, "tag = ?"), append(args, tagJSON)
	}
	if update.OpenGraphMetadata != nil {
		openGraphMetadataBytes, err := protojson.Marshal(update.OpenGraphMetadata)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to marshal activity payload")
		}
		set, args = append(set, "og_metadata = ?"), append(args, string(openGraphMetadataBytes))
	}
	if len(set) == 0 {
		return nil, errors.New("no update specified")
	}
	args = append(args, update.ID)

	stmt := `
		UPDATE shortcut
		SET
			` + strings.Join(set, ", ") + `
		WHERE
			id = ?
		RETURNING id, creator_id, created_ts, updated_ts, name, link, title, description, visibility, tag, og_metadata
	`
	shortcut := &storepb.Shortcut{}
	var visibility, tags, openGraphMetadataString string
	if err := d.db.QueryRowContext(ctx, stmt, args...).Scan(
		&shortcut.Id,
		&shortcut.CreatorId,
		&shortcut.CreatedTs,
		&shortcut.UpdatedTs,
		&shortcut.Name,
		&shortcut.Link,
		&shortcut.Title,
		&shortcut.Description,
		&visibility,
		&tags,
		&openGraphMetadataString,
	); err != nil {
		return nil, err
	}
	shortcut.Visibility = store.ConvertVisibilityStringToStorepb(visibility)
	shortcut.Tags = decodeTags(tags)
	var ogMetadata storepb.OpenGraphMetadata
	if err := protojson.Unmarshal([]byte(openGraphMetadataString), &ogMetadata); err != nil {
		return nil, err
	}
	shortcut.OgMetadata = &ogMetadata
	return shortcut, nil
}

func (d *DB) ListShortcuts(ctx context.Context, find *store.FindShortcut) ([]*storepb.Shortcut, error) {
	where, args := []string{"1 = 1"}, []any{}
	if v := find.ID; v != nil {
		where, args = append(where, "id = ?"), append(args, *v)
	}
	if v := find.CreatorID; v != nil {
		where, args = append(where, "creator_id = ?"), append(args, *v)
	}
	if v := find.Name; v != nil {
		where, args = append(where, "name = ?"), append(args, *v)
	}
	if v := find.VisibilityList; len(v) != 0 {
		list := []string{}
		for _, visibility := range v {
			list = append(list, "?")
			args = append(args, visibility.String())
		}
		where = append(where, fmt.Sprintf("visibility in (%s)", strings.Join(list, ",")))
	}
	if v := find.Tag; v != nil {
		// Exact tag match via json_each — avoids LIKE false positives (e.g. "go" matching "golang").
		where = append(where, "EXISTS (SELECT 1 FROM json_each(tag) WHERE value = ?)")
		args = append(args, *v)
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT
			id,
			creator_id,
			created_ts,
			updated_ts,
			name,
			link,
			title,
			description,
			visibility,
			tag,
			og_metadata
		FROM shortcut
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_ts DESC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*storepb.Shortcut, 0)
	for rows.Next() {
		shortcut := &storepb.Shortcut{}
		var visibility, tags, openGraphMetadataString string
		if err := rows.Scan(
			&shortcut.Id,
			&shortcut.CreatorId,
			&shortcut.CreatedTs,
			&shortcut.UpdatedTs,
			&shortcut.Name,
			&shortcut.Link,
			&shortcut.Title,
			&shortcut.Description,
			&visibility,
			&tags,
			&openGraphMetadataString,
		); err != nil {
			return nil, err
		}
		shortcut.Visibility = store.ConvertVisibilityStringToStorepb(visibility)
		shortcut.Tags = decodeTags(tags)
		var ogMetadata storepb.OpenGraphMetadata
		if err := protojson.Unmarshal([]byte(openGraphMetadataString), &ogMetadata); err != nil {
			return nil, err
		}
		shortcut.OgMetadata = &ogMetadata
		list = append(list, shortcut)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (d *DB) DeleteShortcut(ctx context.Context, delete *store.DeleteShortcut) error {
	if _, err := d.db.ExecContext(ctx, `DELETE FROM shortcut WHERE id = ?`, delete.ID); err != nil {
		return err
	}

	return nil
}

func vacuumShortcut(ctx context.Context, tx *sql.Tx) error {
	stmt := `DELETE FROM shortcut WHERE creator_id NOT IN (SELECT id FROM user)`
	_, err := tx.ExecContext(ctx, stmt)
	if err != nil {
		return err
	}

	return nil
}

// encodeTags serialises a tag slice as a JSON array string (e.g. ["go","python"]).
func encodeTags(tags []string) (string, error) {
	if len(tags) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// decodeTags deserialises a tag value from the database.
// It handles both the new JSON format (["go","python"]) and the legacy
// space-separated format ("go python") for backward compatibility with
// rows that were not yet migrated.
func decodeTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "[") {
		var tags []string
		if err := json.Unmarshal([]byte(raw), &tags); err == nil {
			result := make([]string, 0, len(tags))
			for _, t := range tags {
				if t != "" {
					result = append(result, t)
				}
			}
			return result
		}
	}
	// Legacy: space-separated
	result := []string{}
	for _, t := range strings.Split(raw, " ") {
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}