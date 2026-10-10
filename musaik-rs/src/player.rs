use std::process::{Child, Command, Stdio};
use std::time::Instant;

use anyhow::{Context, Result};

#[derive(Debug, Clone, Default)]
pub struct PlayerState {
    pub path: String,
    pub playing: bool,
    pub paused: bool,
    pub seek_base_secs: f64,
    pub started_at: Option<Instant>,
}

pub struct Player {
    child: Option<Child>,
    pub state: PlayerState,
}

impl Player {
    pub fn new() -> Self {
        Self {
            child: None,
            state: PlayerState::default(),
        }
    }

    pub fn play(&mut self, path: &str, offset_secs: f64) -> Result<()> {
        self.stop();
        let mut cmd = Command::new(find_ffplay()?);
        cmd.arg("-nodisp")
            .arg("-autoexit")
            .arg("-loglevel")
            .arg("error")
            .arg("-ss")
            .arg(format!("{offset_secs:.3}"))
            .arg(path)
            .stdin(Stdio::piped())
            .stdout(Stdio::null())
            .stderr(Stdio::null());
        let child = cmd.spawn().context("start ffplay")?;
        self.child = Some(child);
        self.state.path = path.to_string();
        self.state.playing = true;
        self.state.paused = false;
        self.state.seek_base_secs = offset_secs;
        self.state.started_at = Some(Instant::now());
        Ok(())
    }

    pub fn toggle_pause(&mut self) -> Result<()> {
        if !self.state.playing {
            anyhow::bail!("nothing is playing");
        }
        if self.state.paused {
            let path = self.state.path.clone();
            let offset = self.position_secs();
            self.play(&path, offset)?;
            self.state.paused = false;
            return Ok(());
        }
        self.state.seek_base_secs = self.position_secs();
        self.stop_process_keep_track();
        self.state.paused = true;
        Ok(())
    }

    pub fn seek_to(&mut self, position_secs: f64) -> Result<()> {
        if !self.state.playing {
            anyhow::bail!("nothing is playing");
        }
        let path = self.state.path.clone();
        self.play(&path, position_secs.max(0.0))
    }

    pub fn stop(&mut self) {
        if let Some(child) = &mut self.child {
            let _ = child.kill();
        }
        self.child = None;
        self.state = PlayerState::default();
    }

    pub fn position_secs(&self) -> f64 {
        if !self.state.playing {
            return 0.0;
        }
        if self.state.paused {
            return self.state.seek_base_secs;
        }
        self.state.seek_base_secs
            + self
                .state
                .started_at
                .map(|t| t.elapsed().as_secs_f64())
                .unwrap_or(0.0)
    }

    fn stop_process_keep_track(&mut self) {
        if let Some(child) = &mut self.child {
            let _ = child.kill();
        }
        self.child = None;
        self.state.started_at = None;
    }
}

fn find_ffplay() -> Result<String> {
    for c in [
        "ffplay",
        "/opt/homebrew/bin/ffplay",
        "/usr/local/bin/ffplay",
        "/opt/local/bin/ffplay",
    ] {
        if std::path::Path::new(c).exists() || which_like(c) {
            return Ok(c.to_string());
        }
    }
    anyhow::bail!("ffplay no está disponible (instala ffmpeg)")
}

fn which_like(bin: &str) -> bool {
    Command::new("which")
        .arg(bin)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .map(|s| s.success())
        .unwrap_or(false)
}
