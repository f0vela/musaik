package lastfm

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"musaik/internal/model"
)

const apiURL = "https://ws.audioscrobbler.com/2.0/"

type Client struct {
	APIKey     string
	APISecret  string
	SessionKey string
	Username   string
	HTTP       *http.Client
}

func New(apiKey, apiSecret, sessionKey, username string) *Client {
	return &Client{
		APIKey:     apiKey,
		APISecret:  apiSecret,
		SessionKey: sessionKey,
		Username:   username,
		HTTP:       &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c.APIKey != "" && c.APISecret != "" && c.SessionKey != "" && c.Username != ""
}

func (c *Client) AuthURL(callbackURL string) string {
	v := url.Values{}
	v.Set("api_key", c.APIKey)
	if callbackURL != "" {
		v.Set("cb", callbackURL)
	}
	return "https://www.last.fm/api/auth/?" + v.Encode()
}

func (c *Client) SessionFromToken(token string) (sessionKey string, username string, err error) {
	if c.APIKey == "" || c.APISecret == "" {
		return "", "", fmt.Errorf("missing API key or shared secret")
	}
	params := map[string]string{
		"method":  "auth.getSession",
		"api_key": c.APIKey,
		"token":   token,
		"format":  "json",
	}
	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}
	v.Set("api_sig", c.signature(params))

	resp, err := c.HTTP.PostForm(apiURL, v)
	if err != nil {
		return "", "", fmt.Errorf("post auth.getSession: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read auth.getSession response: %w", err)
	}

	var payload struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
		Session struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		} `json:"session"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", fmt.Errorf("decode auth.getSession response: %w", err)
	}
	if payload.Error != 0 {
		return "", "", fmt.Errorf("last.fm error %d: %s", payload.Error, payload.Message)
	}
	if payload.Session.Key == "" {
		return "", "", fmt.Errorf("last.fm did not return session key")
	}
	return payload.Session.Key, payload.Session.Name, nil
}

func (c *Client) UpdateNowPlaying(song model.Song) error {
	if !c.Enabled() {
		return nil
	}
	params := map[string]string{
		"method":  "track.updateNowPlaying",
		"artist":  song.Artist,
		"track":   song.Title,
		"album":   song.Album,
		"api_key": c.APIKey,
		"sk":      c.SessionKey,
		"format":  "json",
	}
	return c.postSigned(params)
}

func (c *Client) Scrobble(song model.Song, at time.Time) error {
	if !c.Enabled() {
		return nil
	}
	params := map[string]string{
		"method":       "track.scrobble",
		"artist":       song.Artist,
		"track":        song.Title,
		"album":        song.Album,
		"timestamp":    strconv.FormatInt(at.Unix(), 10),
		"api_key":      c.APIKey,
		"sk":           c.SessionKey,
		"chosenByUser": "1",
		"format":       "json",
	}
	return c.postSigned(params)
}

func (c *Client) LoveTrack(song model.Song) error {
	if !c.Enabled() {
		return nil
	}
	params := map[string]string{
		"method":  "track.love",
		"artist":  song.Artist,
		"track":   song.Title,
		"api_key": c.APIKey,
		"sk":      c.SessionKey,
		"format":  "json",
	}
	return c.postSigned(params)
}

func (c *Client) UserPlayCount(song model.Song) (int, error) {
	if c.APIKey == "" || c.Username == "" {
		return 0, nil
	}
	v := url.Values{}
	v.Set("method", "track.getInfo")
	v.Set("api_key", c.APIKey)
	v.Set("artist", song.Artist)
	v.Set("track", song.Title)
	v.Set("username", c.Username)
	v.Set("format", "json")

	reqURL := apiURL + "?" + v.Encode()
	resp, err := c.HTTP.Get(reqURL)
	if err != nil {
		return 0, fmt.Errorf("request track.getInfo: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read track.getInfo response: %w", err)
	}

	var payload struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
		Track   struct {
			UserPlaycount string `json:"userplaycount"`
		} `json:"track"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("decode track.getInfo: %w", err)
	}
	if payload.Error != 0 {
		return 0, fmt.Errorf("last.fm error %d: %s", payload.Error, payload.Message)
	}

	if payload.Track.UserPlaycount == "" {
		return 0, nil
	}
	count, err := strconv.Atoi(payload.Track.UserPlaycount)
	if err != nil {
		return 0, fmt.Errorf("parse userplaycount: %w", err)
	}
	return count, nil
}

func (c *Client) ArtistImageURL(artist string) (string, error) {
	if c.APIKey == "" || strings.TrimSpace(artist) == "" {
		return "", nil
	}
	v := url.Values{}
	v.Set("method", "artist.getinfo")
	v.Set("api_key", c.APIKey)
	v.Set("artist", artist)
	v.Set("autocorrect", "1")
	v.Set("format", "json")

	reqURL := apiURL + "?" + v.Encode()
	resp, err := c.HTTP.Get(reqURL)
	if err != nil {
		return "", fmt.Errorf("request artist.getinfo: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read artist.getinfo response: %w", err)
	}

	var payload struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
		Artist  struct {
			Image []struct {
				URL  string `json:"#text"`
				Size string `json:"size"`
			} `json:"image"`
		} `json:"artist"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode artist.getinfo: %w", err)
	}
	if payload.Error != 0 {
		return "", fmt.Errorf("last.fm error %d: %s", payload.Error, payload.Message)
	}
	if len(payload.Artist.Image) == 0 {
		return "", nil
	}

	sizeRank := map[string]int{
		"small":      1,
		"medium":     2,
		"large":      3,
		"extralarge": 4,
		"mega":       5,
	}
	bestURL := ""
	bestRank := -1
	for _, img := range payload.Artist.Image {
		u := strings.TrimSpace(img.URL)
		if u == "" {
			continue
		}
		r := sizeRank[strings.ToLower(strings.TrimSpace(img.Size))]
		if r > bestRank {
			bestRank = r
			bestURL = u
		}
	}
	return bestURL, nil
}

func (c *Client) postSigned(params map[string]string) error {
	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}
	v.Set("api_sig", c.signature(params))

	resp, err := c.HTTP.PostForm(apiURL, v)
	if err != nil {
		return fmt.Errorf("post last.fm: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read last.fm response: %w", err)
	}

	var payload struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("decode last.fm response: %w", err)
	}
	if payload.Error != 0 {
		return fmt.Errorf("last.fm error %d: %s", payload.Error, payload.Message)
	}
	return nil
}

func (c *Client) signature(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "format" || k == "callback" {
			continue
		}
		keys = append(keys, k)
	}
	// Stable lexical order.
	for i := 0; i < len(keys)-1; i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(params[k])
	}
	b.WriteString(c.APISecret)

	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
