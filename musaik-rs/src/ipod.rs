use std::fs;
use std::io::{BufWriter, Write};
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};

use crate::model::Song;

#[derive(Debug, Clone)]
pub struct Device {
    pub name: String,
    pub mount_path: PathBuf,
}

pub fn detect_classic_mounts() -> Result<Vec<Device>> {
    let volumes = Path::new("/Volumes");
    let mut out = Vec::new();
    for entry in fs::read_dir(volumes).context("read /Volumes")? {
        let entry = entry?;
        if !entry.file_type()?.is_dir() {
            continue;
        }
        let mount = entry.path();
        let control = mount.join("iPod_Control");
        let db = control.join("iTunes/iTunesDB");
        if control.is_dir() && db.is_file() {
            out.push(Device {
                name: entry.file_name().to_string_lossy().to_string(),
                mount_path: mount,
            });
        }
    }
    Ok(out)
}

pub fn sync_playlist(
    device_mount: &Path,
    playlist_name: &str,
    songs: &[Song],
) -> Result<(usize, PathBuf)> {
    let music_dir = device_mount.join("iPod_Control/Music/F00");
    let playlists_dir = device_mount.join("Playlists");
    fs::create_dir_all(&music_dir).context("create music dir")?;
    fs::create_dir_all(&playlists_dir).context("create playlists dir")?;

    let mut copied = 0;
    let mut lines = vec!["#EXTM3U".to_string()];

    for (i, song) in songs.iter().enumerate() {
        let ext = Path::new(&song.path)
            .extension()
            .and_then(|s| s.to_str())
            .unwrap_or("mp3");
        let name = safe_name(&format!(
            "{:03}-{}-{}.{}",
            i + 1,
            song.artist,
            song.title,
            ext
        ));
        let dst = music_dir.join(name);
        if copy_if_needed(Path::new(&song.path), &dst)? {
            copied += 1;
        }
        let rel = dst
            .strip_prefix(device_mount)
            .unwrap_or(&dst)
            .to_string_lossy()
            .replace('\\', "/");
        lines.push(rel);
    }

    let m3u = playlists_dir.join(format!("{}.m3u8", safe_name(playlist_name)));
    let f = fs::File::create(&m3u).context("create playlist file")?;
    let mut writer = BufWriter::new(f);
    for line in lines {
        writeln!(writer, "{line}")?;
    }
    Ok((copied, m3u))
}

fn copy_if_needed(src: &Path, dst: &Path) -> Result<bool> {
    let src_meta = fs::metadata(src).with_context(|| format!("stat source {}", src.display()))?;
    if dst.exists() {
        let dst_meta =
            fs::metadata(dst).with_context(|| format!("stat target {}", dst.display()))?;
        if dst_meta.len() == src_meta.len() {
            return Ok(false);
        }
    }
    fs::copy(src, dst).with_context(|| format!("copy {} -> {}", src.display(), dst.display()))?;
    Ok(true)
}

fn safe_name(input: &str) -> String {
    let mut s = input.trim().replace('/', "_").replace('\\', "_");
    for ch in [':', '*', '?', '"', '<', '>', '|'] {
        s = s.replace(ch, "_");
    }
    if s.len() > 90 {
        s.truncate(90);
    }
    if s.is_empty() {
        "untitled".to_string()
    } else {
        s
    }
}
