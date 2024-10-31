export { Player };

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
    onControlsSeek(func) {
        if (!isFunction(func)) {
            return;
        }
        // an anonymous function is needed to receive arguments from the underlying function
        this.internals.fireControlsSeek = function (timestamp) {
            func(timestamp);
        };
    }
    onControlsVolumeSet(func) {
        if (!isFunction(func)) {
            return;
        }
        // an anonymous function is needed to receive arguments from the underlying function
        this.internals.fireControlsVolumeSet = function (volume) {
            func(volume);
        };
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
        this.htmlTitleContainer.style.visibility = "hidden";
        this.htmlPlayerRoot.appendChild(this.htmlTitleContainer);

        this.htmlTitle = document.createElement("span");
        this.htmlTitle.id = "player_title";
        this.htmlTitleContainer.appendChild(this.htmlTitle);

        this.htmlBuffering = document.createElement("img");
        this.htmlBuffering.id = "player_buffering";
        this.htmlBuffering.src = "svg/buffering.svg";
        this.htmlBuffering.style.visibility = "hidden";
        this.htmlBuffering.setAttribute("class", "unselectable");
        this.htmlPlayerRoot.appendChild(this.htmlBuffering);

        this.htmlControls = {
            timestampSlider: null,
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
            pauseImg: null,
            playImg: null,
            nextImg: null,
            volumeImgFull: null,
            volumeImgMedium: null,
            volumeImgLow: null,
            volumeImgMuted: null,
            downloadImg: null,
            subsImg: null,
            settingsImg: null,
            fullscreenImg: null,
        };

        this.volumeBeforeMute = 0.0;

        this.initializeSvgResources();
        this.createHtmlControls();
        this.attachHtmlEvents();
    }

    fireControlsPlay() {}
    fireControlsPause() {}
    fireControlsNext() {}
    fireControlsSeek(_timestamp) {}
    fireControlsVolumeSet(_volume) {}

    play() {
        this.htmlControls.playToggleButton.getElementsByTagName("img")[0].replaceWith(this.resources.pauseImg);
        this.htmlVideo.play();
    }

    pause() {
        this.htmlControls.playToggleButton.getElementsByTagName("img")[0].replaceWith(this.resources.playImg);
        this.htmlVideo.pause();
    }

    seek(timestamp) {
        this.htmlVideo.currentTime = timestamp;
        this.updateTimestamps(timestamp);
    }

    updateTimestamps(timestamp) {
        let duration = 0.0;
        if (isNaN(this.htmlVideo.duration) || this.htmlVideo.duration === 0.0) {
            this.htmlControls.timestampSlider.value = 0.0;
        } else {
            duration = this.htmlVideo.duration;
            let position = timestamp / duration;
            this.htmlControls.timestampSlider.value = position;
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

        if (volume == 0.0) {
            this.htmlControls.volume.getElementsByTagName("img")[0].replaceWith(this.resources.volumeImgMuted);
        } else if (volume < 0.3) {
            this.htmlControls.volume.getElementsByTagName("img")[0].replaceWith(this.resources.volumeImgLow);
        } else if (volume < 0.6) {
            this.htmlControls.volume.getElementsByTagName("img")[0].replaceWith(this.resources.volumeImgMedium);
        } else {
            this.htmlControls.volume.getElementsByTagName("img")[0].replaceWith(this.resources.volumeImgFull);
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
            this.htmlTitleContainer.style.visibility = "hidden";
        } else {
            this.htmlTitleContainer.style.visibility = "visible";
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
        let source = this.htmlVideo.querySelector("source");
        if (!source) {
            console.debug("Creating a source tag");
            source = document.createElement("source");
            this.htmlVideo.appendChild(source);
        }
        source.setAttribute("src", url);
        source.setAttribute("type", "video/mp4");
        // source.src = url;
        // source.type = "video/mp4";
        this.htmlVideo.load();
    }

    attachHtmlEvents() {
        this.htmlControls.playToggleButton.onclick = () => {
            this.togglePlay();
        };

        this.htmlControls.nextButton.onclick = () => {
            this.fireControlsNext();
        };

        this.htmlControls.volume.onclick = () => {
            if (this.htmlControls.volumeSlider.value == 0) {
                this.fireControlsVolumeSet(this.volumeBeforeMute);
                this.setVolume(this.volumeBeforeMute);
            } else {
                this.volumeBeforeMute = this.htmlControls.volumeSlider.value;
                this.fireControlsVolumeSet(0);
                this.setVolume(0);
            }
        };

        this.htmlVideo.onkeydown = (event) => {
            if (event.key == " " || event.code == "Space" || event.keyCode == 32) {
                this.togglePlay();
                consumeEvent(event);
            }

            if (event.key == "ArrowLeft" || event.keyCode == 37) {
                let timestamp = this.getNewTime(-this.options.seekBy);
                this.fireControlsSeek(timestamp);
                this.seek(timestamp);
                consumeEvent(event);
            }

            if (event.key == "ArrowRight" || event.keyCode == 39) {
                // We should use options here
                let timestamp = this.getNewTime(this.options.seekBy);
                this.fireControlsSeek(timestamp);
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
        };

        this.htmlVideo.onclick = (_event) => {
            this.togglePlay();
        };

        this.htmlVideo.onwaiting = () => {
            this.htmlBuffering.style.visibility = "visible";
        };

        this.htmlVideo.onplaying = () => {
            this.htmlBuffering.style.visibility = "hidden";
        };

        this.htmlControls.fullscreen.onclick = () => {
            // handle with Promise, it has controls on Chromium based browsers?
            this.htmlVideo.requestFullscreen();
        };
        this.htmlControls.volumeSlider.oninput = (_event) => {
            let volume = this.htmlControls.volumeSlider.value;
            this.fireControlsVolumeSet(volume);
            this.setVolume(volume);
        };

        this.htmlControls.timestampSlider.oninput = (_event) => {
            let position = this.htmlControls.timestampSlider.value;
            let timestamp = this.htmlVideo.duration * position;
            this.fireControlsSeek(timestamp);
            this.seek(timestamp);
        };

        this.htmlVideo.ontimeupdate = (_event) => {
            let timestamp = this.htmlVideo.currentTime;
            this.updateTimestamps(timestamp);
        };
    }

    initializeSvgResources() {
        // Lift and shifted
        let res = this.resources;

        res.playImg = document.createElement("img");
        res.playImg.src = "svg/play.svg";
        res.playImg.width = 20;
        res.playImg.height = 20;
        res.playImg.setAttribute("class", "unselectable");

        res.pauseImg = document.createElement("img");
        res.pauseImg.src = "svg/pause.svg";
        res.pauseImg.width = 20;
        res.pauseImg.height = 20;
        res.pauseImg.setAttribute("class", "unselectable");

        res.nextImg = document.createElement("img");
        res.nextImg.src = "svg/next.svg";
        res.nextImg.width = 20;
        res.nextImg.height = 20;
        res.nextImg.setAttribute("class", "unselectable");

        res.loopImg = document.createElement("img");
        res.loopImg.src = "svg/loop.svg";
        res.loopImg.width = 20;
        res.loopImg.height = 20;
        res.loopImg.setAttribute("class", "unselectable");

        res.volumeImgFull = document.createElement("img");
        res.volumeImgFull.src = "svg/volume_full.svg";
        res.volumeImgFull.width = 20;
        res.volumeImgFull.height = 20;
        res.volumeImgFull.setAttribute("class", "unselectable");

        res.volumeImgMedium = document.createElement("img");
        res.volumeImgMedium.src = "svg/volume_medium.svg";
        res.volumeImgMedium.width = 20;
        res.volumeImgMedium.height = 20;
        res.volumeImgMedium.setAttribute("class", "unselectable");

        res.volumeImgLow = document.createElement("img");
        res.volumeImgLow.src = "svg/volume_low.svg";
        res.volumeImgLow.width = 20;
        res.volumeImgLow.height = 20;
        res.volumeImgLow.setAttribute("class", "unselectable");

        res.volumeImgMuted = document.createElement("img");
        res.volumeImgMuted.src = "svg/volume_muted.svg";
        res.volumeImgMuted.width = 20;
        res.volumeImgMuted.height = 20;
        res.volumeImgMuted.setAttribute("class", "unselectable");

        res.downloadImg = document.createElement("img");
        res.downloadImg.src = "svg/download.svg";
        res.downloadImg.width = 20;
        res.downloadImg.height = 20;
        res.downloadImg.setAttribute("class", "unselectable");

        res.subsImg = document.createElement("img");
        res.subsImg.src = "svg/subs.svg";
        res.subsImg.width = 20;
        res.subsImg.height = 20;
        res.subsImg.setAttribute("class", "unselectable");

        res.settingsImg = document.createElement("img");
        res.settingsImg.src = "svg/settings.svg";
        res.settingsImg.width = 20;
        res.settingsImg.height = 20;
        res.settingsImg.setAttribute("class", "unselectable");

        res.fullscreenImg = document.createElement("img");
        res.fullscreenImg.src = "svg/fullscreen.svg";
        res.fullscreenImg.width = 20;
        res.fullscreenImg.height = 20;
        res.fullscreenImg.setAttribute("class", "unselectable");
    }

    createHtmlControls() {
        let playerControls = document.createElement("div");
        playerControls.id = "player_controls";
        playerControls.setAttribute("ondragstart", "return false");

        let timestampSlider = document.createElement("input");
        timestampSlider.id = "player_timestamp_slider";
        timestampSlider.type = "range";
        timestampSlider.min = "0";
        timestampSlider.max = "1";
        timestampSlider.value = "0";
        timestampSlider.step = "any";
        playerControls.appendChild(timestampSlider);
        this.htmlControls.timestampSlider = timestampSlider;

        let playToggle = document.createElement("div");
        playToggle.id = "player_play_toggle";
        playToggle.appendChild(this.resources.playImg);
        playerControls.appendChild(playToggle);
        this.htmlControls.playToggleButton = playToggle;

        let next = document.createElement("div");
        next.id = "player_next";
        next.appendChild(this.resources.nextImg);
        playerControls.appendChild(next);
        this.htmlControls.nextButton = next;

        let loop = document.createElement("div");
        loop.id = "player_loop";
        loop.appendChild(this.resources.loopImg);
        playerControls.appendChild(loop);
        this.htmlControls.loopButton = loop;

        let volume = document.createElement("div");
        volume.id = "player_volume";
        volume.appendChild(this.resources.volumeImgFull);
        playerControls.appendChild(volume);
        this.htmlControls.volume = volume;

        let volumeSlider = document.createElement("input");
        volumeSlider.id = "player_volume_slider";
        volumeSlider.type = "range";
        volumeSlider.min = "0";
        volumeSlider.max = "1";
        volumeSlider.value = "1";
        volumeSlider.step = "any";
        playerControls.appendChild(volumeSlider);
        this.htmlControls.volumeSlider = volumeSlider;

        let timestamp = document.createElement("span");
        timestamp.id = "player_timestamp";
        timestamp.textContent = "00:00 / 00:00";
        playerControls.appendChild(timestamp);
        this.htmlControls.timestamp = timestamp;

        let download = document.createElement("div");
        download.id = "player_download";
        download.appendChild(this.resources.downloadImg);
        playerControls.appendChild(download);
        this.htmlPlayerRoot.appendChild(playerControls);
        this.htmlControls.download = download;

        let subs = document.createElement("div");
        subs.id = "player_subs";
        subs.appendChild(this.resources.subsImg);
        playerControls.appendChild(subs);
        this.htmlPlayerRoot.appendChild(playerControls);
        this.htmlControls.subs = subs;

        let settings = document.createElement("div");
        settings.id = "player_settings";
        settings.appendChild(this.resources.settingsImg);
        playerControls.appendChild(settings);
        this.htmlPlayerRoot.appendChild(playerControls);
        this.htmlControls.settings = settings;

        let fullscreen = document.createElement("div");
        fullscreen.id = "player_fullscreen";
        fullscreen.appendChild(this.resources.fullscreenImg);
        playerControls.appendChild(fullscreen);
        this.htmlPlayerRoot.appendChild(playerControls);
        this.htmlControls.fullscreen = fullscreen;
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

// This is a separate class for more clarity
class Options {
    constructor() {
        this.showPlayToggleButton = true;
        this.showNextButton = false;
        this.showVolumeSlider = true;
        this.showFullscreenButton = true;
        this.showSubtitlesButton = true;
        this.showAutoPlay = true;
        // video.width = video.videoWidth, video.height = video.videoHeight
        this.resizeToMedia = true;
        this.seekBy = 5; // arrow seeking offset provided in seconds
        this.hideControlsDelay = 2.5; // time in seconds before controls disappear
    }
    // Ensure values are the intended type and within some reasonable range
    valid() {
        if (typeof this.seekBy !== "number" || this.seekBy < 0) {
            return false;
        }
        if (typeof this.hideControlsDelay !== "number") {
            return false;
        }
        return true;
    }
}
