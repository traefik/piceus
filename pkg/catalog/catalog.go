// Package catalog reads and repairs the plugin catalog stored in MongoDB by plugin-service.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"golang.org/x/mod/semver"
)

// Same collection name as plugin-service.
const collName = "plugin"

const (
	fieldHashes        = "hashes"
	fieldVersions      = "versions"
	fieldLatestVersion = "latestVersion"
	fieldHidden        = "hidden"
	// fieldRemovedVersions lists the versions RemoveVersions removed, so Sync does not bring them back.
	fieldRemovedVersions = "removedVersions"
)

// Entry is one archive hash recorded by plugin-service, with the plugin it belongs to.
type Entry struct {
	Module   string `json:"module"`
	Version  string `json:"version"`
	Runtime  string `json:"runtime,omitempty"`
	Stars    int    `json:"stars"`
	Listed   bool   `json:"listed"`
	Disabled bool   `json:"disabled,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
	Hash     string `json:"recordedHash"`
}

// Key returns the hash name used by plugin-service: <module>@<version>.
func (e Entry) Key() string {
	return e.Module + "@" + e.Version
}

// IsWasm reports whether the plugin is served from a GitHub release asset.
func (e Entry) IsWasm() bool {
	return strings.EqualFold(e.Runtime, "wasm")
}

// Store gives access to one plugin catalog database.
type Store struct {
	client *mongo.Client
	coll   *mongo.Collection
}

// Open connects to the catalog stored in the given database.
func Open(uri, database string) (*Store, error) {
	if database == "" {
		return nil, errors.New("missing database name")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect to MongoDB: %w", err)
	}

	return &Store{client: client, coll: client.Database(database).Collection(collName)}, nil
}

// Close disconnects from MongoDB.
func (s *Store) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

type hashDoc struct {
	Name string `bson:"name"`
	Hash string `bson:"hash"`
}

type entryDoc struct {
	Name     string    `bson:"name"`
	Runtime  string    `bson:"runtime"`
	Disabled bool      `bson:"disabled"`
	Hidden   bool      `bson:"hidden"`
	Stars    int       `bson:"stars"`
	Versions []string  `bson:"versions"`
	Hashes   []hashDoc `bson:"hashes"`
}

// Entries returns every recorded hash, sorted by module then version.
func (s *Store) Entries(ctx context.Context) ([]Entry, error) {
	projection := bson.D{}
	for _, field := range []string{"name", "runtime", "disabled", fieldHidden, "stars", fieldVersions, fieldHashes} {
		projection = append(projection, bson.E{Key: field, Value: 1})
	}

	coll := s.coll.Database().Collection(collName, options.Collection().SetReadPreference(readpref.SecondaryPreferred()))

	cursor, err := coll.Find(ctx, bson.D{}, options.Find().SetProjection(projection))
	if err != nil {
		return nil, fmt.Errorf("find plugins: %w", err)
	}

	var docs []entryDoc
	if err = cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode plugins: %w", err)
	}

	var entries []Entry

	for _, doc := range docs {
		for _, h := range doc.Hashes {
			version, ok := strings.CutPrefix(h.Name, doc.Name+"@")
			if !ok {
				continue
			}

			entries = append(entries, Entry{
				Module:   doc.Name,
				Version:  version,
				Runtime:  strings.ToLower(doc.Runtime),
				Stars:    doc.Stars,
				Listed:   slices.Contains(doc.Versions, version),
				Disabled: doc.Disabled,
				Hidden:   doc.Hidden,
				Hash:     h.Hash,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Module != entries[j].Module {
			return entries[i].Module < entries[j].Module
		}

		return entries[i].Version < entries[j].Version
	})

	return entries, nil
}

// ErrHashChanged is returned when the catalog changed since it was read.
var ErrHashChanged = errors.New("the recorded hash is not the expected one")

// SetHash replaces the recorded hash of module@version, only if it still holds oldHash.
func (s *Store) SetHash(ctx context.Context, module, version, oldHash, newHash string) error {
	key := module + "@" + version

	filter := bson.D{
		{Key: "name", Value: module},
		{Key: fieldHashes, Value: bson.D{{Key: "$elemMatch", Value: bson.D{
			{Key: "name", Value: key},
			{Key: "hash", Value: oldHash},
		}}}},
	}

	update := bson.D{{Key: "$set", Value: bson.D{{Key: "hashes.$[elem].hash", Value: newHash}}}}

	opts := options.UpdateOne().SetArrayFilters([]any{bson.D{{Key: "elem.name", Value: key}}})

	res, err := s.coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("update %s: %w", key, err)
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("%s: %w", key, ErrHashChanged)
	}

	return nil
}

// Removal is the outcome of RemoveVersions.
type Removal struct {
	Versions      []string `json:"versions"`
	LatestVersion string   `json:"latestVersion"`
	Hidden        bool     `json:"hidden"`
}

// RemoveVersions removes versions, given as version → recorded hash, from the versions and hashes of module.
// latestVersion moves to the newest version left, and the plugin is hidden when none is left.
// The removed versions are recorded in removedVersions for Sync.
// Nothing is written if a recorded hash or the version list changed in the meantime, or in dry-run.
func (s *Store) RemoveVersions(ctx context.Context, module string, versions map[string]string, dryRun bool) (Removal, error) {
	var doc struct {
		Versions      []string `bson:"versions"`
		LatestVersion string   `bson:"latestVersion"`
		Hidden        bool     `bson:"hidden"`
	}

	if err := s.coll.FindOne(ctx, bson.D{{Key: "name", Value: module}}).Decode(&doc); err != nil {
		return Removal{}, fmt.Errorf("find %s: %w", module, err)
	}

	removal := Removal{LatestVersion: doc.LatestVersion, Hidden: doc.Hidden}

	var guards, names bson.A

	for _, version := range sortedKeys(versions) {
		key := module + "@" + version
		names = append(names, key)
		guards = append(guards, bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "name", Value: key}, {Key: "hash", Value: versions[version]}}}})
	}

	for _, v := range doc.Versions {
		if _, ok := versions[v]; !ok {
			removal.Versions = append(removal.Versions, v)
		}
	}

	if !slices.Contains(removal.Versions, removal.LatestVersion) {
		removal.LatestVersion = newest(removal.Versions)
	}

	if len(removal.Versions) == 0 {
		removal.LatestVersion = doc.LatestVersion
		removal.Hidden = true
	}

	if dryRun {
		return removal, nil
	}

	filter := bson.D{
		{Key: "name", Value: module},
		{Key: fieldVersions, Value: doc.Versions},
		{Key: fieldHashes, Value: bson.D{{Key: "$all", Value: guards}}},
	}

	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: fieldVersions, Value: append([]string{}, removal.Versions...)},
			{Key: fieldLatestVersion, Value: removal.LatestVersion},
			{Key: fieldHidden, Value: removal.Hidden},
		}},
		{Key: "$addToSet", Value: bson.D{{Key: fieldRemovedVersions, Value: bson.D{{Key: "$each", Value: sortedKeys(versions)}}}}},
		{Key: "$pull", Value: bson.D{{Key: fieldHashes, Value: bson.D{{Key: "name", Value: bson.D{{Key: "$in", Value: names}}}}}}},
	}

	res, err := s.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return removal, fmt.Errorf("update %s: %w", module, err)
	}

	if res.MatchedCount == 0 {
		return removal, fmt.Errorf("%s: %w", module, ErrHashChanged)
	}

	return removal, nil
}

func newest(versions []string) string {
	var latest string

	for _, v := range versions {
		if latest == "" || semver.Compare(v, latest) > 0 {
			latest = v
		}
	}

	return latest
}
