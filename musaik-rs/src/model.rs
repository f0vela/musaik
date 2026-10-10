use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Song {
    pub id: String,
    pub path: String,
    pub title: String,
    pub artist: String,
    pub album: String,
    pub track_num: Option<u32>,
    pub duration_secs: Option<f64>,
    pub format: String,
    pub play_count: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct SmartPlaylistFilter {
    pub artist_contains: Option<String>,
    pub album_contains: Option<String>,
    pub title_contains: Option<String>,
    pub favorites_only: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Playlist {
    pub name: String,
    pub manual_song: Vec<String>,
    pub smart: Option<SmartPlaylistFilter>,
}
