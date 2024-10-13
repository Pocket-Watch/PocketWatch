export { Player };

class Player {
    constructor() {
        // Div container where either the player or the placeholder resides.
        this.root = document.getElementById("player_container");
        // this.root = document.getElementById("player_root");

        // Corresponds to the actual html player element called either </video> or </audio>. 
        this.player = null;

        this.controls = {
            playButton: null,
            volumeSlider: null,
            seekSlider: null,
        };
    }

    play() {
        // Send server play request here.
        this.player.play();
        this.controls.playButton.textContent = "Pause";
    }

    pause() {
        // Send server pause request here.
        this.player.pause();
        this.controls.playButton.textContent = "Play";
    }

    seek(timestamp) {
        // Send server seek request here.
        this.player.currentTime = timestamp;
    }

    seekRelative(timeOffset) {
        var timestamp = this.player.currentTime + timeOffset;
        if (timestamp < 0) {
            timestamp = 0;
        }

        this.seek(timestamp);
    };

    onUserPlayToggle() {
        if (!this.player) {
            console.warn("WARN: Player::playOrPause was invoked but the player has not been initialized");
            return;
        }

        if (this.player.paused) {
            this.play();
        } else {
            this.pause();
        }
    }

    destroyPlayer() {
    }

    createPlayerVideo(url) {
        let video = document.createElement('video');
        this.root.appendChild(video);

        let width = window.innerWidth * 0.95;
        video.width = width;
        video.height = width * 9 / 16;
        video.id = "player";
        // video.controls = true;
        video.controls = false;
        
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
                    this.player.requestFullscreen();
                } break;
            }
        }

        let source = document.createElement('source');
        video.appendChild(source);
        source.src = url;

        this.player = video;
    }

    createPlayerControls() {
        let controls = document.createElement('div');
        controls.className = "player_controls_root";
        this.root.appendChild(controls);

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

    createPlayer(url) {
        this.createPlayerVideo(url);
        this.createPlayerControls();
    }
}
