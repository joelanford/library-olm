package fbc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	"github.com/joelanford/library-olm/catalog/fbc"
	"github.com/joelanford/library-olm/catalog/fbc/internal/testing/catalogfs"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

type testOLMPackageExtension struct {
	onPackageCalls     []string
	onChannelCalls     []string
	onBundleCalls      []string
	onDeprecationCalls []string
	onOtherCalls       []string
	finalizeErr        error
}

func (e *testOLMPackageExtension) OnPackage(p declcfg.Package) (any, error) {
	e.onPackageCalls = append(e.onPackageCalls, p.Name)
	return map[string]string{"kind": "package", "name": p.Name}, nil
}

func (e *testOLMPackageExtension) OnChannel(ch declcfg.Channel) (any, error) {
	e.onChannelCalls = append(e.onChannelCalls, ch.Name)
	return map[string]string{"kind": "channel", "name": ch.Name}, nil
}

func (e *testOLMPackageExtension) OnBundle(b declcfg.Bundle) (any, error) {
	e.onBundleCalls = append(e.onBundleCalls, b.Name)
	return map[string]string{"kind": "bundle", "name": b.Name}, nil
}

func (e *testOLMPackageExtension) OnDeprecation(d declcfg.Deprecation) (any, error) {
	e.onDeprecationCalls = append(e.onDeprecationCalls, d.Package)
	return map[string]string{"kind": "deprecation", "package": d.Package}, nil
}

func (e *testOLMPackageExtension) OnOther(m declcfg.Meta) (any, error) {
	e.onOtherCalls = append(e.onOtherCalls, m.Schema+"/"+m.Name)
	return map[string]string{"kind": "other", "schema": m.Schema}, nil
}

func (e *testOLMPackageExtension) FinalizePackage(ctx context.Context, pkg fbc.PackageAccessor, w fbc.PropertyWriter) error {
	if e.finalizeErr != nil {
		return e.finalizeErr
	}

	if err := w.SetGraphProperty(ctx, nil, "display-name", json.RawMessage(`"My Package"`)); err != nil {
		return err
	}

	for ch, err := range pkg.Channels() {
		if err != nil {
			return err
		}
		if err := w.SetGraphProperty(ctx, []string{ch.Name()}, "channel-desc", json.RawMessage(`"A channel"`)); err != nil {
			return err
		}
	}

	for b, err := range pkg.Bundles() {
		if err != nil {
			return err
		}
		if err := w.SetBundleProperty(ctx, b.Name(), "icon", json.RawMessage(`"data:image/png;base64,abc"`)); err != nil {
			return err
		}
	}

	return nil
}

func TestOLMPackageExtension_EndToEnd(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("ext-op").
		WithChannel("ext-op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("ext-op", "1.0.0").
		Build()
	ctx := context.Background()

	ext := &testOLMPackageExtension{}
	store, err := catalogv1.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()

	imp := fbc.NewImporter(fsys, fbc.WithOLMPackageExtension(ext))
	cat, err := store.Set(ctx, "test",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "test"),
	)
	require.NoError(t, err)

	assert.Equal(t, []string{"ext-op"}, ext.onPackageCalls)
	assert.Equal(t, []string{"stable"}, ext.onChannelCalls)
	assert.Equal(t, []string{"ext-op.v1.0.0"}, ext.onBundleCalls)

	pkg, err := cat.GetPackage(ctx, "ext-op")
	require.NoError(t, err)

	t.Run("PackageProperty", func(t *testing.T) {
		val, err := pkg.Property(ctx, "display-name")
		require.NoError(t, err)
		assert.Equal(t, json.RawMessage(`"My Package"`), val)
	})

	t.Run("PackageProperty_NotFound", func(t *testing.T) {
		val, err := pkg.Property(ctx, "nonexistent")
		require.NoError(t, err)
		assert.Nil(t, val)
	})

	t.Run("ChannelProperty", func(t *testing.T) {
		composite := pkg.(catalogv1.CompositeUpdateGraph)
		ch, err := composite.GetGraph(ctx, "stable")
		require.NoError(t, err)
		val, err := ch.Property(ctx, "channel-desc")
		require.NoError(t, err)
		assert.Equal(t, json.RawMessage(`"A channel"`), val)
	})

	t.Run("BundleProperty", func(t *testing.T) {
		composite := pkg.(catalogv1.CompositeUpdateGraph)
		ch, err := composite.GetGraph(ctx, "stable")
		require.NoError(t, err)

		var bundles []bundlev1.Bundle
		for b, bErr := range ch.ListBundles(ctx) {
			require.NoError(t, bErr)
			bundles = append(bundles, b)
		}
		require.NotEmpty(t, bundles)
		for _, b := range bundles {
			val, err := b.Property(ctx, "icon")
			require.NoError(t, err)
			assert.Equal(t, json.RawMessage(`"data:image/png;base64,abc"`), val)
		}
	})
}

func TestOLMPackageExtension_CallbacksReceiveCorrectTypes(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("cb-op").
		WithChannel("cb-op", "stable", catalogfs.Entry("1.0.0")).
		WithChannel("cb-op", "fast", catalogfs.Entry("1.0.0")).
		WithBundle("cb-op", "1.0.0").
		WithCustom("cb-op", "olm.custom.thing", "whatever").
		Build()
	ctx := context.Background()

	ext := &testOLMPackageExtension{}
	store, err := catalogv1.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()

	imp := fbc.NewImporter(fsys, fbc.WithOLMPackageExtension(ext))
	_, err = store.Set(ctx, "test",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "test"),
	)
	require.NoError(t, err)

	assert.Equal(t, []string{"cb-op"}, ext.onPackageCalls)
	assert.Len(t, ext.onChannelCalls, 2)
	assert.Contains(t, ext.onChannelCalls, "stable")
	assert.Contains(t, ext.onChannelCalls, "fast")
	assert.Equal(t, []string{"cb-op.v1.0.0"}, ext.onBundleCalls)
	assert.Equal(t, []string{"olm.custom.thing/whatever"}, ext.onOtherCalls)
}

func TestOLMPackageExtension_ExtDataAccessible(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("ed-op").
		WithChannel("ed-op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("ed-op", "1.0.0").
		Build()
	ctx := context.Background()

	var capturedPkgExtData json.RawMessage
	var capturedBundleExtData json.RawMessage
	var capturedChannelExtData json.RawMessage

	ext := &testOLMPackageExtension{}
	origFinalize := ext.FinalizePackage
	_ = origFinalize

	captureExt := &capturingExtension{
		testOLMPackageExtension: ext,
		onFinalize: func(ctx context.Context, pkg fbc.PackageAccessor, w fbc.PropertyWriter) error {
			var extErr error
			capturedPkgExtData, extErr = pkg.ExtData()
			if extErr != nil {
				return extErr
			}
			for b, err := range pkg.Bundles() {
				if err != nil {
					return err
				}
				capturedBundleExtData = b.ExtData()
			}
			for ch, err := range pkg.Channels() {
				if err != nil {
					return err
				}
				capturedChannelExtData = ch.ExtData()
			}
			return nil
		},
	}

	store, err := catalogv1.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()

	imp := fbc.NewImporter(fsys, fbc.WithOLMPackageExtension(captureExt))
	_, err = store.Set(ctx, "test",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "test"),
	)
	require.NoError(t, err)

	assert.NotNil(t, capturedPkgExtData)
	assert.Contains(t, string(capturedPkgExtData), `"package"`)

	assert.NotNil(t, capturedBundleExtData)
	assert.Contains(t, string(capturedBundleExtData), `"bundle"`)

	assert.NotNil(t, capturedChannelExtData)
	assert.Contains(t, string(capturedChannelExtData), `"channel"`)
}

func TestOLMPackageExtension_FinalizeAccessesDeprecationsAndOthers(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("da-op").
		WithChannel("da-op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("da-op", "1.0.0").
		WithDeprecation("da-op").
		WithCustom("da-op", "olm.custom.thing", "my-thing").
		Build()
	ctx := context.Background()

	var capturedDeprecationExtData []json.RawMessage
	var capturedOtherExtData []json.RawMessage
	var capturedOtherSchemas []string
	var capturedOtherNames []string

	ext := &testOLMPackageExtension{}
	captureExt := &capturingExtension{
		testOLMPackageExtension: ext,
		onFinalize: func(_ context.Context, pkg fbc.PackageAccessor, _ fbc.PropertyWriter) error {
			for d, err := range pkg.Deprecations() {
				if err != nil {
					return err
				}
				capturedDeprecationExtData = append(capturedDeprecationExtData, d.ExtData())
			}
			for o, err := range pkg.Others() {
				if err != nil {
					return err
				}
				capturedOtherExtData = append(capturedOtherExtData, o.ExtData())
				capturedOtherSchemas = append(capturedOtherSchemas, o.Schema())
				capturedOtherNames = append(capturedOtherNames, o.Name())
			}
			return nil
		},
	}

	store, err := catalogv1.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()

	imp := fbc.NewImporter(fsys, fbc.WithOLMPackageExtension(captureExt))
	_, err = store.Set(ctx, "test",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "test"),
	)
	require.NoError(t, err)

	require.Len(t, capturedDeprecationExtData, 1)
	assert.Contains(t, string(capturedDeprecationExtData[0]), `"deprecation"`)
	assert.Contains(t, string(capturedDeprecationExtData[0]), `"da-op"`)

	require.Len(t, capturedOtherExtData, 1)
	assert.Contains(t, string(capturedOtherExtData[0]), `"other"`)
	assert.Equal(t, []string{"olm.custom.thing"}, capturedOtherSchemas)
	assert.Equal(t, []string{"my-thing"}, capturedOtherNames)
}

type capturingExtension struct {
	*testOLMPackageExtension
	onFinalize func(ctx context.Context, pkg fbc.PackageAccessor, w fbc.PropertyWriter) error
}

func (e *capturingExtension) FinalizePackage(ctx context.Context, pkg fbc.PackageAccessor, w fbc.PropertyWriter) error {
	return e.onFinalize(ctx, pkg, w)
}

func TestOLMPackageExtension_ChannelEntries(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("ent-op").
		WithChannel("ent-op", "stable",
			catalogfs.Entry("1.0.0"),
			catalogfs.Entry("1.1.0", catalogfs.Replaces("1.0.0"), catalogfs.Skips("0.9.0"), catalogfs.SkipRange(">=0.8.0 <1.0.0")),
		).
		WithBundle("ent-op", "1.0.0").
		WithBundle("ent-op", "1.1.0").
		Build()
	ctx := context.Background()

	type entryData struct {
		BundleName string
		Replaces   string
		Skips      []string
		SkipRange  string
	}
	var capturedEntries []entryData

	captureExt := &capturingExtension{
		testOLMPackageExtension: &testOLMPackageExtension{},
		onFinalize: func(ctx context.Context, pkg fbc.PackageAccessor, w fbc.PropertyWriter) error {
			for ch, err := range pkg.Channels() {
				if err != nil {
					return err
				}
				for entry, err := range ch.Entries() {
					if err != nil {
						return err
					}
					capturedEntries = append(capturedEntries, entryData{
						BundleName: entry.BundleName(),
						Replaces:   entry.Replaces(),
						Skips:      entry.Skips(),
						SkipRange:  entry.SkipRange(),
					})
				}
			}
			return nil
		},
	}

	store, err := catalogv1.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()

	imp := fbc.NewImporter(fsys, fbc.WithOLMPackageExtension(captureExt))
	_, err = store.Set(ctx, "test",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "test"),
	)
	require.NoError(t, err)

	require.Len(t, capturedEntries, 2)

	byName := map[string]entryData{}
	for _, e := range capturedEntries {
		byName[e.BundleName] = e
	}

	e1 := byName["ent-op.v1.0.0"]
	assert.Equal(t, "", e1.Replaces)
	assert.Empty(t, e1.Skips)
	assert.Equal(t, "", e1.SkipRange)

	e2 := byName["ent-op.v1.1.0"]
	assert.Equal(t, "ent-op.v1.0.0", e2.Replaces)
	assert.Equal(t, []string{"ent-op.v0.9.0"}, e2.Skips)
	assert.Equal(t, ">=0.8.0 <1.0.0", e2.SkipRange)
}

func TestOLMPackageExtension_FinalizeError(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("good-op").
		WithPackage("fail-op").
		WithChannel("good-op", "stable", catalogfs.Entry("1.0.0")).
		WithChannel("fail-op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("good-op", "1.0.0").
		WithBundle("fail-op", "1.0.0").
		Build()
	ctx := context.Background()

	failExt := &perPackageFailExtension{
		testOLMPackageExtension: &testOLMPackageExtension{},
		failPackage:             "fail-op",
	}

	store, err := catalogv1.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()

	imp := fbc.NewImporter(fsys, fbc.WithOLMPackageExtension(failExt))
	cat, err := store.Set(ctx, "test",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "test"),
	)
	require.NotNil(t, cat)

	requirePackageError(t, err, "fail-op", "finalize")

	goodPkg, getErr := cat.GetPackage(ctx, "good-op")
	require.NoError(t, getErr)
	assert.Equal(t, "good-op", goodPkg.Name())
}

type perPackageFailExtension struct {
	*testOLMPackageExtension
	failPackage string
}

func (e *perPackageFailExtension) FinalizePackage(ctx context.Context, pkg fbc.PackageAccessor, w fbc.PropertyWriter) error {
	if pkg.Name() == e.failPackage {
		return fmt.Errorf("intentional finalize failure")
	}
	return e.testOLMPackageExtension.FinalizePackage(ctx, pkg, w)
}

func TestOLMPackageExtension_NoExtension(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("no-ext").
		WithChannel("no-ext", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("no-ext", "1.0.0").
		Build()
	ctx := context.Background()

	store, err := catalogv1.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()

	imp := fbc.NewImporter(fsys)
	cat, err := store.Set(ctx, "test",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "test"),
	)
	require.NoError(t, err)

	pkg, err := cat.GetPackage(ctx, "no-ext")
	require.NoError(t, err)
	assert.Equal(t, "no-ext", pkg.Name())
}

func TestProperties_DeletedOnStoreDelete(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("del-op").
		WithChannel("del-op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("del-op", "1.0.0").
		Build()
	ctx := context.Background()

	ext := &testOLMPackageExtension{}
	store, err := catalogv1.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()

	imp := fbc.NewImporter(fsys, fbc.WithOLMPackageExtension(ext))
	cat, err := store.Set(ctx, "test",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "test"),
	)
	require.NoError(t, err)

	pkg, err := cat.GetPackage(ctx, "del-op")
	require.NoError(t, err)
	val, err := pkg.Property(ctx, "display-name")
	require.NoError(t, err)
	require.NotNil(t, val)

	require.NoError(t, store.Delete("test"))

	_, err = store.Get("test")
	require.Error(t, err)
}

func TestProperties_RebuiltOnSchemaChange(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("reb-op").
		WithChannel("reb-op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("reb-op", "1.0.0").
		Build()
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), "test.db")

	ext := &testOLMPackageExtension{}
	store, err := catalogv1.OpenStore(dbPath)
	require.NoError(t, err)

	imp := fbc.NewImporter(fsys, fbc.WithOLMPackageExtension(ext))
	_, err = store.Set(ctx, "test",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "test"),
	)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	store2, err := catalogv1.OpenStore(dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, store2.Close()) }()

	cat, err := store2.Get("test")
	require.NoError(t, err)

	pkg, err := cat.GetPackage(ctx, "reb-op")
	require.NoError(t, err)
	assert.Equal(t, "reb-op", pkg.Name())
}
