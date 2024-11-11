import * as api from "./api.js";

import { Player, Options } from "./custom_player.js";
import { userSelf } from "./main.js"

export { PlayerArea as PlayerArea }

const MAX_DESYNC = 1.5;

class PlayerArea {
    constructor() {
        let video0 = document.getElementById("video0");
        let options = new Option();
        this.player = new Player(video0, options);

        this.htmlInputUrl = document.getElementById("input_url");
        this.htmlRefererInput = document.getElementById("referer");
        this.htmlInputTitle = document.getElementById("input_title");
        this.htmlCurrentUrl = document.getElementById("current_url");
        this.htmlProxyCheckbox = document.getElementById("proxy");
        this.htmlAutoplayCheckbox = document.getElementById("autoplay");

        this.currentEntryId = 0;

        this.attachPlayerEvents();
    }

    loopingEnabled() {
        return this.player.getLoop();
    }

    setLooping(looping) {
        this.player.setLoop(looping);
    }

    setAutoplay(autoplay) {
        this.htmlAutoplayCheckbox.checked = autoplay;
    }

    setEntry(entry) {
        if (!entry || !entry.url) {
            this.player.setVideoTrack("video/nothing_is_playing.mp4");
            this.player.setTitle("Nothing is playing");

            this.currentEntryId = 0;
            this.htmlCurrentUrl.value = "";
            return;
        }

        this.currentEntryId = entry.id
        this.htmlCurrentUrl.value = entry.url;

        let url = entry.url
        if (entry.use_proxy) {
            url = "/watch/proxy/proxy.m3u8"
        }

        if (entry.source_url) {
            url = entry.source_url;
        }

        this.player.setVideoTrack(url);
        this.player.setTitle(entry.title);
    }

    play() {
        this.player.play();
    }

    pause() {
        this.player.pause();
    }

    seek(timestamp) {
        this.player.seek(timestamp);
    }

    resync(timestamp, userId) {
        let desync = timestamp - this.player.getCurrentTime();

        if (userId == 0) {
            console.info("INFO: Received resync event from SERVER at", timestamp, "with desync:", desync);
        } else {
            console.info("INFO: Received resync event from USER id", userId, "at", timestamp, "with desync:", desync);
        }

        if (Math.abs(desync) > MAX_DESYNC) {
            let diff = Math.abs(desync) - MAX_DESYNC
            console.warn("You are desynced! MAX_DESYNC(" + MAX_DESYNC + ") exceeded by:", diff, "Trying to resync now!");
            this.seek(timestamp);
        }
    }

    attachPlayerEvents() {
        this.player.onControlsPlay(() => {
            api.playerPlay(this.player.getCurrentTime());
        });

        this.player.onControlsPause(() => {
            api.playerPause(this.player.getCurrentTime());
        });

        this.player.onControlsSeeked(timestamp => {
            api.playerSeek(timestamp);
        });

        this.player.onControlsNext(() => {
            api.playerNext(this.currentEntryId);
        });

        this.player.onControlsLoop(enabled => {
            api.playerLooping(enabled);
        });

        this.player.onPlaybackEnd(() => {
            if (this.htmlAutoplayCheckbox.checked) {
                api.playerNext(this.currentEntryId);
            }
        });
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
    }
}
