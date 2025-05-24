# Pocket Watch
![Webpage preview](https://github.com/user-attachments/assets/7bdedacf-1a9e-401b-8eda-bf7098ec9c6d)


## Preface
This is a watch party website. Is it like the others? No. <br>
There are many alternatives, yet the majority of them
suffers from the same recurring issues, such as:
- **poor performance** (laggy sliders, stuttery animations)
- **poor design choices** or silly limitations
- **plenty of bugs** and issues which, if reported, are usually ignored, swept aside and never resolved
- **no backwards support** (nowadays web developers can barely support the latest browser release)
- **chromium only** (because other browser engines don't exist) 
- **slow backends** written in JS or other scripting languages
- **little to no support for mobile devices** (no progress bar, no subtitles, extension-based)
- glaring synchronization issues across multiple clients
- lack of server-side support for bypassing CORS

## The goals of this project
- open-source
- cross-browser compatibility
- cross-device compatibility (quality experience for mobile users, hence the name - pocket)
- compatibility with older browsers (4 years back)
- minimal dependencies
- no JS frameworks
- avoiding needlessly complicated or bloated code (let's keep it sane)
- fighting around browser-specific quirks (lack of standardized slider customization, cues stacking)

## Prerequisites
- Go version `1.22` (released in Feb 2024) or newer (supporting `slices` and `sync.Map`)
- Any major browser version released in 2021 or later (due to CSS features)

## Components
- [hls](https://github.com/video-dev/hls.js)
- [pocket-picker](https://github.com/Pocket-Watch/PocketPicker)
- [subtitle-downloader](https://github.com/friskisgit/subtitle-downloader)
- [pocket-yt](https://github.com/Pocket-Watch/PocketYT)
- pocket-player (integrated, needs a repository)

## Running
Adjust the build script corresponding to your platform by setting `-ip` and `-port` arguments. Then execute it:
<br><br>
**Windows**
```bash
build.bat
```
**Linux**
```bash
./build.sh
```

## PostgreSQL database installation & setup [OPTIONAL]:
1. Install PostgreSQL
 - Via an installer:
   - installers for Windows & MacOS can be found at [enterprisedb](https://www.enterprisedb.com/downloads/postgres-postgresql-downloads).
   - for other systems navigate to [postgresql](https://www.postgresql.org/download) and select your platform.

 - Manually:
   - download binaries: https://www.enterprisedb.com/download-postgresql-binaries
   - ensure `postgres` is in PATH, if not add `.../PostgreSQL/bin` directory to PATH
2. Run setup script [setup_database.bat](scripts/setup_database.bat) or [setup_database.sh](scripts/setup_database.sh)
3. Toggle database with [toggle_database.bat](scripts/toggle_database.bat) or [toggle_database.sh](scripts/toggle_database.sh)

## https - How to generate SSL keys
In order to secure incoming and outgoing traffic **TLS** is crucial
```bash
openssl req -newkey rsa:4096  -x509  -sha512  -days 365 -nodes -out certificate.pem -keyout privatekey.pem
```
Git comes with many preinstalled binaries among which is `openssl` <br>
On Windows it can be found at `Git/usr/bin/openssl.exe` where `Git` is git's root installation directory

Additionally, to have your domain verified you can use a free certificate authority like: https://letsencrypt.org

## Custom player
For a watch party to work smoothly it's crucial for the player to be able to distinguish
between **user-initiated** and **programmatic** playback. Unfortunately the `<video>` element
does not provide a mechanism to accomplish that and player libraries usually don't provide a callback.
The most common approach is to try and control the state between events, such as `onclick` and `onplay`. Due to 
synchronization issues between clients and the single-threaded nature of JS it eventually leads to
a disorganized codebase. Furthermore, most video players dispatch events by firing events asynchronously, causing
them to be out of order. <br>
Proper subtitle support is scarce:
- no dynamic loading (only static)
- no customization menus
- no support for SRT (in-built)
- no support for shifting

The default web subtitle API doesn't shine either.
See [WEBVTT API problems](#problems-with-the-standard-subtitle-api).
<br>

### Prerequisites
Include player stylesheet - [player.css](web/css/player.css)
```html
<link rel="stylesheet" href="web/css/player.css">
```

Include player icons svg - [player_icons.svg](web/svg/player_icons.svg)
```js
let options = new Options();
// The path can be changed in options but it must point to the svg file
options.iconsPath = "resources/pocket_icons.svg"
```

Optionally preload player script - [custom_player.js](web/js/custom_player.js)
```html
<body>
    // Webpage elements
    <script type="module" src="web/js/custom_player.js"></script>
</body>
```
Import the symbols in Javascript to use them directly
```js
import {Player, Options} from "./js/custom_player.js";
```

### Integrating into a web extension
If you run into security errors related to loading data, expose the resources in `manifest.json`:
```json
{
  "web_accessible_resources": [
    "player_resources/player_icons.svg"
  ]
}
```
Prepare and load extension resources:
```js
// https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/API/runtime/getURL
// For simplicity it's assumed all resources are in one directory
let resources = browser.runtime.getURL("player_resources");

let module = await import(resources + "player.js");
// Access module.Player, module.Options
let playerCssHref = resources + "player.css";
// Add the stylesheet dynamically to a webpage

let options = new module.Options();
options.iconsPath = resources + "player_icons.svg";
```

### API usage examples
Initialize and attach the player to any video element
```js
let videoElement = document.getElementById("cats-video");
let options = new Options();
// Hide the buttons you don't need
options.hideNextButton = true;
options.hideDownloadButton = true;
// Change seeking to 10s
options.seekBy = 10;
// Passing options is optional
let player = new Player(videoElement, options);
```

Set title, video track and subtitle
```js
player.setTitle("Agent327");
player.setVideoTrack("https://video.blender.org/static/web-videos/264ff760-803e-430e-8d81-15648e904183-720.mp4")
player.setSubtitle("subtitles/Agent327.vtt")
```
Instant callbacks:
```js
// They're dispatched synchronously
player.onControlsPlay(() => {
    player.setToast("You clicked play.");
})

player.onControlsPause(() => {
    player.setToast("You clicked pause.");
})

player.onControlsSeeked((timestamp) => {
    player.setToast("You seeked to " + timestamp.toFixed(3));
})
// Toasts appear in the top right corner as text, also in fullscreen
```

### Custom subtitle implementation
- support for parsing `SRT` and `VTT`
- proper cue content sanitization with `DOMParser`
- addressed bottlenecks related to shifting, setting and parsing
- ability to import subtitle files from disk

```js
// Adds a subtitle and doesn't show it
player.addSubtitle("subtitles/Subtitle0.srt");
// Sets a subtitle (shows it)
player.setSubtitle("subtitles/Subtitle1.vtt");
// Shows the subtitle at the given index
player.enableSubtitleTrackAt(0);
// Shifts the currently selected subtitle (either backwards or forwards) by the given amount of seconds 
player.shiftCurrentSubtitleTrackBy(-5);
```

## Problems with the standard subtitle API
>It is terrible

* **The performance of hiding or showing a track is astonishingly horrible in Firefox**
    * hiding a track with `3000 cues` on a modern CPU takes about `3000 ms` causing UI to be unresponsive
    * when a track is shown it takes roughly the same amount of time it took to hide it
    * adding more tracks only worsens the performance, which also progressively slows down VTT load times
<br><br>

* **Inconsistent styling across browsers**
    * bouncy `VTTCue.line` setting on Firefox (does bound checks, ensuring cue stays within view)
    * changing `style.fontSize` in CSS rule may easily cause subtitles to go out of view on Firefox
<br><br>

* **Confusing and poorly designed API**
    * no ability to set a track, instead you control `TextTrack.mode` for each track separately
    * a CSS stylesheet must be used for styling `::cue` (not ideal for dynamic use cases)
    * dysfunctional or misnamed properties like:
        * `VTTCue.vertical` - represents the cue's writing direction, (could be `writingDirection`?)
        * `VTTCue.line` - in reality represents the vertical position of a cue
        * `VTTCue.snapToLines` - where `false` causes `VTTCue.line` to be interpreted as a % of the video size.
        * `VTTCue.size` - size as a % of the video size (yet it does not change the font size)
    * `video.addTextTrack` method must be used in Chromium otherwise, manually adding cues will have no effect
    * no canonical method for adding `TextTrack` from url, `<track>` element must be created and appended to `<video>`
    * `textTracks` don't maintain order if a track is appended internally (index corresponds to its position as a `<track>`)
<br><br>

* **No standardized approach to shifting causing inefficient solutions**
    * every cue must be shifted in a shift-dependent order otherwise, the cues are instantly reordered
    * some subtitle languages (with 1000+ cues) cause dramatic performance drops during shifting
    * cues on Firefox often stack (pile on top of each other) after shifting and stay on screen after end time

## iPhone's fullscreen and video controls nonsense
- Safari on iOS is the only platform which does not fully support Fullscreen API <br>
  (https://developer.mozilla.org/en-US/docs/Web/API/Fullscreen_API#browser_compatibility)

- on iPhone when a video is played it forces you into fullscreen mode on the `<video>` element <br>
  (https://discussions.apple.com/thread/251266057), <br>
  fortunately since iOS 10 `playsinline` attribute is supported which disables that behavior

- on iPhone the native player is always forced regardless of browser,<br>
  as a result custom controls don't appear in fullscreen and action events cannot be received

## `<video>` element error differences
| CHROMIUM                          | FIREFOX                |
|-----------------------------------|------------------------|
| MEDIA_ELEMENT_ERROR: Format error | 404: Not found         |
| Empty src attribute               | NS_ERROR_DOM_INVALID   |
| DEMUXER_ERROR_COULD_NOT_OPEN      | Failed to init decoder |
