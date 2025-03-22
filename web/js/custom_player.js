export { Player, Options };

class Player {
    constructor(videoElement, options) {
        if (!videoElement || videoElement.tagName.toLowerCase() !== "video") {
            throw new Error("An invalid video element was passed!");
        }

        if (!(options instanceof Options) || !options.valid()) {
            options = new Options();
        }
        this.internals = new Internals(videoElement, options);
    }

    isPlaying() {
        return this.internals.isVideoPlaying();
    }

    play() {
        this.internals.play();
    }

    pause() {
        this.internals.pause();
    }

    seek(timestamp) {
        this.internals.seek(timestamp);
    }

    setVolume(volume) {
        this.internals.setVolume(volume);
    }

    setTitle(title) {
        this.internals.setTitle(title);
    }

    setPoster(url) {
        this.internals.setPoster(url);
    }

    setSpeed(speed) {
        this.internals.setSpeed(speed);
    }

    setToast(toast) {
        this.internals.setToast(toast);
    }

    isLive() {
        return this.internals.isLive;
    }

    getCurrentTime() {
        return this.internals.getCurrentTime();
    }

    getDuration() {
        return this.internals.htmlVideo.duration;
    }

    setSubtitle(url, name, shift = 0.0) {
        let info = FileInfo.fromUrl(url)
        if (name) {
            info.filename = name;
        }

        return this.internals.addSubtitle(url, true, info, shift);
    }

    addSubtitle(url, name, shift = 0.0) {
        let info = FileInfo.fromUrl(url)
        if (name) {
            info.filename = name;
        }

        return this.internals.addSubtitle(url, false, info, shift);
    }

    // Tests each subtitle with the predicate function. Returns true on first match.
    anySubtitleMatch(predicateFunc) {
        if (isFunction(predicateFunc)) {
            return this.internals.anySubtitleMatch(predicateFunc)
        }
        return false;
    }

    // Disables and removes the track at the specified index.
    removeSubtitleTrackAt(index) {
        if (index < 0 || index >= this.internals.subtitles.length) {
            return;
        }
        this.internals.removeSubtitleTrackAt(index);
    }

    // Disables and removes the track identified by the given URL.
    removeSubtitleByUrl(url) {
        this.internals.removeSubtitleByUrl(url);
    }

    clearAllSubtitleTracks() {
        this.internals.clearAllSubtitleTracks();
    }

    setSubtitleFontSize(fontSize) {
        this.internals.setSubtitleFontSize(fontSize);
    }

    setSubtitleVerticalPosition(percentage) {
        this.internals.setSubtitleVerticalPosition(percentage);
    }

    setSubtitleForegroundColor(hexColor) {
        this.internals.setSubtitleForegroundColor(hexColor);
    }

    setSubtitleForegroundOpacity(opacity) {
        this.internals.setSubtitleForegroundOpacity(opacity);
    }

    setSubtitleBackgroundColor(hexColor) {
        this.internals.setSubtitleBackgroundColor(hexColor);
    }

    setSubtitleBackgroundOpacity(opacity) {
        this.internals.setSubtitleBackgroundOpacity(opacity);
    }

    enableSubtitles() {
        this.internals.enableSubtitles();
    }

    // Enable and show the track at the specified index.
    enableSubtitleTrackAt(index) {
        let subtitle = this.internals.subtitles[index];
        this.internals.enableSubtitleTrack(subtitle);
    }

    switchSubtitleTrackByUrl(url) {
        let subs = this.internals.subtitles;
        for (let i = 0; i < subs.length; i++) {
            let sub = subs[i];
            if (sub && sub.url === url) {
                this.internals.switchSubtitleTrack(sub);
                break;
            }
        }
    }

    // The seconds argument is a double, negative shifts back, positive shifts forward
    shiftCurrentSubtitleTrackBy(seconds) {
        this.internals.shiftCurrentSubtitleTrackBy(seconds);
    }

    setCurrentSubtitleShift(seconds) {
        this.internals.setCurrentSubtitleShift(seconds);
    }

    setSubtitleShiftByUrl(url, seconds) {
        this.internals.setSubtitleShiftByUrl(url, seconds);
    }

    getCurrentSubtitleShift() {
        return this.internals.getCurrentSubtitleShift();
    }

    getSubtitleShiftByUrl(url) {
        return this.internals.getSubtitleShiftByUrl(url);
    }

    destroyPlayer() {
        if (!this.internals) {
            return;
        }
        this.internals.destroyPlayer();
        this.internals = null;
    }

    onControlsPlay(func) {
        if (isFunction(func)) {
            this.internals.fireControlsPlay = func;
        }
    }

    onControlsPause(func) {
        if (isFunction(func)) {
            this.internals.fireControlsPause = func;
        }
    }

    onControlsNext(func) {
        if (isFunction(func)) {
            this.internals.fireControlsNext = func;
        }
    }

    onFullscreenChange(func) {
        if (isFunction(func)) {
            this.internals.fireFullscreenChange = func;
        }
    }

    onControlsSeeking(func) {
        if (isFunction(func)) {
            this.internals.fireControlsSeeking = func;
        }
    }

    onControlsSeeked(func) {
        if (isFunction(func)) {
            this.internals.fireControlsSeeked = func;
        }
    }

    onControlsVolumeSet(func) {
        if (isFunction(func)) {
            this.internals.fireControlsVolumeSet = func;
        }
    }

    onSettingsChange(func) {
        if (isFunction(func)) {
            this.internals.fireSettingsChange = func;
        }
    }

    onPlaybackError(func) {
        if (isFunction(func)) {
            this.internals.firePlaybackError = func;
        }
    }

    onPlaybackEnd(func) {
        if (isFunction(func)) {
            this.internals.firePlaybackEnd = func;
        }
    }

    onSubtitleTrackLoad(func) {
        if (isFunction(func)) {
            this.internals.fireSubtitleTrackLoad = func;
        }
    }

    onSubtitleSelect(func) {
        if (isFunction(func)) {
            this.internals.fireSubtitleSelect = func;
        }
    }

    onSubtitleSearch(func) {
        if (isFunction(func)) {
            this.internals.fireSubtitleSearch = func;
        }
    }

    setVideoTrack(url) {
        this.internals.setVideoTrack(url);
    }

    getCurrentUrl() {
        return this.internals.getCurrentUrl();
    }

    discardPlayback() {
        return this.internals.discardPlayback();
    }
}

function hide(element) {
    element.style.display = "none";
}

function show(element) {
    element.style.display = "";
}

function isHidden(element) {
    return element.style.display === "none";
}

const DEFAULT_SUBTITLE_BACKGROUND_COLOR   = "#1d2021"
const DEFAULT_SUBTITLE_BACKGROUND_OPACITY = 80;
const DEFAULT_SUBTITLE_FOREGROUND_COLOR   = "#fbf1c7"
const DEFAULT_SUBTITLE_FOREGROUND_OPACITY = 100;

class Internals {
    constructor(videoElement, options) {
        let initStart = performance.now();
        this.options = options;

        this.hls = null;
        this.playingHls = false;
        this.isLive = false;

        this.htmlVideo = videoElement;
        this.htmlVideo.disablePictureInPicture = true;
        this.htmlVideo.controls = false;
        // Prevents auto-fullscreen on iPhone but perhaps this could render entering fullscreen impossible?
        this.htmlVideo.playsinline = true;

        // Prevents selecting the video element along with the rest of the page
        this.htmlVideo.classList.add("unselectable");

        // Div container where either the player or the placeholder resides.
        this.htmlPlayerRoot = newDiv("player_container");

        // We actually need to append the <div> to document.body (or <video>'s parent)
        // otherwise the <video> tag will disappear entirely!
        let videoParent = this.htmlVideo.parentNode;
        videoParent.insertBefore(this.htmlPlayerRoot, this.htmlVideo);
        this.htmlPlayerRoot.appendChild(this.htmlVideo);

        this.htmlTitleContainer = newDiv("player_title_container");
        hide(this.htmlTitleContainer);
        this.htmlPlayerRoot.appendChild(this.htmlTitleContainer);

        this.htmlTitle = newElement("span", "player_title_text");
        this.htmlTitleContainer.appendChild(this.htmlTitle);

        this.htmlToastContainer = newDiv("player_toast_container");
        hide(this.htmlToastContainer);
        this.htmlPlayerRoot.appendChild(this.htmlToastContainer);
        this.htmlToast = newElement("span", "player_toast_text");
        this.htmlToastContainer.appendChild(this.htmlToast);
        this.playerHideToastTimeout = new Timeout(_ => this.htmlToastContainer.classList.add("hide"), 3000);

        this.icons = {
            play:             "svg/player_icons.svg#play",
            play_popup:       "svg/player_icons.svg#play_popup",
            pause:            "svg/player_icons.svg#pause",
            pause_popup:      "svg/player_icons.svg#pause_popup",
            replay:           "svg/player_icons.svg#replay",
            next:             "svg/player_icons.svg#next",
            volume_full:      "svg/player_icons.svg#volume_full",
            volume_medium:    "svg/player_icons.svg#volume_medium",
            volume_low:       "svg/player_icons.svg#volume_low",
            volume_muted:     "svg/player_icons.svg#volume_muted",
            download:         "svg/player_icons.svg#download",
            speed:            "svg/player_icons.svg#speed",
            subs:             "svg/player_icons.svg#subs",
            settings:         "svg/player_icons.svg#settings",
            fullscreen_enter: "svg/player_icons.svg#fullscreen_enter",
            fullscreen_exit:  "svg/player_icons.svg#fullscreen_exit",
            arrow_left:       "svg/player_icons.svg#arrow_left",
            arrow_right:      "svg/player_icons.svg#arrow_right",
            buffering:        "svg/player_icons.svg#buffering",
        };

        this.svgs = {
            playback:   Svg.new(this.icons.play),
            next:       Svg.new(this.icons.next),
            volume:     Svg.new(this.icons.volume_full),
            download:   Svg.new(this.icons.download),
            speed:      Svg.new(this.icons.speed),
            subs:       Svg.new(this.icons.subs),
            settings:   Svg.new(this.icons.settings),
            fullscreen: Svg.new(this.icons.fullscreen_enter),

            seekForward:   SeekIcon.newForward(options.seekBy + 's', 100, 100),
            seekBackward:  SeekIcon.newBackward(options.seekBy + 's', 100, 100),
            playbackPopup: Svg.new(this.icons.play_popup, 70, 70),

            arrowLeft:  Svg.new(this.icons.arrow_left, 20, 20),
            arrowRight: Svg.new(this.icons.arrow_right, 20, 20),

            buffering: Svg.new(this.icons.buffering, 70, 70),
        };

        this.bufferingSvg = this.svgs.buffering.svg;
        this.bufferingSvg.id = "player_buffering";
        hide(this.bufferingSvg);
        this.htmlPlayerRoot.appendChild(this.bufferingSvg);
        this.bufferingTimeout = new Timeout(_ => show(this.bufferingSvg), 200);

        this.playbackPopupSvg = this.svgs.playbackPopup.svg;
        this.playbackPopupSvg.id = "player_playback_popup";
        hide(this.playbackPopupSvg);
        this.htmlPlayerRoot.appendChild(this.playbackPopupSvg);
        this.playbackPopupTimeout = new Timeout(_ => this.playbackPopupSvg.classList.add("hide"), 200);

        this.htmlControls = {
            root: newDiv("player_controls"),
            progress: {
                root:     newDiv("player_progress_root"),
                current:  newDiv("player_progress_current", "player_progress_bar"),
                buffered: newElement("canvas", "player_progress_buffered", "player_progress_bar"),
                total:    newDiv("player_progress_total", "player_progress_bar"),
                thumb:    newDiv("player_progress_thumb"),
                popup:    newDiv("player_progress_popup"),
            },

            buttons: {
                root:             newDiv("player_control_buttons"),
                playbackButton:   newDiv(null, "player_controls_button"),
                nextButton:       newDiv(null, "player_controls_button"),
                volumeButton:     newDiv(null, "player_controls_button"),
                downloadButton:   newDiv(null, "player_controls_button"),
                speedButton:      newDiv(null, "player_controls_button"),
                subsButton:       newDiv(null, "player_controls_button"),
                settingsButton:   newDiv(null, "player_controls_button"),
                fullscreenButton: newDiv(null, "player_controls_button"),

                volumeProgress: newDiv("player_volume_progress"),
                volumeInput:    newElement("input", "player_volume_input"),
                volumePopup:    newDiv("player_volume_popup"),
                liveIndicator:  newDiv("player_live_indicator"),
                timestamp:      newElement("span",  "player_timestamp"),
            },

            subMenu: {
                root: newDiv(null, "player_menu_root"),

                selected: {
                    tab:    null,
                    view:   null,
                    track:  null,
                },

                // Part of the select view. Switch widget indicating whether subtitles are enabled or not.
                subsSwitcher: new Switcher("Enable subtitles"),

                // Part of the select view, html track elements are appended here.
                trackList: newDiv("subtitle_track_list"),
            },

            settings: {
                root: newDiv(null, "player_menu_root"),

                selected: {
                    tab:    null,
                    view:   null,
                    track:  null,
                },
            }
        };

        this.subtitleShift     = new Slider("Subtitle shift",    -20,  20, 0.1,  0, "s", true);
        this.subtitleSize      = new Slider("Subtitle size",      10, 100, 1.0, 30, "px");
        this.subtitlePos       = new Slider("Vertical position",   0, 100, 1.0, 16, "%");
        this.subtitleFgColor   = new ColorPicker("Foreground color", DEFAULT_SUBTITLE_FOREGROUND_COLOR);
        this.subtitleFgOpacity = new Slider("Foreground opacity", 0, 100, 1.0, DEFAULT_SUBTITLE_FOREGROUND_OPACITY, "%");
        this.subtitleBgColor   = new ColorPicker("Background color", DEFAULT_SUBTITLE_BACKGROUND_COLOR);
        this.subtitleBgOpacity = new Slider("Background opacity", 0, 100, 1.0, DEFAULT_SUBTITLE_BACKGROUND_OPACITY, "%");
        this.playbackSpeed = new Slider("Playback speed", 0.25, 5.0, 0.25, 1.0, "x");

        this.isDraggingProgressBar = false;
        this.isUIVisible = true;
        this.volumeBeforeMute = 0.0;

        this.subtitles = [];
        this.selectedSubtitle = null;
        this.activeCues = [];

        this.htmlSeekForward = newDiv("player_forward_container", "hide", "unselectable");
        this.htmlSeekForward.addEventListener("focusout", _ => this.htmlPlayerRoot.focus());
        this.htmlSeekForward.appendChild(this.svgs.seekForward.svg);
        this.htmlPlayerRoot.appendChild(this.htmlSeekForward);
        this.forwardStack = this.options.seekBy;
        this.seekForwardTimeout = new Timeout(_ => {
            this.htmlSeekForward.classList.add("hide");
            this.forwardStack = this.options.seekBy;
        }, this.options.seekStackingThresholdMs);

        this.htmlSeekBackward = newDiv("player_backward_container", "hide", "unselectable");
        this.htmlSeekBackward.addEventListener("focusout", _ => this.htmlPlayerRoot.focus());
        this.htmlSeekBackward.appendChild(this.svgs.seekBackward.svg);
        this.htmlPlayerRoot.appendChild(this.htmlSeekBackward);
        this.backwardStack = this.options.seekBy;
        this.seekBackwardTimeout = new Timeout(_ => {
            this.htmlSeekBackward.classList.add("hide");
            this.backwardStack = this.options.seekBy;
        }, this.options.seekStackingThresholdMs);

        this.subtitleContainer = newDiv("player_subtitle_container");
        this.subtitleText      = newDiv("player_subtitle_text");
        hide(this.subtitleContainer);

        this.htmlPlayerRoot.appendChild(this.subtitleContainer);
        this.subtitleContainer.appendChild(this.subtitleText);

        this.createHtmlControls();
        this.attachPlayerEvents();

        setInterval(_ => this.redrawBufferedBars(), this.options.bufferingRedrawInterval);

        // Without user interaction audio gain will not take effect anyway (internal browser warning)
        if (this.options.useAudioGain) {
            this.gainNode = this.createAudioGain();
        }
        this.setVolume(1.0);

        let userAgent = navigator.userAgent;
        let isSafari = userAgent.includes("Safari") && userAgent.includes("Mac OS");
        if (isSafari) {
            this.rebindFullscreenAPIFromWebkit();
        }

        this.playerUIHideTimeout = new Timeout(_ => this.hidePlayerUI(), this.options.inactivityTime);

        let end = performance.now();
        console.debug("Internals constructor finished in", end-initStart, "ms")
    }

    destroyPlayer() {
        this.discardPlayback();
        let playerParent = this.htmlPlayerRoot.parentNode;
        playerParent.insertBefore(this.htmlVideo, this.htmlPlayerRoot);
        this.htmlPlayerRoot.remove();
    }

    fireControlsPlay() {}
    fireControlsPause() {}
    fireControlsNext() {}
    fireFullscreenChange(_enabled) {}
    fireControlsSeeking(_timestamp) {}
    fireControlsSeeked(_timestamp) {}
    fireControlsVolumeSet(_volume) {}
    fireSettingsChange(_key, _value) {}
    firePlaybackError(_exception, _mediaError) {}
    firePlaybackEnd() {}
    fireSubtitleTrackLoad(_subtitle) {}
    fireSubtitleSelect(_subtitle) {}
    async fireSubtitleSearch(_search) {}

    isVideoPlaying() {
        return !this.htmlVideo.paused && !this.htmlVideo.ended;
    }

    play() {
        if (this.isVideoPlaying() || !this.getCurrentUrl()) {
            return;
        }

        this.playerUIHideTimeout.schedule();
        this.svgs.playback.setHref(this.icons.pause);

        this.htmlVideo.play().catch(exception => {
            this.bufferingTimeout.cancel();
            hide(this.bufferingSvg);

            this.firePlaybackError(exception, this.htmlVideo.error);
        });
    }

    pause() {
        if (!this.isVideoPlaying()) {
            return;
        }

        if (this.options.showControlsOnPause) {
            this.showPlayerUI();
        }

        this.svgs.playback.setHref(this.icons.play);
        this.htmlVideo.pause();
    }

    seek(timestamp) {
        if (isNaN(timestamp)) {
            return;
        }

        if (this.isVideoPlaying()) {
            this.svgs.playback.setHref(this.icons.pause);
        } else {
            this.svgs.playback.setHref(this.icons.play);
        }

        this.htmlVideo.currentTime = timestamp;
    }

    createAudioGain() {
        if (!window.AudioContext) {
            window.AudioContext = window.webkitAudioContext;
        }

        const audioContext = new AudioContext();
        let gainNode = audioContext.createGain();

        const source = audioContext.createMediaElementSource(this.htmlVideo);
        source.connect(gainNode);
        gainNode.connect(audioContext.destination);
        // Keep a reference so it can be resumed in Chromium based browsers
        this.audioContext = audioContext;
        return gainNode;
    }

    updateProgressBar(progress) {
        this.htmlControls.progress.current.style.width = progress * 100 + "%"

        const width = this.htmlControls.progress.root.clientWidth;
        let thumb_left = width * progress;
        thumb_left -= this.htmlControls.progress.thumb.offsetWidth / 2.0;
        this.htmlControls.progress.thumb.style.left = thumb_left + "px";
    }

    updateTimestamps(timestamp) {
        let duration = 0.0;
        let position = 0.0;

        if (!isNaN(this.htmlVideo.duration) && this.htmlVideo.duration !== 0.0) {
            duration = this.htmlVideo.duration;
            position = timestamp / duration;
        }

        if (!this.isDraggingProgressBar) {
            this.updateProgressBar(position);
        }

        let current_string = createTimestampString(this.htmlVideo.currentTime);
        let duration_string = createTimestampString(duration);

        this.htmlControls.buttons.timestamp.textContent = current_string + " / " + duration_string;
    }

    // Proof of concept:
    // - sanitize cues on load (below 100ms)
    // - how to handle unsorted subtitles
    // - heuristic performance offset?
    updateSubtitles(time) {
        if (!this.selectedSubtitle) {
            return;
        }

        time += this.selectedSubtitle.offset;

        // TODO: check if TextTrack.cues are a live list
        let cues = this.selectedSubtitle.cues;
        let whereabouts = binarySearchForCue(time, cues);
        let freshCues = [];
        for (let r = whereabouts; r < cues.length; r++) {
            let cue = cues[r];
            if (cue.startTime <= time && time <= cue.endTime) {
                freshCues.push(cue)
                continue;
            }
            break;
        }

        for (let l = whereabouts - 1; l > - 1; l--) {
            // If cue time is not less than endTime then there is no time to display it anyway
            let cue = cues[l];
            if (time < cue.startTime || cue.endTime <= time) {
                break;
            }

            freshCues.push(cue);
        }

        if (freshCues.length === 0) {
            if (this.activeCues.length > 0) {
                this.activeCues.length = 0;
                hide(this.subtitleContainer);
            }
            return;
        }

        if (areArraysEqual(freshCues, this.activeCues)) {
            return;
        }

        let captionText = "";
        if (this.options.allowCueOverlap) {
            for (let i = 0; i < freshCues.length; i++) {
                captionText += freshCues[i].text + '\n';
            }
        } else {
            captionText = freshCues[freshCues.length - 1];
        }

        this.activeCues = freshCues;
        this.subtitleText.innerHTML = captionText;

        if (this.htmlControls.subMenu.subsSwitcher.enabled) {
            show(this.subtitleContainer);
        }

        let position = this.subtitlePos.getValue();
        this.updateSubtitleHtmlPosition(position);
    }

    updateProgressPopup(progress) {
        const timestamp = this.htmlVideo.duration * progress;
        const popup = this.htmlControls.progress.popup;

        popup.textContent = createTimestampString(timestamp);

        const popupWidth = popup.clientWidth;
        const rootWidth = this.htmlControls.progress.root.clientWidth;

        let position = rootWidth * progress - popupWidth / 2.0;
        if (position < 0) {
            position = 0;
        } else if (position + popupWidth > rootWidth) {
            position = rootWidth - popupWidth;
        }

        popup.style.left = position + "px";
    }

    updateHtmlVolume(volume) {
        if (volume == 0.0) {
            this.svgs.volume.setHref(this.icons.volume_muted);
        } else if (volume < 0.3) {
            this.svgs.volume.setHref(this.icons.volume_low);
        } else if (volume < 0.6) {
            this.svgs.volume.setHref(this.icons.volume_medium);
        } else {
            this.svgs.volume.setHref(this.icons.volume_full);
        }

        const input = this.htmlControls.buttons.volumeInput;
        input.value = volume;

        const progress = this.htmlControls.buttons.volumeProgress;
        progress.style.width = volume / this.options.maxVolume * 100.0 + "%";

        const popup = this.htmlControls.buttons.volumePopup;
        popup.textContent = Math.round(volume * 100.0) + "%";

        const popupWidth = popup.clientWidth;
        const volumeWidth = this.htmlControls.buttons.volumeInput.clientWidth;

        let percent = volume / this.options.maxVolume;
        let position = volumeWidth * percent - popupWidth / 2.0;
        if (position < 0) {
            position = 0;
        } else if (position + popupWidth > volumeWidth) {
            position = volumeWidth - popupWidth;
        }

        popup.style.left = position + "px";
    }

    getNewTime(timeOffset) {
        let timestamp = this.htmlVideo.currentTime + timeOffset;
        if (timestamp < 0) {
            timestamp = 0;
        }
        return timestamp;
    }

    setVolume(volume) {
        if (volume < 0.0) {
            volume = 0.0;
        }

        let maxVolume = this.options.useAudioGain ? this.options.maxVolume : 1;
        if (volume > maxVolume) {
            volume = maxVolume;
        }

        if (this.options.useAudioGain) {
            if (volume > 1.0) {
                this.htmlVideo.volume = 1;
                this.gainNode.gain.value = volume;
            } else {
                this.gainNode.gain.value = 1;
                this.htmlVideo.volume = volume;
            }
        } else {
            this.htmlVideo.volume = volume;
        }

        this.updateHtmlVolume(volume);
    }

    toggleVolume() {
        let slider = this.htmlControls.buttons.volumeInput;
        if (slider.value == 0) {
            this.fireControlsVolumeSet(this.volumeBeforeMute);
            this.setVolume(this.volumeBeforeMute);
        } else {
            this.volumeBeforeMute = slider.value;
            this.fireControlsVolumeSet(0);
            this.setVolume(0);
        }
    }

    setVolumeRelative(volume) {
        this.setVolume(this.htmlVideo.volume + volume);
    }

    setTitle(title) {
        if (!title) {
            hide(this.htmlTitleContainer);
        } else {
            show(this.htmlTitleContainer);
            this.htmlTitle.textContent = title;
        }
    }

    setSpeed(speed) {
        if (isNaN(speed)) {
            speed = 1;
        }
        this.htmlVideo.playbackRate = speed;
    }

    setPoster(url) {
        if (url) {
            this.htmlVideo.poster = url;
        } else {
            this.htmlVideo.poster = "";
        }
    }

    showPlaybackPopup() {
        show(this.playbackPopupSvg);
        this.playbackPopupSvg.classList.remove("hide");
        this.playbackPopupTimeout.schedule();
    }

    setToast(toast) {
        this.htmlToast.textContent = toast;
        this.htmlToastContainer.classList.remove("hide");
        show(this.htmlToastContainer);

        this.playerHideToastTimeout.schedule();
    }

    getCurrentTime() {
        return this.htmlVideo.currentTime;
    }

    getCurrentUrl() {
        if (this.playingHls) {
            return this.hls.url;
        }
        return this.htmlVideo.src;
    }

    toggleFullscreen() {
        if (document.fullscreenElement) {
            document.exitFullscreen();
            this.svgs.fullscreen.setHref(this.icons.fullscreen_enter);
            this.fireFullscreenChange(false)
        } else {
            this.htmlPlayerRoot.requestFullscreen();
            this.svgs.fullscreen.setHref(this.icons.fullscreen_exit);
            this.fireFullscreenChange(true)
        }
    }

    togglePlayback() {
        if (this.htmlVideo.paused) {
            this.fireControlsPlay();
            this.play();
        } else {
            this.fireControlsPause();
            this.pause();
        }
    }

    setVideoTrack(url) {
        if(URL.canParse && !URL.canParse(url, document.baseURI)){
            console.debug("Failed to set a new URL. It's not parsable.");
            // We should probably inform the user about the error either via debug log or return false
            return;
        }
        // This covers both relative and fully qualified URLs because we always specify the base
        // and when the base is not provided, the second argument is used to construct a valid URL
        let pathname = new URL(url, document.baseURI).pathname;

        this.pause();
        this.seek(0);

        hide(this.htmlControls.buttons.liveIndicator);
        this.isLive = false;

        if (pathname.endsWith(".m3u8") || pathname.endsWith(".ts")) {
            import("../external/hls.js").then(module => {
                if (module.Hls.isSupported()) {
                    if (this.hls == null) {
                        this.hls = new module.Hls(this.options.hlsConfig);

                        this.hls.on(module.Hls.Events.MANIFEST_PARSED, (_, data) => {
                            if (!data.levels || data.levels.length === 0) {
                                return;
                            }

                            let details = data.levels[0].details;
                            if (details) {
                                this.isLive = details.live;
                            }

                            if (this.isLive) {
                                show(this.htmlControls.buttons.liveIndicator);
                            }
                        });
                    }

                    this.hls.loadSource(url);
                    this.hls.attachMedia(this.htmlVideo);
                    this.playingHls = true;
                }
            });
        } else {
            if (this.playingHls) {
                this.hls.detachMedia();
                this.playingHls = false;
                this.isLive = false;
            }
            this.htmlVideo.src = url;
            this.htmlVideo.load();
        }
    }

    discardPlayback() {
        this.pause();
        if (this.playingHls) {
            this.hls.detachMedia();
            this.playingHls = false;
            this.isLive = false;
        }

        this.htmlVideo.currentTime = 0;
        this.htmlVideo.src = "";
        this.htmlVideo.removeAttribute("src");
        this.htmlVideo.load();
        hide(this.htmlControls.buttons.liveIndicator);
    }

    addSubtitle(url, show, info, shift) {
        if (!info) {
            info = FileInfo.fromUrl(url)
        }

        let ext = info.extension;
        if (ext !== "vtt" && ext !== "srt") {
            console.debug("Unsupported subtitle extension:", ext)
            return
        }

        fetch(url).then(async response => {
            let text = await response.text();

            let parseStart = performance.now();
            let cues;
            if (ext === "vtt") {
                cues = parseVtt(text);
            } else {
                cues = parseSrt(text);
            }

            console.debug("Parsed", ext, "track, cue count:", cues.length, "in", performance.now() - parseStart, "ms")

            if (cues.length === 0) {
                return
            }

            if (this.options.sanitizeSubtitles) {
                let sanitizeStart = performance.now();
                for (let i = 0; i < cues.length; i++) {
                    cues[i].text = sanitizeHTMLForDisplay(cues[i].text);
                }

                console.debug("Sanitized in", performance.now() - sanitizeStart, "ms")
            }

            let subtitle = new Subtitle(cues, info.filename, info.extension, url, shift);
            subtitle.htmlTrack.onclick = _ => this.switchSubtitleTrack(subtitle);

            if (this.htmlControls.subMenu.subsSwitcher.enabled && !this.selectedSubtitle || show) {
                this.enableSubtitleTrack(subtitle);
            }

            URL.revokeObjectURL(url)

            let trackList = this.htmlControls.subMenu.trackList;
            trackList.appendChild(subtitle.htmlTrack);
            this.subtitles.push(subtitle);

            this.fireSubtitleTrackLoad(subtitle);
        });
    }

    enableSubtitles() {
        this.htmlControls.subMenu.subsSwitcher.setState(true);
        if (this.subtitles.length !== 0 && !this.selectedSubtitle) {
            this.switchSubtitleTrack(this.subtitles[0])
        } else {
            this.updateSubtitles(this.getCurrentTime());
        }

        if (this.activeCues.length > 0) {
            show(this.subtitleContainer);
        }
    }

    disableSubtitles() {
        this.htmlControls.subMenu.subsSwitcher.setState(false);
        hide(this.subtitleContainer);
    }

    enableSubtitleTrack(subtitle) {
        if (!subtitle) {
            return;
        }

        this.htmlControls.subMenu.subsSwitcher.setState(true);
        this.fireSubtitleSelect(subtitle)

        this.selectedSubtitle = subtitle;
        this.markSubtitleSelected(subtitle);

        this.subtitleShift.setValue(this.selectedSubtitle.offset);
        this.updateSubtitles(this.getCurrentTime());
    }

    switchSubtitleTrack(subtitle) {
        if (!subtitle) {
            return;
        }

        this.fireSubtitleSelect(subtitle)

        this.selectedSubtitle = subtitle;
        this.markSubtitleSelected(subtitle);

        this.subtitleShift.setValue(this.selectedSubtitle.offset);
        this.updateSubtitles(this.getCurrentTime());
    }

    shiftCurrentSubtitleTrackBy(seconds) {
        if (this.selectedSubtitle) {
            this.selectedSubtitle.offset += seconds;
            this.subtitleShift.setValue(this.selectedSubtitle.offset);
            this.updateSubtitles(this.getCurrentTime());
        }
    }

    setCurrentSubtitleShift(offset) {
        if (this.selectedSubtitle) {
            this.selectedSubtitle.offset = offset;
            this.subtitleShift.setValue(this.selectedSubtitle.offset);
            this.updateSubtitles(this.getCurrentTime());
        }
    }

    setSubtitleShiftByUrl(url, seconds) {
        let subs = this.subtitles;
        for (let i = 0; i < subs.length; i++) {
            let sub = subs[i];
            if (sub.url === url) {
                sub.offset = seconds;
                break;
            }
        }

        this.subtitleShift.setValue(this.selectedSubtitle.offset);
        this.updateSubtitles(this.getCurrentTime());
    }

    getCurrentSubtitleShift() {
        if (this.selectedSubtitle) {
            return this.subtitleShift.getValue();
        } else {
            return 0.0;
        }
    }

    getSubtitleShiftByUrl(url) {
        let subtitles = this.subtitles;
        for (let i = 0; i < subtitles.length; i++) {
            const sub = subtitles[i];
            if (sub.url === url) {
                return sub.offset;
            }
        }

        return 0.0;
    }

    anySubtitleMatch(predicate) {
        // One could modify the subtitle inside the predicate function
        let subtitles = this.subtitles;
        let found = false;
        for (let i = 0; i < subtitles.length; i++) {
            if (predicate(subtitles[i])) {
                found = true;
                break;
            }
        }
        return found;
    }

    removeSubtitleByUrl(url) {
        let subtitles = this.subtitles;
        let index = -1;
        for (let i = 0; i < subtitles.length; i++) {
            if (subtitles[i].url === url) {
                index = i;
                break;
            }
        }
        if (index === -1) {
            return;
        }
        this.removeSubtitleTrackAt(index);
    }

    removeSubtitleTrackAt(index) {
        let list = this.htmlControls.subMenu.trackList;
        let track = list.children[index];
        if (this.selectedSubtitle != null && track === this.selectedSubtitle.htmlTrack) {
            this.selectedSubtitle = null;
            this.activeCues.length = 0;
        }
        list.removeChild(track);
        this.subtitles.splice(index, 1);
    }

    clearAllSubtitleTracks() {
        this.subtitles = [];
        this.selectedSubtitle = null;
        this.activeCues = [];

        let list = this.htmlControls.subMenu.trackList;
        while (list.lastChild) {
            list.removeChild(list.lastChild);
        }

        hide(this.subtitleContainer);
    }

    setSubtitleFontSize(fontSize) {
        this.subtitleSize.setValue(fontSize);
        this.subtitleText.style.fontSize = fontSize + "px";
        this.fireSettingsChange(Options.SUBTITLE_FONT_SIZE, fontSize);

        let position = this.subtitlePos.getValue();
        this.updateSubtitleHtmlPosition(position);
    }

    setSubtitleForegroundColor(hexColor) {
        let opacity = this.subtitleFgOpacity.getValue();
        this.subtitleText.style.color = makeRgba(hexColor, opacity);
        this.subtitleFgColor.setValue(hexColor);
        this.fireSettingsChange(Options.SUBTITLE_FOREGROUND_COLOR, hexColor);
    }

    setSubtitleForegroundOpacity(opacity) {
        let hexColor = this.subtitleFgColor.getValue();
        this.subtitleText.style.color = makeRgba(hexColor, opacity);
        this.subtitleFgOpacity.setValue(opacity);
        this.fireSettingsChange(Options.SUBTITLE_FOREGROUND_OPACITY, opacity);
    }

    setSubtitleBackgroundColor(hexColor) {
        let opacity = this.subtitleBgOpacity.getValue();
        this.subtitleText.style.backgroundColor = makeRgba(hexColor, opacity);
        this.subtitleBgColor.setValue(hexColor);
        this.fireSettingsChange(Options.SUBTITLE_BACKGROUND_COLOR, hexColor);
    }

    setSubtitleBackgroundOpacity(opacity) {
        let hexColor = this.subtitleBgColor.getValue();
        this.subtitleText.style.backgroundColor = makeRgba(hexColor, opacity);
        this.subtitleBgOpacity.setValue(opacity);
        this.fireSettingsChange(Options.SUBTITLE_BACKGROUND_OPACITY, opacity);
    }

    updateSubtitleHtmlPosition(percentage) {
        let playerHeight = this.htmlPlayerRoot.offsetHeight;
        let subsHeight = this.subtitleContainer.offsetHeight;

        if (subsHeight > playerHeight) {
            this.subtitleContainer.style.bottom = 0;
        } else {
            let real_percentage = percentage * (playerHeight - subsHeight) / playerHeight;
            this.subtitleContainer.style.bottom = real_percentage + "%";
        }
    }

    setSubtitleVerticalPosition(percentage) {
        this.updateSubtitleHtmlPosition(percentage);
        this.subtitlePos.setValue(percentage);
        this.fireSettingsChange(Options.SUBTITLE_VERTICAL_POSITION, percentage);
    }

    showPlayerUI() {
        this.htmlPlayerRoot.style.cursor = "auto";
        this.htmlControls.root.classList.remove("hide");
        this.htmlTitleContainer.classList.remove("hide");
    }

    hidePlayerUI() {
        if (this.options.alwaysShowControls) {
            return;
        }

        if (!this.isVideoPlaying() && this.options.showControlsOnPause) {
            return;
        }

        if (this.isDraggingProgressBar) {
            return;
        }

        this.htmlPlayerRoot.style.cursor = "none";
        this.htmlControls.root.classList.add("hide");
        this.htmlTitleContainer.classList.add("hide");
    }

    redrawBufferedBars() {
        const context = this.htmlControls.progress.buffered.getContext("2d");
        context.fillStyle = "#bdae93cc";

        const buffered_width = this.htmlControls.progress.buffered.width;
        const buffered_height = this.htmlControls.progress.buffered.height;
        context.clearRect(0, 0, buffered_width, buffered_height);

        const duration = this.htmlVideo.duration;
        for (let i = 0; i < this.htmlVideo.buffered.length; i++) {
            let start = this.htmlVideo.buffered.start(i) / duration;
            let end = this.htmlVideo.buffered.end(i) / duration;

            let x = Math.floor(buffered_width * start);
            let width = Math.ceil(buffered_width * end - buffered_width * start);
            context.fillRect(x, 0, width, buffered_height);
        }
    };

    // 40% | 20% | 40% - triple screen split, returns respectively: 0, 1, 2
    getClickAreaIndex(event) {
        let rect = this.htmlPlayerRoot.getBoundingClientRect();
        // Get the click coordinates relative to the viewport
        const relativeClickX = event.clientX - rect.left;
        let isLeft = relativeClickX < rect.width * 0.4;
        if (isLeft) {
            return 0;
        }
        let isRight = rect.width * 0.6 < relativeClickX;
        if (isRight) {
            return 2;
        }
        return 1;
    }

    seekBackward() {
        this.htmlSeekBackward.classList.remove("hide");
        if (this.seekBackwardTimeout.inProgress()) {
            this.backwardStack += this.options.seekBy;
            this.svgs.seekBackward.setText(this.backwardStack + "s")
        } else {
            this.svgs.seekBackward.setText(this.options.seekBy + "s")
        }

        this.seekBackwardTimeout.schedule();
        let timestamp = this.getNewTime(-this.options.seekBy);
        this.fireControlsSeeked(timestamp);
        this.seek(timestamp);
    }

    seekForward() {
        this.htmlSeekForward.classList.remove("hide");
        if (this.seekForwardTimeout.inProgress()) {
            this.forwardStack += this.options.seekBy;
            this.svgs.seekForward.setText(this.forwardStack + "s")
        } else {
            this.svgs.seekForward.setText(this.options.seekBy + "s")
        }
        this.seekForwardTimeout.schedule();
        let timestamp = this.getNewTime(this.options.seekBy);
        this.fireControlsSeeked(timestamp);
        this.seek(timestamp);
    }

    attachPlayerRootEvents() {
        this.htmlPlayerRoot.addEventListener("touchmove", _ => {
            this.showPlayerUI();
            this.playerUIHideTimeout.schedule();
        });

        this.htmlPlayerRoot.addEventListener("mousemove", _ => {
            this.showPlayerUI();
            this.playerUIHideTimeout.schedule();
        });

        this.htmlPlayerRoot.addEventListener("mousedown", _ => {
            this.showPlayerUI();
            this.playerUIHideTimeout.schedule();
        });

        this.htmlPlayerRoot.addEventListener("mouseup", _ => {
            this.showPlayerUI();
            this.playerUIHideTimeout.schedule();
        });

        this.htmlPlayerRoot.addEventListener("mouseenter", _ => {
            this.showPlayerUI();
            this.playerUIHideTimeout.schedule();
        });

        this.htmlPlayerRoot.addEventListener("mouseleave", _ => {
            this.hidePlayerUI();
        });

        this.htmlPlayerRoot.addEventListener("keydown", event => {
            if (event.key === " " || event.code === "Space") {
                this.togglePlayback();
                consumeEvent(event);

            } else if (event.key === "ArrowLeft") {
                this.seekBackward();
                consumeEvent(event);

            } else if (event.key === "ArrowRight") {
                this.seekForward();
                consumeEvent(event);

            } else if (event.key === "ArrowUp") {
                this.setVolumeRelative(0.1);
                consumeEvent(event);

            } else if (event.key === "ArrowDown") {
                this.setVolumeRelative(-0.1);
                consumeEvent(event);

            } else if (event.key === this.options.fullscreenKeyLetter) {
                this.toggleFullscreen();
                consumeEvent(event);
            } else if (event.key === "m") {
                this.toggleVolume();
            }
        });

        this.doubleTapped = false;
        this.clickTimeout = new Timeout(_ => {
            if (!this.doubleTapped) {
                this.togglePlayback()
            }
        }, this.options.doubleClickThresholdMs);
        this.lastAreaIndex = -1;


        this.htmlPlayerRoot.addEventListener("click", event => {
            if ((event.pointerType === "touch" || event.pointerType === "pen") && !this.isUIVisible) {
                return;
                // We don't allow double tap seeking without the UI shown
            }

            if (!this.options.enableDoubleTapSeek) {
                this.togglePlayback();
                return;
            }

            let areaIndex = this.getClickAreaIndex(event);
            if (areaIndex === 1) {
                this.clickTimeout.cancel();
                this.togglePlayback();
                return;
            }

            if (this.clickTimeout.inProgress()) {
                this.doubleTapped = true;
                if (areaIndex === this.lastAreaIndex) {
                    if (areaIndex === 0) {
                        this.seekBackward();
                    } else {
                        this.seekForward();
                    }
                }
            } else {
                this.doubleTapped = false;
            }
            this.clickTimeout.schedule();
            this.lastAreaIndex = areaIndex;
        });
    }

    attachHtmlVideoEvents() {
        this.htmlVideo.addEventListener("waiting", _ => {
            this.bufferingTimeout.schedule();
        });

        this.htmlVideo.addEventListener("canplay", _ => {
            this.bufferingTimeout.cancel();
            hide(this.bufferingSvg);
        });

        this.htmlVideo.addEventListener("canplaythrough", _ => {
            this.bufferingTimeout.cancel();
            hide(this.bufferingSvg);
        });

        this.htmlVideo.addEventListener("durationchange", _ => {
            if (!this.isVideoPlaying()) {
                let timestamp = this.getCurrentTime();
                this.updateTimestamps(timestamp);
            }
        });

        this.htmlVideo.addEventListener("playing", _ => {
            this.bufferingTimeout.cancel();
            hide(this.bufferingSvg);
        });

        this.htmlVideo.addEventListener("timeupdate", _ => {
            let timestamp = this.getCurrentTime();
            if (this.htmlControls.subMenu.subsSwitcher.enabled && this.selectedSubtitle) {
                this.updateSubtitles(timestamp);
            }

            this.updateTimestamps(timestamp);
        });

        this.htmlVideo.addEventListener("ended", _ => {
            this.bufferingTimeout.cancel();
            hide(this.bufferingSvg);

            this.svgs.playback.setHref(this.icons.replay)
            this.firePlaybackEnd();
        });

        this.htmlVideo.addEventListener("play", _ => {
            if (this.options.useAudioGain && this.audioContext.state === "suspended") {
                this.audioContext.resume().then(_ =>
                    console.debug("Resumed AudioContext which was suspended.")
                );
            }
            this.svgs.playbackPopup.setHref(this.icons.play_popup);
            this.showPlaybackPopup();
        });

        this.htmlVideo.addEventListener("pause", _ => {
            this.svgs.playbackPopup.setHref(this.icons.pause_popup);
            this.showPlaybackPopup();
        });

        this.htmlVideo.addEventListener("ratechange", _ => {
            this.playbackSpeed.setValue(this.htmlVideo.playbackRate)
        });
    }

    attachPlayerControlsEvents() {
        this.htmlControls.buttons.playbackButton.addEventListener("click", _ => {
            this.togglePlayback();
        });

        this.htmlControls.buttons.nextButton.addEventListener("click", _ => {
            this.fireControlsNext();
        });

        this.htmlControls.buttons.volumeButton.addEventListener("click", _ => {
            this.toggleVolume();
        });

        // This roughly simulates a click on an invisible anchor as there's no practical way to trigger "Save As" dialog
        this.htmlControls.buttons.downloadButton.addEventListener("click", _ => {
            const anchor = document.createElement('a');
            anchor.href = this.getCurrentUrl();
            let fileInfo = FileInfo.fromUrl(anchor.href);
            anchor.download = fileInfo.filename;

            document.body.appendChild(anchor);
            anchor.click();
            document.body.removeChild(anchor);
        });

        this.htmlControls.buttons.speedButton.addEventListener("click", _ => {
            // https://developer.mozilla.org/en-US/docs/Web/Media/Audio_and_video_delivery/WebAudio_playbackRate_explained
            // The recommended range is [0.5 - 4.0]
            let newSpeed = this.htmlVideo.playbackRate + 0.25;
            if (newSpeed > 2.5) {
                newSpeed = 1;
            }
            this.setSpeed(newSpeed);
            this.setToast("Speed: " + this.htmlVideo.playbackRate)
        });

        this.htmlControls.buttons.subsButton.addEventListener("click", _ => {
            hide(this.htmlControls.settings.root);

            let root = this.htmlControls.subMenu.root;
            if (isHidden(root)) {
                show(root);
            } else {
                hide(root);
            }
        });

        this.htmlControls.buttons.settingsButton.addEventListener("click", _ => {
            hide(this.htmlControls.subMenu.root);

            let root = this.htmlControls.settings.root;
            if (isHidden(root)) {
                show(root);
            } else {
                hide(root);
            }
        });

        this.htmlControls.buttons.fullscreenButton.addEventListener("click", () => {
            this.toggleFullscreen();
        });

        this.htmlControls.buttons.volumeInput.addEventListener("input", _event => {
            let volume = this.htmlControls.buttons.volumeInput.value;
            this.fireControlsVolumeSet(volume);
            this.setVolume(volume);
        });

        let calculateProgress = (event, element) => {
            let rect = element.getBoundingClientRect();
            let offsetX;

            if (event.touches) {
                let touches = event.touches.length !== 0 ? event.touches : event.changedTouches;
                offsetX = touches[0].clientX - rect.left;
            } else {
                offsetX = event.clientX - rect.left;
            }

            // Ensure the touch doesn't exceed slider bounds
            if (offsetX < 0) offsetX = 0;
            if (offsetX > rect.width) offsetX = rect.width;

            let progress = offsetX / rect.width;
            if (isNaN(progress)) {
                progress = 0;
            }

            return progress;
        }

        this.htmlControls.progress.root.addEventListener("touchstart", _ => {
            const onProgressBarTouchMove = event => {
                event.preventDefault();

                const progressRoot = this.htmlControls.progress.root;
                progressRoot.classList.add("active");

                const progress = calculateProgress(event, progressRoot);
                this.updateProgressBar(progress);
                this.updateProgressPopup(progress);
            }

            const onProgressBarTouchStop = event => {
                this.isDraggingProgressBar = false;
                this.playerUIHideTimeout.schedule();

                document.removeEventListener('touchmove', onProgressBarTouchMove);
                document.removeEventListener('touchend', onProgressBarTouchStop);

                const progressRoot = this.htmlControls.progress.root;
                progressRoot.classList.remove("active");

                const progress = calculateProgress(event, progressRoot);
                const timestamp = this.htmlVideo.duration * progress;

                this.fireControlsSeeked(timestamp);
                this.seek(timestamp);
            }

            this.isDraggingProgressBar = true;
            document.addEventListener('touchmove', onProgressBarTouchMove, { passive: false });
            document.addEventListener('touchend', onProgressBarTouchStop);
        });

        this.htmlControls.progress.root.addEventListener("mousedown", _event => {
            const onProgressBarMouseMove = event => {
                const progressRoot = this.htmlControls.progress.root;
                const progress = calculateProgress(event, progressRoot);
                this.updateProgressBar(progress);
                this.updateProgressPopup(progress);
            }

            const onProgressBarMouseUp = event => {
                this.isDraggingProgressBar = false;
                this.playerUIHideTimeout.schedule();

                document.removeEventListener('mousemove', onProgressBarMouseMove);
                document.removeEventListener('mouseup', onProgressBarMouseUp);

                const progressRoot = this.htmlControls.progress.root;
                const progress = calculateProgress(event, progressRoot);
                const timestamp = this.htmlVideo.duration * progress;

                this.fireControlsSeeked(timestamp);
                this.seek(timestamp);
            }

            this.isDraggingProgressBar = true;
            document.addEventListener('mousemove', onProgressBarMouseMove);
            document.addEventListener('mouseup', onProgressBarMouseUp);
        });

        this.htmlControls.progress.root.addEventListener("mouseenter", _ => {
            this.updateTimestamps(this.htmlVideo.currentTime);
        });

        this.htmlControls.progress.root.addEventListener("mousemove", event => {
            const progress = calculateProgress(event, this.htmlControls.progress.root);
            this.updateProgressPopup(progress);
        });

        this.htmlControls.root.addEventListener("transitionend", event => {
            // NOTE(kihau):
            //     This is a really weird and confusing way of setting the isUIVisible flag.
            //     Probably should be changed and done the proper way at some point.
            if (event.propertyName === "opacity") {
                this.isUIVisible = !event.target.classList.contains("hide");
            }
        });
    }

    attachOtherEvents() {
        document.addEventListener("fullscreenchange", _ => {
            // This is after the fact when a user exited without using the icon
            let href = document.fullscreenElement ? this.icons.fullscreen_exit : this.icons.fullscreen_enter;
            this.svgs.fullscreen.setHref(href);
            this.fireFullscreenChange(document.fullscreenElement != null)
        });
    }

    attachPlayerEvents() {
        this.attachPlayerRootEvents();
        this.attachHtmlVideoEvents();
        this.attachPlayerControlsEvents();
        this.attachOtherEvents();
    }

    rebindFullscreenAPIFromWebkit() {
        // Should remove the logs and toasts once tested successfully
        console.debug("Rebinding webkit fullscreen API")
        this.setToast("Rebinding webkit fullscreen API")
        if (!HTMLElement.prototype.requestFullscreen) {
            HTMLElement.prototype.requestFullscreen = HTMLElement.prototype.webkitRequestFullscreen;
        }
        if (!document.exitFullscreen) {
            document.exitFullscreen = document.webkitExitFullscreen;
        }

        if (Object.getOwnPropertyDescriptor(Document.prototype, 'fullscreenElement')) {
            this.setToast("getOwnPropertyDescriptor determined fullscreenElement already exists")
            return;
        }
        Object.defineProperty(Document.prototype, 'fullscreenElement', {
            get: () => {
                console.debug("Returning document.webkitFullscreenElement", document.webkitFullscreenElement)
                this.setToast("fullscreenElement rebound getter was called " + document.webkitFullscreenElement)
                return document.webkitFullscreenElement;
            }
        });
    }

    assembleProgressBar() {
        let controls = this.htmlControls;
        let root     = this.htmlControls.root;
        let progress = this.htmlControls.progress;

        progress.popup.textContent = "00:00";
        progress.popup.classList.add("unselectable");

        root.appendChild(progress.root); {
            progress.root.appendChild(progress.total);
            progress.root.appendChild(progress.buffered);
            progress.root.appendChild(progress.current);
            progress.root.appendChild(progress.thumb);
            progress.root.appendChild(progress.popup);
        }
    }

    assembleControlButtons() {
        let svgs             = this.svgs;
        let buttonsRoot      = this.htmlControls.buttons.root;
        let playbackButton   = this.htmlControls.buttons.playbackButton;
        let nextButton       = this.htmlControls.buttons.nextButton;
        let volumeButton     = this.htmlControls.buttons.volumeButton;
        let volumeRoot       = newDiv("player_volume_root");
        let volumeSlider     = this.htmlControls.buttons.volumeInput;
        let volumeBar        = newDiv("player_volume_bar");
        let volumeProgress   = this.htmlControls.buttons.volumeProgress;
        let volumePopup      = this.htmlControls.buttons.volumePopup;
        let liveIndicator    = this.htmlControls.buttons.liveIndicator;
        let liveDot          = newDiv("player_live_dot");
        let liveText         = newDiv("player_live_text");
        let timestamp        = this.htmlControls.buttons.timestamp;
        let spacer           = newDiv("player_spacer");
        let downloadButton   = this.htmlControls.buttons.downloadButton;
        let speedButton      = this.htmlControls.buttons.speedButton;
        let subsButton       = this.htmlControls.buttons.subsButton;
        let settingsButton   = this.htmlControls.buttons.settingsButton;
        let fullscreenButton = this.htmlControls.buttons.fullscreenButton;

        playbackButton.title = "Play/Pause";
        nextButton.title     = "Next";
        volumeButton.title   = "Mute/Unmute";

        volumeSlider.type  = "range";
        volumeSlider.min   = "0";
        volumeSlider.max   = this.options.maxVolume;
        volumeSlider.value = "1";
        volumeSlider.step  = "any";

        volumePopup.textContent = "0%";
        volumePopup.classList.add("unselectable");

        liveText.textContent = "LIVE";
        timestamp.textContent = "00:00 / 00:00";

        downloadButton.title   = "Download";
        speedButton.title      = "Playback speed";
        subsButton.title       = "Subtitles";
        settingsButton.title   = "Settings";
        fullscreenButton.title = "Fullscreen";

        if (this.options.hidePlaybackButton)   hide(playbackButton);
        if (this.options.hideNextButton)       hide(nextButton);
        if (this.options.hideVolumeButton)     hide(volumeButton);
        if (this.options.hideVolumeSlider)     hide(volumeRoot)
        if (this.options.hideTimestamps)       hide(timestamp);
        if (this.options.hideDownloadButton)   hide(downloadButton);
        if (this.options.hideSpeedButton)      hide(speedButton);
        if (this.options.hideSubtitlesButton)  hide(subsButton);
        if (this.options.hideSettingsButton)   hide(settingsButton);
        if (this.options.hideFullscreenButton) hide(fullscreenButton);

        hide(liveIndicator);

        this.htmlControls.root.append(buttonsRoot); {
            buttonsRoot.append(playbackButton); {
                playbackButton.append(svgs.playback.svg);
            }

            buttonsRoot.append(nextButton); {
                nextButton.append(svgs.next.svg);
            }

            buttonsRoot.append(volumeButton); {
                volumeButton.appendChild(svgs.volume.svg);
            }

            buttonsRoot.append(volumeRoot); {
                volumeRoot.append(volumeBar);
                volumeRoot.append(volumeProgress);
                volumeRoot.append(volumeSlider);
                volumeRoot.append(volumePopup);
            }

            buttonsRoot.append(liveIndicator); {
                liveIndicator.append(liveDot);
                liveIndicator.append(liveText);
            }

            buttonsRoot.append(timestamp);

            buttonsRoot.append(spacer)

            buttonsRoot.append(downloadButton); {
                downloadButton.appendChild(svgs.download.svg);
            }

            buttonsRoot.append(speedButton); {
                speedButton.appendChild(svgs.speed.svg);
            }

            buttonsRoot.append(subsButton); {
                subsButton.appendChild(svgs.subs.svg);
            }

            buttonsRoot.append(settingsButton); {
                settingsButton.appendChild(svgs.settings.svg);
            }

            buttonsRoot.append(fullscreenButton); {
                fullscreenButton.appendChild(svgs.fullscreen.svg);
            }
        }
    }

    createHtmlControls() {
        let playerControls = this.htmlControls.root;
        playerControls.addEventListener("click", stopPropagation);
        playerControls.addEventListener("focusout", _ => {
            // Otherwise document.body will receive focus
            this.htmlPlayerRoot.focus();
        });

        this.htmlPlayerRoot.appendChild(playerControls);

        this.assembleProgressBar();
        this.assembleControlButtons();
        this.createSubtitleMenu();
        this.createSettingsMenu();
    }

    markSubtitleSelected(subtitle) {
        let menu = this.htmlControls.subMenu;
        let track = subtitle.htmlTrack;

        if (menu.selected.track) {
            menu.selected.track.classList.remove("subtitle_track_selected");
        }

        track.classList.add("subtitle_track_selected");
        menu.selected.track = track;
    }

    createSubtitleMenu() {
        let playerRoot       = this.htmlPlayerRoot;
        let menu             = this.htmlControls.subMenu;
        let menuRoot         = menu.root;
        let menuTabs         = newDiv(null, "player_menu_tabs");
        let menuSeparator    = newDiv(null, "player_menu_separator");
        let menuViews        = newDiv(null, "player_menu_views");
        let selectTab        = newDiv(null, "player_menu_tab");
        let searchTab        = newDiv(null, "player_menu_tab");
        let optionsTab       = newDiv(null, "player_menu_tab");
        let selectView       = newDiv("player_submenu_select_view");
        let subsSwitch       = menu.subsSwitcher;
        let searchView       = newDiv("player_submenu_search_view");
        let subtitleImport   = newElement("input", "player_submenu_import");
        let subtitleName     = newElement("input", null, "player_input_box");
        let subtitleLanguage = newElement("input", null, "player_input_box");
        let subtitleYear     = newElement("input", null, "player_input_box");
        let subtitleSeason   = newElement("input", null, "player_input_box");
        let subtitleEpisode  = newElement("input", null, "player_input_box");
        let searchSubtitle   = newElement("button", "player_subtitle_search");
        let optionsView      = newDiv("player_submenu_options_view");
        let subsShift        = this.subtitleShift;
        let subsSize         = this.subtitleSize;
        let subsPos          = this.subtitlePos;
        let subsFgColor      = this.subtitleFgColor;
        let subsFgOpacity    = this.subtitleFgOpacity;
        let subsBgColor      = this.subtitleBgColor;
        let subsBgOpacity    = this.subtitleBgOpacity;

        hide(menuRoot);
        hide(selectView);
        hide(searchView);
        hide(optionsView);

        this.setSubtitleVerticalPosition(16);

        selectTab.textContent  = "Select";
        searchTab.textContent  = "Search";
        optionsTab.textContent = "Options";

        subtitleImport.textContent = "Import subtitle";
        subtitleImport.type = "file";
        subtitleImport.accept = ".vtt,.srt";

        subtitleName.style.marginTop = "10px";
        subtitleName.placeholder = "Title";
        subtitleLanguage.placeholder = "Language";
        subtitleYear.placeholder = "Year";
        subtitleSeason.placeholder = "Season";
        subtitleEpisode.placeholder = "Episode";

        searchSubtitle.textContent = "Search sub";

        menu.selected.tab  = selectTab;
        menu.selected.view = selectView;

        menu.selected.tab.classList.add("player_menu_tab_selected");
        show(menu.selected.view);

        menuRoot.onclick = stopPropagation;

        let select = (tab, view) => {
            let selected = menu.selected;
            selected.tab.classList.remove("player_menu_tab_selected");
            hide(selected.view);

            selected.tab = tab;
            selected.view = view;

            selected.tab.classList.add("player_menu_tab_selected");
            show(selected.view);
        }

        selectTab.onclick  = _ => select(selectTab,  selectView);
        searchTab.onclick  = _ => select(searchTab,  searchView);
        optionsTab.onclick = _ => select(optionsTab, optionsView);

        subsSwitch.onAction = enabled => {
            this.fireSettingsChange(Options.SUBTITLES_ENABLED, enabled);

            if (enabled) {
                this.enableSubtitles();
            } else {
                this.disableSubtitles();
            }
        };

        subtitleImport.onchange = event => {
            if (event.target.files.length === 0) {
                return;
            }
            // Allow selection of multiple at some point
            const file = event.target.files[0];
            // This object is a blob and will be released with URL.revokeObjectURL on load
            const objectUrl = URL.createObjectURL(file);
            let trackInfo = FileInfo.fromUrl(file.name);
            console.debug("Adding", trackInfo.extension, "subtitle from disk");
            this.addSubtitle(objectUrl, true, trackInfo, 0.0);
        };

        subtitleName.addEventListener("keydown", stopPropagation);
        subtitleLanguage.addEventListener("keydown", stopPropagation);
        subtitleYear.addEventListener("keydown", stopPropagation);

        searchSubtitle.addEventListener("click", async _ => {
            let title = subtitleName.value;
            let lang = subtitleLanguage.value;
            let year = subtitleYear.value;
            let season = subtitleSeason.value;
            let episode = subtitleEpisode.value;
            let search = new Search(title, lang, year, season, episode);
            searchSubtitle.disabled = true;
            let success = await this.fireSubtitleSearch(search);
            searchSubtitle.disabled = false;
            console.debug("Search", success ? "was successful" : "failed");
        });

        subsShift.onInput     = value    => this.setCurrentSubtitleShift(value);
        subsSize.onInput      = size     => this.setSubtitleFontSize(size);
        subsPos.onInput       = position => this.setSubtitleVerticalPosition(position);
        subsFgColor.onInput   = hexColor => this.setSubtitleForegroundColor(hexColor);
        subsFgOpacity.onInput = opacity  => this.setSubtitleForegroundOpacity(opacity);
        subsBgColor.onInput   = hexColor => this.setSubtitleBackgroundColor(hexColor);
        subsBgOpacity.onInput = opacity  => this.setSubtitleBackgroundOpacity(opacity);

        playerRoot.append(menuRoot); {
            menuRoot.append(menuTabs); {
                menuTabs.append(selectTab);
                menuTabs.append(searchTab);
                menuTabs.append(optionsTab);
            }
            menuRoot.append(menuSeparator);
            menuRoot.append(menuViews); {
                menuViews.append(selectView); {
                    selectView.append(subsSwitch.toggleRoot);
                    selectView.append(menu.trackList);
                }
                menuViews.append(searchView); {
                    searchView.append(subtitleImport)
                    searchView.append(subtitleName)
                    searchView.append(subtitleLanguage)
                    searchView.append(subtitleYear)
                    searchView.append(subtitleSeason)
                    searchView.append(subtitleEpisode)
                    searchView.append(searchSubtitle)
                }
                menuViews.append(optionsView); {
                    optionsView.append(subsShift.root);
                    optionsView.append(subsSize.root);
                    optionsView.append(subsPos.root);
                    optionsView.append(subsFgColor.root);
                    optionsView.append(subsFgOpacity.root);
                    optionsView.append(subsBgColor.root);
                    optionsView.append(subsBgOpacity.root);
                }
            }
        }
    }

    createSettingsMenu() {
        let playerRoot     = this.htmlPlayerRoot;
        let menu           = this.htmlControls.settings;
        let menuRoot       = menu.root;
        let menuSeparator  = newDiv(null, "player_menu_separator");
        let menuTabs       = newDiv(null, "player_menu_tabs");
        let menuViews      = newDiv(null, "player_menu_views");
        let generalTab     = newDiv(null, "player_menu_tab");
        let appearanceTab  = newDiv(null, "player_menu_tab");
        let generalView    = newDiv("player_submenu_select_view");
        let appearanceView = newDiv("player_submenu_select_view");
        let alwaysShow     = new Switcher("Always show controls");
        let showOnPause    = new Switcher("Show controls on pause");
        let playbackSpeed  = this.playbackSpeed;
        let brightness     = new Slider("Brightness", 0.2, 2, 0.05, 1.0);

        hide(menuRoot);
        alwaysShow.setState(this.options.alwaysShowControls);
        showOnPause.setState(this.options.showControlsOnPause);

        generalTab.textContent    = "General";
        appearanceTab.textContent = "Appearance";

        menu.selected.tab  = generalTab;
        menu.selected.view = generalView;

        menu.selected.tab.classList.add("player_menu_tab_selected");
        show(menu.selected.view);

        menuRoot.onclick = stopPropagation;

        let select = (tab, view) => {
            let selected = menu.selected;
            selected.tab.classList.remove("player_menu_tab_selected");
            hide(selected.view);

            selected.tab = tab;
            selected.view = view;

            selected.tab.classList.add("player_menu_tab_selected");
            show(selected.view);
        }

        generalTab.onclick     = () => select(generalTab, generalView);
        appearanceTab.onclick  = () => select(appearanceTab, appearanceView);

        menuRoot.onclick = stopPropagation;

        alwaysShow.onAction = state => {
            this.options.alwaysShowControls = state;
            this.fireSettingsChange(Options.ALWAYS_SHOW_CONTROLS, state);
        };

        showOnPause.onAction = state => {
            this.options.showControlsOnPause = state;
            this.fireSettingsChange(Options.SHOW_CONTROLS_ON_PAUSE, state);
        };

        playbackSpeed.onInput = value => {
            this.setSpeed(value);
            this.fireSettingsChange(Options.PLAYBACK_SPEED, value);
        };

        brightness.onInput = value => {
            this.htmlVideo.style.filter = "brightness(" + value + ")";
            this.fireSettingsChange(Options.BRIGHTNESS, value);
        };

        playerRoot.append(menuRoot); {
            menuRoot.append(menuTabs); {
                menuTabs.append(generalTab);
                menuTabs.append(appearanceTab);
            }
            menuRoot.append(menuSeparator);
            menuRoot.append(menuViews); {
                menuViews.append(generalView); {
                    generalView.append(alwaysShow.toggleRoot);
                    generalView.append(showOnPause.toggleRoot);
                    generalView.append(playbackSpeed.root);
                    generalView.append(brightness.root);
                }
                menuViews.append(appearanceView);
            }
        }
    }
}

class ColorPicker {
    constructor(textContent, hexColor = "#FFFFFF") {
        let root   = newDiv(null, "player_color_picker_root");
        let text   = newDiv(null, "player_color_picker_text");
        let picker = newElement("button", null, "player_color_picker_color");
        let input  = newElement("input", null, "player_color_picker_input");

        text.textContent = textContent;

        input.type  = "color";
        input.value = hexColor;

        picker.style.backgroundColor = hexColor;

        picker.onclick = _ => input.click();
        input.oninput  = _ => {
            this.picker.style.backgroundColor = this.input.value;
            this.onInput(this.input.value);
        }

        root.appendChild(text);
        root.appendChild(picker); {
            picker.appendChild(input);
        }

        this.root   = root;
        this.input  = input;
        this.picker = picker;
    }

    onInput(_hexColor) {}

    setValue(value) {
        this.input.value = value;
        this.picker.style.backgroundColor = value;
    }

    getValue() {
        return this.input.value;
    }
}

export class FileInfo {
    constructor(filename, extension) {
        this.filename = filename;
        this.extension = extension;
    }

    static fromUrl(url) {
        let slash = Math.max(url.lastIndexOf("/"), url.lastIndexOf("\\"));
        let filename = url.substring(slash + 1);
        let extension = filename.substring(filename.lastIndexOf(".") + 1).toLowerCase();
        return new FileInfo(filename, extension);
    }
}

class Slider {
    constructor(textContent, min, max, step, initialValue, valueSuffix = "", includeSign = false) {
        let root        = newDiv(null, "player_shifter_root");
        let top         = newDiv(null, "player_shifter_top");
        let text        = newElement("span", null, "player_shifter_text");
        let valueText   = newElement("span", null, "player_shifter_value");
        let bottom      = newDiv(null, "player_shifter_bottom");
        let leftButton  = newElement("button", null, "player_shifter_button");
        let arrowLeft   = Svg.new("svg/player_icons.svg#arrow_left", 20, 20);
        let slider      = newElement("input",  null, "player_shifter_slider");
        let rightButton = newElement("button", null, "player_shifter_button");
        let arrowRight  = Svg.new("svg/player_icons.svg#arrow_right", 20, 20);

        text.textContent = textContent;

        slider.type = "range";
        slider.min = min;
        slider.max = max;
        slider.step = step;

        rightButton.onclick = () => this.shift(step);
        slider.oninput      = () => this.shift(0.0);
        leftButton.onclick  = () => this.shift(-step);

        root.append(top); {
            top.append(text);
            top.append(valueText);
        }
        root.append(bottom); {
            bottom.append(leftButton); {
                leftButton.append(arrowLeft.svg);
            }
            bottom.append(slider);
            bottom.append(rightButton); {
                rightButton.append(arrowRight.svg);
            }
        }

        this.valueSuffix = valueSuffix;

        this.root        = root;
        this.slider      = slider;
        this.valueText   = valueText;
        this.includeSign = includeSign;

        this.precision   = Slider.calculatePrecision(step);
        this.setValue(initialValue);
    }

    static calculatePrecision(step) {
        let strNum = step.toString();
        let dot = strNum.indexOf(".");
        return dot === -1 ? 0 : (strNum.length - dot - 1);
    }

    createValueString(value) {
        let max = Number(this.slider.max);
        if (value > max) {
            value = max;
        }

        let min = Number(this.slider.min);
        if (value < min) {
            value = min;
        }

        value = Number(value).toFixed(this.precision);

        let valueString = "";
        if (this.includeSign && value > 0) {
            valueString = "+";
        }

        valueString += value;

        valueString += this.valueSuffix;
        return valueString;
    }

    onInput(_value) {}

    shift(step) {
        let value = Number(this.slider.value) + step;
        this.setValue(value);
        this.onInput(value);
    }

    setValue(value) {
        this.slider.value = value;
        this.valueText.textContent = this.createValueString(value);
    }

    getValue() {
        return Number(this.slider.value);
    }
}

class Switcher {
    constructor(text, initialState) {
        let toggleRoot   = newDiv(null, "player_toggle_root");
        let toggleText   = newDiv(null, "player_toggle_text");
        let toggleSwitch = newDiv(null, "player_toggle_switch");
        let toggleCircle = newDiv(null, "player_toggle_circle");

        toggleText.textContent = text;

        toggleSwitch.addEventListener("click", _ => {
            this.setState(!this.enabled);
            this.onAction(this.enabled);
        });

        toggleRoot.appendChild(toggleText);
        toggleRoot.appendChild(toggleSwitch); {
            toggleSwitch.appendChild(toggleCircle);
        }

        this.toggleRoot   = toggleRoot;
        this.toggleSwitch = toggleSwitch;
        this.setState(initialState);
    }

    // Changes both the real state and the UI state, for programmatic use to stay in sync with UI
    setState(state) {
        if (state) {
            this.enabled = true;
            this.toggleRoot.classList.add("player_toggle_on");
        } else {
            this.enabled = false;
            this.toggleRoot.classList.remove("player_toggle_on");
        }
    }

    onAction(_state) {}
}

class Cue {
    constructor(start, end, text) {
        this.startTime = start;
        this.endTime = end;
        this.text = text;
    }
    // if (!cue.sanitized) { cue.text = sanitizeHTMLForDisplay(cue.text); cue.sanitized = true; }
}

// Internal representation for a subtitle, a replacement for the TextTrack and <track>
export class Subtitle {
    constructor(cues, name, format, url, offset = 0.0) {
        this.cues = cues;
        this.name = name;
        this.format = format;
        this.url = url;
        this.offset = offset;
        this.htmlTrack = this.createSubtitleTrackElement(name);
    }

    createSubtitleTrackElement(title) {
        let track        = newDiv(null, "subtitle_track");
        let trackTitle   = newElement("input", null, "subtitle_track_text");
        // let trackButtons = newDiv(null, "subtitle_track_buttons");
        // let trackEdit    = newElement("button", null, "subtitle_track_edit_button")
        // let trackRemove  = newElement("button", null, "subtitle_track_remove_button")

        trackTitle.type = "text";
        trackTitle.value = title;
        trackTitle.readOnly = true;

        // trackEdit.textContent = "⚙️";
        // trackRemove.textContent = "🗑";

        track.appendChild(trackTitle);
        // track.appendChild(trackButtons); {
        //     trackButtons.appendChild(trackEdit);
        //     trackButtons.appendChild(trackRemove);
        // }

        return track;
    }
}

function areArraysEqual(arr1, arr2) {
    if (arr1.length !== arr2.length) return false;

    for (let i = 0; i < arr1.length; i++) {
        if (arr1[i] !== arr2[i]) {
            return false;
        }
    }
    return true;
}

// Use binary search to find the index of the cue that should be shown based on startTime only
function binarySearchForCue(time, cues) {
    let left = 0, right = cues.length - 1;
    while (left <= right) {
        let mid = ((left + right) / 2) | 0;
        if (cues[mid].startTime < time) {
            left = mid + 1;
        } else if(cues[mid].startTime > time) {
            right = mid - 1;
        } else {
            return mid;
        }
    }
    return ((left + right) / 2) | 0
}

// Use linear search to find the index of the cue that should be shown based on startTime only
function linearCueSearch(time, cues) {
    let len = cues.length;
    for (let i = 0; i < len; i++) {
        if (cues[i].startTime < time) {
            continue;
        }
        return i-1;
    }
}

// It's possible to attach inlined events to styling tags
function removeInlinedEvents(tag) {
    let attributes = tag.attributes;
    for (let i = 0; i < attributes.length; i++) {
        let attrib = attributes[i];
        if (attrib.name.indexOf("on") === 0) {
            tag.removeAttribute(attrib.name);
            console.warn("Removed", attrib.name, "during events sanitization.")
        }
    }
}

const ALLOWED_STYLE_TAGS = ["i", "b", "u", "ruby", "rt", "c", "v", "lang", "font"]
const PARSER = new DOMParser();
export function sanitizeHTMLForDisplay(html) {
    // Using a temporary element or document fragment preloads <img> tags causing onload to fire
    let sandbox = PARSER.parseFromString(html, "text/html").body;
    sanitizeChildren(sandbox);
    return sandbox.innerHTML;
}

// CSS injections through style attribute: <b style="background: url(https://example.com)">Bold url</b>
function sanitizeChildren(tag) {
    for (let i = 0; i < tag.children.length; i++) {
        let child = tag.children[i];
        let name = child.tagName.toLowerCase();
        if (ALLOWED_STYLE_TAGS.includes(name)) {
            removeInlinedEvents(child);
            child.removeAttribute("style");
            sanitizeChildren(child);
            continue;
        }
        console.warn("Removed", name, "during sanitization.")
        tag.removeChild(child);
        i--;
    }
}

function createTimestampString(timestamp) {
    if (!timestamp) {
        timestamp = 0.0;
    }

    let seconds = Math.floor(timestamp % 60.0);
    timestamp = timestamp / 60.0;
    let minutes = Math.floor(timestamp % 60.0);
    timestamp = timestamp / 60.0;
    let hours = Math.floor(timestamp % 60.0);

    let timestamp_string = "";
    if (hours > 0.0) {
        timestamp_string += hours;
        timestamp_string += ":";
    }

    if (minutes < 10) {
        timestamp_string += "0";
    }

    timestamp_string += minutes;
    timestamp_string += ":";

    if (seconds < 10) {
        timestamp_string += "0";
    }

    timestamp_string += seconds;
    return timestamp_string;
}

// Receives [subtitle file text, decimal separator ('.' = VTT ',' = SRT), skipCounter is true for SRT
function parseSubtitles(subtitleText, decimalMark, skipCounter, skipHeader) {
    let lines = subtitleText.split('\n');
    let cues = []
    let i = 0;
    if (skipHeader) i = 2;
    for (; i < lines.length-2; i++) {
        if (skipCounter) ++i;

        let timestamps = lines[i];
        let [start, end, ok] = parseTimestamps(timestamps, decimalMark)
        if (!ok) {
            continue;
        }
        if (++i >= lines.length) {
            return cues;
        }
        let content = lines[i].trim();
        while (++i < lines.length) {
            let text = lines[i].trim();
            if (text.length === 0) {
                break;
            }
            content += '\n';
            content += text;
        }
        if (content !== "") {
            let newCue = new Cue(start, end, content);
            cues.push(newCue);
        }
    }
    return cues;
}

function parseSrt(srtText) {
    return parseSubtitles(srtText, ',', true, false);
}

function parseVtt(vttText) {
    return parseSubtitles(vttText, '.', false, true);
}

// Returns [seconds start, seconds end, success]
function parseTimestamps(timestamps, decimalMark) {
    if (timestamps.length < 23) {
        return [null, null, false];
    }
    let splitter = timestamps.indexOf(" --> ", 8);
    if (splitter === -1) {
        return [null, null, false];
    }
    let startStamp = parseStamp(timestamps.substring(0, splitter), decimalMark);
    let endStamp = parseStamp(timestamps.substring(splitter+5), decimalMark);
    return [startStamp, endStamp, startStamp != null && endStamp != null];
}

// Returns a timestamp expressed in seconds or null on failure
function parseStamp(stamp, decimalMark) {
    let twoSplit = stamp.split(decimalMark);
    if (twoSplit.length !== 2) {
        return null;
    }
    let hms = twoSplit[0].split(':');
    switch (hms.length) {
        case 3:
            return hms[0] * 3600 + hms[1] * 60 + Number(hms[2]) + twoSplit[1] / 1000;
        case 2:
            return hms[0] * 60 + Number(hms[1]) + twoSplit[1] / 1000;
        default:
            return null;
    }
}

export class Search {
    // Expecting strings for now
    constructor(title, lang, year, season, episode) {
        this.title   = title;
        this.lang    = lang;
        this.year    = year;
        this.season  = season;
        this.episode = episode;
    }
}

function newDiv(id, className) {
    let div = document.createElement("div")
    // tabIndex makes divs focusable so that they can receive and bubble key events
    div.tabIndex = -1
    if (id) {
        div.id = id;
    }

    if (className) {
        div.className = className;
    }

    return div;
}

const SVG_NAMESPACE = "http://www.w3.org/2000/svg";

class SeekIcon {
    constructor(textContent, width=100, height=100) {
        this.svg = document.createElementNS(SVG_NAMESPACE, "svg");
        let svg = this.svg;
        svg.setAttribute("viewBox", "0 0 48 48");
        svg.setAttribute("width", width);
        svg.setAttribute("height", height);
        svg.innerHTML = `
             <path d="M24 4C12.97 4 4 12.97 4 24C4 35.03 12.97 44 24 44C35.03 44 44 35.03 44 24C44 23.83 43.998 23.66 43.994 23.49A1.5 1.5 0 0 0 40.994 23.56C40.998 23.71 41 23.85 41 24C41 33.41 33.41 41 24 41C14.59 41 7 33.41 7 24C7 14.59 14.59 7 24 7C29.38 7 34.16 9.5 37.27 13.38L35.96 13.15A1.5 1.5 0 0 0 35.44 16.11L40.37 16.98A1.5 1.5 0 0 0 42.11 15.76L42.97 10.84A1.5 1.5 0 0 0 41.44 9.06A1.5 1.5 0 0 0 40.02 10.32L39.77 11.72C36.11 7.03 30.4 4 24 4z"/>
             <text x="24" y="24" text-anchor="middle" font-weight="bold" font-size="17" dy=".35em">10s</text>
        `;
        let children = svg.children;
        this.path = children[0];
        this.text = children[1];
        this.setText(textContent)
    }
    static newForward(textContent = "", width, height) {
        return new SeekIcon(textContent, width, height);
    }
    static newBackward(textContent = "", width, height) {
        let seekBackward = new SeekIcon(textContent, width, height);
        seekBackward.path.setAttribute("transform", "scale(-1, 1) translate(-48, 0)");
        return seekBackward;
    }
    setText(text) {
        this.text.textContent = text;
    }
    getText() {
        return this.text.textContent;
    }
}

class Svg {
    constructor(svg, use) {
        this.svg = svg;
        this.use = use;
    }

    setHref(href) {
        this.use.setAttribute("href", href)
    }

    static new(initialHref, width=20, height=20) {
        let svg = document.createElementNS(SVG_NAMESPACE, "svg");
        let use = document.createElementNS(SVG_NAMESPACE, "use");
        use.setAttribute("href", initialHref);

        svg.setAttribute("width", width);
        svg.setAttribute("height", height);
        svg.appendChild(use);
        return new Svg(svg, use);
    }
}

function newElement(tag, id, className) {
    let element = document.createElement(tag);

    if (id) {
        element.id = id;
    }

    if (className) {
        element.className = className;
    }

    return element;
}

function consumeEvent(event) {
    event.stopPropagation();
    event.preventDefault();
}

function stopPropagation(event) {
    event.stopPropagation();
}

function isFunction(func) {
    return func != null && typeof func === "function";
}

function makeRgba(hexColor, opacity) {
    let redHex   = hexColor.substring(1, 3);
    let greenHex = hexColor.substring(3, 5);
    let blueHex  = hexColor.substring(5, 7);

    let red   = parseInt(redHex,   16);
    let green = parseInt(greenHex, 16);
    let blue  = parseInt(blueHex,  16);

    opacity /= 100.0;
    let rgba = `rgba(${red}, ${green}, ${blue}, ${opacity})`;
    return rgba;
}


function addOpacityToColor(hexColor, opacity) {
    let byteOpacity = Math.floor(opacity / 100.0 * 255);
    let hexOpacity  = byteOpacity.toString(16)
    return hexColor + hexOpacity;
}

// For example: Linux cannot be included as a desktop agent because it also appears along Android
// Similarly: Macintosh cannot be included as a desktop agent because it also appears along iPad
// What about TVs?
const MOBILE_AGENTS = ["Mobile", "Tablet", "Android", "iPhone", "iPod", "iPad"];
function isMobileAgent() {
    let userAgent = navigator.userAgent.trim();
    if (!userAgent || userAgent === "") {
        return false;
    }

    let bracketOpen = userAgent.indexOf("(");
    if (bracketOpen === -1) {
        return false;
    }

    let bracketClose = userAgent.indexOf(")", bracketOpen + 1);
    if (bracketClose === -1) {
        return false;
    }

    let systemInfo = userAgent.substring(bracketOpen + 1, bracketClose).trim();
    for (let i = 0; i < systemInfo.length; i++) {
        if (systemInfo.includes(MOBILE_AGENTS[i])) {
            return true;
        }
    }

    return false;
}

// This is a separate class for more clarity
class Options {
    constructor() {
        this.hidePlaybackButton   = false;
        this.hideNextButton       = false;
        this.hideVolumeButton     = false;
        this.hideVolumeSlider     = false;
        this.hideTimestamps       = false;
        this.hideDownloadButton   = false;
        this.hideSpeedButton      = false;
        this.hideSubtitlesButton  = false;
        this.hideSettingsButton   = false;
        this.hideFullscreenButton = false;

        let isMobile = isMobileAgent();
        this.doubleClickThresholdMs = 250;
        this.seekStackingThresholdMs = 250;
        this.enableDoubleTapSeek = isMobile;
        this.sanitizeSubtitles = true;
        this.allowCueOverlap = true;
        this.fullscreenKeyLetter = 'f';
        // Max volume will be respected only if audio gain is enabled
        this.useAudioGain = false;
        this.maxVolume = 1;

        // [Arrow keys/Double tap] seeking offset provided in seconds. (Preferably [1-99])
        this.seekBy = 5;

        // Delay in milliseconds before controls disappear.
        this.inactivityTime = 2500;

        // Disable the auto hide for player controls.
        this.alwaysShowControls  = false;
        this.showControlsOnPause = true;

        this.bufferingRedrawInterval = 1000;
        this.hlsConfig = {
            // If these controllers are used, they'll clear tracks or cues when HLS is attached/detached.
            // HLS does not provide a way to make it optional, therefore we don't want HLS to mess with
            // our subtitle tracks, handling it would require hacky solutions or modifying HLS source code
            timelineController: null,
            subtitleTrackController: null,
            subtitleStreamController: null,
        }
    }

    // Ensure values are the intended type and within some reasonable range
    valid() {
        if (typeof this.seekBy !== "number" || this.seekBy < 0) {
            return false;
        }

        if (typeof this.maxVolume !== "number" || this.maxVolume < 0.1 || this.maxVolume > 10) {
            return false;
        }

        if (typeof this.inactivityTime !== "number" || this.inactivityTime < 0) {
            return false;
        }

        if (typeof this.bufferingRedrawInterval !== "number") {
            return false;
        }

        return true;
    }

    // Constants
    static ALWAYS_SHOW_CONTROLS        = "always_show_controls";
    static SHOW_CONTROLS_ON_PAUSE      = "show_controls_on_pause";
    static BRIGHTNESS                  = "brightness";
    static PLAYBACK_SPEED              = "playback_speed";
    static SUBTITLES_ENABLED           = "subtitles_enabled";
    static SUBTITLE_FONT_SIZE          = "subtitle_font_size";
    static SUBTITLE_VERTICAL_POSITION  = "subtitle_vertical_position";
    static SUBTITLE_FOREGROUND_COLOR   = "subtitle_foreground_color";
    static SUBTITLE_FOREGROUND_OPACITY = "subtitle_foreground_opacity";
    static SUBTITLE_BACKGROUND_COLOR   = "subtitle_background_color";
    static SUBTITLE_BACKGROUND_OPACITY = "subtitle_background_opacity";
}

// Throttling scheduler. After the specified delay elapses, the scheduled action will be executed.
// Although if it's scheduled again the previous timer is always cancelled and a new one is started.
export class Timeout {
    constructor(action, delayMs) {
        this.setAction(action);
        this.delay = delayMs;
        this.timeoutId = 0;
    }
    schedule() {
        this.cancel();
        this.timeoutId = setTimeout(() => {
            this.action();
            this.timeoutId = 0;
        }, this.delay);
    }
    cancel() {
        if (this.timeoutId > 0) {
            clearTimeout(this.timeoutId);
            this.timeoutId = 0;
        }
    }
    inProgress() {
        return this.timeoutId > 0;
    }
    setAction(action) {
        this.action = action;
    }
    setDelay(ms) {
        this.delay = ms;
    }
}
