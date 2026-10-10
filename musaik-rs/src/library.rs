use std::collections::{BTreeSet, HashMap};
use std::fs;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use walkdir::WalkDir;

use crate::model::{Playlist, SmartPlaylistFilter, Song};

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Store {
    pub root: String,
    pub songs: Vec<Song>,
    pub favorites: HashMap<String, bool>,
    pub playlists: Vec<Playlist>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
struct PersistedState {
    favorites: HashMap<String, bool>,
    playlists: Vec<Playlist>,
    root: String,
    songs: Vec<Song>,
}

impl Store {
    pub fn new() -> Self {
        Self {
            playlists: vec![
                Playlist {
                    name: "Todos".to_string(),
                    ..Default::default()
                },
                Playlist {
                    name: "Favoritos".to_string(),
                    smart: Some(SmartPlaylistFilter {
                        favorites_only: true,
                        ..Default::default()
                    }),
                    ..Default::default()
                },
            ],
            ..Default::default()
        }
    }

    pub fn index<P: AsRef<Path>>(&mut self, root: P) -> Result<()> {
        let old_counts: HashMap<String, u32> = self
            .songs
            .iter()
            .filter(|s| s.play_count > 0)
            .map(|s| (s.id.clone(), s.play_count))
            .collect();

        let root = root.as_ref();
        let mut songs = Vec::new();
        for entry in WalkDir::new(root).into_iter().filter_map(|e| e.ok()) {
            if !entry.file_type().is_file() {
                continue;
            }
            let path = entry.path();
            if !is_supported(path) {
                continue;
            }
            let mut song = scan_file(path);
            if let Some(count) = old_counts.get(&song.id) {
                song.play_count = *count;
            }
            songs.push(song);
        }

        self.root = root.to_string_lossy().to_string();
        self.songs = songs;
        Ok(())
    }

    pub fn artists(&self) -> Vec<String> {
        let mut set = BTreeSet::new();
        for song in &self.songs {
            set.insert(song.artist.clone());
        }
        set.into_iter().collect()
    }

    pub fn albums(&self) -> Vec<String> {
        let mut set = BTreeSet::new();
        for song in &self.songs {
            set.insert(song.album.clone());
        }
        set.into_iter().collect()
    }

    pub fn toggle_favorite(&mut self, song_id: &str) {
        if self.favorites.remove(song_id).is_none() {
            self.favorites.insert(song_id.to_string(), true);
        }
    }

    pub fn increment_play_count(&mut self, song_id: &str) -> Option<u32> {
        let song = self.songs.iter_mut().find(|s| s.id == song_id)?;
        song.play_count += 1;
        Some(song.play_count)
    }

    pub fn set_play_count(&mut self, song_id: &str, count: u32) -> bool {
        let Some(song) = self.songs.iter_mut().find(|s| s.id == song_id) else {
            return false;
        };
        song.play_count = count;
        true
    }

    pub fn playlist_songs(&self, playlist: &Playlist) -> Vec<Song> {
        if playlist.smart.is_none() && playlist.manual_song.is_empty() {
            return self.songs.clone();
        }
        if let Some(smart) = &playlist.smart {
            return self
                .songs
                .iter()
                .filter(|song| {
                    if smart.favorites_only && !self.favorites.contains_key(&song.id) {
                        return false;
                    }
                    if let Some(s) = &smart.artist_contains {
                        if !song.artist.to_lowercase().contains(&s.to_lowercase()) {
                            return false;
                        }
                    }
                    if let Some(s) = &smart.album_contains {
                        if !song.album.to_lowercase().contains(&s.to_lowercase()) {
                            return false;
                        }
                    }
                    if let Some(s) = &smart.title_contains {
                        if !song.title.to_lowercase().contains(&s.to_lowercase()) {
                            return false;
                        }
                    }
                    true
                })
                .cloned()
                .collect();
        }

        let map: HashMap<String, Song> = self
            .songs
            .iter()
            .cloned()
            .map(|s| (s.id.clone(), s))
            .collect();
        playlist
            .manual_song
            .iter()
            .filter_map(|id| map.get(id).cloned())
            .collect()
    }

    pub fn add_song_to_manual_playlist(
        &mut self,
        playlist_name: &str,
        song_id: &str,
    ) -> Result<()> {
        let pl = self
            .playlists
            .iter_mut()
            .find(|p| p.name == playlist_name)
            .context("playlist not found")?;
        if pl.smart.is_some() || pl.name == "Todos" {
            anyhow::bail!("playlist {} no admite edición manual", playlist_name);
        }
        if !pl.manual_song.iter().any(|id| id == song_id) {
            pl.manual_song.push(song_id.to_string());
        }
        Ok(())
    }

    pub fn state_path() -> Result<PathBuf> {
        let mut dir = dirs::config_dir().context("config dir unavailable")?;
        dir.push("musaik-rs");
        fs::create_dir_all(&dir).context("create config dir")?;
        Ok(dir.join("state.json"))
    }

    pub fn save_state(&self) -> Result<()> {
        let p = PersistedState {
            favorites: self.favorites.clone(),
            playlists: self.playlists.clone(),
            root: self.root.clone(),
            songs: self.songs.clone(),
        };
        let bytes = serde_json::to_vec_pretty(&p).context("encode state")?;
        fs::write(Self::state_path()?, bytes).context("write state")?;
        Ok(())
    }

    pub fn load_state(&mut self) -> Result<()> {
        let path = Self::state_path()?;
        if !path.exists() {
            return Ok(());
        }
        let data = fs::read(path).context("read state")?;
        let p: PersistedState = serde_json::from_slice(&data).context("decode state")?;
        self.favorites = p.favorites;
        if !p.playlists.is_empty() {
            self.playlists = p.playlists;
        }
        self.root = p.root;
        self.songs = p.songs;
        Ok(())
    }
}

fn is_supported(path: &Path) -> bool {
    matches!(
        path.extension()
            .and_then(|v| v.to_str())
            .map(|v| v.to_lowercase()),
        Some(ext) if ext == "mp3" || ext == "mp4" || ext == "flac" || ext == "ogg"
    )
}

fn scan_file(path: &Path) -> Song {
    let file_name = path
        .file_stem()
        .and_then(|v| v.to_str())
        .unwrap_or("Unknown title")
        .to_string();
    let format = path
        .extension()
        .and_then(|v| v.to_str())
        .unwrap_or_default()
        .to_lowercase();
    Song {
        id: path.to_string_lossy().to_string(),
        path: path.to_string_lossy().to_string(),
        title: file_name,
        artist: "Unknown Artist".into(),
        album: "Unknown Album".into(),
        format,
        ..Default::default()
    }
}
