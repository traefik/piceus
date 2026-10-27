package catalog

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// SyncResult describes what Sync copied, or would copy in dry-run.
type SyncResult struct {
	// Inserted are the plugins created in the source after the dump.
	Inserted []string `json:"inserted"`
	// Updated are the plugins whose metadata, versions or hashes changed in the source.
	Updated []string `json:"updated"`
	// Hashes are the hash entries copied to the target.
	Hashes []Entry `json:"hashes"`
	// Conflicts are hash entries recorded on both sides with different values. The target value is kept.
	Conflicts []Conflict `json:"conflicts"`
	// Skipped are plugins missing in the target but created before the dump, so removed from it on purpose.
	Skipped []string `json:"skipped"`
	// Changed are plugins written in the target while Sync ran, left as is: run it again.
	Changed []string `json:"changed"`
	// MissingInSource are plugins of the target the source does not have, deleted after the dump or added to the target.
	// They are left as is: review them by hand.
	MissingInSource []string `json:"missingInSource"`
}

// Conflict is a hash entry recorded with different values in the source and the target.
type Conflict struct {
	Key    string `json:"key"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type pluginDoc struct {
	Name            string     `bson:"name"`
	Runtime         string     `bson:"runtime"`
	Stars           int        `bson:"stars"`
	CreatedAt       time.Time  `bson:"createdAt"`
	Versions        []string   `bson:"versions"`
	RemovedVersions []string   `bson:"removedVersions"`
	LatestVersion   string     `bson:"latestVersion"`
	Hashes          []bson.Raw `bson:"hashes"`
	raw             bson.Raw
	fields          bson.M
}

// Sync copies into dst what was added to src after the dump taken at since.
//
// Plugins created after since are inserted as is. For the other plugins, metadata is copied,
// versions missing from dst are appended unless RemoveVersions removed them from dst, and hash entries
// missing from dst are added for the versions dst lists. Nothing is removed from dst and no dst hash
// is changed, so the fixes applied to dst survive.
// A plugin written in dst while Sync runs is left as is and reported in Changed.
// Plugins of dst that src lacks are only reported, in MissingInSource.
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

		pushed := push

		if !dryRun {
			var changed bool

			pushed, changed, err = dst.update(ctx, d, set, push)
			if err != nil {
				return result, err
			}

			if changed {
				result.Changed = append(result.Changed, name)
			}
		}

		if len(set) > 0 || len(pushed) > 0 {
			result.Updated = append(result.Updated, name)
		}

		result.Hashes = append(result.Hashes, hashEntries(s, pushed)...)
	}

	for _, name := range sortedKeys(dstDocs) {
		if _, ok := srcDocs[name]; !ok {
			result.MissingInSource = append(result.MissingInSource, name)
		}
	}

	return result, nil
}

// update applies set to dst only if its versions are still the ones diff read,
// and pushes each hash entry only if dst has none for that name yet. It returns the entries pushed.
func (s *Store) update(ctx context.Context, dst pluginDoc, set bson.D, push []bson.Raw) ([]bson.Raw, bool, error) {
	if len(set) > 0 {
		filter := bson.D{{Key: "name", Value: dst.Name}, {Key: fieldVersions, Value: dst.Versions}}

		res, err := s.coll.UpdateOne(ctx, filter, bson.D{{Key: "$set", Value: set}})
		if err != nil {
			return nil, false, fmt.Errorf("update %s: %w", dst.Name, err)
		}

		if res.MatchedCount == 0 {
			return nil, true, nil
		}
	}

	var (
		pushed  []bson.Raw
		changed bool
	)

	for _, h := range push {
		key, _ := hashFields(h)

		filter := bson.D{{Key: "name", Value: dst.Name}, {Key: fieldHashes + ".name", Value: bson.D{{Key: "$ne", Value: key}}}}

		res, err := s.coll.UpdateOne(ctx, filter, bson.D{{Key: "$push", Value: bson.D{{Key: fieldHashes, Value: h}}}})
		if err != nil {
			return pushed, changed, fmt.Errorf("update %s: %w", key, err)
		}

		if res.MatchedCount == 0 {
			changed = true
			continue
		}

		pushed = append(pushed, h)
	}

	return pushed, changed, nil
}

func diff(src, dst pluginDoc) (bson.D, []bson.Raw, []Conflict) {
	set := bson.D{}

	for _, key := range sortedKeys(src.fields) {
		switch key {
		case "_id", "id", "name", fieldVersions, fieldRemovedVersions, fieldLatestVersion, fieldHashes:
			// Identity, and what is merged below.
			continue
		}

		if !reflect.DeepEqual(src.fields[key], dst.fields[key]) {
			set = append(set, bson.E{Key: key, Value: src.fields[key]})
		}
	}

	versions := mergeVersions(src, dst)

	if len(versions) != len(dst.Versions) {
		set = append(set, bson.E{Key: fieldVersions, Value: versions})
	}

	// RemoveVersions hid the plugin when it removed its last version.
	if len(versions) == 0 {
		set = slices.DeleteFunc(set, func(e bson.E) bool { return e.Key == fieldHidden })
	}

	if latest := latestVersion(src, versions); latest != "" && latest != dst.LatestVersion {
		set = append(set, bson.E{Key: fieldLatestVersion, Value: latest})
	}

	recorded := map[string]string{}
	for _, h := range dst.Hashes {
		name, hash := hashFields(h)
		recorded[name] = hash
	}

	var (
		push      []bson.Raw
		conflicts []Conflict
	)

	for _, h := range src.Hashes {
		name, hash := hashFields(h)

		if got, ok := recorded[name]; ok {
			if got != hash {
				conflicts = append(conflicts, Conflict{Key: name, Source: hash, Target: got})
			}

			continue
		}

		if slices.Contains(versions, strings.TrimPrefix(name, src.Name+"@")) {
			push = append(push, h)
		}
	}

	return set, push, conflicts
}

// mergeVersions appends to the versions of dst those of src it lacks, except the ones RemoveVersions removed.
func mergeVersions(src, dst pluginDoc) []string {
	versions := slices.Clone(dst.Versions)

	for _, v := range src.Versions {
		if !slices.Contains(versions, v) && !slices.Contains(dst.RemovedVersions, v) {
			versions = append(versions, v)
		}
	}

	return versions
}

// latestVersion is the latestVersion of src, or the newest of versions when dst removed it.
func latestVersion(src pluginDoc, versions []string) string {
	if slices.Contains(versions, src.LatestVersion) {
		return src.LatestVersion
	}

	return newest(versions)
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
