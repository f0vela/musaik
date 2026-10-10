use std::collections::BTreeMap;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use reqwest::blocking::Client;
use serde::Deserialize;
use url::Url;

use crate::model::Song;

const API_URL: &str = "https://ws.audioscrobbler.com/2.0/";

#[derive(Clone)]
pub struct LastFmClient {
    pub api_key: String,
    pub api_secret: String,
    pub session_key: String,
    pub username: String,
    http: Client,
}

impl LastFmClient {
    pub fn new(api_key: String, api_secret: String, session_key: String, username: String) -> Self {
        Self {
            api_key,
            api_secret,
            session_key,
            username,
            http: Client::builder()
                .timeout(Duration::from_secs(15))
                .build()
                .expect("create lastfm client"),
        }
    }

    pub fn enabled(&self) -> bool {
        !self.api_key.is_empty()
            && !self.api_secret.is_empty()
            && !self.session_key.is_empty()
            && !self.username.is_empty()
    }

    pub fn auth_url(&self, callback_url: &str) -> Result<String> {
        let mut u = Url::parse("https://www.last.fm/api/auth/")?;
        u.query_pairs_mut()
            .append_pair("api_key", &self.api_key)
            .append_pair("cb", callback_url);
        Ok(u.to_string())
    }

    pub fn session_from_token(&self, token: &str) -> Result<(String, String)> {
        let mut params = BTreeMap::new();
        params.insert("method", "auth.getSession".to_string());
        params.insert("api_key", self.api_key.clone());
        params.insert("token", token.to_string());
        params.insert("format", "json".to_string());
        params.insert("api_sig", self.signature(&params));

        let payload: SessionPayload = self
            .http
            .post(API_URL)
            .form(&params)
            .send()
            .context("post auth.getSession")?
            .json()
            .context("decode auth.getSession")?;
        if let Some(err) = payload.error {
            anyhow::bail!(
                "last.fm error {}: {}",
                err,
                payload.message.unwrap_or_default()
            );
        }
        let s = payload.session.context("session missing")?;
        Ok((s.key, s.name))
    }

    pub fn update_now_playing(&self, song: &Song) -> Result<()> {
        if !self.enabled() {
            return Ok(());
        }
        let mut params = self.signed_base("track.updateNowPlaying");
        params.insert("artist", song.artist.clone());
        params.insert("track", song.title.clone());
        params.insert("album", song.album.clone());
        self.post_signed(params)
    }

    pub fn scrobble(&self, song: &Song) -> Result<()> {
        if !self.enabled() {
            return Ok(());
        }
        let mut params = self.signed_base("track.scrobble");
        let ts = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        params.insert("artist", song.artist.clone());
        params.insert("track", song.title.clone());
        params.insert("album", song.album.clone());
        params.insert("timestamp", ts.to_string());
        params.insert("chosenByUser", "1".to_string());
        self.post_signed(params)
    }

    pub fn love_track(&self, song: &Song) -> Result<()> {
        if !self.enabled() {
            return Ok(());
        }
        let mut params = self.signed_base("track.love");
        params.insert("artist", song.artist.clone());
        params.insert("track", song.title.clone());
        self.post_signed(params)
    }

    pub fn user_play_count(&self, song: &Song) -> Result<u32> {
        if self.api_key.is_empty() || self.username.is_empty() {
            return Ok(0);
        }
        let payload: TrackInfoPayload = self
            .http
            .get(API_URL)
            .query(&[
                ("method", "track.getInfo"),
                ("api_key", self.api_key.as_str()),
                ("artist", song.artist.as_str()),
                ("track", song.title.as_str()),
                ("username", self.username.as_str()),
                ("format", "json"),
            ])
            .send()
            .context("request track.getInfo")?
            .json()
            .context("decode track.getInfo")?;
        if let Some(err) = payload.error {
            anyhow::bail!(
                "last.fm error {}: {}",
                err,
                payload.message.unwrap_or_default()
            );
        }
        let count = payload
            .track
            .and_then(|t| t.userplaycount)
            .unwrap_or_else(|| "0".to_string());
        Ok(count.parse::<u32>().unwrap_or(0))
    }

    fn signed_base(&self, method: &str) -> BTreeMap<&'static str, String> {
        let mut params = BTreeMap::new();
        params.insert("method", method.to_string());
        params.insert("api_key", self.api_key.clone());
        params.insert("sk", self.session_key.clone());
        params.insert("format", "json".to_string());
        params
    }

    fn post_signed(&self, mut params: BTreeMap<&str, String>) -> Result<()> {
        let sig = self.signature(&params);
        params.insert("api_sig", sig);
        let payload: GenericPayload = self
            .http
            .post(API_URL)
            .form(&params)
            .send()
            .context("post lastfm")?
            .json()
            .context("decode lastfm")?;
        if let Some(err) = payload.error {
            anyhow::bail!(
                "last.fm error {}: {}",
                err,
                payload.message.unwrap_or_default()
            );
        }
        Ok(())
    }

    fn signature<K: AsRef<str>>(&self, params: &BTreeMap<K, String>) -> String {
        let mut payload = String::new();
        for (k, v) in params {
            let key = k.as_ref();
            if key == "format" || key == "callback" {
                continue;
            }
            payload.push_str(key);
            payload.push_str(v);
        }
        payload.push_str(&self.api_secret);
        format!("{:x}", md5::compute(payload))
    }
}

#[derive(Deserialize)]
struct GenericPayload {
    error: Option<i64>,
    message: Option<String>,
}

#[derive(Deserialize)]
struct SessionPayload {
    error: Option<i64>,
    message: Option<String>,
    session: Option<SessionData>,
}

#[derive(Deserialize)]
struct SessionData {
    name: String,
    key: String,
}

#[derive(Deserialize)]
struct TrackInfoPayload {
    error: Option<i64>,
    message: Option<String>,
    track: Option<TrackInfo>,
}

#[derive(Deserialize)]
struct TrackInfo {
    userplaycount: Option<String>,
}
