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
)

// Same collection name as plugin-service.
const collName = "plugin"

const (
	fieldHashes   = "hashes"
	fieldVersions = "versions"
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
	for _, field := range []string{"name", "runtime", "disabled", "hidden", "stars", fieldVersions, fieldHashes} {
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

// ErrHashChanged is returned when the recorded hash is no longer the expected one.
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
