import { Playlist } from "./playlist.js"

export { findUserById, apiPlaylistRemove, apiPlaylistMove }

const DELTA = 1.5;

var allUsers = [];
var token = "";
var connectionId = 0;
var currentEntryId = 0;

var userSelf = {
    id: 0,
    username: "",
    avatar: "",
};

var player;
var video;
var subtitles = []

var playlist = new Playlist();

var input_url = document.getElementById("input_url");
var referer_input = document.getElementById("referer");
var input_title = document.getElementById("input_title");
var current_url = document.getElementById("current_url");
var input_username = document.getElementById("input_username");
var proxy_checkbox = document.getElementById("proxy");
var autoplay_checkbox = document.getElementById("autoplay");
var looping_checkbox = document.getElementById("looping");
var audioonly_checkbox = document.getElementById("audioonly");
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
    headers.set("Authorization", token);

    const options = {
        method: "POST",
        body: JSON.stringify(data),
        headers: headers,
    };

    try {
        const response = await fetch(endpoint, options);
        if (!response.ok) {
            console.error("ERROR: POST request for endpoint: " + endpoint + " failed: " + response.status);
            return null;
        }

        // TODO(kihau): 
        //     Throws exception when response is not a valid json.
        //     This should be handled this in a nicer way.
        return await response.json();
    } catch (error) {
        // console.error("ERROR: POST request for endpoint: " + endpoint + " failed: " + error);
        return null;
    }
}

async function httpGet(endpoint) {
    const headers = new Headers();
    headers.set("Content-Type", "application/json");
    headers.set("Authorization", token);

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

function findUserById(userId) {
    for (let i = 0; i < allUsers.length; i++) {
        const user = allUsers[i];
        if (user.id == userId) {
            return user;
        }
    }

    const user = {
        id: 0,
        username: "",
        avatar: "",
    };

    return user;
}

/// --------------- SERVER REQUESTS: ---------------

function createApiEntry(url) {
    const entry = {
        id: 0,
        url: url,
        title: input_title.value,
        user_id: userSelf.id,
        use_proxy: proxy_checkbox.checked,
        referer_url: referer_input.value,
        created: new Date,
    };

    return entry;
}

async function apiUserCreate() {
    let data = await httpGet("/watch/api/user/create");
    console.info("INFO: Received data from createuser request to the server: ", data);
    return data;
}

async function apiUserGetAll() {
    let data = await httpGet("/watch/api/user/getall");
    console.info("INFO: Received data from user getall request to the server: ", data);
    return data;
}

async function apiUserVerify() {
    let data = await httpPost("/watch/api/user/verify");
    console.info("INFO: Received data from user verify request to the server: ", data);
    return data;
}

async function apiUserUpdateName(username) {
    console.info("INFO: Sending update username request.");
    httpPost("/watch/api/user/updatename", username);
}

async function apiPlayerGet() {
    let data = await httpGet("/watch/api/player/get");
    console.info("INFO: Received data from player get request to the server: ", data);
    return data;
}

async function apiPlayerSet(url) {
    const payload = {
        connection_id: connectionId,
        entry: createApiEntry(url),
    };

    console.info("INFO: Sending player set request for a entry");
    httpPost("/watch/api/player/set", payload);
}

async function apiPlayerNext() {
    const payload = {
        connection_id: connectionId,
        entry_id: currentEntryId,
    };

    console.info("INFO: Sending player next request.");
    httpPost("/watch/api/player/next", payload);
}

async function apiPlayerPlay() {
    const payload = {
        connection_id: connectionId,
        timestamp: video.currentTime,
    };

    console.info("INFO: Sending player play request to the server.");
    httpPost("/watch/api/player/play", payload);
}

async function apiPlayerPause() {
    const payload = {
        connection_id: connectionId,
        timestamp: video.currentTime,
    };

    console.info("INFO: Sending player pause request to the server.");
    httpPost("/watch/api/player/pause", payload);
}

async function apiPlayerSeek() {
    const payload = {
        connection_id: connectionId,
        timestamp: video.currentTime,
    };

    console.info("INFO: Sending player seek request to the server.");
    httpPost("/watch/api/player/seek", payload);
}

async function apiPlayerAutoplay(state) {
    console.info("INFO: Sending player autoplay request.");
    httpPost("/watch/api/player/autoplay", state);
}

async function apiPlayerLooping(state) {
    console.info("INFO: Sending player autoplay request.");
    httpPost("/watch/api/player/looping", state);
}

async function apiPlaylistGet() {
    console.info("INFO: Sending playlist get request.");
    return await httpGet("/watch/api/playlist/get");
}

async function apiPlaylistAdd(url) {
    const payload = {
        connection_id: connectionId,
        entry: createApiEntry(url),
    };

    console.info("INFO: Sending playlist add request for entry: ", payload);
    httpPost("/watch/api/playlist/add", payload);
}

async function apiPlaylistClear() {
    console.info("INFO: Sending playlist clear request.");
    httpPost("/watch/api/playlist/clear", connectionId);
}

async function apiPlaylistRemove(entryId, index) {
    const payload = {
        connection_id: connectionId,
        entry_id: entryId,
        index: index,
    };

    console.info("INFO: Sending playlist remove request.");
    httpPost("/watch/api/playlist/remove", payload);
}

async function apiPlaylistShuffle() {
    console.info("INFO: Sending playlist shuffle request.");
    httpPost("/watch/api/playlist/shuffle", null);
}

async function apiPlaylistMove(entryId, source, dest) {
    const payload = {
        connection_id: connectionId, 
        entry_id: entryId,
        source_index: source,
        dest_index: dest,
    }

    console.info("INFO: Sending playlist move request with: ", payload);
    httpPost("/watch/api/playlist/move", payload);
}

async function apiHistoryGet() {
    console.info("INFO: Sending history get request.");
    return await httpGet("/watch/api/history/get");
}

async function apiHistoryClear() {
    console.info("INFO: Sending history clear request.");
    httpPost("/watch/api/history/clear", null);
}

/// --------------- HTML ELEMENT CALLBACKS: ---------------

function inputUrlOnKeypress(event) {
    if (event.key === "Enter") {
        let url = input_url.value;
        input_url.value = "";

        console.info("INFO: Current video source url: ", url);
        apiPlayerSet(url);
    }
}

function playerSetOnClick() {
    let url = input_url.value;
    input_url.value = "";

    console.info("INFO: Current video source url: ", url);
    apiPlayerSet(url);
}

function playerNextOnClick() {
    console.info("INFO: Next button was clicked");
    apiPlayerNext();
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

function playlistAddTopOnClick() {
    let url = input_url.value;
    input_url.value = "";

    if (!url) {
        console.warn("WARNING: Url is empty, not adding to the playlist.");
        return;
    }

    apiPlaylistAdd(url);
}

function autoplayOnClick() {
    console.info("INFO: Autoplay button clicked");
    apiPlayerAutoplay(autoplay_checkbox.checked);
}

function loopingOnClick() {
    console.info("INFO: Looping button clicked");
    apiPlayerLooping(looping_checkbox.checked);
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

function updateUsernameOnClick() {
    apiUserUpdateName(input_username.value);
}

function clearSessionOnClick() {
    localStorage.removeItem("token");
    window.location.reload();
}

/// --------------- HISTORY: ---------------

function addHistoryElement(entry) {
    if (entry.url === "") {
        return;
    }

    let tr = document.createElement("tr");
    historyEntries.appendChild(tr);

    let th = document.createElement("th");
    th.textContent = entry.url;
    th.scope = "row";
    tr.appendChild(th);
}

function getHistory() {
    apiHistoryGet().then((history) => {
        if (!history) {
            return;
        }

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

let div = 121;
var divFooBar = 12122;

/// --------------- CONNECTED USERS: ---------------

function updateConnectedUsers() {
    let connected_tr = document.getElementById("connected_users");
    while (connected_tr.firstChild) {
        connected_tr.removeChild(connected_tr.firstChild);
    }

    for (var i = 0; i < allUsers.length; i++) {
        if (allUsers[i].connections > 0) {
            let cell = connected_tr.insertCell(-1);
            cell.textContent = allUsers[i].username;
        }
    }
}

/// --------------- SERVER EVENTS: ---------------

function readEventMaybeResync(type, event) {
    let jsonData = JSON.parse(event.data);

    let timestamp = jsonData.timestamp;
    let userId = jsonData.user_id;

    let deSync = timestamp - video.currentTime;

    if (userId == 0) {
        console.info("INFO: Recieved resync event from SERVER for", type, "at", timestamp, "with desync:", deSync);
    } else {
        console.info("INFO: Recieved resync event from USER id", userId, "for", type, "at", timestamp, "with desync:", deSync);
    }

    if (type === "seek") {
        programmaticSeek = true;
        player.skipTo(timestamp);
        return;
    }

    if (DELTA < Math.abs(deSync)) {
        let diff = Math.abs(deSync) - DELTA
        console.warn("You are desynced! DELTA(" + DELTA + ") exceeded by", diff, "Trying to resync now!");
        programmaticSeek = true;
        player.skipTo(timestamp);
    }
}

function subscribeToServerEvents() {
    let eventSource = new EventSource("/watch/api/events?token=" + token);

    eventSource.addEventListener("welcome", function(event) {
        connectionId = JSON.parse(event.data);
        console.info("INFO: Received a welcome request with connection id: ", connectionId);

        apiUserGetAll().then((users) => {
            allUsers = users;
            updateConnectedUsers();

            apiPlaylistGet().then(entries => {
                playlist.loadNew(entries);
            });

            getHistory();
        })
    });

    eventSource.addEventListener("connectionadd", function(event) {
        let userId = JSON.parse(event.data);
        console.info("INFO: New connection added for user id: ", userId)

        for (var i = 0; i < allUsers.length; i++) {
            if (allUsers[i].id == userId) {
                allUsers[i].connections += 1;
                break;
            }
        }

        updateConnectedUsers();
    });

    eventSource.addEventListener("connectiondrop", function(event) {
        let userId = JSON.parse(event.data);
        console.info("INFO: Connection dropped for user id: ", userId)

        for (var i = 0; i < allUsers.length; i++) {
            if (allUsers[i].id == userId) {
                allUsers[i].connections -= 1;
                break;
            }
        }

        updateConnectedUsers();
    });

    eventSource.addEventListener("usercreate", function(event) {
        let newUser = JSON.parse(event.data)
        allUsers.push(newUser)
        console.info("INFO: New user has beed created: ", newUser)
        updateConnectedUsers();
    });

    eventSource.addEventListener("usernameupdate", function(event) {
        let updatedUser = JSON.parse(event.data);
        console.info("INFO: User updated its name: ", updatedUser)

        if (updatedUser.id == userSelf.id) {
            userSelf = updatedUser
            input_username.value = userSelf.username;
        }

        for (var i = 0; i < allUsers.length; i++) {
            if (allUsers[i].id == updatedUser.id) {
                allUsers[i] = updatedUser;
                break;
            }
        }

        updateConnectedUsers();
        playlist.updateUsernames(updatedUser);
    });

    eventSource.addEventListener("playerset", function(event) {
        let response = JSON.parse(event.data);
        console.info("INFO: Received player set event: ", response);

        addHistoryElement(response.prev_entry)

        currentEntryId = response.new_entry.id
        playerSetUrl(response.new_entry.url, response.new_entry.title);
    });

    eventSource.addEventListener("playernext", function(event) {
        let response = JSON.parse(event.data);
        console.info("INFO: Received player next event: ", response);

        addHistoryElement(response.prev_entry)

        if (looping_checkbox.checked) {
            playlist.add(response.prev_entry);
        }

        playlist.removeFirst();

        // NOTE(kihau): new_entry can be removed.
        currentEntryId = response.new_entry.id
        playerSetUrl(response.new_entry.url, response.new_entry.title);
    });

    eventSource.addEventListener("sync", function(event) {
        let data = JSON.parse(event.data);
        if (!data) {
            console.error("ERROR: Failed to parse event data")
            return;
        }

        if (!player) {
            return;
        }

        switch (data.action) {
            case "play": {
                readEventMaybeResync("play", event);

                if (!isVideoPlaying()) {
                    programmaticPlay = true;
                    player.play();
                }
            } break;

            case "pause": {
                readEventMaybeResync("pause", event);

                if (isVideoPlaying()) {
                    programmaticPause = true;
                    player.pause();
                }
            } break;

            case "seek": {
                readEventMaybeResync("seek", event);
            } break;

            default: {
                console.error("ERROR: Unknown sync action found", data.action)
            } break;
        }
    });

    eventSource.addEventListener("playerautoplay", function(event) {
        let autoplay_enabled = JSON.parse(event.data);
        console.info("INFO: Received player autoplay event: ", autoplay_enabled);
        autoplay_checkbox.checked = autoplay_enabled;
    });

    eventSource.addEventListener("playerlooping", function(event) {
        let looping_enabled = JSON.parse(event.data);
        console.info("INFO: Received player looping event: ", looping_enabled);
        looping_checkbox.checked = looping_enabled;
    });

    eventSource.addEventListener("playlistadd", function(event) {
        let entry = JSON.parse(event.data);
        console.info("INFO: Received playlist add event:", entry);
        playlist.add(entry);
    });

    eventSource.addEventListener("playlistclear", function(_event) {
        console.info("INFO: Received playlist clear event");
        playlist.clear();
    });

    eventSource.addEventListener("playlistremove", function(event) {
        let index = JSON.parse(event.data);
        console.info("INFO: Received playlist remove event:", index);
        playlist.removeAt(index);
    });

    eventSource.addEventListener("playlistshuffle", function(event) {
        let newPlaylist = JSON.parse(event.data);
        console.info("INFO: Received playlist shuffle event: ", newPlaylist);
        playlist.loadNew(newPlaylist);
    });

    eventSource.addEventListener("playlistmove", function(event) {
        let response = JSON.parse(event.data);
        console.info("INFO: Received playlist move event:", response);
        playlist.move(response.source_index, response.dest_index);
    });

    eventSource.addEventListener("historyclear", function(_event) {
        console.info("INFO: Received history clear event");
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

function playerSetUrl(url, title) {
    destroyPlayer();
    createFluidPlayer(url, title);
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

function createFluidPlayer(url, title) {
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
                title: title,
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

    apiPlayerPlay();
}

function playerOnPause(_event) {
    if (programmaticPause) {
        programmaticPause = false;
        return;
    }

    apiPlayerPause();
}

function playerOnSeek(_event) {
    if (programmaticSeek) {
        console.info("INFO: Programmatic seek caught");
        programmaticSeek = false;
        return;
    }

    apiPlayerSeek();
}

function playerOnEnded(_event) {
    if (autoplay_checkbox.checked) {
        apiPlayerNext();
    }
}

function subscribeToPlayerEvents(player) {
    player.on("play", playerOnPlay);
    player.on("pause", playerOnPause);
    player.on("seeked", playerOnSeek);
    player.on("ended", playerOnEnded);
}

function unsubscribeFromPlayerEvents(player) {
    let emptyFunc = function() { }
    player.on("play", emptyFunc);
    player.on("pause", emptyFunc);
    player.on("seeked", emptyFunc);
    player.on("ended", emptyFunc);
}

async function getOrCreateUserInAnExtremelyUglyWay() {
    let user = null

    token = localStorage.getItem("token");
    if (!token) {
        token = await apiUserCreate();
        localStorage.setItem("token", token)
        user = await apiUserVerify(token);
    } else {
        user = await apiUserVerify(token);
        if (!user) {
            token = await apiUserCreate();
            localStorage.setItem("token", token)
            user = await apiUserVerify(token);
        }
    }

    return user;
}

function shiftSubtitlesBack() {
    if (video.textTracks.length === 0) {
        console.warn("NO SUBTITLE TRACKS")
        return;
    }
    let track = video.textTracks[0];
    console.info("CUES", track.cues)
    for (let i = 0; i < track.cues.length; i++) {
        let cue = track.cues[i];
        cue.startTime -= 0.5;
        cue.endTime -= 0.5;
    }
}

function shiftSubtitlesForward() {
    if (video.textTracks.length === 0) {
        console.warn("NO SUBTITLE TRACKS")
        return;
    }
    let track = video.textTracks[0];
    console.info("CUES", track.cues)
    for (let i = 0; i < track.cues.length; i++) {
        let cue = track.cues[i];
        cue.startTime += 0.5;
        cue.endTime += 0.5;
    }
}

function attachHtmlHandlers() {
    window.inputUrlOnKeypress = inputUrlOnKeypress;
    window.playerSetOnClick = playerSetOnClick;
    window.playerNextOnClick = playerNextOnClick;
    window.inputPlaylistOnKeypress = inputPlaylistOnKeypress;
    window.playlistAddOnClick = playlistAddOnClick;
    window.historyClearOnClick = historyClearOnClick;
    window.updateUsernameOnClick = updateUsernameOnClick;
    window.playlistAddTopOnClick = playlistAddTopOnClick;
    window.clearSessionOnClick = clearSessionOnClick;
    window.playlistShuffleOnClick = playlistShuffleOnClick;
    window.playlistClearOnClick = playlistClearOnClick;
    window.autoplayOnClick = autoplayOnClick;
    window.loopingOnClick = loopingOnClick;
    window.uploadFile = uploadFile;
    window.shiftSubtitlesBack  = shiftSubtitlesBack;
    window.shiftSubtitlesForward  = shiftSubtitlesForward;
}

async function main() {
    attachHtmlHandlers();

    userSelf = await getOrCreateUserInAnExtremelyUglyWay();
    input_username.value = userSelf.username;

    // dummy player
    createFluidPlayer("", "");

    let state = await apiPlayerGet();
    autoplay_checkbox.checked = state.player.autoplay;
    looping_checkbox.checked = state.player.looping;
    currentEntryId = state.entry.id;
    subtitles = state.subtitles;

    playerSetUrl(state.entry.url, state.entry.title);
    subscribeToServerEvents();
}

main();
