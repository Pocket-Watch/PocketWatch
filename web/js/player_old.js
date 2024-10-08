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

const MAX_DESYNC = 2.0

class Player {
    constructor() {
        // Div container where either the player or the placeholder resides.
        this.container = document.getElementById("player_container");

        // Corresponds to the actual html player element called </video>. 
        this.htmlPlayer = null;

        // Updates before programmatic player play and in eventOnPlay to discard events that
        // were not invoked by the user.
        this.programmaticPlay = false;

        // Updates before programmatic player pause and in eventOnPause to discard events that
        // were not invoked by the user.
        this.programmaticPause = false;

        // Updates before programmatic currentTime assignment and in eventOnSeek to discard events
        // that were not invoked by the user.
        this.programmaticSeek = false;
    }

    isDesynced(server_timestamp) {
        if (!this.htmlPlayer) {
            return false;
        }

        let desync = server_timestamp - this.htmlPlayer.currentTime;
        console.info("INFO: Client is desynced by: " + desync + " seconds");

        return MAX_DESYNC < Math.abs(desync);
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

    eventOnSeek() { 
        let data = this;
        return function onSeek(_event) {
            console.info("INFO: Player seek event was triggered.");
            if (data.programmaticSeek) {
                data.programmaticSeek = false;
                return;
            }

            let uuid = getRandomId();
            const payload = {
                uuid: uuid,
                timestamp: data.htmlPlayer.currentTime,
                username: "dummy",
            };

            console.info("INFO: Sending seek request.");
            httpPostAsync("/watch/api/seek", payload);
        }
    }

    subscribeToPlayerEvents() {
        if (!this.htmlPlayer) {
            console.warn("WARNING: Failed to subscribe to player events. The player is null.");
            return;
        }

        this.htmlPlayer.addEventListener("play", this.eventOnPlay());
        this.htmlPlayer.addEventListener("pause", this.eventOnPause());
        this.htmlPlayer.addEventListener("seeked", this.eventOnSeek());
    }

    isVideoPlaying() {
        return this.htmlPlayer && this.htmlPlayer.currentTime > 0 && !this.htmlPlayer.paused && !this.htmlPlayer.ended;
    }

    destroyPlayer() {
        if (this.htmlPlayer) {
            this.htmlPlayer.destroy();
        }

        this.htmlPlayer = null;

        while (this.container.firstChild) {
            this.container.removeChild(this.container.lastChild);
        }
    }

    createHtmlPlayer(url) {
        let video = document.createElement('video');
        this.container.appendChild(video);

        let width = window.innerWidth * 0.95;
        video.width = width;
        video.height = width * 9 / 16;
        video.id = "player";
        video.controls = true;

        let source = document.createElement('source');
        video.appendChild(source);

        source.src = url;
        source.type = getUrlMediaType(url);

        this.htmlPlayer = video;
    }

    createPlayerPlaceholder() {
        let img = document.createElement('img');
        img.src = "nothing_is_playing.png";
        img.width = window.innerWidth;
        this.container.appendChild(img);
    }

    play() {
        if (!this.htmlPlayer) {
            console.warn("WARNING: Play was triggered, but the player is not initialized.");
            return;
        }

        if (this.isVideoPlaying()) {
            console.warn("WARNING: Play was triggered, but the video is already playing.");
            return;
        }

        this.programmaticPlay = true;
        this.htmlPlayer.play();
        console.info("INFO: Player play() was called");
    }

    pause() {
        if (!this.htmlPlayer) {
            console.warn("WARNING: Pause was triggered, but the player is not initialized.");
            return;
        }

        if (!this.isVideoPlaying()) {
            console.warn("WARNING: Pause was triggered, but the video is already paused.");
            return;
        }

        this.programmaticPause = true;
        this.htmlPlayer.pause();
        console.info("INFO: Player pause() was called");
    }

    seek(timestamp) { 
        if (!this.htmlPlayer) {
            console.warn("WARNING: Seek was triggered, but the player is not initialized.");
            return;
        }

        this.programmaticSeek = true;
        this.htmlPlayer.currentTime = timestamp;
        console.info("INFO: Player seek() was called");
    }

    setUrl(url) {
        this.destroyPlayer();

        if (url) {
            this.createHtmlPlayer(url);
            this.subscribeToPlayerEvents();
        } else {
            this.createPlayerPlaceholder();
        }
    }
}
