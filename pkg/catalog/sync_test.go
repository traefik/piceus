package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func newDoc(t *testing.T, doc bson.D) pluginDoc {
	t.Helper()

	raw, err := bson.Marshal(doc)
	require.NoError(t, err)

	var p pluginDoc
	require.NoError(t, bson.Unmarshal(raw, &p))
	require.NoError(t, bson.Unmarshal(raw, &p.fields))

	p.raw = raw

	return p
}

func hash(name, value string) bson.D {
	return bson.D{{Key: "name", Value: name}, {Key: "hash", Value: value}}
}

func TestDiff(t *testing.T) {
	src := newDoc(t, bson.D{
		{Key: "id", Value: "src-id"},
		{Key: "name", Value: "github.com/a/b"},
		{Key: "stars", Value: 12},
		{Key: "readme", Value: "same"},
		{Key: "latestVersion", Value: "v1.3.0"},
		{Key: "versions", Value: bson.A{"v1.0.0", "v1.1.0", "v1.2.0", "v1.3.0"}},
		{Key: "hashes", Value: bson.A{
			hash("github.com/a/b@v1.0.0", "removed-in-target"),
			hash("github.com/a/b@v1.1.0", "prod"),
			hash("github.com/a/b@v1.2.0", "same"),
			hash("github.com/a/b@v1.3.0", "new"),
		}},
	})

	// v1.0.0 was pruned from the target and v1.1.0 realigned there.
	dst := newDoc(t, bson.D{
		{Key: "id", Value: "dst-id"},
		{Key: "name", Value: "github.com/a/b"},
		{Key: "stars", Value: 10},
		{Key: "readme", Value: "same"},
		{Key: "latestVersion", Value: "v1.2.0"},
		{Key: "versions", Value: bson.A{"v1.1.0", "v1.2.0"}},
		{Key: "hashes", Value: bson.A{
			hash("github.com/a/b@v1.1.0", "fixed"),
			hash("github.com/a/b@v1.2.0", "same"),
		}},
	})

	set, push, conflicts := diff(src, dst)

	assert.Equal(t, bson.D{
		{Key: "stars", Value: int32(12)},
		{Key: "versions", Value: []string{"v1.1.0", "v1.2.0", "v1.3.0"}},
		{Key: "latestVersion", Value: "v1.3.0"},
	}, set)

	require.Len(t, push, 1)

	name, value := hashFields(push[0])
	assert.Equal(t, "github.com/a/b@v1.3.0", name)
	assert.Equal(t, "new", value)

	assert.Equal(t, []Conflict{{Key: "github.com/a/b@v1.1.0", Source: "prod", Target: "fixed"}}, conflicts)
}

func TestDiff_noChange(t *testing.T) {
	doc := bson.D{
		{Key: "name", Value: "github.com/a/b"},
		{Key: "latestVersion", Value: "v1.0.0"},
		{Key: "versions", Value: bson.A{"v1.0.0"}},
		{Key: "hashes", Value: bson.A{hash("github.com/a/b@v1.0.0", "h")}},
	}

	set, push, conflicts := diff(newDoc(t, doc), newDoc(t, doc))

	assert.Empty(t, set)
	assert.Empty(t, push)
	assert.Empty(t, conflicts)
}
