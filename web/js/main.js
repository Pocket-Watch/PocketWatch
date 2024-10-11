const DELTA = 1.5;

var player;
var video;

var input_url = document.getElementById("input_url");
var current_url = document.getElementById("current_url");
var name_field = document.getElementById("user_name");
var proxy_checkbox = document.getElementById("proxy");
var autoplay_checkbox = document.getElementById("autoplay");
var looping_checkbox = document.getElementById("looping");

var programmaticPlay = false; // Updates before programmatic play() and in .onplay
var programmaticPause = false; // Updates before programmatic pause() and in .onpause
var programmaticSeek = false; // Updates before programmatic currentTime assignment and in .onseeked
var ignoreNextPlayRequest = false; // 'true' before sending a hasty play request, 'false' when its caught
var ignoreNextPauseRequest = false; // 'true' before sending a hasty pause request, 'false' when its caught
var ignoreNextSeekRequest = false; // 'true' before sending a hasty seek request, 'false' when its caught

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

async function apiPlaylistNext(current_url) {
    console.info("INFO: Sending playlist next request.");
    httpPost("/watch/api/playlist/next", current_url);
}

// NOTE(kihau): Might be faulty. See comment in watchPlaylistRemove() in serve.go.
async function apiPlaylistRemove(index) {
    console.info("INFO: Sending playlist remove request.");
    httpPost("/watch/api/playlist/remove", index);
}

async function apiPlaylistAutoplay(state) {
    console.info("INFO: Sending playlist autoplay request.");
    httpPost("/watch/api/playlist/autoplay", state);
}

async function apiPlaylistLooping(state) {
    console.info("INFO: Sending playlist autoplay request.");
    httpPost("/watch/api/playlist/looping", state);
}

async function apiPlaylistShuffle() {
    console.info("INFO: Sending playlist shuffle request.");
    httpPost("/watch/api/playlist/shuffle", null);
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

function inputUrlOnKeypress(event) {
    if (event.key === "Enter") {
        let url = input_url.value;
        input_url.value = "";

        console.info("INFO: Current video source url: ", url);
        apiSetUrl(url);
    }
}

function setUrlOnClick() {
    let url = input_url.value;
    input_url.value = "";

    console.info("INFO: Current video source url: ", url);
    apiSetUrl(url);
}

function skipOnClick() {
    let url = current_url.value;

    console.info("INFO: Current video source url: ", url);
    apiPlaylistNext(url);
}

function inputPlaylistOnKeypress(event) {
    if (event.key === "Enter") {
        let input_playlist = document.getElementById("input_playlist");
        let url = input_playlist.value;
        input_playlist.value = "";

        if (!url) {
            console.warn("WARNING: Url is empty, not adding to the playlist.");
            return;
        }

        apiPlaylistAdd(url);
    }
}

function playlistAddInputOnClick() {
    let url = input_url.value;
    input_url.value = "";

    if (!url) {
        console.warn("WARNING: Url is empty, not adding to the playlist.");
        return;
    }

    apiPlaylistAdd(url);
}

function autoplayOnClick() {
    console.info("Autoplay clicked");
    apiPlaylistAutoplay(autoplay_checkbox.checked);
}

function loopingOnClick() {
    console.info("Looping clicked");
    apiPlaylistLooping(looping_checkbox.checked);
}

function playlistAddOnClick() {
    let input_playlist = document.getElementById("input_playlist");
    let url = input_playlist.value;
    input_playlist.value = "";

    if (!url) {
        console.warn("WARNING: Url is empty, not adding to the playlist.");
        return;
    }

    apiPlaylistAdd(url);
}

function playlistShuffleOnClick() {
    apiPlaylistShuffle();
}

function playlistClearOnClick() {
    apiPlaylistClear();
}

const fileInput = document.getElementById('file_input');
const progressBar = document.getElementById('progressBar');
function uploadFile() {
    const file = fileInput.files[0];
    const formData = new FormData();
    formData.append('file', file);

    const request = new XMLHttpRequest();

    request.upload.addEventListener('progress', (event) => {
        if (event.lengthComputable) {
            progressBar.value = (event.loaded / event.total) * 100;
        }
    });

    request.open('POST', '/watch/api/upload', true);
    request.send(formData);
}

/// --------------- PLAYLIST: ---------------

// NOTE(kihau): This function is a big hack. There should be a better way to do it.
function playlistEntryRemoveOnClick(event) {
    let entry = event.target.parentElement.parentElement;

    let th = entry.getElementsByTagName("th")[0];
    let index_string = th.textContent;
    index_string = index_string.substring(0, index_string.length - 1);

    let index = Number(index_string) - 1;
    // console.log(index);
    apiPlaylistRemove(index);
}

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

    let button = document.createElement("button");
    button.onclick = playlistEntryRemoveOnClick;
    button.textContent = "Remove";
    cell.appendChild(button);
}

function getPlaylist() {
    apiPlaylistGet().then((playlist) => {
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

function removeFirstPlaylistElement() {
    let table = document.getElementById("playlist_entries").rows;
    if (table.length !== 0) {
        table[0].parentNode.removeChild(table[0]);
    }

    for (var i = 0; i < table.length; i++) {
        table[i].getElementsByTagName("th")[0].textContent = i + 1 + ".";
    }
}

function removePlaylistElementAt(index) {
    let table = document.getElementById("playlist_entries").rows;

    if (index < 0 || index >= table.length) {
        console.error("ERROR: Cannot remove playlist entry, index " + index + " is out of bounds.");
    } else {
        table[index].parentNode.removeChild(table[index]);
    }

    for (var i = 0; i < table.length; i++) {
        table[i].getElementsByTagName("th")[0].textContent = i + 1 + ".";
    }
}

function removeAllPlaylistElements() {
    let container = document.getElementById("playlist_entries");
    while (container.firstChild) {
        container.removeChild(container.lastChild);
    }
}

/// --------------- SERVER EVENTS: ---------------

function readEventMaybeResync(type, event) {
    let jsonData = JSON.parse(event.data);
    let timestamp = jsonData["timestamp"];
    let priority = jsonData["priority"];
    let origin = jsonData["origin"];

    // The next request will always be outdated so we can safely ignore it
    switch (type) {
        case "play":
            if (ignoreNextPlayRequest) {
                ignoreNextPlayRequest = false;
                console.log("Ignored ", priority, "play from", origin, "at", timestamp);
            }
            break;
        case "pause":
            if (ignoreNextPauseRequest) {
                ignoreNextPauseRequest = false;
                console.log("Ignored ", priority, "pause from", origin, "at", timestamp);
            }
            break;
        case "seek":
            if (ignoreNextSeekRequest) {
                ignoreNextSeekRequest = false;
                console.log("Ignored ", priority, "seek from", origin, "at", timestamp);
            }
            break;
    }

    console.log(priority, type, "from", origin, "at", timestamp);

    let deSync = timestamp - video.currentTime;
    console.log("Your deSync:", deSync);
    if (type === "seek") {
        programmaticSeek = true;
        player.skipTo(timestamp);
        return;
    }

    if (DELTA < Math.abs(deSync)) {
        let diff = Math.abs(deSync) - DELTA
        console.log("Resyncing! DELTA(" + DELTA + ") exceeded by", diff);
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
        } else {
            console.log("TIME:", video.currentTime, "PAUSED:", video.paused, "ENDED:", video.ended)
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
        removeAllPlaylistElements();
    });

    eventSource.addEventListener("playlistnext", function (event) {
        console.log("Got playlist next event: ", event.data);

        let url = JSON.parse(event.data);
        console.log("Media url received from the server: ", url);

        if (looping_checkbox.checked) {
            // TODO(kihau): This needs to be changed.
            const dummyEntry = {
                url: current_url.value,
                username: "<unknown>",
            };

            let playlistHtml = document.getElementById("playlist_entries");
            addPlaylistElement(playlistHtml, playlistHtml.childElementCount + 1, dummyEntry);
        }

        destroyPlayer();
        createPlayer(url);

        removeFirstPlaylistElement();
    });

    eventSource.addEventListener("playlistremove", function (event) {
        console.log("Got playlist remove event: ", event.data);
        removePlaylistElementAt(JSON.parse(event.data));
    });

    eventSource.addEventListener("playlistautoplay", function (event) {
        console.log("Got playlist autoplay event: ", event.data);
        let autoplay_enabled = JSON.parse(event.data);
        if (autoplay_enabled === null) {
            console.error("ERROR: Failed to parse autoplay json event");
            return;
        }

        autoplay_checkbox.checked = autoplay_enabled;
    });

    eventSource.addEventListener("playlistlooping", function (event) {
        console.log("Got playlist looping event: ", event.data);
        let looping_enabled = JSON.parse(event.data);
        if (looping_enabled === null) {
            console.error("ERROR: Failed to parse looping json event");
            return;
        }

        looping_checkbox.checked = looping_enabled;
    });

    eventSource.addEventListener("playlistshuffle", function (event) {
        console.log("Got playlist autoplay event: ", event.data);
        let playlist = JSON.parse(event.data);
        if (playlist === null) {
            console.error("ERROR: Failed to parse playlist shuffle json event.");
            return;
        }

        removeAllPlaylistElements();

        let playlistHtml = document.getElementById("playlist_entries");
        let playlistSize = playlistHtml.childElementCount;
        for (var i = 0; i < playlist.length; i++) {
            addPlaylistElement(playlistHtml, i + playlistSize, playlist[i]);
        }
    });
}

/// --------------- PLAYER: ---------------

function isVideoPlaying() {
    return !video.paused && !video.ended;
}

function destroyPlayer() {
    if (player) {
        unsubscribeFromPlayerEvents(player);
        player.pause();
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
                autoPlay: autoplay_checkbox.checked,
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

function playerOnPlay(_event) { 
    if (programmaticPlay) {
        programmaticPlay = false;
        return;
    }

    ignoreNextPlayRequest = true;
    apiPlay();
}

function playerOnPause(_event) { 
    if (programmaticPause) {
        programmaticPause = false;
        return;
    }

    ignoreNextPauseRequest = true;
    apiPause();
}

function playerOnSeek(_event) { 
    if (programmaticSeek) {
        console.log("Programmatic seek caught");
        programmaticSeek = false;
        return;
    }

    ignoreNextSeekRequest = true;
    apiSeek(video.currentTime);
}

function playerOnEnded(_event) { 
    if (autoplay_checkbox.checked) {
        let url = current_url.value;
        apiPlaylistNext(url);
    }
}

function subscribeToPlayerEvents(player) {
    player.on("play", playerOnPlay);
    player.on("pause", playerOnPause);
    player.on("seeked", playerOnSeek);
    player.on("ended", playerOnEnded);
}

function unsubscribeFromPlayerEvents(player) {
    let emptyFunc = function() {}
    player.on("play", emptyFunc);
    player.on("pause", emptyFunc);
    player.on("seeked", emptyFunc);
    player.on("ended", emptyFunc);
}

function main() {
    getPlaylist();
    createPlayer("");

    apiGet().then((state) => {
        autoplay_checkbox.checked = state.autoplay;
        looping_checkbox.checked = state.looping;

        destroyPlayer();
        createPlayer(state.url);
        subscribeToServerEvents();
    });
}

main();
