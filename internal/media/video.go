package media

import "net/url"

// VideoProvider resolves a provider-specific video id to a playable embed URL.
// It is an opt-in plugin (Beslutning 9: Vimeo via their API, pluggable) selected
// by config; other providers (e.g. Bunny Stream) can be added later.
//
// v1 builds the deterministic embed URL only (no external API call, no
// per-tenant token). Fetching title/thumbnail/duration via Vimeo oEmbed (public)
// or the authenticated Vimeo API (per-tenant token, owned by Etude's
// tenant_registry.video_account_ref) is a follow-up that respects that boundary.
type VideoProvider interface {
	Name() string
	EmbedURL(videoID string) string
}

// VimeoProvider builds Vimeo player embed URLs.
type VimeoProvider struct{}

// Name identifies the provider.
func (VimeoProvider) Name() string { return "vimeo" }

// EmbedURL returns the Vimeo player URL for a video id.
func (VimeoProvider) EmbedURL(videoID string) string {
	return "https://player.vimeo.com/video/" + url.PathEscape(videoID)
}

// ProviderByName returns the registered video provider for a name.
func ProviderByName(name string) (VideoProvider, bool) {
	switch name {
	case "vimeo":
		return VimeoProvider{}, true
	default:
		return nil, false
	}
}
