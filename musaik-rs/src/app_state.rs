use std::collections::{BTreeMap, BTreeSet};
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::Result;

use crate::lastfm::LastFmClient;
use crate::library::Store;
use crate::model::Song;
use crate::player::Player;
use crate::settings::AppSettings;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Section {
    Explore,
    Recent,
    Library,
    Artists,
    Albums,
    Favorites,
    Playlists,
    Settings,
}

impl Section {
    pub fn from_label(s: &str) -> Self {
        match s {
            "Recientes" => Self::Recent,
            "Librería" | "Canciones" => Self::Library,
            "Artistas" => Self::Artists,
            "Álbumes" => Self::Albums,
            "Favoritos" => Self::Favorites,
            "Playlists" => Self::Playlists,
            "Ajustes" => Self::Settings,
            _ => Self::Explore,
        }
    }

    pub fn label(self) -> &'static str {
        match self {
            Self::Explore => "Explorar",
            Self::Recent => "Recientes",
            Self::Library => "Librería",
            Self::Artists => "Artistas",
            Self::Albums => "Álbumes",
            Self::Favorites => "Favoritos",
            Self::Playlists => "Playlists",
            Self::Settings => "Ajustes",
        }
    }
}

#[derive(Debug, Clone)]
pub struct ArtistAlbumGroup {
    pub album: String,
    pub songs: Vec<Song>,
}

#[derive(Debug, Clone)]
pub struct ActionHit {
    pub kind: String,
    pub action_id: Option<String>,
    pub song_id: Option<String>,
    pub artist_name: Option<String>,
    pub album_name: Option<String>,
    pub title: String,
    pub subtitle: String,
}

pub struct AppState {
    pub store: Store,
    pub settings: AppSettings,
    pub player: Player,
    pub lastfm: LastFmClient,
    pub section: Section,
    pub status: String,
    pub selected_song_id: Option<String>,
    pub selected_artist: Option<String>,
    pub action_query: String,
    pub quit_requested: bool,
}

impl AppState {
    pub fn load() -> Result<Self> {
        let mut store = Store::new();
        let _ = store.load_state();
        let settings = AppSettings::load().unwrap_or_default();
        let lastfm = LastFmClient::new(
            settings.lastfm_api_key.clone(),
            settings.lastfm_api_secret.clone(),
            settings.lastfm_session_key.clone(),
            settings.lastfm_username.clone(),
        );

        let status = if store.songs.is_empty() {
            "Listo para indexar música".to_string()
        } else {
            format!("Biblioteca cargada: {} canciones", store.songs.len())
        };

        let mut state = Self {
            store,
            settings,
            player: Player::new(),
            lastfm,
            section: Section::Explore,
            status,
            selected_song_id: None,
            selected_artist: None,
            action_query: String::new(),
            quit_requested: false,
        };
        state.hydrate_runtime_from_settings();
        Ok(state)
    }

    pub fn persist_all(&self) -> Result<()> {
        self.store.save_state()?;
        self.settings.save()?;
        Ok(())
    }

    pub fn refresh_lastfm_client(&mut self) {
        self.lastfm = LastFmClient::new(
            self.settings.lastfm_api_key.clone(),
            self.settings.lastfm_api_secret.clone(),
            self.settings.lastfm_session_key.clone(),
            self.settings.lastfm_username.clone(),
        );
    }

    pub fn hydrate_runtime_from_settings(&mut self) {
        self.section = Section::from_label(&self.settings.section);
        if self.selected_artist.is_none() {
            self.selected_artist = self.store.artists().first().cloned();
        }
    }

    pub fn index_library(&mut self) -> Result<()> {
        self.store.index(&self.settings.root_input)?;
        self.status = format!("Indexadas {} canciones", self.store.songs.len());
        if self.selected_song_id.is_none() {
            self.selected_song_id = self.store.songs.first().map(|s| s.id.clone());
        }
        if self.selected_artist.is_none() {
            self.selected_artist = self.store.artists().first().cloned();
        }
        self.persist_all()?;
        Ok(())
    }

    pub fn play_song(&mut self, song: &Song) -> Result<()> {
        self.player.play(&song.path, 0.0)?;
        self.store.increment_play_count(&song.id);
        self.selected_song_id = Some(song.id.clone());
        self.status = format!("Reproduciendo: {} — {}", song.artist, song.title);
        let _ = self.lastfm.update_now_playing(song);
        let _ = self.lastfm.scrobble(song);
        let _ = self.persist_all();
        Ok(())
    }

    pub fn toggle_like(&mut self, song: &Song) {
        let was = self.store.favorites.contains_key(&song.id);
        self.store.toggle_favorite(&song.id);
        if !was && self.lastfm.enabled() {
            let _ = self.lastfm.love_track(song);
        }
        let _ = self.store.save_state();
    }

    pub fn set_section(&mut self, section: Section) {
        self.section = section;
        self.settings.section = section.label().to_string();
        let _ = self.settings.save();
    }

    pub fn all_songs_filtered(&self, query: &str) -> Vec<Song> {
        let q = query.trim().to_lowercase();
        if q.is_empty() {
            return self.store.songs.clone();
        }
        self.store
            .songs
            .iter()
            .filter(|s| {
                s.title.to_lowercase().contains(&q)
                    || s.artist.to_lowercase().contains(&q)
                    || s.album.to_lowercase().contains(&q)
            })
            .cloned()
            .collect()
    }

    pub fn favorite_songs_filtered(&self, query: &str) -> Vec<Song> {
        self.all_songs_filtered(query)
            .into_iter()
            .filter(|s| self.store.favorites.contains_key(&s.id))
            .collect()
    }

    pub fn album_representatives(&self, query: &str, limit: usize) -> Vec<Song> {
        let mut out = Vec::new();
        let mut seen = BTreeSet::new();
        for song in self.all_songs_filtered(query) {
            let key = format!(
                "{}||{}",
                song.artist.trim().to_lowercase(),
                song.album.trim().to_lowercase()
            );
            if seen.contains(&key) {
                continue;
            }
            seen.insert(key);
            out.push(song);
            if out.len() >= limit {
                break;
            }
        }
        out
    }

    pub fn artist_album_groups(&self, artist: &str) -> Vec<ArtistAlbumGroup> {
        let mut by_album: BTreeMap<String, Vec<Song>> = BTreeMap::new();
        for song in &self.store.songs {
            if song.artist != artist {
                continue;
            }
            let album = if song.album.trim().is_empty() {
                "Unknown Album".to_string()
            } else {
                song.album.clone()
            };
            by_album.entry(album).or_default().push(song.clone());
        }

        by_album
            .into_iter()
            .map(|(album, mut songs)| {
                songs.sort_by(
                    |a, b| match (a.track_num.unwrap_or(0), b.track_num.unwrap_or(0)) {
                        (ta, tb) if ta > 0 && tb > 0 && ta != tb => ta.cmp(&tb),
                        (ta, 0) if ta > 0 => std::cmp::Ordering::Less,
                        (0, tb) if tb > 0 => std::cmp::Ordering::Greater,
                        _ => a.title.to_lowercase().cmp(&b.title.to_lowercase()),
                    },
                );
                ArtistAlbumGroup { album, songs }
            })
            .collect()
    }

    pub fn action_hits(&self, query: &str) -> Vec<ActionHit> {
        let raw = query.trim().to_lowercase();
        let (scope, q) = if let Some(rest) = raw.strip_prefix("action:") {
            ("action", rest.trim().to_string())
        } else if let Some(rest) = raw.strip_prefix("artist:") {
            ("artist", rest.trim().to_string())
        } else if let Some(rest) = raw.strip_prefix("album:") {
            ("album", rest.trim().to_string())
        } else if let Some(rest) = raw.strip_prefix("song:") {
            ("song", rest.trim().to_string())
        } else {
            ("all", raw)
        };
        let mut out = Vec::new();

        if scope == "all" || scope == "action" {
            for (action_id, title) in [
                ("play", "Play"),
                ("pause", "Pause"),
                ("next", "Next"),
                ("previous", "Previous"),
                ("stop", "Stop"),
                ("settings", "Ajustes"),
                ("quit", "Quit"),
            ] {
                if !q.is_empty() && !title.to_lowercase().contains(&q) && !action_id.contains(&q) {
                    continue;
                }
                out.push(ActionHit {
                    kind: "action".into(),
                    action_id: Some(action_id.into()),
                    song_id: None,
                    artist_name: None,
                    album_name: None,
                    title: title.into(),
                    subtitle: "Acción".into(),
                });
            }
        }

        if scope == "all" || scope == "artist" {
            for artist in self.store.artists() {
                if !q.is_empty() && !artist.to_lowercase().contains(&q) {
                    continue;
                }
                out.push(ActionHit {
                    kind: "artist".into(),
                    action_id: None,
                    song_id: None,
                    artist_name: Some(artist.clone()),
                    album_name: None,
                    title: artist,
                    subtitle: "Artista".into(),
                });
                if out.len() > 14 {
                    break;
                }
            }
        }

        let mut seen_albums = BTreeSet::new();
        if scope != "action" && scope != "artist" {
            for song in &self.store.songs {
                if !q.is_empty()
                    && !song.title.to_lowercase().contains(&q)
                    && !song.artist.to_lowercase().contains(&q)
                    && !song.album.to_lowercase().contains(&q)
                {
                    continue;
                }
                if scope == "all" || scope == "song" {
                    out.push(ActionHit {
                        kind: "song".into(),
                        action_id: None,
                        song_id: Some(song.id.clone()),
                        artist_name: Some(song.artist.clone()),
                        album_name: Some(song.album.clone()),
                        title: song.title.clone(),
                        subtitle: format!("{} • {}", song.artist, song.album),
                    });
                }

                let album_key = format!(
                    "{}||{}",
                    song.artist.to_lowercase(),
                    song.album.to_lowercase()
                );
                if (scope == "all" || scope == "album")
                    && !song.album.trim().is_empty()
                    && !seen_albums.contains(&album_key)
                {
                    seen_albums.insert(album_key);
                    out.push(ActionHit {
                        kind: "album".into(),
                        action_id: None,
                        song_id: None,
                        artist_name: Some(song.artist.clone()),
                        album_name: Some(song.album.clone()),
                        title: song.album.clone(),
                        subtitle: format!("Álbum • {}", song.artist),
                    });
                }

                if out.len() > 42 {
                    break;
                }
            }
        }

        out
    }

    pub fn apply_action_hit(&mut self, hit: &ActionHit) {
        match hit.kind.as_str() {
            "action" => {
                if let Some(id) = &hit.action_id {
                    self.apply_quick_action(id);
                }
            }
            "artist" => {
                if let Some(name) = &hit.artist_name {
                    self.set_section(Section::Artists);
                    self.selected_artist = Some(name.clone());
                    self.status = format!("Artista seleccionado: {name}");
                }
            }
            "song" => {
                if let Some(id) = &hit.song_id {
                    self.set_section(Section::Library);
                    if let Some(song) = self.store.songs.iter().find(|s| s.id == *id).cloned() {
                        let _ = self.play_song(&song);
                    }
                }
            }
            "album" => {
                if let Some(name) = &hit.album_name {
                    self.set_section(Section::Albums);
                    self.settings.search_query = name.clone();
                    self.status = format!("Álbum seleccionado: {name}");
                }
            }
            _ => {}
        }
    }

    pub fn apply_quick_action(&mut self, action_id: &str) {
        match action_id {
            "play" => {
                if self.player.state.playing && self.player.state.paused {
                    if self.player.toggle_pause().is_ok() {
                        self.status = "Reproducción reanudada".into();
                    }
                    return;
                }
                if self.player.state.playing {
                    self.status = "Ya está reproduciendo".into();
                    return;
                }
                self.play_selected_or_first();
            }
            "pause" => {
                if self.player.state.playing && !self.player.state.paused {
                    if self.player.toggle_pause().is_ok() {
                        self.status = "Reproducción pausada".into();
                    }
                } else {
                    self.status = "No hay reproducción activa para pausar".into();
                }
            }
            "next" => self.play_next(),
            "previous" => self.play_previous(),
            "stop" => {
                self.player.stop();
                self.status = "Reproducción detenida".into();
            }
            "settings" => {
                self.set_section(Section::Settings);
                self.status = "Ajustes".into();
            }
            "quit" => {
                self.status = "Saliendo de Musaik RS…".into();
                self.quit_requested = true;
                let _ = self.persist_all();
            }
            _ => {}
        }
    }

    pub fn play_selected_or_first(&mut self) {
        if let Some(id) = self.selected_song_id.clone() {
            if self.play_song_by_id(&id).is_ok() {
                return;
            }
        }
        if let Some(song) = self.store.songs.first().cloned() {
            let _ = self.play_song(&song);
        }
    }

    pub fn play_song_by_id(&mut self, song_id: &str) -> Result<()> {
        let song = self
            .store
            .songs
            .iter()
            .find(|s| s.id == song_id)
            .cloned()
            .ok_or_else(|| anyhow::anyhow!("song not found"))?;
        self.play_song(&song)
    }

    pub fn play_next(&mut self) {
        if self.store.songs.is_empty() {
            self.status = "Biblioteca vacía".into();
            return;
        }
        let Some(current_idx) = self.current_song_index() else {
            self.play_selected_or_first();
            return;
        };

        let next_idx = if self.settings.shuffle {
            self.random_song_index()
        } else if current_idx + 1 >= self.store.songs.len() {
            if self.settings.repeat {
                0
            } else {
                self.status = "Fin de la cola".into();
                return;
            }
        } else {
            current_idx + 1
        };

        if let Some(song) = self.store.songs.get(next_idx).cloned() {
            let _ = self.play_song(&song);
        }
    }

    pub fn play_previous(&mut self) {
        if self.store.songs.is_empty() {
            self.status = "Biblioteca vacía".into();
            return;
        }
        let Some(current_idx) = self.current_song_index() else {
            self.play_selected_or_first();
            return;
        };
        let prev_idx = if current_idx == 0 {
            if self.settings.repeat {
                self.store.songs.len().saturating_sub(1)
            } else {
                0
            }
        } else {
            current_idx - 1
        };
        if let Some(song) = self.store.songs.get(prev_idx).cloned() {
            let _ = self.play_song(&song);
        }
    }

    fn current_song_index(&self) -> Option<usize> {
        let id = self.selected_song_id.as_ref()?;
        self.store.songs.iter().position(|s| &s.id == id)
    }

    fn random_song_index(&self) -> usize {
        if self.store.songs.is_empty() {
            return 0;
        }
        let nanos = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|d| d.as_nanos() as usize)
            .unwrap_or(0);
        nanos % self.store.songs.len()
    }
}
