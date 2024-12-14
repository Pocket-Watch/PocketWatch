import { PlayerArea } from "./player_area.js"
import { Playlist } from "./playlist_old.js"
import * as api from "./api_old.js";

export { findUserById }

// User and connection data.
export var token = "";
export var connectionId = 0;
export var allUsers = [];
export var userSelf = {
    id: 0,
    username: "",
    avatar: "",
};
var input_username = document.getElementById("input_username");

// Player
export var playerArea = new PlayerArea();

// Playlist and history.
var playlist = new Playlist();
var historyEntries = document.getElementById("history_entries");

/// --------------- HELPER FUNCTIONS: ---------------

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

/// --------------- HTML ELEMENT CALLBACKS: ---------------

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

function attachHtmlHandlers() {
    window.uploadFile = uploadFile;
    window.clearSessionOnClick = clearSessionOnClick;
    window.updateUsernameOnClick = updateUsernameOnClick;
    window.historyClearOnClick = historyClearOnClick;

    playlist.attachHtmlEventHandlers();
    playerArea.attachHtmlEventHandlers();
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

/// --------------- SERVER EVENTS: ---------------

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
        playerArea.setEntry(response.new_entry);
    });

    eventSource.addEventListener("playernext", function(event) {
        let response = JSON.parse(event.data);
        console.info("INFO: Received player next event: ", response);

        addHistoryElement(response.prev_entry)

        if (playerArea.loopingEnabled()) {
            playlist.add(response.prev_entry);
        }

        playlist.removeFirst();
        playerArea.setEntry(response.new_entry);
    });

    eventSource.addEventListener("sync", function(event) {
        let data = JSON.parse(event.data);
        if (!data) {
            console.error("ERROR: Failed to parse event data")
            return;
        }

        let timestamp = data.timestamp;
        let userId = data.user_id;

        switch (data.action) {
            case "play": {
                playerArea.resync(timestamp, userId);
                playerArea.play();
            } break;

            case "pause": {
                playerArea.resync(timestamp, userId);
                playerArea.pause();
            } break;

            case "seek": {
                playerArea.seek(timestamp);
            } break;

            default: {
                console.error("ERROR: Unknown sync action found", data.action)
            } break;
        }
    });

    eventSource.addEventListener("playerautoplay", function(event) {
        let autoplay_enabled = JSON.parse(event.data);
        console.info("INFO: Received player autoplay event: ", autoplay_enabled);
        playerArea.setAutoplay(autoplay_enabled);
    });

    eventSource.addEventListener("playerlooping", function(event) {
        let looping_enabled = JSON.parse(event.data);
        console.info("INFO: Received player looping event: ", looping_enabled);
        playerArea.setLooping(looping_enabled);
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

async function main() {
    attachHtmlHandlers();

    userSelf = await getOrCreateUserInAnExtremelyUglyWay();
    input_username.value = userSelf.username;

    let state = await api.playerGet();
    playerArea.setAutoplay(state.player.autoplay);
    playerArea.setLooping(state.player.looping);

    playerArea.currentEntryId = state.entry.id;
    playerArea.subtitles = state.subtitles;

    playerArea.setEntry(state.entry);
    subscribeToServerEvents();
}

main();
