import { Playlist } from "./playlist.js"
import * as api from "./api.js";

export { findUserById, createApiEntry }

const DELTA = 1.5;


// User and connection data.
export var token = "";
export var connectionId = 0;

var allUsers = [];
var input_username = document.getElementById("input_username");
var userSelf = {
    id: 0,
    username: "",
    avatar: "",
};


// Player relevant data.
var player;
var video;
var subtitles = []
var currentEntryId = 0;

var input_url = document.getElementById("input_url");
var referer_input = document.getElementById("referer");
var input_title = document.getElementById("input_title");
var current_url = document.getElementById("current_url");
var proxy_checkbox = document.getElementById("proxy");
var autoplay_checkbox = document.getElementById("autoplay");
var audioonly_checkbox = document.getElementById("audioonly");
var looping_checkbox = document.getElementById("looping");

var programmaticPlay = false; // Updates before programmatic play() and in .onplay
var programmaticPause = false; // Updates before programmatic pause() and in .onpause
var programmaticSeek = false; // Updates before programmatic currentTime assignment and in .onseeked


// Playlist and history.
var playlist = new Playlist();
var historyEntries = document.getElementById("history_entries");

/// --------------- HELPER FUNCTIONS: ---------------

function getUrlMediaType(url) {
    if (url.endsWith(".m3u8")) {
        return "application/x-mpegURL";
    }

    return "";
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

function setNewEntry() {
    let url = input_url.value;
    input_url.value = "";

    console.info("INFO: Current video source url: ", url);

    let entry = createApiEntry(url);
    api.playerSet(entry);
}

/// --------------- HTML ELEMENT CALLBACKS: ---------------

function inputUrlOnKeypress(event) {
    if (event.key === "Enter") {
        setNewEntry();
    }
}

function playerSetOnClick() {
    setNewEntry();
}

function playerNextOnClick() {
    console.info("INFO: Next button was clicked");
    api.playerNext(currentEntryId);
}

function playlistAddTopOnClick() {
    let url = input_url.value;
    input_url.value = "";

    if (!url) {
        console.warn("WARNING: Url is empty, not adding to the playlist.");
        return;
    }

    let entry = createApiEntry(url);
    api.playlistAdd(entry);
}

function autoplayOnClick() {
    console.info("INFO: Autoplay button clicked");
    api.playerAutoplay(autoplay_checkbox.checked);
}

function loopingOnClick() {
    console.info("INFO: Looping button clicked");
    api.playerLooping(looping_checkbox.checked);
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
    api.historyClear();
}

function updateUsernameOnClick() {
    api.userUpdateName(input_username.value);
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
    api.historyGet().then((history) => {
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

        api.userGetAll().then((users) => {
            allUsers = users;
            updateConnectedUsers();

            api.playlistGet().then(entries => {
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

    eventSource.addEventListener("playlist", function(event) {
        let response = JSON.parse(event.data);
        console.info("INFO: Received playlist event for:", response.action, "with:", response.data);
        playlist.handleServerEvent(response.action, response.data);
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

    api.playerPlay(video.currentTime);
}

function playerOnPause(_event) {
    if (programmaticPause) {
        programmaticPause = false;
        return;
    }

    api.playerPause(video.currentTime);
}

function playerOnSeek(_event) {
    if (programmaticSeek) {
        console.info("INFO: Programmatic seek caught");
        programmaticSeek = false;
        return;
    }

    api.playerSeek(video.currentTime);
}

function playerOnEnded(_event) {
    if (autoplay_checkbox.checked) {
        api.playerNext(currentEntryId);
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
        token = await api.userCreate();
        localStorage.setItem("token", token)
        user = await api.userVerify(token);
    } else {
        user = await api.userVerify(token);
        if (!user) {
            token = await api.userCreate();
            localStorage.setItem("token", token)
            user = await api.userVerify(token);
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
    window.historyClearOnClick = historyClearOnClick;
    window.updateUsernameOnClick = updateUsernameOnClick;
    window.playlistAddTopOnClick = playlistAddTopOnClick;
    window.clearSessionOnClick = clearSessionOnClick;
    window.autoplayOnClick = autoplayOnClick;
    window.loopingOnClick = loopingOnClick;
    window.uploadFile = uploadFile;
    window.shiftSubtitlesBack  = shiftSubtitlesBack;
    window.shiftSubtitlesForward  = shiftSubtitlesForward;

    playlist.attachHtmlEventHandlers();
}

async function main() {
    attachHtmlHandlers();

    userSelf = await getOrCreateUserInAnExtremelyUglyWay();
    input_username.value = userSelf.username;

    // dummy player
    createFluidPlayer("", "");

    let state = await api.playerGet();
    autoplay_checkbox.checked = state.player.autoplay;
    looping_checkbox.checked = state.player.looping;
    currentEntryId = state.entry.id;
    subtitles = state.subtitles;

    playerSetUrl(state.entry.url, state.entry.title);
    subscribeToServerEvents();
}

main();
