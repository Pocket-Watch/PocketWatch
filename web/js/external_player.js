import * as api from "./api.js";
import { userSelf } from "./main.js"

export { Player }

const MAX_DESYNC = 1.5;

function getUrlMediaType(url) {
    if (url.endsWith(".m3u8")) {
        return "application/x-mpegURL";
    }

    return "";
}

class Player {
    constructor() {
        this.htmlPlayerRoot = document.getElementById("player_container");
        this.fluidPlayer = null;
        this.htmlPlayer = null;

        this.htmlInputUrl = document.getElementById("input_url");
        this.htmlRefererInput = document.getElementById("referer");
        this.htmlInputTitle = document.getElementById("input_title");
        this.htmlCurrentUrl = document.getElementById("current_url");
        this.htmlProxyCheckbox = document.getElementById("proxy");
        this.htmlAutoplayCheckbox = document.getElementById("autoplay");
        this.htmlAudioonlyCheckbox = document.getElementById("audioonly");
        this.htmlLoopingCheckbox = document.getElementById("looping");

        this.programmaticPlay = false; // Updates before programmatic play() and in .onplay
        this.programmaticPause = false; // Updates before programmatic pause() and in .onpause
        this.programmaticSeek = false; // Updates before programmatic currentTime assignment and in .onseeked

        this.currentEntryId = 0;
        this.subtitles = [];
    }

    loopingEnabled() {
        return this.htmlLoopingCheckbox.checked;
    }

    loopingSet(looping) {
        this.htmlLoopingCheckbox.checked = looping;
    }

    autoplaySet(looping) {
        this.htmlLoopingCheckbox.checked = looping;
    }

    setUrl(entry) {
        this.destroyFluidPlayer();

        if (!entry || !entry.url) {
            this.createDummyPlayer();
            return;
        }

        this.currentEntryId = entry.id
        this.htmlCurrentUrl.value = entry.url;

        this.createHtmlPlayer(entry);
        this.createFluidPlayer(entry);
        this.subscribeToHtmlPlayerEvents();
    }

    isVideoPlaying() {
        return !this.htmlPlayer.paused && !this.htmlPlayer.ended;
    }

    play() {
        if (!this.htmlPlayer) {
            console.warn("WARN: Player::play was called but html player is null");
            return;
        }

        if (this.isVideoPlaying()) {
            return;
        }

        this.programmaticPlay = true;
        this.fluidPlayer.play();
    }

    pause() {
        if (!this.htmlPlayer) {
            console.warn("WARN: Player::pause was called but html player is null");
            return;
        }

        if (!this.isVideoPlaying()) {
            return;
        }

        this.programmaticPause = true;
        this.fluidPlayer.pause();
    }

    seek(timestamp) {
        if (!this.htmlPlayer) {
            console.warn("WARN: Player::seek was called but html player is null");
            return;
        }

        this.programmaticSeek = true;
        this.fluidPlayer.skipTo(timestamp);
    }

    resync(timestamp, userId) {
        let desync = timestamp - this.htmlPlayer.currentTime;

        if (userId == 0) {
            console.info("INFO: Recieved resync event from SERVER at", timestamp, "with desync:", desync);
        } else {
            console.info("INFO: Recieved resync event from USER id", userId, "at", timestamp, "with desync:", desync);
        }

        if (Math.abs(desync) > MAX_DESYNC) {
            let diff = Math.abs(desync) - MAX_DESYNC
            console.warn("You are desynced! MAX_DESYNC(" + MAX_DESYNC + ") exceeded by:", diff, "Trying to resync now!");
            this.seek(timestamp);
        }
    }

    htmlPlayerOnPlay(_event) {
        let data = this;

        return function() {
            if (data.programmaticPlay) {
                data.programmaticPlay = false;
                return;
            }

            api.playerPlay(data.htmlPlayer.currentTime);
        }
    }

    htmlPlayerOnPause(_event) {
        let data = this;
        return function() {
            if (data.programmaticPause) {
                data.programmaticPause = false;
                return;
            }

            api.playerPause(data.htmlPlayer.currentTime);
        }
    }

    htmlPlayerOnSeek(_event) {
        let data = this;
        return function() {
            if (data.programmaticSeek) {
                data.programmaticSeek = false;
                return;
            }

            api.playerSeek(data.htmlPlayer.currentTime);
        }
    }

    htmlPlayerOnEnded(_event) {
        let data = this;
        return function() {
            if (data.htmlAutoplayCheckbox.checked) {
                api.playerNext(data.currentEntryId);
            }
        }
    }

    subscribeToHtmlPlayerEvents() {
        if (!this.htmlPlayer) {
            console.warn("WARN: Html player is null, failed to subscribe to player events");
        }

        this.htmlPlayer.addEventListener("play", this.htmlPlayerOnPlay());
        this.htmlPlayer.addEventListener("pause", this.htmlPlayerOnPause());
        this.htmlPlayer.addEventListener("seeked", this.htmlPlayerOnSeek());
        this.htmlPlayer.addEventListener("ended", this.htmlPlayerOnEnded());
    }

    unsubscribeFromHtmlPlayerEvents() {
        if (!this.htmlPlayer) {
            console.warn("WARN: Html player is null, failed to subscribe to player events");
        }

        this.htmlPlayer.removeEventListener("play", this.htmlPlayerOnPlay());
        this.htmlPlayer.removeEventListener("pause", this.htmlPlayerOnPause());
        this.htmlPlayer.removeEventListener("seeked", this.htmlPlayerOnSeek());
        this.htmlPlayer.removeEventListener("ended", this.htmlPlayerOnEnded());
    }

    createHtmlPlayer(entry) {
        let video = document.createElement("video");
        video.width = window.innerWidth;
        // video.height = window.innerHeight;
        video.id = "player";
        if (this.subtitles.length > 0) {
            for (let i = 0; i < this.subtitles.length; i++) {
                let track = document.createElement("track")
                track.label = this.subtitles[i]
                track.kind = "metadata"
                track.src = this.subtitles[i]
                video.appendChild(track)
            }
        }

        let url = entry.url
        if (entry.use_proxy) {
            url = "/watch/proxy/proxy.m3u8"
        }

        if (entry.source_url) {
            url = entry.source_url;
        }

        let source = document.createElement("source");
        source.src = url;
        source.type = getUrlMediaType(entry.url);
        video.appendChild(source);

        // TOOD(kihau): Remove invalid entries.
        // source.addEventListener("error", () => {
        //     // TODO(kihau): Display pop-up notification that the playback failed.
        //     api.playerNext(currentEntryId);
        // });

        this.htmlPlayerRoot.appendChild(video);
        this.htmlPlayer = video;
    }

    createFluidPlayer(entry) {
        let player = fluidPlayer("player", {
            hls: {
                overrideNative: true,
            },
            layoutControls: {
                title: entry.title,
                doubleclickFullscreen: true,
                subtitlesEnabled: true,
                autoPlay: this.htmlAutoplayCheckbox.checked,
                controlBar: {
                    autoHide: true,
                    autoHideTimeout: 2.5,
                    animated: true,
                    playbackRates: ["x2", "x1.5", "x1", "x0.5"],
                },
                miniPlayer: {
                    enabled: false,
                    width: 400,
                    height: 225,
                },
            },
        });

        this.fluidPlayer = player;
    }

    createDummyPlayer() {
        let video = document.createElement("video");
        video.width = window.innerWidth * 0.95;
        video.id = "player";

        let source = document.createElement("source");
        video.poster = "img/nothing_is_playing.png";
        source.src = "video/nothing_is_playing.mp4";
        video.appendChild(source);

        this.htmlPlayerRoot.appendChild(video);

        this.htmlPlayer = video;
        this.fluidPlayer = null;
    }

    destroyFluidPlayer() {
        this.currentEntryId = 0;
        this.htmlCurrentUrl.value = "";

        if (this.fluidPlayer) {
            this.unsubscribeFromHtmlPlayerEvents();
            this.fluidPlayer.pause();
            this.fluidPlayer.destroy();
            this.fluidPlayer = null;
        } else if (this.htmlPlayer) {
            this.htmlPlayerRoot.removeChild(this.htmlPlayer);
        }
    }

    createApiEntry(url) {
        const entry = {
            id: 0,
            url: url,
            title: this.htmlInputTitle.value,
            user_id: userSelf.id,
            use_proxy: this.htmlProxyCheckbox.checked,
            referer_url: this.htmlRefererInput.value,
            created: new Date,
        };

        return entry;
    }

    setNewEntry() {
        let url = this.htmlInputUrl.value;
        this.htmlInputUrl.value = "";

        console.info("INFO: Current video source url: ", url);

        let entry = this.createApiEntry(url);
        api.playerSet(entry);
    }

    attachHtmlEventHandlers() {
        window.inputUrlOnKeypress = (event) => {
            if (event.key === "Enter") {
                this.setNewEntry();
            }
        };

        window.playerSetOnClick = () => {
            this.setNewEntry();
        };

        window.playerNextOnClick = () => {
            api.playerNext(this.currentEntryId);
        };

        window.playlistAddTopOnClick = () => {
            let url = this.htmlInputUrl.value;
            this.htmlInputUrl.value = "";

            if (!url) {
                console.warn("WARNING: Url is empty, not adding to the playlist.");
                return;
            }

            let entry = this.createApiEntry(url);
            api.playlistAdd(entry);
        };

        window.autoplayOnClick = () => {
            api.playerAutoplay(this.htmlAutoplayCheckbox.checked);
        };

        window.loopingOnClick = () => {
            api.playerLooping(this.htmlLoopingCheckbox.checked);
        };

        window.shiftSubtitlesBack = () => {
            if (this.htmlPlayer.textTracks.length === 0) {
                console.warn("NO SUBTITLE TRACKS")
                return;
            }

            let track = this.htmlPlayer.textTracks[0];
            console.debug("DEBUG: CUES", track.cues)
            for (let i = 0; i < track.cues.length; i++) {
                let cue = track.cues[i];
                cue.startTime -= 0.5;
                cue.endTime -= 0.5;
            }
        };

        window.shiftSubtitlesForward = () => {
            if (this.htmlPlayer.textTracks.length === 0) {
                console.warn("NO SUBTITLE TRACKS")
                return;
            }

            let track = this.htmlPlayer.textTracks[0];
            console.info("CUES", track.cues)
            for (let i = 0; i < track.cues.length; i++) {
                let cue = track.cues[i];
                cue.startTime += 0.5;
                cue.endTime += 0.5;
            }
        };
    }
}
