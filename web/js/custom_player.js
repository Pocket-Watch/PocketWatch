// import { Hls } from "../external/hls.js"

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

    // isVideoPlaying() {
    //     return !this.htmlVideo.paused && !this.htmlPlayer.ended;
    // }

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

    // How to set a track once it has been added? Programmatic selection through setSubtitleTrack?
    addSubtitleTrack(subtitleUrl) {
        this.internals.addSubtitleTrack(subtitleUrl);
    }

    // How to set a track once it has been added? Programmatic selection through setSubtitleTrack?
    setSubtitleTrack(subtitleUrl) {
        this.internals.addSubtitleTrack(subtitleUrl);
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

    setVideoTrack(url) {
        this.internals.setVideoTrack(url);
    }
}

class Internals {
    constructor(videoElement, options) {
        console.log("OPTIONS:", options);
        this.options = options;
        // Corresponds to the actual html player element called either </video> or </audio>.
        this.htmlVideo = videoElement;

        // Div container where either the player or the placeholder resides.
        this.htmlPlayerRoot = document.createElement("div");
        this.htmlPlayerRoot.id = "player_container";

        // We actually need to append the <div> to document.body (or <video>'s parent)
        // otherwise the <video> tag will disappear entirely!
        let videoParent = this.htmlVideo.parentNode;
        videoParent.appendChild(this.htmlPlayerRoot);
        this.htmlPlayerRoot.appendChild(this.htmlVideo);

        this.htmlTitleContainer = document.createElement("div");
        this.htmlTitleContainer.id = "player_title_container";
        this.htmlTitleContainer.style.display = "none";
        this.htmlPlayerRoot.appendChild(this.htmlTitleContainer);

        this.htmlTitle = document.createElement("span");
        this.htmlTitle.id = "player_title_text";
        this.htmlTitleContainer.appendChild(this.htmlTitle);

        this.htmlBuffering = document.createElement("img");
        this.htmlBuffering.id = "player_buffering";
        this.htmlBuffering.src = "svg/buffering.svg";
        this.htmlBuffering.style.display = "none";
        this.htmlBuffering.setAttribute("class", "unselectable");
        this.htmlPlayerRoot.appendChild(this.htmlBuffering);

        this.htmlControls = {
            root: null,
            progress: {
                root: null,
                current: null,
                buffered: null,
                total: null,
                thumb: null,
                popupRoot: null,
                popupText: null,
            },
            subtitleMenu: {
                root: null,
                customization: null,
                selection: null,
                subtitleList: null,
                back: null,
            },
            playToggleButton: null,
            nextButton: null,
            volume: null,
            volumeSlider: null,
            timestamp: null,
            download: null,
            subs: null,
            settings: null,
            fullscreen: null,
        };

        // We could store references to images/svg/videos here for easy access
        // TODO? IDK if we need to store references to 'use' ones
        this.resources = {
            seekForwardImg: null,
            seekBackwardImg: null,
            pauseImg: null,
            playImg: null,
            nextImg: null,
            volumeFullImg: null,
            volumeMediumImg: null,
            volumeLowImg: null,
            volumeMutedImg: null,
            downloadImg: null,
            subsImg: null,
            settingsImg: null,
            fullscreenImg: null,
            fullscreenExitImg: null,
        };

        this.htmlImgs = {
            seekForward: null,
            seekBackward: null,
            playToggle: null,
            next: null,
            volume: null,
            download: null,
            subs: null,
            settings: null,
            fullscreen: null,
        };

        this.isDraggingProgressBar = false;
        this.volumeBeforeMute = 0.0;

        this.initializeImageSources();
        this.createHtmlControls();
        this.createSubtitleMenu();

        this.htmlSeekForward = document.createElement("div");
        this.htmlSeekForward.id = "player_forward_container";
        this.htmlSeekForward.appendChild(this.htmlImgs.seekForward);
        this.htmlPlayerRoot.appendChild(this.htmlSeekForward);

        this.htmlSeekBackward = document.createElement("div");
        this.htmlSeekBackward.id = "player_backward_container";
        this.htmlSeekBackward.appendChild(this.htmlImgs.seekBackward);
        this.htmlPlayerRoot.appendChild(this.htmlSeekBackward);

        this.attachHtmlEvents();
        this.setProgressMargin(5);
        setInterval(() => this.redrawBufferedBars(), this.options.bufferingRedrawInterval);
    }

    fireControlsPlay() {}
    fireControlsPause() {}
    fireControlsNext() {}
    fireControlsSeeking(_timestamp) {}
    fireControlsSeeked(_timestamp) {}
    fireControlsVolumeSet(_volume) {}

    play() {
        this.htmlImgs.playToggle.src = this.resources.pauseImg;
        this.htmlVideo.play();
    }

    pause() {
        this.htmlImgs.playToggle.src = this.resources.playImg;
        this.htmlVideo.pause();
    }

    seek(timestamp) {
        this.htmlVideo.currentTime = timestamp;
    }

    updateProgressBar(progress) {
        this.htmlControls.progress.current.style.width = progress * 100 + "%"

        const width = this.htmlControls.progress.root.clientWidth;
        let thumb_left = width * progress; 
        thumb_left -= this.htmlControls.progress.thumb.offsetWidth / 2.0;
        this.htmlControls.progress.thumb.style.left = thumb_left + "px";
    }

    setProgressMargin(marginSize) {
        let margin = marginSize + "px";
        this.htmlControls.progress.total.style.marginTop = margin;
        this.htmlControls.progress.current.style.marginTop = margin;
        this.htmlControls.progress.buffered.style.marginTop = margin;

        this.htmlControls.progress.total.style.marginBottom = margin;
        this.htmlControls.progress.current.style.marginBottom = margin;
        this.htmlControls.progress.buffered.style.marginBottom = margin;

        let rootHeight = this.htmlControls.progress.root.clientHeight;
        let height = (rootHeight - marginSize * 2.0) + "px";
        this.htmlControls.progress.total.style.height = height;
        this.htmlControls.progress.current.style.height = height;
        this.htmlControls.progress.buffered.style.height = height;
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
        // NOTE(kihau): This duration string does not need to be updated every time since the duration does not change?
        let duration_string = createTimestampString(duration);

        this.htmlControls.timestamp.textContent = current_string + " / " + duration_string;
    }

    updateHtmlVolume(volume) {
        if (volume > 1.0) {
            volume = 1.0;
        }

        if (volume < 0.0) {
            volume = 0.0;
        }

        if (volume === 0.0) {
            this.htmlImgs.volume.src = this.resources.volumeMutedImg;
        } else if (volume < 0.3) {
            this.htmlImgs.volume.src = this.resources.volumeLowImg;
        } else if (volume < 0.6) {
            this.htmlImgs.volume.src = this.resources.volumeMediumImg;
        } else {
            this.htmlImgs.volume.src = this.resources.volumeFullImg;
        }

        this.htmlControls.volumeSlider.value = volume;
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
            this.htmlTitleContainer.style.display = "none";
        } else {
            this.htmlTitleContainer.style.display = "";
            this.htmlTitle.textContent = title;
        }
    }

    togglePlay() {
        if (this.htmlVideo.paused) {
            this.fireControlsPlay();
            this.play();
        } else {
            this.fireControlsPause();
            this.pause();
        }
    }

    setVideoTrack(url) {
        if(!URL.canParse(url, document.baseURI)){
            console.debug("Failed to set a new URL. It's not parsable.")
            // We should probably inform the user about the error either via debug log or return false
            return
        }
        // This covers both relative and fully qualified URLs because we always specify the base
        // and when the base is not provided, the second argument is used to construct a valid URL
        let pathname = new URL(url, document.baseURI).pathname;

        if (pathname.endsWith(".m3u8")) {
            import("../external/hls.js").then(module => {
                if (module.Hls.isSupported()) {
                    const hls = new module.Hls();
                    hls.loadSource(url);
                    hls.attachMedia(this.htmlVideo);
                }
            });
        } else {
            this.htmlVideo.src = url;
            this.htmlVideo.load();
        }
    }

    addSubtitleTrack(url, set) {
        let filename = url.substring(url.lastIndexOf("/") + 1);
        let extension = filename.substring(filename.lastIndexOf(".") + 1).toLowerCase();
        if (extension != "vtt" && extension != "srt") {
            console.debug("Unsupported extension:", extension)
            return
        }

        let track = document.createElement("track")
        track.label = filename
        track.kind = "subtitles"
        track.src = url

        // This will cause a new text track to appear in video.textTracks even if it's invalid
        this.htmlVideo.appendChild(track)

        let lastIndex = this.htmlVideo.textTracks.length - 1;
        let textTrack = this.htmlVideo.textTracks[lastIndex];

        // By default, every track is appended in the 'disabled' mode which prevents any initialization
        textTrack.mode = set ? "showing" : "hidden";
        // Although we cannot access cues immediately here (not loaded yet)
        // We do have access to the textTrack and can attach a listener to it
        track.addEventListener("load", (event) => {
            console.info("Text track loaded successfully", event)
        });
    }

    showPlayerUI() {
        this.htmlVideo.style.cursor = "auto";
        this.htmlControls.root.classList.remove("player_fade_out");
        this.htmlControls.root.classList.add("player_fade_in");

        this.htmlTitleContainer.classList.remove("player_fade_out");
        this.htmlTitleContainer.classList.add("player_fade_in");
    }

    hidePlayerUI() {
        if (this.options.disableControlsAutoHide) {
            return;
        }

        this.htmlVideo.style.cursor = "none";
        this.htmlControls.root.classList.remove("player_fade_in");
        this.htmlControls.root.classList.add("player_fade_out");

        this.htmlTitleContainer.classList.remove("player_fade_in");
        this.htmlTitleContainer.classList.add("player_fade_out");
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

        this.htmlControls.playToggleButton.addEventListener("click", () => {
            this.togglePlay();
        });

        this.htmlControls.nextButton.addEventListener("click", () => {
            this.fireControlsNext();
        });

        this.htmlControls.volume.addEventListener("click", () => {
            if (this.htmlControls.volumeSlider.value == 0) {
                this.fireControlsVolumeSet(this.volumeBeforeMute);
                this.setVolume(this.volumeBeforeMute);
            } else {
                this.volumeBeforeMute = this.htmlControls.volumeSlider.value;
                this.fireControlsVolumeSet(0);
                this.setVolume(0);
            }
        });

        this.htmlControls.subs.addEventListener("click", () => {
            let menuRootElement = this.htmlControls.subtitleMenu.root;
            let visible = menuRootElement.style.display !== "none";
            if (visible) {
                menuRootElement.style.display = "none";
            } else {
                menuRootElement.style.display = "";
            }
        });

        this.htmlControls.subtitleMenu.selection.addEventListener("click", () => {
            this.htmlControls.subtitleMenu.selection.style.display = "none";
            this.htmlControls.subtitleMenu.customization.style.display = "none";
            this.htmlControls.subtitleMenu.back.style.display = "";
            let textTracks = this.htmlVideo.textTracks;
            let subtitleList = this.htmlControls.subtitleMenu.subtitleList;
            for (let i = 0; i < textTracks.length; i++) {
                let track = textTracks[i];
                const trackDiv = document.createElement("a");
                trackDiv.textContent = track.label;
                console.log("Adding", track.label)
                trackDiv.onclick = () => {
                    console.log("Clicked on", track.label)
                    track.mode = "showing";
                }
                subtitleList.appendChild(trackDiv);
            }
            subtitleList.style.display = "block";
        });

        this.htmlControls.subtitleMenu.back.addEventListener("click", () => {
            this.htmlControls.subtitleMenu.selection.style.display = "";
            this.htmlControls.subtitleMenu.customization.style.display = "";
            this.htmlControls.subtitleMenu.subtitleList.style.display = "none";
            this.htmlControls.subtitleMenu.back.style.display = "none";
            this.htmlControls.subtitleMenu.subtitleList.innerHTML = ""
        });

        this.htmlVideo.addEventListener("keydown", (event) => {
            if (event.key == " " || event.code == "Space" || event.keyCode == 32) {
                this.togglePlay();
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

        this.htmlVideo.addEventListener("click", (_event) => {
            this.togglePlay();
        });

        this.htmlVideo.addEventListener("waiting", () => {
            this.bufferingTimeoutId = setTimeout(() => {
            this.htmlBuffering.style.display = "";
            }, 200);
        });

        this.htmlVideo.addEventListener("playing", () => {
            clearTimeout(this.bufferingTimeoutId);
            this.htmlBuffering.style.display = "none";
        });

        this.htmlVideo.addEventListener("timeupdate", (_event) => {
            let timestamp = this.htmlVideo.currentTime;
            this.updateTimestamps(timestamp);
        });

        this.htmlControls.fullscreen.addEventListener("click", () => {
            if (document.fullscreenElement) {
                document.exitFullscreen();
                this.htmlImgs.fullscreen.src = this.resources.fullscreenImg;
            } else {
                this.htmlPlayerRoot.requestFullscreen();
                this.htmlImgs.fullscreen.src = this.resources.fullscreenExitImg;
            }
        });

        this.htmlControls.volumeSlider.addEventListener("input", _event => {
            let volume = this.htmlControls.volumeSlider.value;
            this.fireControlsVolumeSet(volume);
            this.setVolume(volume);
        });

        // TODO(kihau): Discover behaviour of this function.
        // NOTE(kihau): Helper function grabbed from fluid-player source code.
        let getEventOffsetX = (event, element) => {
            let x = 0;

            while (element && !isNaN(element.offsetLeft)) {
                if (element.tagName === 'BODY') {
                    x += element.offsetLeft + element.clientLeft - (element.scrollLeft || document.documentElement.scrollLeft);
                } else {
                    x += element.offsetLeft + element.clientLeft - element.scrollLeft;
                }

                element = element.offsetParent;
            }

            let eventX;
            if (typeof event.touches !== 'undefined' && typeof event.touches[0] !== 'undefined') {
                eventX = event.touches[0].clientX;
            } else {
                eventX = event.clientX
            }

            return eventX - x;
        };

        let calculateProgress = (event) => {
            const width = this.htmlControls.progress.root.clientWidth;
            const offsetX = getEventOffsetX(event, this.htmlControls.progress.root);
            const progress = offsetX / width;

            if (isNaN(progress)) {
                return 0.0;
            }

            if (progress > 1.0) {
                return 1.0;
            }

            if (progress < 0.0) {
                return 0.0;
            }

            return progress;
        }

        this.htmlControls.progress.root.addEventListener("mousedown", _event => {
            const onProgressBarMouseMove = event => {
                const progress = calculateProgress(event);
                this.updateProgressBar(progress);
            }

            const onProgressBarMouseUp = event => {
                this.isDraggingProgressBar = false;
                document.removeEventListener('mousemove', onProgressBarMouseMove);
                document.removeEventListener('mouseup', onProgressBarMouseUp);

                const progress = calculateProgress(event);
                const timestamp = this.htmlVideo.duration * progress;

                this.fireControlsSeeked(timestamp);
                this.seek(timestamp);
            }

            this.isDraggingProgressBar = true;
            document.addEventListener('mousemove', onProgressBarMouseMove);
            document.addEventListener('mouseup', onProgressBarMouseUp);
        });

        this.htmlControls.progress.root.addEventListener("mouseenter", _event => {
            this.htmlControls.progress.thumb.style.display = "";
            this.htmlControls.progress.popupRoot.style.display = "";
            this.setProgressMargin(4);
            this.updateTimestamps(this.htmlVideo.currentTime);
        });

        this.htmlControls.progress.root.addEventListener("mousemove", event => {
            const width = this.htmlControls.progress.root.clientWidth;
            const value = getEventOffsetX(event, this.htmlControls.progress.root) / width;
            const timestamp = this.htmlVideo.duration * value;

            this.htmlControls.progress.popupRoot.style.left = value * 100 + "%";
            this.htmlControls.progress.popupRoot.style.display = "";
            this.htmlControls.progress.popupText.textContent = createTimestampString(timestamp);
        });

        this.htmlControls.progress.root.addEventListener("mouseleave", _event => {
            this.htmlControls.progress.thumb.style.display = "none";
            this.htmlControls.progress.popupRoot.style.display = "none";
            this.setProgressMargin(5);
        });

        this.htmlSeekBackward.addEventListener("transitionend", () => {
            this.htmlSeekBackward.classList.remove("animate");
        });

        this.htmlSeekForward.addEventListener("transitionend", () => {
            this.htmlSeekForward.classList.remove("animate");
        });
    }

    initializeImageSources() {
        let res = this.resources;
        res.seekForwardImg = "svg/seek10.svg";
        res.seekBackwardImg = "svg/seek10.svg";
        res.playImg = "svg/play.svg";
        res.pauseImg = "svg/pause.svg";
        res.nextImg = "svg/next.svg";
        res.loopImg = "svg/loop.svg";
        res.volumeFullImg = "svg/volume_full.svg";
        res.volumeMediumImg = "svg/volume_medium.svg";
        res.volumeLowImg = "svg/volume_low.svg";
        res.volumeMutedImg = "svg/volume_muted.svg";
        res.downloadImg = "svg/download.svg";
        res.subsImg = "svg/subs.svg";
        res.settingsImg = "svg/settings.svg";
        res.fullscreenImg = "svg/fullscreen.svg";
        res.fullscreenExitImg = "svg/fullscreen_exit.svg";

        this.preloadResources()

        let imgs = this.htmlImgs;
        imgs.seekForward = this.createImgElementWithSrc(res.seekForwardImg, 70, 70);
        imgs.seekBackward = this.createImgElementWithSrc(res.seekBackwardImg, 70, 70);
        imgs.playToggle = this.createImgElementWithSrc(res.playImg, 20, 20)
        imgs.next = this.createImgElementWithSrc(res.nextImg, 20, 20);
        imgs.loop = this.createImgElementWithSrc(res.loopImg, 20, 20)
        imgs.volume = this.createImgElementWithSrc(res.volumeFullImg, 20, 20);
        imgs.download = this.createImgElementWithSrc(res.downloadImg, 20, 20);
        imgs.subs = this.createImgElementWithSrc(res.subsImg, 20, 20)
        imgs.settings = this.createImgElementWithSrc(res.settingsImg, 20, 20)
        imgs.fullscreen = this.createImgElementWithSrc(res.fullscreenImg, 20, 20)

    }

    preloadResources() {
        // Not preloading swappable graphic is very likely to trigger multiple NS_BINDING_ABORTED exceptions
        // and also lag the browser, therefore we must preload or merge all icons into a single .svg file
        let res = this.resources;
        new Image().src = res.playImg;
        new Image().src = res.pauseImg;
        new Image().src = res.volumeFullImg;
        new Image().src = res.volumeMediumImg;
        new Image().src = res.volumeLowImg;
        new Image().src = res.volumeMutedImg;
    }

    createImgElementWithSrc(src, width, height) {
        let img = document.createElement("img");
        img.src = src;
        img.width = width;
        img.height = height;
        img.setAttribute("class", "unselectable");
        return img;
    }

    createProgressBar() {
        let progressRoot = document.createElement("div");
        progressRoot.id = "player_progress_root";
        this.htmlControls.root.appendChild(progressRoot);
        this.htmlControls.progress.root = progressRoot;

        let progressTotal = document.createElement("div");
        progressTotal.id = "player_progress_total";
        progressRoot.appendChild(progressTotal);
        this.htmlControls.progress.total = progressTotal;

        let progressBuffered = document.createElement("canvas");
        progressBuffered.id = "player_progress_buffered";
        progressRoot.appendChild(progressBuffered);
        this.htmlControls.progress.buffered = progressBuffered;

        let progressCurrent = document.createElement("div");
        progressCurrent.id = "player_progress_current";
        progressRoot.appendChild(progressCurrent);
        this.htmlControls.progress.current = progressCurrent;

        let progressThumb = document.createElement("div");
        progressThumb.id = "player_progress_thumb";
        progressRoot.appendChild(progressThumb);
        this.htmlControls.progress.thumb = progressThumb;

        let progressPopupRoot = document.createElement("div");
        progressPopupRoot.id = "player_progress_popup_root";
        progressPopupRoot.style.display = "none";
        progressRoot.appendChild(progressPopupRoot);
        this.htmlControls.progress.popupRoot = progressPopupRoot;

        let progressPopupText = document.createElement("div");
        progressPopupText.id = "player_progress_popup_text";
        progressPopupText.textContent = "00:00";
        progressPopupRoot.appendChild(progressPopupText);
        this.htmlControls.progress.popupText = progressPopupText;
    }

    createHtmlControls() {
        let playerControls = document.createElement("div");
        playerControls.id = "player_controls";
        playerControls.setAttribute("ondragstart", "return false");
        this.htmlControls.root = playerControls;

        this.createProgressBar();

        let playToggle = document.createElement("div");
        playToggle.id = "player_play_toggle";
        playToggle.appendChild(this.htmlImgs.playToggle);
        playToggle.style.display = this.options.hidePlayToggleButton ? "none" : "";
        playerControls.appendChild(playToggle);
        this.htmlControls.playToggleButton = playToggle;

        let next = document.createElement("div");
        next.id = "player_next";
        next.appendChild(this.htmlImgs.next);
        next.style.display = this.options.hideNextButton ? "none" : "";
        playerControls.appendChild(next);
        this.htmlControls.nextButton = next;

        let loop = document.createElement("div");
        loop.id = "player_loop";
        loop.appendChild(this.htmlImgs.loop);
        loop.style.display = this.options.hideLoopingButton ? "none" : "";
        playerControls.appendChild(loop);
        this.htmlControls.loopButton = loop;

        let volume = document.createElement("div");
        volume.id = "player_volume";
        volume.appendChild(this.htmlImgs.volume);
        volume.style.display = this.options.hideVolumeButton ? "none" : "";
        playerControls.appendChild(volume);
        this.htmlControls.volume = volume;

        let volumeSlider = document.createElement("input");
        volumeSlider.id = "player_volume_slider";
        volumeSlider.type = "range";
        volumeSlider.min = "0";
        volumeSlider.max = "1";
        volumeSlider.value = "1";
        volumeSlider.step = "any";
        volumeSlider.style.display = this.options.hideVolumeSlider ? "none" : "";
        playerControls.appendChild(volumeSlider);
        this.htmlControls.volumeSlider = volumeSlider;

        let timestamp = document.createElement("span");
        timestamp.id = "player_timestamp";
        timestamp.textContent = "00:00 / 00:00";
        timestamp.style.display = this.options.hideTimestamps ? "none" : "";
        playerControls.appendChild(timestamp);
        this.htmlControls.timestamp = timestamp;

        let firstAutoMargin = true;

        let download = document.createElement("div");
        download.id = "player_download";
        download.appendChild(this.htmlImgs.download);
        if (this.options.hideDownloadButton) {
            download.style.display = "none";
        } else {
            download.style.marginLeft = firstAutoMargin ? "auto" : "0";
            firstAutoMargin = false;
        }

        playerControls.appendChild(download);
        this.htmlPlayerRoot.appendChild(playerControls);
        this.htmlControls.download = download;

        let subs = document.createElement("div");
        subs.id = "player_subs";
        subs.appendChild(this.htmlImgs.subs);
        if (this.options.hideSubtitlesButton) {
            subs.style.display = "none";
        } else {
            subs.style.marginLeft = firstAutoMargin ? "auto" : "0";
            firstAutoMargin = false;
        }
        playerControls.appendChild(subs);
        this.htmlPlayerRoot.appendChild(playerControls);
        this.htmlControls.subs = subs;

        let settings = document.createElement("div");
        settings.id = "player_settings";
        settings.appendChild(this.htmlImgs.settings);
        if (this.options.hideSettingsButton) {
            settings.style.display = "none";
        } else {
            settings.style.marginLeft = firstAutoMargin ? "auto" : "0";
            firstAutoMargin = false;
        }
        playerControls.appendChild(settings);
        this.htmlPlayerRoot.appendChild(playerControls);
        this.htmlControls.settings = settings;

        let fullscreen = document.createElement("div");
        fullscreen.id = "player_fullscreen";
        fullscreen.appendChild(this.htmlImgs.fullscreen);
        if (this.options.hideFullscreenButton) {
            fullscreen.style.display = "none";
        } else {
            fullscreen.style.marginLeft = firstAutoMargin ? "auto" : "0";
        }
        playerControls.appendChild(fullscreen);
        this.htmlPlayerRoot.appendChild(playerControls);
        this.htmlControls.fullscreen = fullscreen;
    }

    createSubtitleMenu() {
        this.htmlControls.subtitleMenu.root = document.createElement("div");
        let menuRoot = this.htmlControls.subtitleMenu.root;
        menuRoot.id = "player_subtitle_menu"
        menuRoot.style.display = "none"

        this.htmlControls.subtitleMenu.selection = document.createElement("div");
        let selection = this.htmlControls.subtitleMenu.selection;
        selection.classList.add("menu_item")
        selection.classList.add("unselectable")
        selection.innerHTML = "Select";
        menuRoot.appendChild(selection);

        this.htmlControls.subtitleMenu.customization = document.createElement("div");
        let customization = this.htmlControls.subtitleMenu.customization;
        customization.classList.add("menu_item")
        customization.classList.add("unselectable")
        customization.innerHTML = "Customize";
        menuRoot.appendChild(customization);

        // Scrollable pane for selection
        this.htmlControls.subtitleMenu.subtitleList = document.createElement("div");
        let subtitleList = this.htmlControls.subtitleMenu.subtitleList;
        subtitleList.id = "subtitle_list";
        subtitleList.classList.add("scrollable_pane")
        subtitleList.classList.add("unselectable")
        subtitleList.style.display = "none"
        menuRoot.appendChild(subtitleList);

        // Back button for any action item
        this.htmlControls.subtitleMenu.back = document.createElement("div");
        let back = this.htmlControls.subtitleMenu.back;
        back.innerHTML = "←"
        back.classList.add("menu_item")
        back.classList.add("unselectable")
        back.style.display = "none"
        menuRoot.appendChild(back);

        this.htmlPlayerRoot.appendChild(menuRoot);
    }
}

function createTimestampString(timestamp) {
    let seconds = Math.floor(timestamp % 60.0);
    let minutes = Math.floor(timestamp / 60.0);

    let timestamp_string = "";
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

function consumeEvent(event) {
    event.stopPropagation();
    event.preventDefault();
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
        this.hidePlayToggleButton = false;
        this.hideNextButton = false;
        this.hideLoopingButton = false;
        this.hideVolumeButton = false;
        this.hideVolumeSlider = false;
        this.hideTimestamps = false;
        this.hideDownloadButton = false;
        this.hideSubtitlesButton = false;
        this.hideSettingsButton = false;
        this.hideFullscreenButton = false;

        // video.width = video.videoWidth, video.height = video.videoHeight
        this.resizeToMedia = true;

        // [Arrow keys/Double tap] seeking offset provided in seconds.
        this.seekBy = 5;

        // Delay in milliseconds before controls disappear.
        this.inactivityTime = 2500;

        // Disable the auto hide for player controls.
        this.disableControlsAutoHide = true;

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
                this.hidePlayToggleButton,
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
