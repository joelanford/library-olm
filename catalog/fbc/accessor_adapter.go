package fbc

import (
	"encoding/json"
	"iter"

	"github.com/joelanford/library-olm/catalog/fbc/internal"
)

type packageAccessorAdapter struct {
	a *internal.PackageAccessor
}

func (p *packageAccessorAdapter) Name() string                      { return p.a.Name() }
func (p *packageAccessorAdapter) ExtData() (json.RawMessage, error) { return p.a.ExtData() }

func (p *packageAccessorAdapter) Bundles() iter.Seq2[BundleAccessor, error] {
	return func(yield func(BundleAccessor, error) bool) {
		for b, err := range p.a.Bundles() {
			if !yield(b, err) {
				return
			}
		}
	}
}

func (p *packageAccessorAdapter) Channels() iter.Seq2[ChannelAccessor, error] {
	return func(yield func(ChannelAccessor, error) bool) {
		for ch, err := range p.a.Channels() {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(&channelAccessorAdapter{ch: ch}, nil) {
				return
			}
		}
	}
}

func (p *packageAccessorAdapter) Deprecations() iter.Seq2[DeprecationAccessor, error] {
	return func(yield func(DeprecationAccessor, error) bool) {
		for d, err := range p.a.Deprecations() {
			if !yield(d, err) {
				return
			}
		}
	}
}

func (p *packageAccessorAdapter) Others() iter.Seq2[OtherAccessor, error] {
	return func(yield func(OtherAccessor, error) bool) {
		for o, err := range p.a.Others() {
			if !yield(o, err) {
				return
			}
		}
	}
}

type channelAccessorAdapter struct {
	ch internal.ChannelAccessor
}

func (c *channelAccessorAdapter) Name() string             { return c.ch.Name() }
func (c *channelAccessorAdapter) ExtData() json.RawMessage { return c.ch.ExtData() }

func (c *channelAccessorAdapter) Entries() iter.Seq2[ChannelEntryAccessor, error] {
	return func(yield func(ChannelEntryAccessor, error) bool) {
		for e, err := range c.ch.Entries() {
			if !yield(e, err) {
				return
			}
		}
	}
}
