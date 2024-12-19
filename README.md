# Watch Locally
This project is under construction...

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
- creating a custom player which can distinguish between _user-initiated_ and _programmatic_ playback amongst other things


## Prerequisites
- Go version `1.21` (released 2023-08-08) or newer (supporting `slices`)
- Any browser supporting `ECMAScript6` (2015), preferably newer than 2020

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

## https - How to generate SSL keys
In order to secure incoming and outgoing traffic **TLS** is crucial
```bash
openssl req -newkey rsa:4096  -x509  -sha512  -days 365 -nodes -out certificate.pem -keyout privatekey.pem
```
Git comes with many preinstalled binaries among which is `openssl` <br>
On Windows it can be found at `Git/usr/bin/openssl.exe` where `Git` is git's root installation directory

Additionally, to have your domain verified you can use a free certificate authority like: https://letsencrypt.org

## Problems with the standard subtitle API
>It is terrible

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
    * no canonical method for adding `TextTrack` from url
<br><br>

* **No standardized approach to shifting causing inefficient solutions**
    * every cue must be shifted in a shift-dependent order otherwise, the cues are instantly reordered
    * some subtitle languages (with 1000+ cues) cause dramatic performance drops during shifting
    * cues on Firefox often stack (pile on top of each other) after shifting and stay on screen after end time

