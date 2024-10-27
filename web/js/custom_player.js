import * as api from "./api.js";

export { Player };

const MAX_DESYNC = 1.5;

class Player {
    constructor() {
        // Div container where either the player or the placeholder resides.
        this.htmlPlayerRoot = document.getElementById("player_container");
        // this.root = document.getElementById("player_root");

        // Corresponds to the actual html player element called either </video> or </audio>. 
        this.htmlPlayer = null;

        this.controls = {
            playButton: null,
            volumeSlider: null,
            seekSlider: null,
        };

        // this.htmlInputUrl = document.getElementById("input_url");
        // this.htmlRefererInput = document.getElementById("referer");
        // this.htmlInputTitle = document.getElementById("input_title");
        // this.htmlCurrentUrl = document.getElementById("current_url");
        // this.htmlProxyCheckbox = document.getElementById("proxy");
        // this.htmlAutoplayCheckbox = document.getElementById("autoplay");
        // this.htmlAudioonlyCheckbox = document.getElementById("audioonly");
        // this.htmlLoopingCheckbox = document.getElementById("looping");
    }

    // loopingEnabled() {
    //     return this.htmlLoopingCheckbox.checked;
    // }
    //
    // loopingSet(looping) {
    //     this.htmlLoopingCheckbox.checked = looping;
    // }
    //
    // autoplaySet(looping) {
    //     this.htmlLoopingCheckbox.checked = looping;
    // }

    play() {
        // Send server play request here.
        this.htmlPlayer.play();
        this.controls.playButton.textContent = "Pause";
    }

    pause() {
        // Send server pause request here.
        this.htmlPlayer.pause();
        this.controls.playButton.textContent = "Play";
    }

    seek(timestamp) {
        // Send server seek request here.
        this.htmlPlayer.currentTime = timestamp;
    }

    seekRelative(timeOffset) {
        var timestamp = this.htmlPlayer.currentTime + timeOffset;
        if (timestamp < 0) {
            timestamp = 0;
        }

        this.seek(timestamp);
    };

    onUserPlayToggle() {
        if (!this.htmlPlayer) {
            console.warn("WARN: Player::onUserPlayToggle was invoked but the player has not been initialized");
            return;
        }

        if (this.htmlPlayer.paused) {
            this.play();
        } else {
            this.pause();
        }
    }

    destroyPlayer() {
    }

    createPlayerVideo(url) {
        let video = document.createElement('video');
        this.htmlPlayerRoot.appendChild(video);

        let track = document.createElement("track")
        track.label = "foo";
        track.kind = "subtitles";
        track.src = "/watch/media/Cars.vtt";
        video.appendChild(track)

        let width = window.innerWidth * 0.95;
        video.width = width;
        video.height = width * 9 / 16;
        video.id = "player";
        video.controls = true;
        
        // let data = this;
        video.onclick = () => {
            this.onUserPlayToggle();
        };

        video.onkeydown = (event) => {
            console.debug(event);

            switch (event.keyCode) {
                // Space
                case 32: {
                    this.onUserPlayToggle();
                } break;

                // Left arrow
                case 37: {
                    this.seekRelative(-10.0);
                } break;

                // Right arrow
                case 39: {
                    this.seekRelative(10.0);
                } break;

                // F key
                case 70: {
                    this.htmlPlayer.requestFullscreen();
                } break;
            }
        }

        let source = document.createElement('source');
        video.appendChild(source);
        source.src = url;

        this.htmlPlayer = video;
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

    createPlayerControls() {
        let controls = document.createElement('div');
        controls.className = "player_controls_root";
        this.htmlPlayerRoot.appendChild(controls);

        let playButton = document.createElement('button');
        playButton.id = "play_button";
        playButton.textContent = "Play";
        playButton.onclick = (_event) => {
            this.onUserPlayToggle();
        };

        controls.appendChild(playButton);

        let volumeSlider = document.createElement('input');
        volumeSlider.id = "volume_slider";
        volumeSlider.type = "range";
        volumeSlider.min = 0;
        volumeSlider.max = 100;
        controls.appendChild(volumeSlider);

        // let volumeLabel = document.createElement('label');
        // volumeLabel.textContent = "Volume";
        // controls.appendChild(volumeLabel);

        let seekSlider = document.createElement('input');
        seekSlider.id = "seek_slider";
        seekSlider.type = "range";
        seekSlider.min = 0;
        seekSlider.max = 100;
        controls.appendChild(seekSlider);

        this.controls.playButton = playButton;
        this.controls.volumeSlider = volumeSlider;
        this.controls.seekSlider = seekSlider;
    }

    createPlayer(entry) {
        this.createPlayerVideo(entry.url);
        this.createPlayerControls();
    }

    setUrl(url) {
        // this.createPlayerVideo(url);
        // this.createPlayerControls();
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
