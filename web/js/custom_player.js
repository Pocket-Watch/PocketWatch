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

    setToast(toast) {
        this.internals.setToast(toast);
    }

    getLoop() {
        return this.internals.loopEnabled;
    }

    setLoop(enabled) {
        this.internals.setLoop(enabled);
    }

    setAutoplay(enabled) {
        this.internals.setAutoplay(enabled);
    }

    getAutoplay() {
        return this.internals.autoplayEnabled;
    }

    getCurrentTime() {
        return this.internals.htmlVideo.currentTime;
    }

    getDuration() {
        return this.internals.htmlVideo.duration;
    }

    addSubtitleTrack(subtitleUrl) {
        if (subtitleUrl.endsWith(".srt")) {
            return this.internals.addSrtTrack(subtitleUrl, false);
        } else if (subtitleUrl.endsWith(".vtt")) {
            return this.internals.addVttTrack(subtitleUrl, false);
        }
    }

    // Adds a new subtitle track in the 'showing' mode, hiding the previous track.
    setVttTrack(subtitleUrl) {
        this.internals.addVttTrack(subtitleUrl, true);
    }

    // Adds a new subtitle track in the 'hidden' mode.
    addVttTrack(subtitleUrl) {
        return this.internals.addVttTrack(subtitleUrl, false);
    }

    setSrtTrack(subtitleUrl) {
        return this.internals.addSrtTrack(subtitleUrl, true);
    }

    addSrtTrack(subtitleUrl) {
        return this.internals.addSrtTrack(subtitleUrl, false);
    }

    // Disables and removes the track at the specified index.
    removeSubtitleTrackAt(index) {
        this.internals.removeSubtitleTrackAt(index);
    }

    // Hides the previously selected track. Shows the track at the specified index.
    enableSubtitleTrackAt(index) {
        this.internals.enableSubtitleTrackAt(index);
    }

    // The seconds argument is a double, negative shifts back, positive shifts forward
    shiftCurrentSubtitleTrackBy(seconds) {
        return this.internals.shiftCurrentSubtitleTrackBy(seconds)
    }

    destroyPlayer() {}

    onControlsPlay(func) {
        if (!isFunction(func)) {
            return;
        }
        this.internals.fireControlsPlay = func;
    }

    onControlsPause(func) {
        if (!isFunction(func)) {
            return;
        }
        this.internals.fireControlsPause = func;
    }

    onControlsNext(func) {
        if (!isFunction(func)) {
            return;
        }
        this.internals.fireControlsNext = func;
    }

    onControlsLoop(func) {
        if (!isFunction(func)) {
            return;
        }
        this.internals.fireControlsLoop = func;
    }

    onControlsAutoplay(func) {
        if (!isFunction(func)) {
            return;
        }
        this.internals.fireControlsAutoplay = func;
    }

    onControlsSeeking(func) {
        if (!isFunction(func)) {
            return;
        }
        this.internals.fireControlsSeeking = func;
    }

    onControlsSeeked(func) {
        if (!isFunction(func)) {
            return;
        }
        this.internals.fireControlsSeeked = func;
    }

    onControlsVolumeSet(func) {
        if (!isFunction(func)) {
            return;
        }
        this.internals.fireControlsVolumeSet = func;
    }

    onPlaybackError(func) {
        if (!isFunction(func)) {
            return;
        }
        this.internals.firePlaybackError = func;
    }

    onPlaybackEnd(func) {
        if (!isFunction(func)) {
            return;
        }
        this.internals.firePlaybackEnd = func;
    }

    onSubtitleTrackLoad(func) {
        if (!isFunction(func)) {
            return;
        }
        this.internals.fireSubtitleTrackLoad = func;
    }

    setVideoTrack(url) {
        this.internals.setVideoTrack(url);
    }
}

function hideElement(element) {
    element.style.display = "none";
}

class Internals {
    constructor(videoElement, options) {
        let initStart = performance.now();
        this.isMobile = isMobileAgent();
        this.options = options;

        this.hls = null;
        this.playingHls = false;

        this.loopEnabled = false;
        this.autoplayEnabled = false;

        this.htmlVideo = videoElement;
        this.htmlVideo.disablePictureInPicture = true;
        this.htmlVideo.controls = false;

        // Div container where either the player or the placeholder resides.
        this.htmlPlayerRoot = newDiv("player_container");

        // We actually need to append the <div> to document.body (or <video>'s parent)
        // otherwise the <video> tag will disappear entirely!
        let videoParent = this.htmlVideo.parentNode;
        videoParent.appendChild(this.htmlPlayerRoot);
        this.htmlPlayerRoot.appendChild(this.htmlVideo);

        this.htmlTitleContainer = newDiv("player_title_container");
        hideElement(this.htmlTitleContainer);
        this.htmlPlayerRoot.appendChild(this.htmlTitleContainer);

        this.htmlTitle = newElement("span", "player_title_text");
        this.htmlTitleContainer.appendChild(this.htmlTitle);

        this.htmlToastContainer = newDiv("player_toast_container");
        hideElement(this.htmlToastContainer);
        this.htmlPlayerRoot.appendChild(this.htmlToastContainer);
        this.htmlToast = newElement("span", "player_toast_text");
        this.htmlToastContainer.appendChild(this.htmlToast);

        this.icons = {
            play:             "svg/player_icons.svg#play",
            play_popup:       "svg/player_icons.svg#play_popup",
            pause:            "svg/player_icons.svg#pause",
            pause_popup:      "svg/player_icons.svg#pause_popup",
            replay:           "svg/player_icons.svg#replay",
            next:             "svg/player_icons.svg#next",
            loop:             "svg/player_icons.svg#loop",
            volume_full:      "svg/player_icons.svg#volume_full",
            volume_medium:    "svg/player_icons.svg#volume_medium",
            volume_low:       "svg/player_icons.svg#volume_low",
            volume_muted:     "svg/player_icons.svg#volume_muted",
            download:         "svg/player_icons.svg#download",
            autoplay:         "svg/player_icons.svg#autoplay",
            subs:             "svg/player_icons.svg#subs",
            settings:         "svg/player_icons.svg#settings",
            fullscreen_enter: "svg/player_icons.svg#fullscreen_enter",
            fullscreen_exit:  "svg/player_icons.svg#fullscreen_exit",
            arrow_left:       "svg/player_icons.svg#arrow_left",
            arrow_right:      "svg/player_icons.svg#arrow_right",
            seek_forward:     "svg/player_icons.svg#seek_forward",
            seek_backward:    "svg/player_icons.svg#seek_backward",
            buffering:        "svg/player_icons.svg#buffering",
        };

        this.svgs = {
            playback:   Svg.new(this.icons.play),
            next:       Svg.new(this.icons.next),
            loop:       Svg.new(this.icons.loop),
            volume:     Svg.new(this.icons.volume_full),
            download:   Svg.new(this.icons.download),
            autoplay:   Svg.new(this.icons.autoplay),
            subs:       Svg.new(this.icons.subs),
            settings:   Svg.new(this.icons.settings),
            fullscreen: Svg.new(this.icons.fullscreen_enter),

            seekForward:   Svg.new(this.icons.seek_forward, 100, 100),
            seekBackward:  Svg.new(this.icons.seek_backward, 100, 100),
            playbackPopup: Svg.new(this.icons.play_popup, 70, 70),

            arrowLeft:  Svg.new(this.icons.arrow_left, 20, 20),
            arrowRight: Svg.new(this.icons.arrow_right, 20, 20),

            buffering: Svg.new(this.icons.buffering, 70, 70),
        };

        this.bufferingSvg = this.svgs.buffering.svg;
        this.bufferingSvg.id = "player_buffering";
        hideElement(this.bufferingSvg);
        this.htmlPlayerRoot.appendChild(this.bufferingSvg);

        this.playbackPopupSvg = this.svgs.playbackPopup.svg;
        this.playbackPopupSvg.id = "player_playback_popup";
        this.htmlPlayerRoot.appendChild(this.playbackPopupSvg);

        this.htmlControls = {
            root: newDiv("player_controls"),
            progress: {
                root:      newDiv("player_progress_root"),
                current:   newDiv("player_progress_current", "player_progress_bar"),
                buffered:  newElement("canvas", "player_progress_buffered", "player_progress_bar"),
                total:     newDiv("player_progress_total", "player_progress_bar"),
                thumb:     newDiv("player_progress_thumb"),
                popupRoot: newDiv("player_progress_popup_root"),
                popupText: newDiv("player_progress_popup_text"),
            },

            buttons: {
                root:             newDiv("player_control_buttons"),
                playbackButton:   newDiv(null, "player_controls_button"),
                nextButton:       newDiv(null, "player_controls_button"),
                loopButton:       newDiv(null, "player_controls_button"),
                volumeButton:     newDiv(null, "player_controls_button"),
                downloadButton:   newDiv(null, "player_controls_button"),
                autoplayButton:   newDiv(null, "player_controls_button"),
                subsButton:       newDiv(null, "player_controls_button"),
                settingsButton:   newDiv(null, "player_controls_button"),
                fullscreenButton: newDiv(null, "player_controls_button"),

                volumeProgress: newDiv("player_volume_progress"),
                volumeInput:    newElement("input", "player_volume_input"),
                timestamp:      newElement("span",  "player_timestamp"),
            },

            subMenu: {
                root: newDiv("player_submenu_root"),

                selected: {
                    button: null,
                    bottom: null,
                    track:  null,
                },

                tabs: {
                    selectButton:  newDiv("player_submenu_select_button"),
                    searchButton:  newDiv("player_submenu_search_button"),
                    optionsButton: newDiv("player_submenu_options_button"),
                },

                bottom: {
                    selectRoot:  newDiv("player_submenu_bottom_select"),
                    searchRoot:  newDiv("player_submenu_bottom_search"),
                    optionsRoot: newDiv("player_submenu_bottom_options"),
                },

                /// Part of the bottom selection panel, html track elements are appended here.
                trackList: newDiv("subtitle_track_list"),
            },

            settings: {
                root: newDiv("player_settings_root"),
            }
        };

        this.isDraggingProgressBar = false;
        this.isUIVisible = true;
        this.volumeBeforeMute = 0.0;
        this.selectedSubtitleIndex = -1;

        this.subsSwitcher = null;

        this.htmlSeekForward = newDiv("player_forward_container");
        this.htmlSeekForward.appendChild(this.svgs.seekForward.svg);
        this.htmlPlayerRoot.appendChild(this.htmlSeekForward);

        this.htmlSeekBackward = newDiv("player_backward_container");
        this.htmlSeekBackward.appendChild(this.svgs.seekBackward.svg);
        this.htmlPlayerRoot.appendChild(this.htmlSeekBackward);


        this.createHtmlControls();
        this.attachHtmlEvents();

        setInterval(() => this.redrawBufferedBars(), this.options.bufferingRedrawInterval);
        let end = performance.now();
        console.log("Internals constructor finished in", end-initStart, "ms")

        this.setVolume(1.0);
    }

    fireControlsPlay() {}
    fireControlsPause() {}
    fireControlsNext() {}
    fireControlsLoop(_enabled) {}
    fireControlsAutoplay(_enabled) {}
    fireControlsSeeking(_timestamp) {}
    fireControlsSeeked(_timestamp) {}
    fireControlsVolumeSet(_volume) {}
    firePlaybackError(_event) {}
    firePlaybackEnd() {}
    fireSubtitleTrackLoad(_event) {}

    isVideoPlaying() {
        return !this.htmlVideo.paused && !this.htmlVideo.ended;
    }

    play() {
        if (this.isVideoPlaying()) {
            return;
        }

        this.svgs.playbackPopup.setHref(this.icons.play_popup);
        this.playbackPopupSvg.classList.add("animate");
        this.svgs.playback.setHref(this.icons.pause);
        this.htmlVideo.play().catch(e => {
            this.firePlaybackError(e);
        });
    }

    pause() {
        if (!this.isVideoPlaying()) {
            return;
        }

        this.svgs.playbackPopup.setHref(this.icons.pause_popup);
        this.playbackPopupSvg.classList.add("animate");
        this.svgs.playback.setHref(this.icons.play);
        this.htmlVideo.pause();
    }

    seek(timestamp) {
        if (isNaN(timestamp)) {
            return
        }
        if (this.isVideoPlaying()) {
            this.svgs.playback.setHref(this.icons.pause);
        } else {
            this.svgs.playback.setHref(this.icons.play);
        }
        this.htmlVideo.currentTime = timestamp;
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

    updateProgressPopup(progress) {
        let timestamp = this.htmlVideo.duration * progress;
        this.htmlControls.progress.popupText.textContent = createTimestampString(timestamp);

        const popup = this.htmlControls.progress.popupRoot;
        const popupWidth = popup.clientWidth;
        const rootWidth = this.htmlControls.progress.root.clientWidth;

        let position = rootWidth * progress - popupWidth / 2.0;

        if (position < 0) {
            position = 0;
        } else if (position + popupWidth > rootWidth) {
            position = rootWidth - popupWidth;
        }

        this.htmlControls.progress.popupRoot.style.left = position + "px";
    }

    updateHtmlVolume(volume) {
        if (volume > 1.0) {
            volume = 1.0;
        }

        if (volume < 0.0) {
            volume = 0.0;
        }

        if (volume == 0.0) {
            this.svgs.volume.setHref(this.icons.volume_muted);
        } else if (volume < 0.3) {
            this.svgs.volume.setHref(this.icons.volume_low);
        } else if (volume < 0.6) {
            this.svgs.volume.setHref(this.icons.volume_medium);
        } else {
            this.svgs.volume.setHref(this.icons.volume_full);
        }

        let input = this.htmlControls.buttons.volumeInput;
        input.value = volume;

        let progress = this.htmlControls.buttons.volumeProgress;
        progress.style.width = volume * 100.0 + "%";
    }

    getNewTime(timeOffset) {
        let timestamp = this.htmlVideo.currentTime + timeOffset;
        if (timestamp < 0) {
            timestamp = 0;
        }
        return timestamp;
    }

    setVolume(volume) {
        if (volume > 1.0) {
            volume = 1.0;
        }

        if (volume < 0.0) {
            volume = 0.0;
        }

        this.htmlVideo.volume = volume;
        this.updateHtmlVolume(volume);
    }

    // TODO(kihau): Non linear scaling?
    setVolumeRelative(volume) {
        this.setVolume(this.htmlVideo.volume + volume);
    }

    setTitle(title) {
        if (!title) {
            hideElement(this.htmlTitleContainer);
        } else {
            this.htmlTitleContainer.style.display = "";
            this.htmlTitle.textContent = title;
        }
    }

    setToast(toast) {
        this.htmlToast.textContent = toast;
        this.htmlToastContainer.classList.remove("player_ui_hide");
        this.htmlToastContainer.style.display = "flex";

        clearTimeout(this.playerHideToastTimeoutId);
        this.playerHideToastTimeoutId = setTimeout(() => {
            this.htmlToastContainer.classList.add("player_ui_hide");
        }, 3000);
    }

    setLoop(enabled) {
        this.loopEnabled = enabled;
        let loop = this.htmlControls.buttons.loopButton;
        if (enabled) {
            loop.classList.add("player_controls_button_selected");
        } else {
            loop.classList.remove("player_controls_button_selected");
        }
    }

    setAutoplay(enabled) {
        this.autoplayEnabled = enabled;
        let autoplay = this.htmlControls.buttons.autoplayButton;
        if (enabled) {
            autoplay.classList.add("player_controls_button_selected");
        } else {
            autoplay.classList.remove("player_controls_button_selected");
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
            console.debug("Failed to set a new URL. It's not parsable.")
            // We should probably inform the user about the error either via debug log or return false
            return
        }
        // This covers both relative and fully qualified URLs because we always specify the base
        // and when the base is not provided, the second argument is used to construct a valid URL
        let pathname = new URL(url, document.baseURI).pathname;

        this.seek(0);

        if (pathname.endsWith(".m3u8") || pathname.endsWith(".ts")) {
            import("../external/hls.js").then(module => {
                if (module.Hls.isSupported()) {
                    if (this.hls == null) {
                        this.hls = new module.Hls({
                            // If these controllers are used, they'll clear tracks or cues when HLS is attached/detached.
                            // HLS does not provide a way to make it optional, therefore we don't want HLS to mess with
                            // our subtitle tracks, handling it would require hacky solutions or modifying HLS source code
                            timelineController: null,
                            subtitleTrackController: null,
                            subtitleStreamController: null,
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
            }
            this.htmlVideo.src = url;
            this.htmlVideo.load();
        }
    }

    addSrtTrack(url, show, trackInfo) {
        if (!trackInfo) {
            trackInfo = TrackInfo.fromUrl(url)
        }
        fetch(url)
            .then(response => response.text())
            .then(srtText => parseSrt(srtText))
            .then(cues => {
                if (cues.length === 0) {
                    return
                }
                console.info("Parsed SRT track, cue count:", cues.length)
                // addTextTrack must be used or otherwise track.cues.length will stay 0 on Chromium-based browsers
                let newTrack = this.htmlVideo.addTextTrack("subtitles", trackInfo.filename);
                let newIndex = this.htmlVideo.textTracks.length - 1;
                newTrack.mode = "hidden";
                cues.forEach(cue => {
                    newTrack.addCue(cue);
                });

                if (show) {
                    this.enableSubtitleTrackAt(newIndex);
                }
                URL.revokeObjectURL(url)
                this.fireSubtitleTrackLoad(newTrack);

                let trackList = this.htmlControls.subMenu.trackList;
                let htmlTrack = this.createSubtitleTrackElement(trackInfo.filename, newIndex);
                trackList.appendChild(htmlTrack);
            });
    }

    addVttTrack(url, show, info) {
        if (!info) {
            info = TrackInfo.fromUrl(url)
        }
        if (info.extension !== "vtt") {
            console.debug("Unsupported subtitle extension:", info.extension)
            return
        }

        let track = document.createElement("track")
        track.label = info.filename
        track.kind = "subtitles"
        track.src = url

        // This will cause a new text track to appear in video.textTracks even if it's invalid
        this.htmlVideo.appendChild(track)

        let textTracks = this.htmlVideo.textTracks;
        let newIndex = textTracks.length - 1;
        let newTrack = textTracks[newIndex];
        // By default, every track is appended in the 'disabled' mode which prevents any initialization
        newTrack.mode = "hidden";
        if (show) {
            this.enableSubtitleTrackAt(newIndex);
        }

        // Although we cannot access cues immediately here (not loaded yet)
        // we do have access to the textTrack so it's possible to change its mode
        track.addEventListener("load", (event) => {
            URL.revokeObjectURL(url)
            this.fireSubtitleTrackLoad(event);
            console.info("Text track loaded successfully", event.target)

            let trackList = this.htmlControls.subMenu.trackList;
            let htmlTrack = this.createSubtitleTrackElement(info.filename, newIndex);
            trackList.appendChild(htmlTrack);
        });
    }

    enableSubtitleTrackAt(index) {
        let textTracks = this.htmlVideo.textTracks;
        let previous = this.selectedSubtitleIndex;
        if (previous !== index && 0 <= previous && previous < textTracks.length) {
            textTracks[previous].mode = "hidden";
        }
        if (0 <= index && index < textTracks.length) {
            textTracks[index].mode = "showing";
            this.selectedSubtitleIndex = index;
            this.subsSwitcher.setState(true)
        }
    }

    // INTERNAL ONLY: Switch subtitle track and respect the current visibility setting
    switchSubtitleTrack(index) {
        let textTracks = this.htmlVideo.textTracks;
        let current = this.selectedSubtitleIndex;

        if (0 <= current && current < textTracks.length) {
            textTracks[current].mode = "hidden";
        }
        if (index < 0 || textTracks.length <= index) {
            return;
        }

        this.selectedSubtitleIndex = index;
        if (this.subsSwitcher.enabled) {
            textTracks[index].mode = "showing";
        }
    }

    // Returns the number of cues shifted, it's possible to call this method when the cues are not yet loaded returning 0
    shiftCurrentSubtitleTrackBy(seconds) {
        let index = this.selectedSubtitleIndex;
        let textTracks = this.htmlVideo.textTracks;
        if (index < 0 || index >= textTracks.length) {
            return 0;
        }

        let track = textTracks[index];

        let shifted = 0;
        let cues = track.cues;
        // Whenever cues timings are changed they're reordered by the runtime so they're always sorted increasingly
        // This happens during iteration, as a result the same cue may be shifted twice and some cues are skipped entirely
        if (seconds > 0) {
            for (let i = cues.length - 1; i >= 0; i--) {
                cues[i].endTime += seconds;
                cues[i].startTime += seconds;
                shifted++;
            }
        } else if (seconds < 0) {
            for (let i = 0; i < cues.length; i++) {
                cues[i].startTime += seconds;
                cues[i].endTime += seconds;
                shifted++;
            }
        }

        return shifted;
    }

    removeSubtitleTrackAt(index) {
        let textTracks = this.htmlVideo.textTracks;
        if (index < 0 || index >= textTracks.length) {
            return;
        }
        textTracks[index].mode = "disabled";
        let tracks = this.htmlVideo.getElementsByTagName("track");
        this.htmlVideo.removeChild(tracks[index]);
        // Index-tracking mechanism
        if (index < this.selectedSubtitleIndex) {
            this.selectedSubtitleIndex--;
        }
    }

    showPlayerUI() {
        this.htmlPlayerRoot.style.cursor = "auto";
        this.htmlControls.root.classList.remove("player_ui_hide");
        this.htmlTitleContainer.classList.remove("player_ui_hide");
    }

    hidePlayerUI() {
        if (this.options.disableControlsAutoHide) {
            return;
        }

        if (!this.isVideoPlaying() && this.options.showControlsOnPause) {
            return;
        }

        this.htmlPlayerRoot.style.cursor = "none";
        this.htmlControls.root.classList.add("player_ui_hide");
        this.htmlTitleContainer.classList.add("player_ui_hide");
    }

    resetPlayerUIHideTimeout() {
        clearTimeout(this.playerUIHideTimeoutID);
        this.playerUIHideTimeoutID = setTimeout(() => {
            this.hidePlayerUI();
        }, this.options.inactivityTime);
    }

    redrawBufferedBars() {
        const context = this.htmlControls.progress.buffered.getContext("2d");
        context.fillStyle = "rgb(204, 204, 204, 0.5)";

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

    attachHtmlEvents() {
        this.htmlSeekBackward.addEventListener("dblclick", (e) => {
            if (!this.options.enableDoubleTapSeek) {
                return;
            }

            this.htmlSeekBackward.classList.add("animate");
            let timestamp = this.getNewTime(-this.options.seekBy);
            this.fireControlsSeeked(timestamp);
            this.seek(timestamp);
            consumeClick(e);
        });

        this.htmlSeekForward.addEventListener("dblclick", (e) => {
            if (!this.options.enableDoubleTapSeek) {
                return;
            }

            this.htmlSeekForward.classList.add("animate");
            let timestamp = this.getNewTime(this.options.seekBy);
            this.fireControlsSeeked(timestamp);
            this.seek(timestamp);
            consumeClick(e);
        });

        // Prevents selecting the video element along with the rest of the page
        this.htmlVideo.classList.add("unselectable");

        this.htmlPlayerRoot.addEventListener("touchmove", () => {
            this.showPlayerUI();
            this.resetPlayerUIHideTimeout();
        });

        this.htmlPlayerRoot.addEventListener("mousemove", () => {
            this.showPlayerUI();
            this.resetPlayerUIHideTimeout();
        });

        this.htmlPlayerRoot.addEventListener("mousedown", () => {
            this.showPlayerUI();
            this.resetPlayerUIHideTimeout();
        });

        this.htmlPlayerRoot.addEventListener("mouseup", () => {
            this.showPlayerUI();
            this.resetPlayerUIHideTimeout();
        });

        this.htmlPlayerRoot.addEventListener("mouseenter", () => {
            this.showPlayerUI();
            this.resetPlayerUIHideTimeout();
        });

        this.htmlPlayerRoot.addEventListener("mouseleave", () => {
            this.hidePlayerUI();
        });

        this.htmlControls.buttons.playbackButton.addEventListener("click", () => {
            this.togglePlayback();
        });

        this.htmlControls.buttons.nextButton.addEventListener("click", () => {
            this.fireControlsNext();
        });

        this.htmlControls.buttons.loopButton.addEventListener("click", () => {
            this.loopEnabled = !this.loopEnabled;
            this.fireControlsLoop(this.loopEnabled);
            this.htmlControls.buttons.loopButton.classList.toggle("player_controls_button_selected");
        });

        this.htmlControls.buttons.autoplayButton.addEventListener("click", () => {
            this.autoplayEnabled = !this.autoplayEnabled;
            this.fireControlsAutoplay(this.autoplayEnabled);
            this.htmlControls.buttons.autoplayButton.classList.toggle("player_controls_button_selected");
        });

        this.htmlControls.buttons.volumeButton.addEventListener("click", () => {
            let slider = this.htmlControls.buttons.volumeInput;
            if (slider.value == 0) {
                this.fireControlsVolumeSet(this.volumeBeforeMute);
                this.setVolume(this.volumeBeforeMute);
            } else {
                this.volumeBeforeMute = slider.value;
                this.fireControlsVolumeSet(0);
                this.setVolume(0);
            }
        });

        this.htmlControls.buttons.subsButton.addEventListener("click", () => {
            hideElement(this.htmlControls.settings.root);

            let menuRootElement = this.htmlControls.subMenu.root;
            let visible = menuRootElement.style.display !== "none";
            if (visible) {
                hideElement(menuRootElement);
            } else {
                menuRootElement.style.display = "";
            }
        });

        this.htmlControls.buttons.settingsButton.addEventListener("click", () => {
            hideElement(this.htmlControls.subMenu.root);

            let root = this.htmlControls.settings.root;
            let visible = root.style.display !== "none";
            if (visible) {
                hideElement(root);
            } else {
                root.style.display = "";
            }
        });

        this.htmlPlayerRoot.addEventListener("keydown", (event) => {
            if (event.key == " " || event.code == "Space" || event.keyCode == 32) {
                this.togglePlayback();
                consumeEvent(event);
            }

            if (event.key == "ArrowLeft" || event.keyCode == 37) {
                this.htmlSeekBackward.classList.add("animate");

                let timestamp = this.getNewTime(-this.options.seekBy);
                this.fireControlsSeeked(timestamp);
                this.seek(timestamp);
                consumeEvent(event);
            }

            if (event.key == "ArrowRight" || event.keyCode == 39) {
                this.htmlSeekForward.classList.add("animate");

                // We should use options here
                let timestamp = this.getNewTime(this.options.seekBy);
                this.fireControlsSeeked(timestamp);
                this.seek(timestamp);
                consumeEvent(event);
            }

            if (event.key == "ArrowUp" || event.keyCode == 38) {
                this.setVolumeRelative(0.1);
                consumeEvent(event);
            }

            if (event.key == "ArrowDown" || event.keyCode == 40) {
                this.setVolumeRelative(-0.1);
                consumeEvent(event);
            }
        });

        this.htmlPlayerRoot.addEventListener("click", event => {
            if (event.pointerType === "touch" || event.pointerType === "pen") {
                if (!this.isUIVisible) {
                    return;
                }
            }
            this.togglePlayback();
        });

        this.htmlVideo.addEventListener("waiting", () => {
            this.bufferingTimeoutId = setTimeout(() => {
            this.bufferingSvg.style.display = "";
            }, 200);
        });

        this.htmlVideo.addEventListener("playing", () => {
            clearTimeout(this.bufferingTimeoutId);
            hideElement(this.bufferingSvg);
        });

        this.htmlVideo.addEventListener("timeupdate", (_event) => {
            let timestamp = this.htmlVideo.currentTime;
            this.updateTimestamps(timestamp);
        });

        this.htmlVideo.addEventListener("ended", (_event) => {
            this.svgs.playback.setHref(this.icons.replay)
            this.firePlaybackEnd();
        });

        this.htmlControls.buttons.fullscreenButton.addEventListener("click", () => {
            if (document.fullscreenElement) {
                document.exitFullscreen();
                this.svgs.fullscreen.setHref(this.icons.fullscreen_enter);
            } else {
                this.htmlPlayerRoot.requestFullscreen();
                this.svgs.fullscreen.setHref(this.icons.fullscreen_exit);
            }
        });

        document.addEventListener("fullscreenchange", () => {
            // This is after the fact when a user exited without using the icon
            let href = document.fullscreenElement ? this.icons.fullscreen_exit : this.icons.fullscreen_enter;
            this.svgs.fullscreen.setHref(href);
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

        this.htmlControls.progress.root.addEventListener("touchstart", _event => {
            const onProgressBarTouchMove = event => {
                const progressRoot = this.htmlControls.progress.root;
                const progress = calculateProgress(event, progressRoot);
                this.updateProgressBar(progress);
                this.updateProgressPopup(progress);
            }

            const onProgressBarTouchStop = event => {
                this.setToast("Touch end fire");
                this.isDraggingProgressBar = false;
                document.removeEventListener('touchmove', onProgressBarTouchMove);
                document.removeEventListener('touchend', onProgressBarTouchStop);

                const progressRoot = this.htmlControls.progress.root;
                const progress = calculateProgress(event, progressRoot);
                const timestamp = this.htmlVideo.duration * progress;

                this.fireControlsSeeked(timestamp);
                this.seek(timestamp);
            }

            this.isDraggingProgressBar = true;
            document.addEventListener('touchmove', onProgressBarTouchMove);
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

        this.htmlControls.progress.root.addEventListener("mouseenter", _event => {
            this.updateTimestamps(this.htmlVideo.currentTime);
        });

        this.htmlControls.progress.root.addEventListener("mousemove", event => {
            const progress = calculateProgress(event, this.htmlControls.progress.root);
            this.updateProgressPopup(progress);
        });

        this.htmlSeekBackward.addEventListener("transitionend", () => {
            this.htmlSeekBackward.classList.remove("animate");
        });

        this.htmlSeekForward.addEventListener("transitionend", () => {
            this.htmlSeekForward.classList.remove("animate");
        });

        this.playbackPopupSvg.addEventListener("transitionend", () => {
            this.playbackPopupSvg.classList.remove("animate");
        });

        this.htmlControls.root.addEventListener("transitionend", (e) => {
            // NOTE(kihau):
            //     This is a really weird and confusing way of setting the isUIVisible flag.
            //     Probably should be changed and done the proper way at some point.
            if (e.propertyName === "opacity") {
                this.isUIVisible = !e.target.classList.contains("player_ui_hide");
            }
        });
    }

    assembleProgressBar() {
        let progress =  this.htmlControls.progress;
        this.htmlControls.root.appendChild(progress.root);

        progress.root.appendChild(progress.total);
        progress.root.appendChild(progress.buffered);
        progress.root.appendChild(progress.current);
        progress.root.appendChild(progress.thumb);
        progress.root.appendChild(progress.popupRoot);

        progress.popupText.textContent = "00:00";
        progress.popupText.classList.add("unselectable");
        progress.popupRoot.appendChild(progress.popupText);
    }

    assembleControlButtons() {
        let buttons = this.htmlControls.buttons.root;
        this.htmlControls.root.appendChild(buttons);

        let svgs = this.svgs;

        let playback = this.htmlControls.buttons.playbackButton;
        playback.title = "Play/Pause";
        playback.appendChild(svgs.playback.svg);
        if (this.options.hidePlaybackButton) hideElement(playback);
        buttons.appendChild(playback);

        let next = this.htmlControls.buttons.nextButton;
        next.title = "Next";
        next.appendChild(svgs.next.svg);
        if (this.options.hideNextButton) hideElement(next);
        buttons.appendChild(next);

        let loop = this.htmlControls.buttons.loopButton;
        loop.title = "Loop";
        loop.appendChild(svgs.loop.svg);
        if (this.options.hideLoopingButton) hideElement(loop);
        buttons.appendChild(loop);

        let volume = this.htmlControls.buttons.volumeButton;
        volume.title = "Mute/Unmute";
        volume.appendChild(svgs.volume.svg);
        if (this.options.hideVolumeButton) hideElement(volume);
        buttons.appendChild(volume);

        let volumeRoot = newDiv("player_volume_root");
        let volumeSlider = this.htmlControls.buttons.volumeInput;
        volumeSlider.type = "range";
        volumeSlider.min = "0";
        volumeSlider.max = "1";
        volumeSlider.value = "1";
        volumeSlider.step = "any";

        let volumeBar = newDiv("player_volume_bar");
        let volumeProgress = this.htmlControls.buttons.volumeProgress;

        volumeRoot.appendChild(volumeBar);
        volumeRoot.appendChild(volumeProgress);
        volumeRoot.appendChild(volumeSlider);

        if (this.options.hideVolumeSlider) hideElement(volumeRoot)
        buttons.appendChild(volumeRoot);

        let timestamp = this.htmlControls.buttons.timestamp;
        timestamp.textContent = "00:00 / 00:00";
        if (this.options.hideTimestamps) hideElement(timestamp);
        buttons.appendChild(timestamp);

        buttons.appendChild(newDiv("player_spacer"))

        let download = this.htmlControls.buttons.downloadButton;
        download.title = "Download";
        download.appendChild(svgs.download.svg);
        if (this.options.hideDownloadButton) {
            hideElement(download);
        }
        buttons.appendChild(download);

        let autoplay = this.htmlControls.buttons.autoplayButton;
        autoplay.title = "Autoplay";
        autoplay.appendChild(svgs.autoplay.svg);
        if (this.options.hideAutoplayButton) {
            hideElement(autoplay);
        }
        buttons.appendChild(autoplay);

        let subs = this.htmlControls.buttons.subsButton;
        subs.title = "Subtitles";
        subs.appendChild(svgs.subs.svg);
        if (this.options.hideSubtitlesButton) {
            hideElement(subs);
        }
        buttons.appendChild(subs);

        let settings = this.htmlControls.buttons.settingsButton;
        settings.title = "Settings";
        settings.appendChild(svgs.settings.svg);
        if (this.options.hideSettingsButton) {
            hideElement(settings);
        }
        buttons.appendChild(settings);

        let fullscreen = this.htmlControls.buttons.fullscreenButton;
        fullscreen.title = "Fullscreen";
        fullscreen.appendChild(svgs.fullscreen.svg);
        if (this.options.hideFullscreenButton) {
            hideElement(fullscreen);
        }
        buttons.appendChild(fullscreen);
    }

    createHtmlControls() {
        let playerControls = this.htmlControls.root;
        playerControls.addEventListener("click", consumeClick);
        playerControls.addEventListener("focusout", () => {
            // otherwise document.body will receive focus
            this.htmlPlayerRoot.focus();
        });

        this.htmlPlayerRoot.appendChild(playerControls);

        this.assembleProgressBar();
        this.assembleControlButtons();
        this.createSubtitleMenu();
        this.createSettingsMenu();
    }

    createSubtitleTrackElement(title, index) {
        let menu = this.htmlControls.subMenu;

        let track = newDiv(null, "subtitle_track");
        track.onclick = _event => {
            if (menu.selected.track) {
                menu.selected.track.classList.remove("player_submenu_selected");
            }

            track.classList.add("player_submenu_selected");
            menu.selected.track = track;

            this.switchSubtitleTrack(index);
        }

        let trackTitle = newElement("input", null, "subtitle_track_text");
        trackTitle.type = "text";
        trackTitle.value = title;
        trackTitle.readOnly = true;

        let trackButtons = newDiv(null, "subtitle_track_buttons");

        let trackEdit = newElement("button", null, "subtitle_track_edit_button")
        trackEdit.textContent = "⚙️";
        let trackRemove = newElement("button", null, "subtitle_track_remove_button")
        trackRemove.textContent = "🗑";

        trackButtons.appendChild(trackEdit);
        trackButtons.appendChild(trackRemove);

        track.appendChild(trackTitle);
        track.appendChild(trackButtons);

        return track;
    }

    createSubtitleMenu() {
        let menu = this.htmlControls.subMenu;

        let root = menu.root;
        root.onclick = consumeClick;
        hideElement(root);
        this.htmlPlayerRoot.appendChild(root);

        { // player_submenu_top
            let top = newDiv("player_submenu_top");
            root.appendChild(top);

            let select = menu.tabs.selectButton;
            select.innerHTML = "Select"
            select.classList.add("player_submenu_top_button", "unselectable")
            select.style.display = ""
            top.appendChild(select);

            let search = menu.tabs.searchButton
            search.innerHTML = "Search"
            search.classList.add("player_submenu_top_button", "unselectable")
            search.style.display = ""
            top.appendChild(search);

            let options = menu.tabs.optionsButton;
            options.innerHTML = "Options"
            options.classList.add("player_submenu_top_button", "unselectable")
            options.style.display = ""
            top.appendChild(options);

            let attachSelectionClick = (button, bottom) => {
                button.onclick = () => {
                    let selected = this.htmlControls.subMenu.selected;
                    selected.button.classList.remove("player_submenu_selected");
                    hideElement(selected.bottom)

                    selected.button = button
                    selected.bottom = bottom;

                    selected.button.classList.add("player_submenu_selected");
                    selected.bottom.style.display = "";
                };
            }

            attachSelectionClick(menu.tabs.selectButton, menu.bottom.selectRoot);
            attachSelectionClick(menu.tabs.searchButton, menu.bottom.searchRoot);
            attachSelectionClick(menu.tabs.optionsButton, menu.bottom.optionsRoot);
        }

        { // player_submenu_bottom
            let bottom = newDiv("player_submenu_bottom");
            root.appendChild(bottom);

            let select = menu.bottom.selectRoot;
            hideElement(select);

            this.subsSwitcher = Switcher.new("Enable subtitles", state => {
                let textTracks = this.htmlVideo.textTracks;
                let index = this.selectedSubtitleIndex;

                if (0 <= index && index < textTracks.length) {
                    textTracks[index].mode = state ? "showing" : "hidden";
                }
            });
            let toggleBox = newElement("div", null, "player_submenu_box");
            toggleBox.appendChild(this.subsSwitcher.toggleRoot);

            select.appendChild(toggleBox);
            select.appendChild(menu.trackList);
            bottom.appendChild(select);

            // // NOTE(kihau): Dummy code for testing:
            /*menu.trackList.appendChild(this.createSubtitleTrackElement("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"));
            menu.trackList.appendChild(this.createSubtitleTrackElement("This is a long subtitle name.vtt"));
            menu.trackList.appendChild(this.createSubtitleTrackElement("Foo Bar"));
            menu.trackList.appendChild(this.createSubtitleTrackElement("AAAAAA"));
            menu.trackList.appendChild(this.createSubtitleTrackElement("BBBBBB"));
            menu.trackList.appendChild(this.createSubtitleTrackElement("CCCCCC"));
            menu.trackList.appendChild(this.createSubtitleTrackElement("DDDDDD"));
            menu.trackList.appendChild(this.createSubtitleTrackElement("EEEEEE"));
            menu.trackList.appendChild(this.createSubtitleTrackElement("FFFFFF"));
            menu.trackList.appendChild(this.createSubtitleTrackElement("GGGGGG"));
            menu.trackList.appendChild(this.createSubtitleTrackElement("HHHHHH"));
            menu.trackList.appendChild(this.createSubtitleTrackElement("IIIIII"));*/
            // // -----------------------------------

            let search = menu.bottom.searchRoot;
            let subtitleImport = newElement("input", "player_submenu_import");
            subtitleImport.textContent = "Import subtitle";
            subtitleImport.type = "file";
            subtitleImport.accept = ".vtt,.srt";
            subtitleImport.addEventListener("change", event => {
                console.log(event.target.files)
                if (event.target.files.length === 0) {
                    return;
                }
                const file = event.target.files[0];
                // This object is a blob and will be released with URL.revokeObjectURL on load
                const objectUrl = URL.createObjectURL(file);
                let trackInfo = TrackInfo.fromUrl(file.name);
                let ext = trackInfo.extension;
                if (ext === "vtt") {
                    console.log("Adding vtt track")
                    this.addVttTrack(objectUrl, true, trackInfo)
                } else if (ext === "srt") {
                    console.log("Adding srt track")
                    this.addSrtTrack(objectUrl, true, trackInfo)
                }
            });
            search.appendChild(subtitleImport)
            hideElement(search)
            bottom.appendChild(search);

            let options = menu.bottom.optionsRoot;
            hideElement(options)

            { // player_submenu_shift_root
                let root = newDiv("player_submenu_shift_root");

                // Top container:
                let top = newDiv("player_submenu_shift_top");
                let textSpan = newElement("span", "player_submenu_shift_text");
                textSpan.classList.add("unselectable");
                textSpan.textContent = "Subtitle shift";

                let valueSpan = newElement("span", "player_submenu_shift_value");
                valueSpan.classList.add("unselectable");
                valueSpan.textContent = "+0.0s";

                // Bottom container:
                let bottom = newDiv("player_submenu_shift_bottom");

                let leftButton = newElement("button", null, "player_submenu_shift_button");
                leftButton.appendChild(this.svgs.arrowLeft.svg);

                let slider = newElement("input", "player_submenu_shift_slider");
                slider.type = "range";
                slider.min = -10.0;
                slider.max = 10.0;
                slider.step = 0.1;
                slider.value = 0.0;

                let rightButton = newElement("button", null, "player_submenu_shift_button");
                rightButton.appendChild(this.svgs.arrowRight.svg);

                let setValueSpan = (value) => {
                    let max = Number(slider.max);
                    if (value > max) {
                        value = max;
                    }

                    let min = Number(slider.min);
                    if (value < min) {
                        value = min;
                    }

                    // Set precision to a single digit of the fractional part;
                    value = Math.round(value * 10.0) / 10.0;

                    let valueString = "";
                    if (value >= 0) {
                        valueString = "+";
                    }

                    valueString += value;

                    // Append ".0" when the value has no fractional part.
                    if ((value * 10) % 10 === 0.0) {
                        valueString += ".0";
                    }

                    valueString += "s";
                    valueSpan.textContent = valueString;
                }

                let lastSliderValue = 0.0;

                let shift = (offset) => {
                    let value = Number(slider.value) + offset;

                    let delta = value - lastSliderValue;
                    delta = Math.round(delta * 1000.0) / 1000.0;

                    // console.log("Last slider value:", lastSliderValue);
                    console.log("Slider value delta:", delta);

                    lastSliderValue = value;
                    setValueSpan(value);
                    slider.value = value;

                    let res = this.shiftCurrentSubtitleTrackBy(delta);
                    console.log("The result is:", res);
                }

                rightButton.onclick = () => shift(0.3);
                slider.oninput = () => shift(0.0);
                leftButton.onclick = () => shift(-0.3);

                top.appendChild(textSpan);
                top.appendChild(valueSpan);

                bottom.appendChild(leftButton);
                bottom.appendChild(slider);
                bottom.appendChild(rightButton);

                root.appendChild(top);
                root.appendChild(bottom);

                options.appendChild(root);
            }

            bottom.appendChild(options);
        }

        menu.selected.button = menu.tabs.selectButton;
        menu.selected.bottom = menu.bottom.selectRoot;

        menu.selected.button.classList.add("player_submenu_selected");
        menu.selected.bottom.style.display = "";
    }

    createSettingsMenu() {
        let menu = this.htmlControls.settings;

        let root = menu.root;
        root.onclick = consumeClick;
        hideElement(root);

        let autohide = Switcher.new("Auto-hide controls", state => {
            this.options.disableControlsAutoHide = !state;
        }, !this.options.disableControlsAutoHide);

        root.appendChild(autohide.toggleRoot);

        let showOnPause = Switcher.new("Show controls on pause", state => {
            this.options.showControlsOnPause = state;
        }, this.options.showControlsOnPause);
        root.appendChild(showOnPause.toggleRoot);

        this.htmlPlayerRoot.appendChild(root);
    }
}

class TrackInfo {
    constructor(filename, extension) {
        this.filename = filename;
        this.extension = extension;
    }
    static fromUrl(url) {
        let filename = url.substring(url.lastIndexOf("/") + 1);
        let extension = filename.substring(filename.lastIndexOf(".") + 1).toLowerCase();
        return new TrackInfo(filename, extension);
    }
}

class Switcher {
    constructor(toggleRoot, toggleSwitch, initialState) {
        this.toggleRoot = toggleRoot;
        this.toggleSwitch = toggleSwitch;
        this.setState(initialState)
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

    addAction(func) {
        this.toggleSwitch.addEventListener("click", () => {
            this.setState(!this.enabled)
            func(this.enabled);
        });
    }

    static new(text, onclick, initialState) {
        let toggleRoot   = newDiv(null, "player_toggle_root");
        let toggleText   = newDiv(null, "player_toggle_text");
        toggleText.textContent = text;
        let toggleSwitch = newDiv(null, "player_toggle_switch");
        let toggleCircle = newDiv(null, "player_toggle_circle");

        toggleRoot.appendChild(toggleText);
        toggleSwitch.appendChild(toggleCircle);
        toggleRoot.appendChild(toggleSwitch);

        let switcher = new Switcher(toggleRoot, toggleSwitch);
        switcher.setState(initialState);
        switcher.addAction(onclick);
        return switcher;
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

function parseSrt(srtText) {
    let lines = srtText.split('\n');
    let vttCues = []
    for (let i = 0; i < lines.length-2; i++) {
        // counter at lines[i]
        let timestamps = lines[++i];
        let [start, end, ok] = parseSrtTimestamps(timestamps)
        if (!ok) {
            return vttCues;
        }
        let content = ""
        while (++i < lines.length) {
            let text = lines[i];
            if (text.length === 0 || (text.length === 1 && (text[0] === '\r' || text[0] === '\n'))) {
                break;
            }
            content += text;
            content += '\n';
        }
        if (content !== "") {
            let newCue = new VTTCue(start, end, content)
            vttCues.push(newCue);
        }
    }
    return vttCues;
}

// Returns [seconds start, seconds end, success]
function parseSrtTimestamps(timestamps) {
    if (timestamps.length < 23) {
        return [null, null, false];
    }
    let splitter = timestamps.indexOf(" --> ", 8);
    if (splitter === -1) {
        return [null, null, false];
    }
    let startStamp = parseSrtStamp(timestamps.substring(0, splitter));
    let endStamp = parseSrtStamp(timestamps.substring(splitter+5));
    return [startStamp, endStamp, startStamp != null && endStamp != null];
}

// Returns a timestamp expressed in seconds or null on failure
function parseSrtStamp(stamp) {
    let twoSplit = stamp.split(',');
    if (twoSplit.length !== 2) {
        return null;
    }
    let hms = twoSplit[0].split(':');
    if (hms.length !== 3) {
        return null;
    }

    return hms[0] * 3600 + hms[1] * 60 + Number(hms[2]) + twoSplit[1] / 1000;
}

function newDiv(id, className) {
    let div = document.createElement("div")
    // tabIndex makes divs focusable so that they can receive and bubble key events
    div.tabIndex = -1
    if (id) {
        div.id = id
    }

    if (className) {
        div.className = className;
    }

    return div;
}

class Svg {
    static NAMESPACE = "http://www.w3.org/2000/svg";
    constructor(svg, use) {
        this.svg = svg;
        this.use = use;
    }

    setHref(href) {
        this.use.setAttribute("href", href)
    }

    static new(initialHref, width=20, height=20) {
        let svg = document.createElementNS(Svg.NAMESPACE, "svg");
        let use = document.createElementNS(Svg.NAMESPACE, "use");
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

function consumeClick(event) {
    event.stopPropagation();
}

function isFunction(func) {
    return func != null && typeof func === "function";
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
    console.log(systemInfo);
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
        this.hidePlaybackButton = false;
        this.hideNextButton = false;
        this.hideLoopingButton = false;
        this.hideVolumeButton = false;
        this.hideVolumeSlider = false;
        this.hideTimestamps = false;
        this.hideDownloadButton = false;
        this.hideAutoplayButton = false;
        this.hideSubtitlesButton = false;
        this.hideSettingsButton = false;
        this.hideFullscreenButton = false;

        this.doubleTapThresholdMs = 300;
        this.enableDoubleTapSeek = isMobileAgent();

        // [Arrow keys/Double tap] seeking offset provided in seconds.
        this.seekBy = 5;

        // Delay in milliseconds before controls disappear.
        this.inactivityTime = 2500;

        // Disable the auto hide for player controls.
        this.disableControlsAutoHide = false;
        this.showControlsOnPause = true;

        this.bufferingRedrawInterval = 1000;

    }

    // Ensure values are the intended type and within some reasonable range
    valid() {
        if (typeof this.seekBy !== "number" || this.seekBy < 0) {
            return false;
        }
        if (typeof this.inactivityTime !== "number" || this.inactivityTime < 0) {
            return false;
        }
        if (
            !this.areAllBooleans(
                this.hidePlaybackButton,
                this.hideNextButton,
                this.hideLoopingButton,
                this.hideVolumeButton,
                this.hideVolumeSlider,
                this.hideTimestamps,
                this.hideDownloadButton,
                this.hideSubtitlesButton,
                this.hideSettingsButton,
                this.hideFullscreenButton,
            )
        ) {
            console.debug("Visibility flags are not all booleans!");
            return false;
        }
        return true;
    }
    areAllBooleans(...variables) {
        for (let i = 0; i < variables.length; i++) {
            if (typeof variables[i] != "boolean") {
                return false;
            }
        }
        return true;
    }
}
