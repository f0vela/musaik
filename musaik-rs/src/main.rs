mod app_state;
mod ipod;
mod lastfm;
mod library;
mod model;
mod player;
mod settings;

use gpui::*;
use gpui::StatefulInteractiveElement;

use app_state::{AppState, Section};

actions!(
    musaik_rs,
    [
        ToggleActionBar,
        ActionBarSelectNext,
        ActionBarSelectPrev,
        ActionBarRunSelected,
        ActionBarClose
    ]
);

struct RootView {
    state: AppState,
    focus_handle: FocusHandle,
    action_bar_open: bool,
    action_selected_idx: usize,
}

impl RootView {
    fn new(cx: &mut Context<Self>) -> Self {
        let state = AppState::load().unwrap_or_else(|err| AppState {
            store: library::Store::new(),
            settings: settings::AppSettings::default(),
            player: player::Player::new(),
            lastfm: lastfm::LastFmClient::new(
                String::new(),
                String::new(),
                String::new(),
                String::new(),
            ),
            section: app_state::Section::Explore,
            status: format!("Error cargando estado: {err}"),
            selected_song_id: None,
            selected_artist: None,
            action_query: String::new(),
            quit_requested: false,
        });
        Self {
            state,
            focus_handle: cx.focus_handle(),
            action_bar_open: false,
            action_selected_idx: 0,
        }
    }

    fn toggle_action_bar(
        &mut self,
        _: &ToggleActionBar,
        _window: &mut Window,
        _cx: &mut Context<Self>,
    ) {
        self.action_bar_open = !self.action_bar_open;
        if self.action_bar_open {
            self.action_selected_idx = 0;
            self.state.action_query.clear();
        }
    }

    fn action_bar_select_next(
        &mut self,
        _: &ActionBarSelectNext,
        _window: &mut Window,
        _cx: &mut Context<Self>,
    ) {
        if !self.action_bar_open {
            return;
        }
        let len = self.state.action_hits(&self.state.action_query).len();
        if len > 0 {
            self.action_selected_idx = (self.action_selected_idx + 1) % len;
        }
    }

    fn action_bar_select_prev(
        &mut self,
        _: &ActionBarSelectPrev,
        _window: &mut Window,
        _cx: &mut Context<Self>,
    ) {
        if !self.action_bar_open {
            return;
        }
        let len = self.state.action_hits(&self.state.action_query).len();
        if len > 0 {
            self.action_selected_idx = (self.action_selected_idx + len - 1) % len;
        }
    }

    fn action_bar_run_selected(
        &mut self,
        _: &ActionBarRunSelected,
        _window: &mut Window,
        _cx: &mut Context<Self>,
    ) {
        if !self.action_bar_open {
            return;
        }
        let hits = self.state.action_hits(&self.state.action_query);
        if let Some(hit) = hits.get(self.action_selected_idx).cloned() {
            self.state.apply_action_hit(&hit);
            self.action_bar_open = false;
        }
    }

    fn action_bar_close(
        &mut self,
        _: &ActionBarClose,
        _window: &mut Window,
        _cx: &mut Context<Self>,
    ) {
        self.action_bar_open = false;
    }
}

impl Render for RootView {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        if self.state.quit_requested {
            window.remove_window();
        }
        let songs = self.state.store.songs.len();
        let artists = self.state.store.artists().len();
        let albums = self.state.store.albums().len();
        let section = self.state.section.label();
        let query = self.state.settings.search_query.clone();
        let view_hint = match self.state.section {
            Section::Library => format!(
                "Librería: {} resultados",
                self.state.all_songs_filtered(&query).len()
            ),
            Section::Favorites => format!(
                "Favoritos: {} resultados",
                self.state.favorite_songs_filtered(&query).len()
            ),
            Section::Albums => format!(
                "Álbumes: {} representantes",
                self.state.album_representatives(&query, 24).len()
            ),
            Section::Artists => {
                if let Some(artist) = &self.state.selected_artist {
                    format!(
                        "Artista activo: {} ({} álbumes)",
                        artist,
                        self.state.artist_album_groups(artist).len()
                    )
                } else {
                    "Artistas: selecciona uno".into()
                }
            }
            _ => "Vista en construcción".into(),
        };
        let action_hits = self.state.action_hits(&self.state.action_query);
        if self.action_selected_idx >= action_hits.len() && !action_hits.is_empty() {
            self.action_selected_idx = action_hits.len() - 1;
        }
        let filtered_songs = self.state.all_songs_filtered(&query);
        let favorite_songs = self.state.favorite_songs_filtered(&query);
        let album_reps = self.state.album_representatives(&query, 30);
        let artists_list = self.state.store.artists();
        let current_song = self
            .state
            .selected_song_id
            .as_ref()
            .and_then(|id| self.state.store.songs.iter().find(|s| &s.id == id))
            .cloned();
        let selected_artist = self.state.selected_artist.clone();
        let selected_artist_groups = selected_artist
            .as_ref()
            .map(|a| self.state.artist_album_groups(a))
            .unwrap_or_default();

        let sidebar_item = |nav_id: usize, label: &'static str, section_key: Section| {
            let is_active = self.state.section == section_key;
            let bg = if is_active {
                gpui::rgb(0x2A2F3F)
            } else {
                gpui::rgb(0x1A1D26)
            };
            let fg = if is_active {
                gpui::rgb(0xF8FAFC)
            } else {
                gpui::rgb(0x94A3B8)
            };
            div()
                .id(("nav", nav_id))
                .px_3()
                .py_2()
                .rounded_md()
                .bg(bg)
                .text_color(fg)
                .child(label)
                .on_click(cx.listener(move |this, _, _, _| {
                    this.state.set_section(section_key);
                    this.state.status = format!("Sección: {}", section_key.label());
                }))
        };

        div()
            .track_focus(&self.focus_handle)
            .key_context("MusaikRS")
            .on_action(cx.listener(Self::toggle_action_bar))
            .on_action(cx.listener(Self::action_bar_select_next))
            .on_action(cx.listener(Self::action_bar_select_prev))
            .on_action(cx.listener(Self::action_bar_run_selected))
            .on_action(cx.listener(Self::action_bar_close))
            .relative()
            .size_full()
            .flex()
            .flex_col()
            .bg(gpui::rgb(0x121318))
            .p_4()
            .text_color(gpui::rgb(0xF8FAFC))
            .child(
                div()
                    .flex()
                    .justify_between()
                    .items_center()
                    .child(div().text_xl().child("Musaik RS (GPUI)"))
                    .child(
                        div()
                            .flex()
                            .gap_2()
                            .child(
                                div()
                                    .id("quick-play")
                                    .px_2()
                                    .py_1()
                                    .rounded_md()
                                    .bg(gpui::rgb(0x1E2430))
                                    .child("Play")
                                    .on_click(cx.listener(|this, _, _, _| {
                                        this.state.apply_quick_action("play");
                                    })),
                            )
                            .child(
                                div()
                                    .id("quick-pause")
                                    .px_2()
                                    .py_1()
                                    .rounded_md()
                                    .bg(gpui::rgb(0x1E2430))
                                    .child("Pause")
                                    .on_click(cx.listener(|this, _, _, _| {
                                        this.state.apply_quick_action("pause");
                                    })),
                            )
                            .child(
                                div()
                                    .id("quick-next")
                                    .px_2()
                                    .py_1()
                                    .rounded_md()
                                    .bg(gpui::rgb(0x1E2430))
                                    .child("Next")
                                    .on_click(cx.listener(|this, _, _, _| {
                                        this.state.apply_quick_action("next");
                                    })),
                            )
                            .child(
                                div()
                                    .id("quick-prev")
                                    .px_2()
                                    .py_1()
                                    .rounded_md()
                                    .bg(gpui::rgb(0x1E2430))
                                    .child("Prev")
                                    .on_click(cx.listener(|this, _, _, _| {
                                        this.state.apply_quick_action("previous");
                                    })),
                            ),
                    ),
            )
            .child(
                div()
                    .mt(px(8.))
                    .text_sm()
                    .text_color(gpui::rgb(0x94A3B8))
                    .child("Port separado del proyecto Go"),
            )
            .child(
                div()
                    .mt(px(12.))
                    .flex()
                    .gap_3()
                    .child(
                        div()
                            .w(px(210.))
                            .p_2()
                            .rounded_lg()
                            .bg(gpui::rgb(0x181C25))
                            .child(div().text_sm().text_color(gpui::rgb(0x94A3B8)).child("Navegación"))
                            .child(div().mt(px(8.)).child(sidebar_item(0, "Explorar", Section::Explore)))
                            .child(div().mt(px(4.)).child(sidebar_item(1, "Recientes", Section::Recent)))
                            .child(div().mt(px(4.)).child(sidebar_item(2, "Librería", Section::Library)))
                            .child(div().mt(px(4.)).child(sidebar_item(3, "Artistas", Section::Artists)))
                            .child(div().mt(px(4.)).child(sidebar_item(4, "Álbumes", Section::Albums)))
                            .child(div().mt(px(4.)).child(sidebar_item(5, "Favoritos", Section::Favorites)))
                            .child(div().mt(px(4.)).child(sidebar_item(6, "Playlists", Section::Playlists)))
                            .child(div().mt(px(4.)).child(sidebar_item(7, "Ajustes", Section::Settings))),
                    )
                    .child(
                        div()
                            .flex_1()
                            .p_3()
                            .rounded_lg()
                            .bg(gpui::rgb(0x161A22))
                            .child(div().text_lg().child(format!("{section}")))
                            .child(
                                div()
                                    .mt(px(6.))
                                    .text_sm()
                                    .text_color(gpui::rgb(0x94A3B8))
                                    .child(format!("Estado: {}", self.state.status)),
                            )
                            .child(
                                div()
                                    .mt(px(10.))
                                    .text_sm()
                                    .child(format!("Buscar: {}", query)),
                            )
                            .child(
                                div()
                                    .mt(px(6.))
                                    .flex()
                                    .gap_2()
                                    .child(
                                        div()
                                            .id("search-clear")
                                            .px_2()
                                            .py_1()
                                            .rounded_sm()
                                            .bg(gpui::rgb(0x273042))
                                            .child("Clear")
                                            .on_click(cx.listener(|this, _, _, _| {
                                                this.state.settings.search_query.clear();
                                                let _ = this.state.settings.save();
                                            })),
                                    )
                                    .child(
                                        div()
                                            .id("search-rock")
                                            .px_2()
                                            .py_1()
                                            .rounded_sm()
                                            .bg(gpui::rgb(0x273042))
                                            .child("Query: rock")
                                            .on_click(cx.listener(|this, _, _, _| {
                                                this.state.settings.search_query = "rock".to_string();
                                                let _ = this.state.settings.save();
                                            })),
                                    )
                                    .child(
                                        div()
                                            .id("search-jazz")
                                            .px_2()
                                            .py_1()
                                            .rounded_sm()
                                            .bg(gpui::rgb(0x273042))
                                            .child("Query: jazz")
                                            .on_click(cx.listener(|this, _, _, _| {
                                                this.state.settings.search_query = "jazz".to_string();
                                                let _ = this.state.settings.save();
                                            })),
                                    ),
                            )
                            .child(
                                div()
                                    .mt(px(4.))
                                    .text_sm()
                                    .child(format!("Canciones: {songs} · Artistas: {artists} · Álbumes: {albums}")),
                            )
                            .child(div().mt(px(8.)).text_sm().child(view_hint))
                            .child(
                                div()
                                    .mt(px(12.))
                                    .text_sm()
                                    .text_color(gpui::rgb(0xCBD5E1))
                                    .child("Canciones (primeras 18):"),
                            )
                            .child(if self.state.section == Section::Library {
                                div().children(
                                    filtered_songs
                                        .into_iter()
                                        .take(24)
                                        .enumerate()
                                        .map(|(idx, song)| {
                                            let song_for_play = song.clone();
                                            let song_for_like = song.clone();
                                            let is_fav = self.state.store.favorites.contains_key(&song.id);
                                            div()
                                                .mt(px(4.))
                                                .px_2()
                                                .py_2()
                                                .rounded_md()
                                                .bg(gpui::rgb(0x1E2430))
                                                .flex()
                                                .justify_between()
                                                .items_center()
                                                .child(
                                                    div()
                                                        .child(song.title)
                                                        .child(
                                                            div()
                                                                .text_xs()
                                                                .text_color(gpui::rgb(0x94A3B8))
                                                                .child(format!(
                                                                    "#{} · {} • {} · plays {}",
                                                                    song.track_num.unwrap_or(0),
                                                                    song.artist,
                                                                    song.album,
                                                                    song.play_count
                                                                )),
                                                        ),
                                                )
                                                .child(
                                                    div()
                                                        .flex()
                                                        .gap_2()
                                                        .child(
                                                            div()
                                                                .id(("song-play", idx))
                                                                .px_2()
                                                                .py_1()
                                                                .rounded_sm()
                                                                .bg(gpui::rgb(0x273042))
                                                                .child("▶")
                                                                .on_click(cx.listener(move |this, _, _, _| {
                                                                    let _ = this.state.play_song(&song_for_play);
                                                                })),
                                                        )
                                                        .child(
                                                            div()
                                                                .id(("song-like", idx))
                                                                .px_2()
                                                                .py_1()
                                                                .rounded_sm()
                                                                .bg(if is_fav { gpui::rgb(0x4B1F2A) } else { gpui::rgb(0x273042) })
                                                                .child(if is_fav { "♥" } else { "♡" })
                                                                .on_click(cx.listener(move |this, _, _, _| {
                                                                    this.state.toggle_like(&song_for_like);
                                                                })),
                                                        ),
                                                )
                                        })
                                        .collect::<Vec<_>>(),
                                )
                            } else {
                                div()
                            })
                            .child(if self.state.section == Section::Favorites {
                                div().children(
                                    favorite_songs
                                        .into_iter()
                                        .take(24)
                                        .enumerate()
                                        .map(|(idx, song)| {
                                            let song_for_play = song.clone();
                                            div()
                                                .mt(px(4.))
                                                .px_2()
                                                .py_2()
                                                .rounded_md()
                                                .bg(gpui::rgb(0x1E2430))
                                                .flex()
                                                .justify_between()
                                                .items_center()
                                                .child(
                                                    div()
                                                        .child(song.title)
                                                        .child(
                                                            div()
                                                                .text_xs()
                                                                .text_color(gpui::rgb(0x94A3B8))
                                                                .child(format!("{} • {}", song.artist, song.album)),
                                                        ),
                                                )
                                                .child(
                                                    div()
                                                        .id(("fav-play", idx))
                                                        .px_2()
                                                        .py_1()
                                                        .rounded_sm()
                                                        .bg(gpui::rgb(0x273042))
                                                        .child("▶")
                                                        .on_click(cx.listener(move |this, _, _, _| {
                                                            let _ = this.state.play_song(&song_for_play);
                                                        })),
                                                )
                                        })
                                        .collect::<Vec<_>>(),
                                )
                            } else {
                                div()
                            })
                            .child(if self.state.section == Section::Albums {
                                div().children(
                                    album_reps
                                        .into_iter()
                                        .take(24)
                                        .map(|song| {
                                            div()
                                                .mt(px(4.))
                                                .px_2()
                                                .py_2()
                                                .rounded_md()
                                                .bg(gpui::rgb(0x1E2430))
                                                .child(song.album)
                                                .child(
                                                    div()
                                                        .text_xs()
                                                        .text_color(gpui::rgb(0x94A3B8))
                                                        .child(song.artist),
                                                )
                                        })
                                        .collect::<Vec<_>>(),
                                )
                            } else {
                                div()
                            })
                            .child(if self.state.section == Section::Artists {
                                div()
                                    .mt(px(8.))
                                    .child(div().text_sm().child("Artistas"))
                                    .child(
                                        div().children(
                                            artists_list
                                                .iter()
                                                .take(18)
                                                .enumerate()
                                                .map(|(idx, artist)| {
                                                    let artist_name = artist.clone();
                                                    let active = selected_artist
                                                        .as_ref()
                                                        .map(|s| s == artist)
                                                        .unwrap_or(false);
                                                    div()
                                                        .id(("artist", idx))
                                                        .mt(px(3.))
                                                        .px_2()
                                                        .py_1()
                                                        .rounded_sm()
                                                        .bg(if active { gpui::rgb(0x3A2430) } else { gpui::rgb(0x1E2430) })
                                                        .child(artist_name.clone())
                                                        .on_click(cx.listener(move |this, _, _, _| {
                                                            this.state.selected_artist = Some(artist_name.clone());
                                                        }))
                                                })
                                                .collect::<Vec<_>>(),
                                        ),
                                    )
                                    .child(
                                        div()
                                            .mt(px(8.))
                                            .text_sm()
                                            .text_color(gpui::rgb(0xCBD5E1))
                                            .child("Álbumes del artista seleccionado"),
                                    )
                                    .child(
                                        div().children(
                                            selected_artist_groups
                                                .into_iter()
                                                .take(12)
                                                .map(|group| {
                                                    div()
                                                        .mt(px(4.))
                                                        .px_2()
                                                        .py_2()
                                                        .rounded_md()
                                                        .bg(gpui::rgb(0x1E2430))
                                                        .child(format!("{} ({} canciones)", group.album, group.songs.len()))
                                                        .child(
                                                            div()
                                                                .text_xs()
                                                                .text_color(gpui::rgb(0x94A3B8))
                                                                .child(
                                                                    group
                                                                        .songs
                                                                        .iter()
                                                                        .take(3)
                                                                        .map(|s| s.title.clone())
                                                                        .collect::<Vec<_>>()
                                                                        .join(" · "),
                                                                ),
                                                        )
                                                })
                                                .collect::<Vec<_>>(),
                                        ),
                                    )
                            } else {
                                div()
                            })
                            .child(if self.state.section == Section::Playlists {
                                div()
                                    .mt(px(8.))
                                    .text_sm()
                                    .text_color(gpui::rgb(0xCBD5E1))
                                    .child("Playlists")
                                    .child(
                                        div().children(
                                            self.state
                                                .store
                                                .playlists
                                                .iter()
                                                .map(|pl| {
                                                    let count = self.state.store.playlist_songs(pl).len();
                                                    div()
                                                        .mt(px(4.))
                                                        .px_2()
                                                        .py_2()
                                                        .rounded_md()
                                                        .bg(gpui::rgb(0x1E2430))
                                                        .child(format!("{} ({count})", pl.name))
                                                })
                                                .collect::<Vec<_>>(),
                                        ),
                                    )
                            } else {
                                div()
                            })
                            .child(if self.state.section == Section::Settings {
                                div()
                                    .mt(px(8.))
                                    .child(
                                        div()
                                            .id("settings-index")
                                            .px_2()
                                            .py_1()
                                            .rounded_sm()
                                            .bg(gpui::rgb(0x273042))
                                            .child("Indexar biblioteca")
                                            .on_click(cx.listener(|this, _, _, _| {
                                                if let Err(err) = this.state.index_library() {
                                                    this.state.status = format!("Error indexando: {err}");
                                                }
                                            })),
                                    )
                                    .child(
                                        div()
                                            .id("settings-save")
                                            .mt(px(4.))
                                            .px_2()
                                            .py_1()
                                            .rounded_sm()
                                            .bg(gpui::rgb(0x273042))
                                            .child("Guardar ajustes")
                                            .on_click(cx.listener(|this, _, _, _| {
                                                if let Err(err) = this.state.persist_all() {
                                                    this.state.status = format!("Error guardando: {err}");
                                                } else {
                                                    this.state.status = "Ajustes guardados".into();
                                                }
                                            })),
                                    )
                                    .child(
                                        div()
                                            .id("settings-ipod-detect")
                                            .mt(px(4.))
                                            .px_2()
                                            .py_1()
                                            .rounded_sm()
                                            .bg(gpui::rgb(0x273042))
                                            .child("Detectar iPod Classic")
                                            .on_click(cx.listener(|this, _, _, _| {
                                                match crate::ipod::detect_classic_mounts() {
                                                    Ok(devices) if !devices.is_empty() => {
                                                        this.state.settings.ipod_mount_path = devices[0].mount_path.to_string_lossy().to_string();
                                                        this.state.status = format!("iPod detectado: {}", devices[0].name);
                                                        let _ = this.state.settings.save();
                                                    }
                                                    Ok(_) => this.state.status = "No se detectó iPod en /Volumes".into(),
                                                    Err(err) => this.state.status = format!("Error detectando iPod: {err}"),
                                                }
                                            })),
                                    )
                                    .child(
                                        div()
                                            .id("settings-ipod-sync")
                                            .mt(px(4.))
                                            .px_2()
                                            .py_1()
                                            .rounded_sm()
                                            .bg(gpui::rgb(0x273042))
                                            .child("Sync playlist Todos a iPod")
                                            .on_click(cx.listener(|this, _, _, _| {
                                                let mount = std::path::PathBuf::from(this.state.settings.ipod_mount_path.clone());
                                                let songs = this.state.store.songs.clone();
                                                if songs.is_empty() {
                                                    this.state.status = "Biblioteca vacía".into();
                                                    return;
                                                }
                                                match crate::ipod::sync_playlist(&mount, "Todos", &songs) {
                                                    Ok((copied, m3u)) => {
                                                        this.state.status = format!("Sync iPod OK: {copied} copias, playlist {}", m3u.display());
                                                    }
                                                    Err(err) => this.state.status = format!("Error sync iPod: {err}"),
                                                }
                                            })),
                                    )
                            } else {
                                div()
                            })
                            .child(
                                div()
                                    .mt(px(14.))
                                    .text_sm()
                                    .text_color(gpui::rgb(0xCBD5E1))
                                    .child(format!("Action hits (Cmd+P base lógica): {}", action_hits.len())),
                            )
                            .child(
                                div().children(
                                    action_hits
                                        .iter()
                                        .take(12)
                                        .enumerate()
                                        .map(|(idx, hit)| {
                                            let hit_copy = hit.clone();
                                            div()
                                                .id(("action-hit", idx))
                                                .mt(px(3.))
                                                .px_2()
                                                .py_1()
                                                .rounded_sm()
                                                .bg(gpui::rgb(0x1E2430))
                                                .child(format!("{} — {}", hit.title, hit.subtitle))
                                                .on_click(cx.listener(move |this, _, _, _| {
                                                    this.state.apply_action_hit(&hit_copy);
                                                }))
                                        })
                                        .collect::<Vec<_>>(),
                                ),
                            ),
                    ),
            )
            .child(
                div()
                    .mt(px(18.))
                    .text_sm()
                    .text_color(gpui::rgb(0xCBD5E1))
                    .child("Base lista: modelos, store, settings, player shell, Last.fm, iPod y lógica de vistas/Action Bar."),
            )
    }
}

fn main() {
    Application::new().run(|cx: &mut App| {
        let bounds = Bounds::centered(None, gpui::size(px(1100.), px(780.)), cx);
        cx.open_window(
            WindowOptions {
                window_bounds: Some(WindowBounds::Windowed(bounds)),
                titlebar: Some(gpui::TitlebarOptions {
                    title: Some("Musaik RS".into()),
                    appears_transparent: true,
                    ..Default::default()
                }),
                ..Default::default()
            },
            |_window, cx| cx.new(|_cx| RootView::new()),
        )
        .expect("open window");
    });
}
