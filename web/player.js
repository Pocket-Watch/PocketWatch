export { Player };

function getUrlMediaType(url) {
    if (url.endsWith(".m3u8")) {
        return "application/x-mpegURL";
    }

    return ""
}

// NOTE(kihau): Code duplicate from rewrite.js. Should be imported instead.
async function httpPostAsync(endpoint, data) {
    const headers = new Headers();
    headers.set("Content-Type", "application/json");

    const options = {
        method: "POST",
        body: JSON.stringify(data),
        headers: headers,
    };

    try {
        const response = await fetch(endpoint, options);
        if (!response.ok) {
            console.error("ERROR: POST request for endpoint: " + endpoint + " failed: " + response.status)
        }
    } catch (error) {
        console.error("ERROR: POST request for endpoint: " + endpoint + " failed: " + error)
    }
}

// NOTE(kihau): Code duplicate from rewrite.js. Should be imported instead.
function getRandomId() {
    const min = 1;
    const max = 999999999999999;
    const number = Math.floor(Math.random() * (max - min) + min);
    return number.toString();
}

class Player {
    constructor() {
        // Div container where either the player or the placeholder resides.
        this.container = document.getElementById("player_container");

        // Corresponds to the fluid player "class" responsible for managing the fluid player.
        this.fluidPlayer = null;

        // Corresponds to the actual html player element called </video>. The fluid player attaches to it.
        this.htmlPlayer = null;

        // Updates before programmatic player play and in eventOnPlay to discard events that
        // were not invoked by the user.
        this.programmaticPlay = false;

        // Updates before programmatic player pause and in eventOnPause to discard events that
        // were not invoked by the user.
        this.programmaticPause = false;

        // Updates before programmatic currentTime assignment and in eventOnSeek to discard events
        // that were not invoked by the user.
        // this.programmaticSeek = false;
    }

    eventOnPlay() {
        let data = this;
        return function onPlay(_event) {
            console.info("INFO: Player play event was triggered.");
            if (data.programmaticPlay) {
                data.programmaticPlay = false;
                return;
            }

            let uuid = getRandomId();
            const payload = {
                uuid: uuid,
                timestamp: data.htmlPlayer.currentTime,
                username: "dummy",
            };

            console.info("INFO: Sending play request.");
            httpPostAsync("/watch/api/play", payload);
        }
    }

    eventOnPause() {
        let data = this;
        return function onPause(_event) {
            console.info("INFO: Player pause event was triggered.");
            if (data.programmaticPause) {
                data.programmaticPause = false;
                return;
            }

            let uuid = getRandomId();
            const payload = {
                uuid: uuid,
                timestamp: data.htmlPlayer.currentTime,
                username: "dummy",
            };

            console.info("INFO: Sending pause request.");
            httpPostAsync("/watch/api/pause", payload);
        }
    }

    eventOnSeek() { }

    subscribeToPlayerEvents() {
        if (!this.fluidPlayer) {
            console.warn("WARNING: Failed to subscribe to player events. The player is null.");
            return;
        }

        this.fluidPlayer.on("play", this.eventOnPlay());
        this.fluidPlayer.on("pause", this.eventOnPause());
    }

    isVideoPlaying() {
        return !this.htmlPlayer && this.htmlPlayer.currentTime > 0 && !this.htmlPlayer.paused && !this.htmlPlayer.ended;
    }

    destroyPlayer() {
        if (this.fluidPlayer) {
            this.fluidPlayer.destroy();
        }

        this.htmlPlayer = null;
        this.fluidPlayer = null;

        while (this.container.firstChild) {
            this.container.removeChild(this.container.lastChild);
        }
    }

    createFluidPlayer(url) {
        let video = document.createElement('video');
        video.width = window.innerWidth;
        video.id = "player";

        let source = document.createElement('source');
        source.src = url;
        source.type = getUrlMediaType(url);
        video.appendChild(source);

        this.container.appendChild(video);

        this.htmlPlayer = video;

        this.fluidPlayer = fluidPlayer('player', {
            hls: {
                overrideNative: true
            },
            layoutControls: {
                title: "TITLE PLACEHOLDER",
                doubleclickFullscreen: true,
                subtitlesEnabled: true,
                autoPlay: true,
                controlBar: {
                    autoHide: true,
                    autoHideTimeout: 2.5,
                    animated: true,
                    playbackRates: ['x2', 'x1.5', 'x1', 'x0.5']
                },
                miniPlayer: {
                    enabled: false,
                    width: 400,
                    height: 225
                }
            }
        });
    }

    createPlayerPlaceholder() {
        let img = document.createElement('img');
        img.src = "nothing_is_playing.png";
        img.width = window.innerWidth;
        this.container.appendChild(img);
    }

    play() {
        if (!this.fluidPlayer) {
            console.warn("WARNING: Play was triggered, but the player is not initialized.");
            return;
        }

        if (this.isVideoPlaying()) {
            console.warn("WARNING: Play was triggered, but the video is already playing.");
            return;
        }

        this.programmaticPlay = true;
        this.fluidPlayer.play();
        console.info("INFO: Player play() was called");
    }

    pause() {
        if (!this.fluidPlayer) {
            console.warn("WARNING: Pause was triggered, but the player is not initialized.");
            return;
        }

        if (!this.isVideoPlaying()) {
            console.warn("WARNING: Pause was triggered, but the video is already paused.");
            return;
        }

        this.programmaticPause = true;
        this.fluidPlayer.pause();
        console.info("INFO: Player pause() was called");
    }

    seek() { }

    setUrl(url) {
        this.destroyPlayer();

        if (url) {
            this.createFluidPlayer(url);
            this.subscribeToPlayerEvents();
        } else {
            this.createPlayerPlaceholder();
        }
    }
}
