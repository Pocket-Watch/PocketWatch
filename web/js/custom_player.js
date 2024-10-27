export { Player };

class Player {
    constructor() {
        // Div container where either the player or the placeholder resides.
        this.htmlPlayerRoot = document.getElementById("player_container");

        // Corresponds to the actual html player element called either </video> or </audio>. 
        this.htmlVideo = null;

        this.htmlControls = {
            playToggleButton: null,
            nextButton: null,
            volume: null,
            volumeSlider: null,
        };

    }

    // isVideoPlaying() {
    //     return !this.htmlVideo.paused && !this.htmlPlayer.ended;
    // }

    play() {
        // TODO(kihau): Do not recreate those every time and instead create them once and then reuse them?
        let pauseSvg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
        let pauseUse = document.createElementNS("http://www.w3.org/2000/svg", "use");
        pauseSvg.appendChild(pauseUse);
        pauseUse.setAttribute("href", "svg/player_icons.svg#pause");
        this.htmlControls.playToggleButton.getElementsByTagName("svg")[0].replaceWith(pauseSvg);

        this.htmlVideo.play();
    }

    pause() {
        // TODO(kihau): Do not recreate those every time and instead create them once and then reuse them?
        let playSvg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
        let playUse = document.createElementNS("http://www.w3.org/2000/svg", "use");
        playUse.setAttribute("href", "svg/player_icons.svg#play");
        playSvg.appendChild(playUse);
        this.htmlControls.playToggleButton.getElementsByTagName("svg")[0].replaceWith(playSvg);
        this.htmlVideo.pause();
    }

    seek(timestamp) {
    }

    seekRelative(timeOffset) {
    };

    destroyPlayer() {
    }

    createPlayer() {
        this.createHtmlVideo();
        this.createHtmlControls();
        this.attachHtmlEvents();
    }

    createHtmlVideo() {
        let video = document.createElement("video");
        video.id = "player_video"
        this.htmlPlayerRoot.appendChild(video);
        this.htmlVideo = video;
    }

    onControlsPlay() { }
    onControlsPause() { }
    onControlsNext() { }

    togglePlay() {
        if (this.htmlVideo.paused) {
            this.onControlsPlay();
            this.play();
        } else {
            this.onControlsPause();
            this.pause();
        }
    }

    attachHtmlEvents() {
        this.htmlControls.playToggleButton.onclick = () => {
            this.togglePlay();
        };

        this.htmlControls.nextButton.onclick = () => {
            this.onControlsNext();
        };

        this.htmlVideo.onkeydown = (event) => {
            if (event.key == " " || event.code == "Space" || event.keyCode == 32) {
                this.togglePlay();
            }
        }

        this.htmlVideo.onclick = (event) => {
            this.togglePlay();
        }
    }

    createHtmlControls() {
        let timestampSlider = document.createElement("input");
        timestampSlider.id = "player_timestamp_slider";
        timestampSlider.type = "range";
        timestampSlider.min = "0";
        timestampSlider.max = "100";
        timestampSlider.value = "0";
        this.htmlPlayerRoot.appendChild(timestampSlider);

        let playerControls = document.createElement("div");
        playerControls.id = "player_controls";

        let playToggle = document.createElement("div");
        playToggle.id = "player_play_toggle";
        let playSvg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
        let playUse = document.createElementNS("http://www.w3.org/2000/svg", "use");
        playUse.setAttribute("href", "svg/player_icons.svg#play");
        playSvg.appendChild(playUse);
        playToggle.appendChild(playSvg);
        playerControls.appendChild(playToggle);
        this.htmlControls.playToggleButton = playToggle;

        let next = document.createElement("div");
        next.id = "player_next";
        let nextSvg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
        let nextUse = document.createElementNS("http://www.w3.org/2000/svg", "use");
        nextUse.setAttribute("href", "svg/player_icons.svg#next");
        nextSvg.appendChild(nextUse);
        next.appendChild(nextSvg);
        playerControls.appendChild(next);
        this.htmlControls.nextButton = next;

        let volume = document.createElement("div");
        volume.id = "player_volume";
        let volumeSvg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
        let volumeUse = document.createElementNS("http://www.w3.org/2000/svg", "use");
        volumeUse.setAttribute("href", "svg/player_icons.svg#volume");
        volumeSvg.appendChild(volumeUse);
        volume.appendChild(volumeSvg);
        playerControls.appendChild(volume);
        this.htmlControls.volume = volume;

        let volumeSlider = document.createElement("input");
        volumeSlider.id = "volume_slider";
        volumeSlider.type = "range";
        volumeSlider.min = "0";
        volumeSlider.max = "100";
        volumeSlider.value = "50";
        playerControls.appendChild(volumeSlider);
        this.htmlControls.volumeSlider = volumeSlider;

        let timestamp = document.createElement("span");
        timestamp.id = "timestamp";
        timestamp.textContent = "00:00 / 12:34";
        playerControls.appendChild(timestamp);

        let fullscreen = document.createElement("div");
        fullscreen.id = "player_fullscreen";
        let fullscreenSvg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
        let fullscreenUse = document.createElementNS("http://www.w3.org/2000/svg", "use");
        fullscreenUse.setAttribute("href", "svg/player_icons.svg#fullscreen");
        fullscreenSvg.appendChild(fullscreenUse);
        fullscreen.appendChild(fullscreenSvg);
        playerControls.appendChild(fullscreen);
        this.htmlPlayerRoot.appendChild(playerControls);
    }

    setVideoTrack(url) {
        let source = document.createElement("source");
        source.src = url;
        source.type = "video/mp4";
        this.htmlVideo.appendChild(source);
    }
}
