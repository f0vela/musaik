---
name: Sonora macOS Audio Player
colors:
  surface: '#121318'
  surface-dim: '#121318'
  surface-bright: '#38393f'
  surface-container-lowest: '#0d0e13'
  surface-container-low: '#1a1b21'
  surface-container: '#1e1f25'
  surface-container-high: '#292a2f'
  surface-container-highest: '#34343a'
  on-surface: '#e3e1e9'
  on-surface-variant: '#e3bdbf'
  inverse-surface: '#e3e1e9'
  inverse-on-surface: '#2f3036'
  outline: '#aa888a'
  outline-variant: '#5b4041'
  surface-tint: '#ffb2b7'
  primary: '#ffb2b7'
  on-primary: '#67001b'
  primary-container: '#ff516a'
  on-primary-container: '#5b0017'
  inverse-primary: '#bc0b3b'
  secondary: '#d0bcff'
  on-secondary: '#3c0091'
  secondary-container: '#571bc1'
  on-secondary-container: '#c4abff'
  tertiary: '#4cd7f6'
  on-tertiary: '#003640'
  tertiary-container: '#009eb9'
  on-tertiary-container: '#002f38'
  error: '#ffb4ab'
  on-error: '#690005'
  error-container: '#93000a'
  on-error-container: '#ffdad6'
  primary-fixed: '#ffdadb'
  primary-fixed-dim: '#ffb2b7'
  on-primary-fixed: '#40000d'
  on-primary-fixed-variant: '#92002a'
  secondary-fixed: '#e9ddff'
  secondary-fixed-dim: '#d0bcff'
  on-secondary-fixed: '#23005c'
  on-secondary-fixed-variant: '#5516be'
  tertiary-fixed: '#acedff'
  tertiary-fixed-dim: '#4cd7f6'
  on-tertiary-fixed: '#001f26'
  on-tertiary-fixed-variant: '#004e5c'
  background: '#121318'
  on-background: '#e3e1e9'
  surface-variant: '#34343a'
typography:
  display-lg:
    fontFamily: Inter
    fontSize: 32px
    fontWeight: '700'
    lineHeight: 38px
    letterSpacing: -0.025em
  display-md:
    fontFamily: Inter
    fontSize: 26px
    fontWeight: '700'
    lineHeight: 32px
    letterSpacing: -0.02em
  headline-lg:
    fontFamily: Inter
    fontSize: 20px
    fontWeight: '600'
    lineHeight: 26px
    letterSpacing: -0.015em
  headline-md:
    fontFamily: Inter
    fontSize: 16px
    fontWeight: '600'
    lineHeight: 22px
    letterSpacing: -0.01em
  title-sm:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: '600'
    lineHeight: 18px
    letterSpacing: -0.005em
  body-lg:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: '400'
    lineHeight: 20px
    letterSpacing: 0em
  body-md:
    fontFamily: Inter
    fontSize: 13px
    fontWeight: '400'
    lineHeight: 18px
    letterSpacing: 0em
  label-md:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: '500'
    lineHeight: 16px
    letterSpacing: 0.01em
  label-sm:
    fontFamily: Inter
    fontSize: 11px
    fontWeight: '500'
    lineHeight: 14px
    letterSpacing: 0.02em
  mono-badge:
    fontFamily: JetBrains Mono
    fontSize: 10px
    fontWeight: '600'
    lineHeight: 12px
    letterSpacing: 0.05em
  mono-timer:
    fontFamily: JetBrains Mono
    fontSize: 11px
    fontWeight: '400'
    lineHeight: 14px
    letterSpacing: 0em
rounded:
  sm: 0.25rem
  DEFAULT: 0.5rem
  md: 0.75rem
  lg: 1rem
  xl: 1.5rem
  full: 9999px
spacing:
  gutter: 1.25rem
  gutter-compact: 0.75rem
  margin: 1.5rem
  margin-window: 1.25rem
  space-xs: 0.25rem
  space-sm: 0.5rem
  space-md: 0.875rem
  space-lg: 1.25rem
  space-xl: 2rem
---

## Brand & Style

The design system embodies modern macOS craft: native vibrancy, surgical precision, and deep acoustic focus. Tailored for audiophiles, digital collectors, and discerning macOS power users, it prioritizes the tactile emotional intimacy of personal media libraries over web-like streaming interfaces.

The design philosophy combines **macOS Sequoia-style Material Translucency (Vibrancy & Frosted Glass)** with **Precision Dark-Mode Minimalism**:
- Surfaces employ multi-tiered Apple Liquid Glass materials with native backdrop blurs (`NSVisualEffectView` blending modes) rather than artificial flat opacities.
- Album artwork commands center stage; ambient UI elements dynamically inherit chromatic tints derived from prominent album hues while anchoring to deep, noise-free slate neutrals.
- Visual elements emphasize desktop craftsmanship: hair-thin 0.5pt inner borders, soft inset specular highlights, native window traffic light controls, and fluid media playback surfaces.

## Colors

The palette is engineered exclusively for an immersive, deep dark-mode desktop environment.

### Core Semantic Tokens
- **Canvas Base (`#0C0D12`)**: The structural floor beneath translucent materials. Deep graphite tinted with a faint cobalt coolness to eliminate pure black OLED harshness.
- **Sidebar Material (`rgba(22, 24, 34, 0.72)`)**: Translucent frosted blur surface with `backdrop-filter: blur(40px) saturate(190%)`.
- **Canvas View (`rgba(16, 18, 25, 0.85)`)**: The primary content pane layer, providing depth distinction against the sidebar.
- **Chrome Unified Toolbar (`rgba(28, 30, 42, 0.65)`)**: Top-level window header integrating window controls and track navigation.
- **Primary Accent (`#F43F5E` - Electric Rose)**: Default system tint for playback scrubbers, active states, and volume indicators. When playing music, this token dynamically adapts to the dominant vibrant extraction from the active album cover art.
- **Secondary Accent (`#8B5CF6` - Deep Amethyst)**: Used for playlist highlights, secondary indicators, and multi-track selections.
- **Tertiary Accent (`#06B6D4` - Cyan Stream)**: Reserved strictly for high-fidelity hardware markers: **Lossless**, **DSD**, and **FLAC 24-bit / 192kHz** indicators.
- **Inner Rim Stroke (`rgba(255, 255, 255, 0.08)`)**: 0.5pt to 1pt hairline border applied to windows, popovers, and elevated album tiles to mimic light refraction on glass edges.
- **Text & Foregrounds**:
  - `text-primary`: `#F8FAFC` (Pure optical clarity)
  - `text-secondary`: `rgba(248, 250, 252, 0.64)` (Subtitles, artists, bitrates)
  - `text-tertiary`: `rgba(248, 250, 252, 0.38)` (Timestamps, disabled actions, track indexes)

## Typography

Typography prioritizes high-legibility system grotesk mechanics (`Inter` serving as the multi-platform analog for native macOS SF Pro), accented with monospaced precision numerals for acoustic telemetries.

### Hierarchy Guidelines
- **Track Numbers & Timers**: Use `mono-timer` featuring tabular figures (`font-variant-numeric: tabular-nums`) so scrubbers and running track durations do not cause visual horizontal jitter.
- **Audio Specs Badges**: Render lossless encodings (e.g., `FLAC 192kHz`, `ALAC 24-bit`, `DSD64`) exclusively in `mono-badge` with uppercase transformation and slight tracking expansion.
- **Album & Artist Display**: `display-lg` and `display-md` enforce tight tracking to echo Apple HIG desktop titling, retaining crispness across Retina displays.

## Layout & Spacing

The architecture utilizes a strict macOS three-column desktop grid with structural constraints:
- **Unified Window Structure**:
  - **Sidebar Pane**: Collapsible, width constrained between `220px` and `280px`. Contains native window traffic light clearance (`height: 52px`, `padding-left: 20px`), library categories, and user crates.
  - **Main Canvas View**: Fluid grid spanning flexible widths. Employs a multi-column album grid scaling from 3 to 6 columns depending on the macOS window frame dimension.
  - **Inspector / Queue Panel (Right)**: Optional sliding panel (`280px` fixed width) displaying lyrics, signal path, and technical audio chain information.
  - **Bottom Docked Player Bar**: Fixed `76px` height extending edge-to-edge with continuous vibrancy material blur.

### Breakpoints & Window Reflow Rules
- **Compact Window (`< 900px`)**: Collapse the right inspector panel into a popover; sidebar automatically collapses into an overlay drawer.
- **Standard Window (`900px – 1380px`)**: Two-column layout (Sidebar + Main Content Grid); 4-column album card arrangement with `gutter: 1.25rem`.
- **Expanded Window (`> 1380px`)**: Three-column fully open workspace (Sidebar + 5/6-column Album Grid + Queue/Lyrics Inspector).

## Elevation & Depth

Depth in this design system is established through optical translucency, light scattering, and multi-tier glass materials rather than dense opaque shadows:

1. **Window Background & Base**: Level 0 (`#0C0D12` solid foundation).
2. **Sidebar & Window Chrome**: Level 1 Vibrancy (`rgba(22, 24, 34, 0.72)` with `backdrop-filter: blur(40px) saturate(190%)`). Inset 0.5pt right border (`rgba(255, 255, 255, 0.06)`).
3. **Album Artwork & Content Containers**: Level 2. Album artwork floats with an ambient shadow tinted with the artwork's own spectrum: `box-shadow: 0 12px 32px -4px rgba(0, 0, 0, 0.5), 0 4px 12px 0 rgba(var(--artwork-accent-rgb), 0.18)`.
4. **Docked Player Bar & Modals**: Level 3 Frosted Surface (`rgba(26, 29, 41, 0.82)` with `backdrop-filter: blur(50px)`). Framed with a 1px top stroke: `border-top: 1px solid rgba(255, 255, 255, 0.08)`.
5. **Popovers, Volume Sliders & Context Menus**: Level 4 Top Layer (`rgba(35, 38, 54, 0.94)` with `backdrop-filter: blur(30px)`, `box-shadow: 0 16px 40px rgba(0, 0, 0, 0.65)`, and a complete outer 0.5pt rim `rgba(255, 255, 255, 0.12)`).

## Shapes

The design system implements macOS continuous squircle curvature (Apple smooth corner smoothing at 60% curvature):
- **Base Rounding (`roundedness: 2`, `8px / 0.5rem`)**: Used for playlist table rows, sidebar menu highlights, search input bars, and badge surfaces.
- **Cards & Covers (`rounded-lg`, `12px – 16px`)**: High-fidelity album art is crowned with a `12px` border-radius to preserve cover art proportion without clipping critical typography or sleeve margins.
- **Transport Controls & Floating Action Elements**: Circular pills (`rounded-full` / `9999px`) reserved strictly for the primary Play/Pause transport toggle, scrubber thumbs, and dynamic status tags.
- **Window Corners**: Window frame implements native macOS standard `10px` squircle corners with 0.5pt outer bezel clip.

## Components

### Window Chrome & Traffic Lights
- Unified toolbar height: `52px`.
- Traffic light buttons (Close `#FF5F56`, Minimize `#FFBD2E`, Fullscreen `#27C93F`) anchored with `margin-left: 18px` and centered vertically at `y: 26px`.
- Drag region active across all empty toolbar spaces via `-webkit-app-region: drag`.

### Transport Media Bar
- **Play/Pause Button**: Central circular accent button (`44px` diameter). Filled with the dynamic primary color or solid white, containing a centered glyph in dark canvas slate. Supports a micro-spring scale interaction (`scale(0.96)` on press).
- **Secondary Controls (Shuffle, Previous, Next, Repeat)**: `28px` hit areas with high-contrast monochrome icons. Hover state introduces a circular glass background: `rgba(255, 255, 255, 0.08)`.
- **Scrubber Bar**: `4px` baseline height, expanding to `6px` on track hover. Background track: `rgba(255, 255, 255, 0.12)`. Progress fill uses accent color with a subtle radial glow at the playhead thumb.

### Album Art Cards
- Ratio: Strict 1:1 square.
- Surface: Rounded `12px` corners, overlaid with an inner 0.5pt stroke `inset 0 0 0 1px rgba(255, 255, 255, 0.08)` to prevent edge bleed on dark backdrops.
- Hover State: Elevates by `-3px` with instant display of a floating quick-play button and dynamic drop-shadow enhancement.

### Audio Specification Badges (Lossless / Hi-Res)
- Small structural capsules (`height: 18px`, `padding: 0 6px`, `border-radius: 4px`).
- Background: `rgba(6, 182, 212, 0.12)` with a subtle cyan border `rgba(6, 182, 212, 0.28)`.
- Label: Monospace 10px uppercase (`text-tertiary-color` or `#22D3EE`). Examples: `HI-RES LOSSLESS`, `FLAC 96/24`, `DSD 2.8MHz`.

### Track List Rows
- Height: `36px` compact, `48px` standard (with thumbnail preview).
- Default: Transparent background.
- Hover: Subtle vibrancy wash `rgba(255, 255, 255, 0.05)`.
- Selected / Playing: `rgba(244, 63, 94, 0.15)` with track number replaced by an animated 3-bar equalizer glyph moving in the active accent color.

### Form Inputs & Search Field
- Background: `rgba(255, 255, 255, 0.06)` with inset border `rgba(255, 255, 255, 0.08)`.
- Height: `28px` macOS-native field standard.
- Integrated magnifying glass icon (`14px`) positioned with `8px` leading offset and placeholder text in `text-tertiary`.