const DELTA = 1.5;

var uuid = null;

var player;
var video;
var subtitles = []

var input_url = document.getElementById("input_url");
var current_url = document.getElementById("current_url");
var name_field = document.getElementById("user_name");
var proxy_checkbox = document.getElementById("proxy");
var autoplay_checkbox = document.getElementById("autoplay");
var looping_checkbox = document.getElementById("looping");
var audioonly_checkbox = document.getElementById("audioonly");
var playlistEntries = document.getElementById("playlist_entries");
var historyEntries = document.getElementById("history_entries");

var programmaticPlay = false; // Updates before programmatic play() and in .onplay
var programmaticPause = false; // Updates before programmatic pause() and in .onpause
var programmaticSeek = false; // Updates before programmatic currentTime assignment and in .onseeked

/// --------------- HELPER FUNCTIONS: ---------------

function getUrlMediaType(url) {
    if (url.endsWith(".m3u8")) {
        return "application/x-mpegURL";
    }

    return "";
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
            return false;
        }
        return true;
    } catch (error) {
        console.error("ERROR: POST request for endpoint: " + endpoint + " failed: " + error);
        return false;
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

async function apiPlaylistAdd(entry) {
    console.info("INFO: Sending playlist add request for entry: ", entry);
    return await httpPost("/watch/api/playlist/add", entry);
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

async function apiHistoryGet() {
    console.info("INFO: Sending history get request.");
    return await httpGet("/watch/api/history/get");
}

async function apiHistoryClear() {
    console.info("INFO: Sending history clear request.");
    httpPost("/watch/api/history/clear", null);
}

async function apiPlaylistMove(source, dest) {
    const payload = {
        source_index: source,
        dest_index: dest,
    }

    console.info("INFO: Sending playlist move request with: ", payload);
    httpPost("/watch/api/playlist/move", payload);
}

async function apiSetUrl(url) {
    const payload = {
        uuid: uuid,
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
        uuid: uuid,
        timestamp: video.currentTime,
        username: name_field.value,
    };

    console.info("INFO: Sending play request to the server.");
    httpPost("/watch/api/play", payload);
}

async function apiPause() {
    const payload = {
        uuid: uuid,
        timestamp: video.currentTime,
        username: name_field.value,
    };

    console.info("INFO: Sending pause request to the server.");
    httpPost("/watch/api/pause", payload);
}

async function apiSeek(timestamp) {
    const payload = {
        uuid: uuid,
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

        addHistoryElement(current_url.value);

        console.info("INFO: Current video source url: ", url);
        apiSetUrl(url);
        playerSetUrl(url);
    }
}

function setUrlOnClick() {
    let url = input_url.value;
    input_url.value = "";

    addHistoryElement(current_url.value);

    console.info("INFO: Current video source url: ", url);
    apiSetUrl(url);
    playerSetUrl(url);
}

function skipOnClick() {
    let url = current_url.value;

    addHistoryElement(current_url.value);

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

        sendRequestAndAddToPlaylist(url);
    }
}

function playlistAddInputOnClick() {
    let url = input_url.value;
    input_url.value = "";

    if (!url) {
        console.warn("WARNING: Url is empty, not adding to the playlist.");
        return;
    }

    sendRequestAndAddToPlaylist(url);
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

    sendRequestAndAddToPlaylist(url);
}

function playlistShuffleOnClick() {
    apiPlaylistShuffle();
}

function playlistClearOnClick() {
    apiPlaylistClear();
}

const fileInput = document.getElementById("file_input");
const progressBar = document.getElementById("progressBar");
function uploadFile() {
    const file = fileInput.files[0];
    const formData = new FormData();
    formData.append("file", file);

    const request = new XMLHttpRequest();

    request.upload.addEventListener("progress", (event) => {
        if (event.lengthComputable) {
            progressBar.value = (event.loaded / event.total) * 100;
        }
    });

    request.open("POST", "/watch/api/upload", true);
    request.send(formData);
}

function historyClearOnClick() {
    apiHistoryClear();
}

/// --------------- PLAYLIST: ---------------

function sendRequestAndAddToPlaylist(url) {
    const entry = {
        uuid: uuid,
        username: name_field.value,
        url: url,
    };

    apiPlaylistAdd(entry).then(success => {
        if (success) {
            addPlaylistElement(entry);
        }
    });
}

// NOTE(kihau): This function is a big hack. There should be a better way to do it.
function playlistIndexFromEntry(entry) {
    let th = entry.getElementsByTagName("th")[0];
    let index_string = th.textContent;
    index_string = index_string.substring(0, index_string.length - 1);

    let index = Number(index_string) - 1;
    return index;
}

function playlistEntryRemoveOnClick(event) {
    let entry = event.target.parentElement.parentElement;
    let index = playlistIndexFromEntry(entry);
    apiPlaylistRemove(index);
}

var dragTarget = null;
var startIndex = null;

function isBefore(element1, element2) {
    if (element1.parentNode !== element2.parentNode) {
        return false;
    }

    for (var item = element1.previousSibling; item && item.nodeType !== 9; item = item.previousSibling) {
        if (item === element2) {
            return true;
        }
    }
}

function addPlaylistElement(entry) {
    let tr = document.createElement("tr");
    tr.draggable = true;

    tr.ondragstart = (event) => {
        console.debug(event.target);

        event.dataTransfer.effectAllowed = "move";
        // event.dataTransfer.setData("text/plain", null);

        startIndex = playlistIndexFromEntry(event.target);
        dragTarget = event.target;
    };

    tr.ondragover = (event) => {
        let dragDest = event.target.parentNode;

        let targetTh = dragTarget.getElementsByTagName("th")[0];
        let destTh = dragDest.getElementsByTagName("th")[0];

        let tempTh = targetTh.textContent;
        targetTh.textContent = destTh.textContent;
        destTh.textContent = tempTh;

        if (isBefore(dragTarget, dragDest)) {
            dragDest.parentNode.insertBefore(dragTarget, dragDest);
        } else {
            dragDest.parentNode.insertBefore(dragTarget, dragDest.nextSibling);
        }
    };

    tr.ondragend = (event) => {
        let endIndex = playlistIndexFromEntry(event.target);
        apiPlaylistMove(startIndex, endIndex);
    };

    playlistEntries.appendChild(tr);

    let th = document.createElement("th");
    th.textContent =  playlistEntries.childElementCount + 1 + ".";
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

        for (var i = 0; i < playlist.length; i++) {
            addPlaylistElement(playlist[i]);
        }
    });
}

function removeFirstPlaylistElement() {
    let table = playlistEntries.rows;
    if (table.length !== 0) {
        table[0].parentNode.removeChild(table[0]);
    }

    for (var i = 0; i < table.length; i++) {
        table[i].getElementsByTagName("th")[0].textContent = i + 1 + ".";
    }
}

function removePlaylistElementAt(index) {
    let table = playlistEntries.rows;

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
    let container = playlistEntries;
    while (container.firstChild) {
        container.removeChild(container.lastChild);
    }
}

/// --------------- HISTORY: ---------------

function addHistoryElement(url) {
    if (url === "") {
        return;
    }

    let tr = document.createElement("tr");
    historyEntries.appendChild(tr);

    let th = document.createElement("th");
    th.textContent = url;
    th.scope = "row";
    tr.appendChild(th);

    // let cell = tr.insertCell(-1);
    // cell.textContent = entry.url;
}

function getHistory() {
    apiHistoryGet().then((history) => {
        if (!history) {
            return;
        }

        console.log(history);

        for (var i = 0; i < history.length; i++) {
            addHistoryElement(history[i]);
        }
    });
}

function removeAllHistoryElements() {
    let container = historyEntries;
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

    eventSource.addEventListener("welcome", function (event) {
        console.info("Got a welcome request");
        console.info(event.data);
        uuid = JSON.parse(event.data);
    });

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
        let response = JSON.parse(event.data);
        console.info("INFO: Media url received from the server: ", response.url);
        playerSetUrl(response.url);
    });

    eventSource.addEventListener("playlistadd", function (event) {
        console.log("Got playlist add event " + event.data);
        let entry = JSON.parse(event.data);

        if (!entry) {
            return;
        }

        addPlaylistElement(entry);
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

            addPlaylistElement(dummyEntry);
        }

        destroyPlayer();
        createFluidPlayer(url);

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
        console.log("Got playlist shuffle event: ", event.data);
        let playlist = JSON.parse(event.data);
        if (playlist === null) {
            console.error("ERROR: Failed to parse playlist shuffle json event.");
            return;
        }

        removeAllPlaylistElements();

        for (var i = 0; i < playlist.length; i++) {
            addPlaylistElement(playlist[i]);
        }
    });

    eventSource.addEventListener("playlistmove", function (event) {
        console.log("Got playlist move event: ", event.data);
        let playlist = JSON.parse(event.data);
        if (playlist === null) {
            console.error("ERROR: Failed to parse playlist move json event.");
            return;
        }

        removeAllPlaylistElements();

        for (var i = 0; i < playlist.length; i++) {
            addPlaylistElement(playlist[i]);
        }
    });

    eventSource.addEventListener("historyclear", function (_event) {
        console.log("Got history clear event");
        removeAllHistoryElements();
    });
}

// label: String, src: String
function appendSubtitleTrack(video_element, label, src) {
    let track = document.createElement("track")
    track.label = label
    track.kind = "metadata"
    track.src = src
    video_element.appendChild(track)
}

/// --------------- PLAYER: ---------------

function playerSetUrl(url) {
    destroyPlayer();
    createFluidPlayer(url);
}

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

function createFluidPlayer(url) {
    current_url.value = url;

    let url_missing = url === "";

    let container = document.getElementById("player_container");
    let new_video = document.createElement("video");
    new_video.width = window.innerWidth;
    // new_video.height = window.innerHeight;
    new_video.id = "player";
    if (subtitles.length > 0) {
        for (let i = 0; i < subtitles.length; i++) {
            appendSubtitleTrack(new_video, subtitles[i], subtitles[i]);
        }
    }

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

    apiPlay();
}

function playerOnPause(_event) {
    if (programmaticPause) {
        programmaticPause = false;
        return;
    }

    apiPause();
}

function playerOnSeek(_event) {
    if (programmaticSeek) {
        console.log("Programmatic seek caught");
        programmaticSeek = false;
        return;
    }

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
    getHistory();

    // dummy player
    createFluidPlayer("");

    apiGet().then((state) => {
        autoplay_checkbox.checked = state.autoplay;
        looping_checkbox.checked = state.looping;

        subtitles = state.subtitles;

        playerSetUrl(state.url);
        subscribeToServerEvents();
    });
}

main();
