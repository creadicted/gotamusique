package database

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// MusicRecord mirrors a row in the music table.
type MusicRecord struct {
	ID        string
	Type      string
	Title     string
	Keywords  string
	Tags      []string
	Path      string
	Metadata  map[string]any
	CreatedAt time.Time
}

// MusicDB persists the music library.
type MusicDB interface {
	Insert(m MusicRecord) error
	Query(cond *Condition) ([]MusicRecord, error)
	QueryByID(id string) (*MusicRecord, error)
	QueryByKeywords(keywords []string) ([]MusicRecord, error)
	QueryByTags(tags []string) ([]MusicRecord, error)
	QueryRandom(count int, cond *Condition) ([]MusicRecord, error)
	Delete(cond *Condition) error
	AllPaths() ([]string, error)
	AllTags() ([]string, error)
	Close() error
}

type musicDB struct {
	db *sql.DB
}

// OpenMusic opens the music database at path. Migration must be run first.
func OpenMusic(path string) (MusicDB, error) {
	db, err := openSQLite(path)
	if err != nil {
		return nil, err
	}
	return &musicDB{db: db}, nil
}

func (m *musicDB) Insert(rec MusicRecord) error {
	tags := encodeTags(rec.Tags)
	meta, err := json.Marshal(rec.Metadata)
	if err != nil {
		return err
	}

	var exists int
	_ = m.db.QueryRow("SELECT 1 FROM music WHERE id=?", rec.ID).Scan(&exists)

	if exists == 0 {
		_, err = m.db.Exec(
			"INSERT INTO music (id, type, title, metadata, tags, path, keywords) VALUES (?, ?, ?, ?, ?, ?, ?)",
			rec.ID, rec.Type, rec.Title, string(meta), tags, rec.Path, rec.Keywords,
		)
	} else {
		_, err = m.db.Exec(
			"UPDATE music SET type=?, title=?, metadata=?, tags=?, path=?, keywords=? WHERE id=?",
			rec.Type, rec.Title, string(meta), tags, rec.Path, rec.Keywords, rec.ID,
		)
	}
	return err
}

func (m *musicDB) Query(cond *Condition) ([]MusicRecord, error) {
	if cond == nil {
		cond = &Condition{}
	}
	query := "SELECT id, type, title, metadata, tags, path, keywords, create_at FROM music WHERE id != 'info' AND " + cond.SQL()
	rows, err := m.db.Query(query, cond.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func (m *musicDB) QueryByID(id string) (*MusicRecord, error) {
	cond := (&Condition{}).AndEqual("id", id, true)
	results, err := m.Query(cond)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

func (m *musicDB) QueryByKeywords(keywords []string) ([]MusicRecord, error) {
	cond := &Condition{}
	for _, kw := range keywords {
		cond.AndLike("title", "%"+kw+"%", false)
	}
	return m.Query(cond)
}

func (m *musicDB) QueryByTags(tags []string) ([]MusicRecord, error) {
	cond := &Condition{}
	for _, tag := range tags {
		cond.AndLike("tags", "%"+tag+",%", false)
	}
	return m.Query(cond)
}

func (m *musicDB) QueryRandom(count int, cond *Condition) ([]MusicRecord, error) {
	if cond == nil {
		cond = (&Condition{}).AndNotSubCondition((&Condition{}).AndEqual("id", "info", true))
	}
	args := append(cond.Args(), count)
	query := "SELECT id, type, title, metadata, tags, path, keywords, create_at FROM music " +
		"WHERE id IN (SELECT id FROM music WHERE " + cond.SQL() + " ORDER BY RANDOM() LIMIT ?) " +
		"ORDER BY RANDOM()"
	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func (m *musicDB) Delete(cond *Condition) error {
	_, err := m.db.Exec("DELETE FROM music WHERE id != 'info' AND "+cond.SQL(), cond.Args()...)
	return err
}

func (m *musicDB) AllPaths() ([]string, error) {
	rows, err := m.db.Query("SELECT path FROM music WHERE id != 'info' AND type = 'file'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p sql.NullString
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if p.Valid && p.String != "" {
			paths = append(paths, p.String)
		}
	}
	return paths, rows.Err()
}

func (m *musicDB) AllTags() ([]string, error) {
	rows, err := m.db.Query("SELECT tags FROM music WHERE id != 'info'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	var tags []string
	for rows.Next() {
		var raw sql.NullString
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		for _, tag := range decodeTags(raw.String) {
			if !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}
	return tags, rows.Err()
}

func (m *musicDB) Close() error { return m.db.Close() }

func scanRecords(rows *sql.Rows) ([]MusicRecord, error) {
	var out []MusicRecord
	for rows.Next() {
		var r MusicRecord
		var metaRaw sql.NullString
		var tagsRaw, pathRaw, keywordsRaw, createdAt sql.NullString
		if err := rows.Scan(&r.ID, &r.Type, &r.Title, &metaRaw, &tagsRaw, &pathRaw, &keywordsRaw, &createdAt); err != nil {
			return nil, err
		}
		r.Path = pathRaw.String
		r.Keywords = keywordsRaw.String
		if metaRaw.Valid {
			if err := json.Unmarshal([]byte(metaRaw.String), &r.Metadata); err != nil {
				r.Metadata = map[string]any{}
			}
		} else {
			r.Metadata = map[string]any{}
		}
		r.Tags = decodeTags(tagsRaw.String)
		if createdAt.Valid {
			r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt.String)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// encodeTags produces the Python-compatible "tag1,tag2," format.
func encodeTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	seen := map[string]bool{}
	var b strings.Builder
	for _, t := range tags {
		if t != "" && !seen[t] {
			seen[t] = true
			b.WriteString(t)
			b.WriteByte(',')
		}
	}
	return b.String()
}

// decodeTags parses the "tag1,tag2," comma-separated format.
func decodeTags(raw string) []string {
	parts := strings.Split(strings.Trim(raw, ","), ",")
	var tags []string
	for _, p := range parts {
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}
