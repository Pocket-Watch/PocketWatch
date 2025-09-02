const API_PATH = "/watch/api/";

export const EVENT_PLAYER_PLAY         = 0;
export const EVENT_PLAYER_PAUSE        = 1;
export const EVENT_PLAYER_SEEK         = 2;
export const EVENT_PLAYER_SET          = 3;
export const EVENT_PLAYER_NEXT         = 4;
export const EVENT_PLAYER_AUTOPLAY     = 5;
export const EVENT_PLAYER_LOOPING      = 6;
export const EVENT_PLAYER_UPDATE_TITLE = 7;

export const EVENT_CHAT_SEND   = 8;
export const EVENT_CHAT_EDIT   = 9;
export const EVENT_CHAT_DELETE = 10;

export const EVENT_PLAYLIST_ADD     = 11;
export const EVENT_PLAYLIST_PLAY    = 12;
export const EVENT_PLAYLIST_MOVE    = 13;
export const EVENT_PLAYLIST_CLEAR   = 14;
export const EVENT_PLAYLIST_DELETE  = 15;
export const EVENT_PLAYLIST_UPDATE  = 16;
export const EVENT_PLAYLIST_SHUFFLE = 17;

let websocket = null;
let token     = null;

export class JsonResponse {
    constructor(status, method, endpoint) {
        this.ok           = status >= 200 && status < 300;
        this.status       = status;
        this.method       = method;
        this.endpoint     = endpoint;
        this.json         = null;
        this.errorMessage = null;
    }

    static fromPost(status, jsonObj, endpoint) {
        let response = new JsonResponse(status, "POST", endpoint);
        response.json = jsonObj;
        return response;
    }

    static fromPostError(status, error, endpoint) {
        let response = new JsonResponse(status, "POST", endpoint);
        response.errorMessage = error;
        return response;
    }

    checkError() {
        if (this.ok) {
            return false;
        }

        this.logError();
        return true;
    }

    logError() {
        console.error(this.errorMessage, "[from", this.method, "to", this.endpoint + "]");
    }
}

async function httpPostFile(endpoint, file, filename) {
    endpoint = API_PATH + endpoint;

    const headers = new Headers();
    headers.set("Authorization", token);

    let formdata = new FormData();
    if (filename) {
        formdata.append("file", file, filename);
    } else {
        formdata.append("file", file);
    }

    const options = {
        method: "POST",
        body: formdata,
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

async function httpPostFileWithProgress(endpoint, file, onprogress) {
    endpoint = API_PATH + endpoint;

    return new Promise((resolve, _) => {
        const request = new XMLHttpRequest();
        request.open("POST", endpoint, true);
        request.setRequestHeader("Authorization", token);

        let formdata = new FormData();
        formdata.append("file", file);

        request.upload.onprogress = onprogress;

        request.onreadystatechange = _ => {
            if (request.readyState === XMLHttpRequest.DONE) {
                if (request.status !== 200) {
                    let errorText = request.responseText;
                    let response = JsonResponse.fromPostError(request.status, errorText, endpoint);
                    resolve(response);
                    return;
                }

                let jsonResponse;
                try {
                    jsonResponse = JSON.parse(request.responseText);
                } catch {
                    jsonResponse = {};
                }

                let response = JsonResponse.fromPost(request.status, jsonResponse, endpoint);
                resolve(response);
            }
        };

        request.send(formdata);
    });
}

// It sends a JSON body and receives a JSON body, on error receives error as text (http.Error in go)
// Unfortunately there does not seem to be an option to disable the ugly response status console log
async function httpPost(endpoint, data) {
    console.info("INFO: Sending", endpoint.replace("/", " "), "API request with data:", data);

    endpoint = API_PATH + endpoint;

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
            let errorText = await response.text();
            return JsonResponse.fromPostError(response.status, errorText, endpoint);
        }

        let jsonResponse;
        try {
            jsonResponse = await response.json();
        } catch {
            jsonResponse = {};
        }

        return JsonResponse.fromPost(response.status, jsonResponse, endpoint);
    } catch (error) {
        throw new Error("ERROR: POST request to endpoint: " + endpoint + " failed " + error);
    }
}

async function httpGet(endpoint) {
    let endpointName = endpoint.replace("/", " ");
    endpoint = API_PATH + endpoint;

    console.info("INFO: Sending", endpointName, "API request.");

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
            console.error("ERROR: GET request for endpoint: " + endpoint + " returned status: " + response.status);
            return null;
        }

        let data = await response.json();
        console.info("INFO: Received data for", endpointName ,"request to the server: ", data);
        return data;
    } catch (error) {
        console.error("ERROR: GET request for endpoint: " + endpoint + " failed: " + error);
    }

    return null;
}

export function setToken(t) {
    token = t;
}

export function getToken() {
    return token;
}

export async function version() {
    return await httpGet("version");
}

export async function uptime() {
    return await httpGet("uptime");
}

export async function uploadMedia(file, filename) {
    console.info("INFO: Uploading a file to the server.");
    let fileUrl = await httpPostFile("uploadmedia", file, filename);
    return fileUrl;
}

export async function uploadMediaWithProgress(file, onprogress) {
    console.info("INFO: Uploading a file to the server (with progress callback).");
    let fileUrl = await httpPostFileWithProgress("uploadmedia", file, onprogress);
    return fileUrl;
}

export async function userCreate() {
    return await httpGet("user/create");
}

export async function userGetAll() {
    return await httpGet("user/getall");
}

export async function userVerify(token) {
    return httpPost("user/verify", token);
}

export async function userUpdateName(username) {
    return httpPost("user/updatename", username);
}

export async function userUpdateAvatar(file) {
    return await httpPostFile("user/updateavatar", file);
}

export async function userDelete(token) {
    return await httpPost("user/delete", token);
}

export async function playerGet() {
    return await httpGet("player/get");
}

export async function playerSet(requestEntry) {
    return httpPost("player/set", requestEntry);
}

export async function playerNext(currentEntryId) {
    return await httpPost("player/next", currentEntryId);
}

export async function playerPlay(timestamp) {
    await httpPost("player/play", timestamp);
}

export async function playerPause(timestamp) {
    await httpPost("player/pause", timestamp);
}

export async function playerSeek(timestamp) {
    await httpPost("player/seek", timestamp);
}

export async function playerAutoplay(state) {
    await httpPost("player/autoplay", state);
}

export async function playerLooping(state) {
    await httpPost("player/looping", state);
}

export async function playerUpdateTitle(title) {
    await httpPost("player/updatetitle", title);
}

export async function subtitleDelete(subtitleId) {
    await httpPost("subtitle/delete", subtitleId);
}

export async function subtitleUpdate(id, name) {
    let data = {
        id:    id,
        name:  name,
    };

    await httpPost("subtitle/update", data);
}

export async function subtitleAttach(subtitle) {
    await httpPost("subtitle/attach", subtitle);
}

export async function subtitleShift(id, shift) {
    let data = {
        id:    id,
        shift: shift,
    };

    await httpPost("subtitle/shift", data);
}

export async function subtitleSearch(search) {
    return httpPost("subtitle/search", search);
}

export async function subtitleUpload(file, filename) {
    console.info("INFO: Uploading a subtitle file to the server.");
    let subtitle = await httpPostFile("subtitle/upload", file, filename);
    return subtitle;
}

export async function subtitleDownload(url, name, referer) {
    let data = {
        url:  url,
        name: name,
        referer: referer
    };

    return await httpPost("subtitle/download", data);
}

export async function playlistGet() {
    return await httpGet("playlist/get");
}

export async function playlistPlay(entryId) {
    return await httpPost("playlist/play", entryId);
}

export async function playlistAdd(requestEntry) {
    return await httpPost("playlist/add", requestEntry);
}

export async function playlistClear() {
    return await httpPost("playlist/clear");
}

export async function playlistDelete(entryId) {
    return await httpPost("playlist/delete", entryId);
}

export async function playlistShuffle() {
    return await httpPost("playlist/shuffle", null);
}

export async function playlistMove(entryId, dest) {
    const payload = {
        entry_id:   entryId,
        dest_index: dest,
    };

    return await httpPost("playlist/move", payload);
}

export async function playlistUpdate(entry) {
    return await httpPost("playlist/update", entry);
}

export async function historyGet() {
    return await httpGet("history/get");
}

export async function historyClear() {
    return await httpPost("history/clear", null);
}

export async function historyPlay(entryId) {
    return await httpPost("history/play", entryId);
}

export async function historyDelete(entryId) {
    return await httpPost("history/delete", entryId);
}

// CHAT requests

export async function chatSend(messageContent) {
    await httpPost("chat/send", messageContent);
}

export async function chatEdit(message, messageId) {
    const payload = {
        editedMessage: message,
        id: messageId
    };

    await httpPost("chat/edit", payload);
}

export async function chatGet(count, backwardOffset) {
    let data = {
        count: count,
        backwardOffset: backwardOffset,
    };

    return await httpPost("chat/get", data);
}

export async function chatDelete(messageId) {
    return await httpPost("/chat/delete", messageId);
}

export function setWebSocket(ws) {
    websocket = ws;
}

export function closeWebSocket() {
    if (!websocket) {
        return
    }

    websocket.onclose = _ => {};
    websocket.close();
    websocket = null;
}

function wsSendEvent(type, data) {
    if (!websocket) {
        console.error("ERROR: Failed to send WebSocket '" + type + "'. Server connection is closed.");
        return;
    }

    console.info("INFO: Sending WS event '" + type + "' with data:", data);

    const event = {
        type: type,
        data: data,
    };

    let json = JSON.stringify(event);
    websocket.send(json);
}

export function wsPlayerSet(requestedEntry) {
    wsSendEvent(EVENT_PLAYER_SET, requestedEntry);
}

export function wsPlayerPlay(timestamp) {
    wsSendEvent(EVENT_PLAYER_PLAY, timestamp);
}

export function wsPlayerPause(timestamp) {
    wsSendEvent(EVENT_PLAYER_PAUSE, timestamp);
}

export function wsPlayerSeek(timestamp) {
    wsSendEvent(EVENT_PLAYER_SEEK, timestamp);
}

export function wsPlayerNext(currentEntryId) {
    wsSendEvent(EVENT_PLAYER_NEXT, currentEntryId);
}

export function wsPlayerAutoplay(state) {
    wsSendEvent(EVENT_PLAYER_AUTOPLAY, state);
}

export function wsPlayerLooping(state) {
    wsSendEvent(EVENT_PLAYER_LOOPING, state);
}

export function wsPlayerUpdateTitle(title) {
    wsSendEvent(EVENT_PLAYER_UPDATE_TITLE, title);
}

export function wsChatSend(messageContent) {
    wsSendEvent(EVENT_CHAT_SEND, messageContent);
}

export function wsChatEdit(messageId, messageContent) {
    const data = {
        id: messageId,
        editedMessage: messageContent,
    };

    wsSendEvent(EVENT_CHAT_EDIT, data);
}

export function wsChatDelete(messageId) {
    wsSendEvent(EVENT_CHAT_DELETE, messageId);
}

export function wsPlaylistAdd(requestEntry) {
    wsSendEvent(EVENT_PLAYLIST_ADD, requestEntry);
}

export function wsPlaylistMove(entryId, dest) {
    const data = {
        entry_id:   entryId,
        dest_index: dest,
    };

    wsSendEvent(EVENT_PLAYLIST_MOVE, data);
}

export function wsPlaylistClear() {
    wsSendEvent(EVENT_PLAYLIST_CLEAR, null);
}

export function wsPlaylistPlay(entryId) {
    wsSendEvent(EVENT_PLAYLIST_PLAY, entryId);
}

export function wsPlaylistShuffle() {
    wsSendEvent(EVENT_PLAYLIST_SHUFFLE, null);
}

export function wsPlaylistDelete(entryId) {
    wsSendEvent(EVENT_PLAYLIST_DELETE, entryId);
}

export function wsPlaylistUpdate(entry) {
    wsSendEvent(EVENT_PLAYLIST_UPDATE, entry);
}
