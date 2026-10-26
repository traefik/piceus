package catalog

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/mod/semver"
)

// SyncResult describes what Sync copied, or would copy in dry-run.
type SyncResult struct {
	// Inserted are the plugins created in the source after the dump.
	Inserted []string `json:"inserted"`
	// Updated are the plugins whose metadata or versions changed in the source.
	Updated []string `json:"updated"`
	// Hashes are the hash entries copied to the target.
	Hashes []Entry `json:"hashes"`
	// Conflicts are hash entries recorded on both sides with different values. The target value is kept.
	Conflicts []Conflict `json:"conflicts"`
	// Skipped are plugins missing in the target but created before the dump, so removed from it on purpose.
	Skipped []string `json:"skipped"`
}

// Conflict is a hash entry recorded with different values in the source and the target.
type Conflict struct {
	Key    string `json:"key"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type pluginDoc struct {
	Name          string     `bson:"name"`
	Runtime       string     `bson:"runtime"`
	Stars         int        `bson:"stars"`
	CreatedAt     time.Time  `bson:"createdAt"`
	Versions      []string   `bson:"versions"`
	LatestVersion string     `bson:"latestVersion"`
	Hashes        []bson.Raw `bson:"hashes"`
	raw           bson.Raw
	fields        bson.M
}

// Sync copies into dst what was added to src after the dump taken at since.
//
// Plugins created after since are inserted as is. For the other plugins, metadata is copied,
// versions newer than every version of dst are appended, and hash entries missing from dst
// are added for the versions dst lists. Nothing is removed from dst and no dst hash is changed,
// so the fixes applied to dst survive.
func Sync(ctx context.Context, src, dst *Store, since time.Time, dryRun bool) (SyncResult, error) {
	srcDocs, err := src.plugins(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("read source: %w", err)
	}

	dstDocs, err := dst.plugins(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("read target: %w", err)
	}

	var result SyncResult

	for _, name := range sortedKeys(srcDocs) {
		s := srcDocs[name]

		d, ok := dstDocs[name]
		if !ok {
			if !s.CreatedAt.After(since) {
				result.Skipped = append(result.Skipped, name)
				continue
			}

			if !dryRun {
				if _, err = dst.coll.InsertOne(ctx, s.raw); err != nil {
					return result, fmt.Errorf("insert %s: %w", name, err)
				}
			}

			result.Inserted = append(result.Inserted, name)
			result.Hashes = append(result.Hashes, hashEntries(s, s.Hashes)...)

			continue
		}

		set, push, conflicts := diff(s, d)
		result.Conflicts = append(result.Conflicts, conflicts...)

		if !dryRun {
			if err = dst.update(ctx, name, set, push); err != nil {
				return result, err
			}
		}

		if len(set) > 0 {
			result.Updated = append(result.Updated, name)
		}

		result.Hashes = append(result.Hashes, hashEntries(s, push)...)
	}

	return result, nil
}

func (s *Store) update(ctx context.Context, name string, set bson.D, push []bson.Raw) error {
	update := bson.D{}
	if len(set) > 0 {
		update = append(update, bson.E{Key: "$set", Value: set})
	}

	if len(push) > 0 {
		update = append(update, bson.E{Key: "$push", Value: bson.D{{Key: fieldHashes, Value: bson.D{{Key: "$each", Value: push}}}}})
	}

	if len(update) == 0 {
		return nil
	}

	if _, err := s.coll.UpdateOne(ctx, bson.D{{Key: "name", Value: name}}, update); err != nil {
		return fmt.Errorf("update %s: %w", name, err)
	}

	return nil
}

func diff(s, d pluginDoc) (bson.D, []bson.Raw, []Conflict) {
	set := bson.D{}

	for _, key := range sortedKeys(s.fields) {
		switch key {
		case "_id", "id", "name", fieldVersions, "latestVersion", fieldHashes:
			// Identity, and what is merged below.
			continue
		}

		if !reflect.DeepEqual(s.fields[key], d.fields[key]) {
			set = append(set, bson.E{Key: key, Value: s.fields[key]})
		}
	}

	// piceus only appends new releases, so a source version older than the target's newest
	// one was removed from the target on purpose.
	versions := slices.Clone(d.Versions)

	for _, v := range s.Versions {
		if !slices.Contains(versions, v) && newerThanAll(v, d.Versions) {
			versions = append(versions, v)
		}
	}

	if len(versions) != len(d.Versions) {
		set = append(set, bson.E{Key: fieldVersions, Value: versions})
	}

	if s.LatestVersion != d.LatestVersion && slices.Contains(versions, s.LatestVersion) {
		set = append(set, bson.E{Key: "latestVersion", Value: s.LatestVersion})
	}

	recorded := map[string]string{}
	for _, h := range d.Hashes {
		name, hash := hashFields(h)
		recorded[name] = hash
	}

	var (
		push      []bson.Raw
		conflicts []Conflict
	)

	for _, h := range s.Hashes {
		name, hash := hashFields(h)

		if got, ok := recorded[name]; ok {
			if got != hash {
				conflicts = append(conflicts, Conflict{Key: name, Source: hash, Target: got})
			}

			continue
		}

		if slices.Contains(versions, strings.TrimPrefix(name, s.Name+"@")) {
			push = append(push, h)
		}
	}

	return set, push, conflicts
}

func newerThanAll(v string, versions []string) bool {
	for _, other := range versions {
		if semver.Compare(v, other) <= 0 {
			return false
		}
	}

	return true
}

func (s *Store) plugins(ctx context.Context) (map[string]pluginDoc, error) {
	cursor, err := s.coll.Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}

	defer func() { _ = cursor.Close(ctx) }()

	docs := map[string]pluginDoc{}

	for cursor.Next(ctx) {
		raw := slices.Clone(cursor.Current)

		var doc pluginDoc
		if err = bson.Unmarshal(raw, &doc); err != nil {
			return nil, err
		}

		if err = bson.Unmarshal(raw, &doc.fields); err != nil {
			return nil, err
		}

		doc.raw = raw
		docs[doc.Name] = doc
	}

	return docs, cursor.Err()
}

func hashFields(h bson.Raw) (string, string) {
	name, _ := h.Lookup("name").StringValueOK()
	hash, _ := h.Lookup("hash").StringValueOK()

	return name, hash
}

func hashEntries(doc pluginDoc, hashes []bson.Raw) []Entry {
	entries := make([]Entry, 0, len(hashes))

	for _, h := range hashes {
		name, hash := hashFields(h)

		version := strings.TrimPrefix(name, doc.Name+"@")

		entries = append(entries, Entry{
			Module:  doc.Name,
			Version: version,
			Runtime: strings.ToLower(doc.Runtime),
			Stars:   doc.Stars,
			Listed:  slices.Contains(doc.Versions, version),
			Hash:    hash,
		})
	}

	return entries
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	return keys
}
