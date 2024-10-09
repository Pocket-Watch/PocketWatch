const DELTA = 1.5;

var player;
var video;

var input_url = document.getElementById("input_url");
var current_url = document.getElementById("current_url");
var name_field = document.getElementById("user_name");
var proxy_checkbox = document.getElementById("proxy");

var programmaticPlay = false; // Updates before programmatic play() and in .onplay
var programmaticPause = false; // Updates before programmatic pause() and in .onpause
var programmaticSeek = false; // Updates before programmatic currentTime assignment and in .onseeked
var ignoreNextRequest = false; // Updates before sending a sync request and on hasty events

/// --------------- HELPER FUNCTIONS: ---------------

function getUrlMediaType(url) {
    if (url.endsWith(".m3u8")) {
        return "application/x-mpegURL";
    }

    return "";
}

function getRandomId() {
    const min = 1;
    const max = 999999999999999;
    const number = Math.floor(Math.random() * (max - min) + min);
    return number.toString();
}

async function httpPost(endpoint, data) {
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
            console.error("ERROR: POST request for endpoint: " + endpoint + " failed: " + response.status);
        }
    } catch (error) {
        console.error("ERROR: POST request for endpoint: " + endpoint + " failed: " + error);
    }
}

async function httpGet(endpoint) {
    const headers = new Headers();
    headers.set("Content-Type", "application/json");

    const options = {
        method: "GET",
        body: null,
        headers: headers,
    };

    try {
        const response = await fetch(endpoint, options);
        if (!response.ok) {
            console.error("ERROR: POST request for endpoint: " + endpoint + " failed: " + response.status);
            return null;
        }

        return await response.json();
    } catch (error) {
        console.error("ERROR: POST request for endpoint: " + endpoint + " failed: " + error);
    }

    return null;
}

/// --------------- SERVER REQUESTS: ---------------

async function apiPlaylistAdd(url) {
    const entry = {
        uuid: getRandomId(),
        username: name_field.value,
        url: url,
    };

    console.info("INFO: Sending playlist add request for url: ", url);
    httpPost("/watch/api/playlist/add", entry);
}

async function apiPlaylistClear() {
    console.info("INFO: Sending playlist clear request.");
    httpPost("/watch/api/playlist/clear", null);
}

async function apiPlaylistGet() {
    console.info("INFO: Sending playlist get request.");
    return await httpGet("/watch/api/playlist/get");
}

async function apiSetUrl(url) {
    const payload = {
        uuid: getRandomId(),
        url: url,
        proxy: proxy_checkbox.checked,
    };

    console.info("INFO: Sending seturl request for a new url");
    httpPost("/watch/api/seturl", payload);
}

async function apiGet() {
    let data = await httpGet("/watch/api/get");
    console.info("INFO: Received data from get request to the server:");
    console.log(data);
    return data;
}

async function apiPlay() {
    const payload = {
        uuid: getRandomId(),
        timestamp: video.currentTime,
        username: name_field.value,
    };

    console.info("INFO: Sending play request to the server.");
    httpPost("/watch/api/play", payload);
}

async function apiPause() {
    const payload = {
        uuid: getRandomId(),
        timestamp: video.currentTime,
        username: name_field.value,
    };

    console.info("INFO: Sending pause request to the server.");
    httpPost("/watch/api/pause", payload);
}

async function apiSeek(timestamp) {
    const payload = {
        uuid: getRandomId(),
        timestamp: timestamp,
        username: name_field.value,
    };

    console.info("INFO: Sending seek request to the server.");
    httpPost("/watch/api/seek", payload);
}

/// --------------- HTML ELEMENT CALLBACKS: ---------------

function addToPlaylistButton() {
    let input_playlist = document.getElementById("input_playlist");
    let url = input_playlist.value;
    input_playlist.value = "";

    if (!url) {
        console.warn("WARNING: Url is empty, not adding to the playlist.");
        return;
    }

    apiPlaylistAdd(url);
}

function clearPlaylistButton() {
    apiPlaylistClear();
}

function setUrlButton() {
    let url = input_url.value;
    input_url.value = "";

    console.info("INFO: Current video source url: ", url);
    apiSetUrl(url);
}

/// --------------- PLAYLIST: ---------------

function addPlaylistElement(playlistHtml, index, entry) {
    let tr = document.createElement("tr");
    playlistHtml.appendChild(tr);

    let th = document.createElement("th");
    th.textContent = index + 1 + ".";
    th.scope = "row";
    tr.appendChild(th);

    let username = entry.username;
    if (!username) {
        username = "<unknown>";
    }

    let cell = tr.insertCell(-1);
    cell.textContent = username;

    cell = tr.insertCell(-1);
    cell.textContent = entry.url;

    cell = tr.insertCell(-1);
    cell.innerHTML = "<button>Remove</button>";
}

function getPlaylist() {
    apiPlaylistGet().then(playlist => {
        if (!playlist) {
            return;
        }

        console.log(playlist);

        let playlistHtml = document.getElementById("playlist_entries");
        let playlistSize = playlistHtml.childElementCount;
        for (var i = 0; i < playlist.length; i++) {
            addPlaylistElement(playlistHtml, i + playlistSize, playlist[i]);
        }
    });
}

/// --------------- SERVER EVENTS: ---------------

function readEventMaybeResync(type, event) {
    let jsonData = JSON.parse(event.data);
    let timestamp = jsonData["timestamp"];
    let priority = jsonData["priority"];
    let origin = jsonData["origin"];

    if (ignoreNextRequest) {
        // The next request will always be outdated so we can safely ignore it
        ignoreNextRequest = false;
        console.log("IGNORED:", priority, type, "from", origin, "at", timestamp);
        return;
    }

    console.log(priority, type, "from", origin, "at", timestamp);

    let deSync = timestamp - video.currentTime;
    console.log("Your deSync: ", deSync);
    if (type === "seek") {
        programmaticSeek = true;
        player.skipTo(timestamp);
        return;
    }

    if (DELTA < Math.abs(deSync)) {
        console.log("EXCEEDED DELTA=", DELTA, " resyncing!");
        programmaticSeek = true;
        player.skipTo(timestamp);
    }
}

function subscribeToServerEvents() {
    let eventSource = new EventSource("/watch/api/events");

    // Allow user to de-sync themselves freely and watch at their own pace
    eventSource.addEventListener("play", function (event) {
        if (!player) {
            return;
        }

        readEventMaybeResync("play", event);

        if (!isVideoPlaying()) {
            programmaticPlay = true;
            // this will merely append 'onplay' to the synchronous JS event queue
            // so there's no guarantee that it will be executed next, same logic follows for 'onpause'
            player.play();
        }
    });

    eventSource.addEventListener("pause", function (event) {
        if (!player) {
            return;
        }

        readEventMaybeResync("pause", event);

        if (isVideoPlaying()) {
            programmaticPause = true;
            player.pause();
        }
    });

    eventSource.addEventListener("seek", function (event) {
        if (!player) {
            return;
        }

        readEventMaybeResync("seek", event);
    });

    eventSource.addEventListener("seturl", function (event) {
        let jsonData = JSON.parse(event.data);
        let url = jsonData["url"];
        console.log("Media url received from the server: ", url);

        destroyPlayer();
        createPlayer(url);
    });

    eventSource.addEventListener("playlistadd", function (event) {
        console.log("Got playlist add event " + event.data);
        let entry = JSON.parse(event.data);

        if (!entry) {
            return;
        }

        let playlistHtml = document.getElementById("playlist_entries");
        let playlistSize = playlistHtml.childElementCount;
        addPlaylistElement(playlistHtml, playlistSize, entry);
    });

    eventSource.addEventListener("playlistclear", function (_event) {
        console.log("Got playlist clear event");
        let container = document.getElementById("playlist_entries");
        while (container.firstChild) {
            container.removeChild(container.lastChild);
        }
    });
}

/// --------------- PLAYER: ---------------

function isVideoPlaying() {
    return video.currentTime > 0 && !video.paused && !video.ended;
}

function destroyPlayer() {
    if (player) {
        player.destroy();
        player = null;
    } else {
        video.parentNode.removeChild(video);
    }
}

function createPlayer(url) {
    current_url.value = url;

    let url_missing = url === "";

    let container = document.getElementById("player_container");
    let new_video = document.createElement("video");
    new_video.width = window.innerWidth;
    // new_video.height = window.innerHeight;
    new_video.id = "player";

    let new_source = document.createElement("source");
    if (url_missing) {
        new_video.poster = "img/nothing_is_playing.png";
        url = "video/nothing_is_playing.mp4";
    }

    new_source.src = url;
    new_source.type = getUrlMediaType(url);
    new_video.appendChild(new_source);

    container.appendChild(new_video);

    let new_player = null;
    if (!url_missing) {
        new_player = fluidPlayer("player", {
            hls: {
                overrideNative: true,
            },
            layoutControls: {
                title: "TITLE PLACEHOLDER",
                doubleclickFullscreen: true,
                subtitlesEnabled: true,
                autoPlay: false,
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

        subscribeToPlayerEvents(new_player);
    }

    player = new_player;
    video = new_video;
}

function subscribeToPlayerEvents(new_player) {
    new_player.on("seeked", function () {
        if (programmaticSeek) {
            console.log("Programmatic seek caught");
            programmaticSeek = false;
            return;
        }

        ignoreNextRequest = true;
        apiSeek(video.currentTime);
    });

    new_player.on("play", function () {
        if (programmaticPlay) {
            programmaticPlay = false;
            return;
        }

        // We cannot make assumptions about the next state of the server because our request will not have any priority
        ignoreNextRequest = true;
        apiPlay();
    });

    new_player.on("pause", function () {
        if (programmaticPause) {
            programmaticPause = false;
            return;
        }

        ignoreNextRequest = true;
        // This request might not even come through
        apiPause();
    });
}

function main() {
    getPlaylist();
    createPlayer("");

    apiGet().then(state => {
        destroyPlayer();
        createPlayer(state.url);
        subscribeToServerEvents();
    });
}

main();
