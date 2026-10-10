use std::fs;
use std::path::PathBuf;

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AppSettings {
    pub section: String,
    pub shuffle: bool,
    pub repeat: bool,
    pub root_input: String,
    pub search_query: String,
    pub lastfm_api_key: String,
    pub lastfm_api_secret: String,
    pub lastfm_session_key: String,
    pub lastfm_username: String,
    pub ipod_mount_path: String,
    pub ipod_playlist_name: String,
}

impl Default for AppSettings {
    fn default() -> Self {
        Self {
            section: "Explorar".into(),
            shuffle: false,
            repeat: false,
            root_input: default_music_dir(),
            search_query: String::new(),
            lastfm_api_key: String::new(),
            lastfm_api_secret: String::new(),
            lastfm_session_key: String::new(),
            lastfm_username: String::new(),
            ipod_mount_path: String::new(),
            ipod_playlist_name: "Todos".into(),
        }
    }
}

impl AppSettings {
    pub fn path() -> Result<PathBuf> {
        let mut dir = dirs::config_dir().context("config dir unavailable")?;
        dir.push("musaik-rs");
        fs::create_dir_all(&dir).context("create config dir")?;
        Ok(dir.join("settings.json"))
    }

    pub fn load() -> Result<Self> {
        let p = Self::path()?;
        if !p.exists() {
            return Ok(Self::default());
        }
        let data = fs::read(p).context("read settings")?;
        let s = serde_json::from_slice(&data).context("decode settings")?;
        Ok(s)
    }

    pub fn save(&self) -> Result<()> {
        let bytes = serde_json::to_vec_pretty(self).context("encode settings")?;
        fs::write(Self::path()?, bytes).context("write settings")?;
        Ok(())
    }
}

pub fn default_music_dir() -> String {
    dirs::home_dir()
        .map(|h| h.join("Music").to_string_lossy().to_string())
        .unwrap_or_else(|| ".".to_string())
}
