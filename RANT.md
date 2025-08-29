## Problems with the standard subtitle API

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


## CORS nonsense (compatibility table)
|       `<video>` config        | Access-Control-Allow-Origin | AUDIO | VIDEO |
|:-----------------------------:|:---------------------------:|-------|-------|
|             none              |              *              | ✔️    | ✔️    |
|             none              |            none             | ✔️    | ✔️    |
|          audio gain           |              *              | ❌     | ✔️    |
|          audio gain           |            none             | ❌     | ✔️    |
| audio gain + `crossOrigin=""` |              *              | ✔️    | ✔️    |
| audio gain + `crossOrigin=""` |            none             | ❌     | ❌     |

On top of that there's no way to revert or undo the
[creation of MediaStreamAudioSourceNode](https://developer.mozilla.org/en-US/docs/Web/API/AudioContext/createMediaStreamSource)
from a `<video>` or `<audio>` element.<br>
In Firefox, if **AudioContext** is used and the `muted` property is set to `true`, `onplay` will not fire until playback is
programmatically resumed (for example, by calling [play()](https://developer.mozilla.org/en-US/docs/Web/API/HTMLMediaElement/play)).


## Other nonsense
 - [getDay()](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Date/getDay) doesn't return the day number but a 0-indexed week day
 - [getDate()](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Date/getDate) doesn't return the date but the day number (1-31)
 - there's no **unfocus()** method, instead there's [blur()](https://developer.mozilla.org/en-US/docs/Web/API/HTMLElement/blur)