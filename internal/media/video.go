package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// VideoMeta is the resolved metadata for a video (for the editor: show a
// thumbnail + title + duration when authoring a video-embed block).
type VideoMeta struct {
	Title           string `json:"title"`
	ThumbnailURL    string `json:"thumbnailUrl"`
	DurationSeconds int    `json:"durationSeconds"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
}

// VideoProvider resolves a provider-specific video id to a playable embed URL
// and (best-effort) metadata. It is an opt-in plugin (Beslutning 9: Vimeo via
// their API, pluggable) selected by config; other providers (e.g. Bunny Stream)
// can be added later.
//
// Metadata uses the provider's PUBLIC oEmbed endpoint (no auth) — sufficient for
// public/unlisted videos. Private/tenant-restricted videos need the per-tenant
// token (owned by Etude's tenant_registry.video_account_ref) — a follow-up that
// respects that boundary.
type VideoProvider interface {
	Name() string
	EmbedURL(videoID string) string
	Metadata(ctx context.Context, videoID string) (VideoMeta, error)
}

// VimeoProvider builds Vimeo player embed URLs and resolves oEmbed metadata.
type VimeoProvider struct {
	oembedBase string
	client     *http.Client
}

// NewVimeoProvider constructs a Vimeo provider against the public oEmbed API.
func NewVimeoProvider() *VimeoProvider {
	return &VimeoProvider{
		oembedBase: "https://vimeo.com/api/oembed.json",
		client:     &http.Client{Timeout: 8 * time.Second},
	}
}

// Name identifies the provider.
func (p *VimeoProvider) Name() string { return "vimeo" }

// EmbedURL returns the Vimeo player URL for a video id.
func (p *VimeoProvider) EmbedURL(videoID string) string {
	return "https://player.vimeo.com/video/" + url.PathEscape(videoID)
}

// vimeoOEmbed is the subset of the Vimeo oEmbed response we surface.
type vimeoOEmbed struct {
	Title        string `json:"title"`
	ThumbnailURL string `json:"thumbnail_url"`
	Duration     int    `json:"duration"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
}

// Metadata fetches public oEmbed metadata for a video id.
func (p *VimeoProvider) Metadata(ctx context.Context, videoID string) (VideoMeta, error) {
	q := url.Values{"url": {"https://vimeo.com/" + videoID}}
	endpoint := p.oembedBase + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return VideoMeta{}, fmt.Errorf("media: vimeo oembed request: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return VideoMeta{}, fmt.Errorf("media: vimeo oembed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return VideoMeta{}, fmt.Errorf("media: vimeo oembed status %d (video private or not found)", resp.StatusCode)
	}
	var oe vimeoOEmbed
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&oe); err != nil {
		return VideoMeta{}, fmt.Errorf("media: decode vimeo oembed: %w", err)
	}
	return VideoMeta{
		Title:           oe.Title,
		ThumbnailURL:    oe.ThumbnailURL,
		DurationSeconds: oe.Duration,
		Width:           oe.Width,
		Height:          oe.Height,
	}, nil
}

// ProviderByName returns the registered video provider for a name.
func ProviderByName(name string) (VideoProvider, bool) {
	switch name {
	case "vimeo":
		return NewVimeoProvider(), true
	default:
		return nil, false
	}
}
