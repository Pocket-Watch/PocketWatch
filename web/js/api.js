/* Event response constants for (potential) future use.
export const EVENT_PING     = 0;
export const EVENT_USER     = 1;
export const EVENT_PLAYER   = 2;
export const EVENT_PLAYLIST = 3;
export const EVENT_HISTORY  = 4;
export const EVENT_CHAT     = 5;
export const EVENT_SUBTITLE = 6;

export const EVENT_USER_CREATE       = 0;
export const EVENT_USER_DELETE       = 1;
export const EVENT_USER_CONNECTED    = 2;
export const EVENT_USER_DISCONNECTED = 3;
export const EVENT_USER_UPDATE       = 4;

export const EVENT_PLAYER_SET          = 0;
export const EVENT_PLAYER_LOOPING      = 1;
export const EVENT_PLAYER_AUTOPLAY     = 2;
export const EVENT_PLAYER_UPDATE_TITLE = 3;
export const EVENT_PLAYER_WAITING      = 4;
export const EVENT_PLAYER_ERROR        = 5;
export const EVENT_PLAYER_SYNC         = 6;

export const EVENT_PLAYLIST_ADD          = 0;
export const EVENT_PLAYLIST_ADD_TOP      = 1;
export const EVENT_PLAYLIST_ADD_MANY     = 2;
export const EVENT_PLAYLIST_ADD_MANY_TOP = 3;
export const EVENT_PLAYLIST_CLEAR        = 4;
export const EVENT_PLAYLIST_REMOVE       = 5;
export const EVENT_PLAYLIST_SHUFFLE      = 6;
export const EVENT_PLAYLIST_MOVE         = 7;
export const EVENT_PLAYLIST_UPDATE       = 8;

export const EVENT_HISTORY_ADD    = 0;
export const EVENT_HISTORY_CLEAR  = 1;
export const EVENT_HISTORY_DELETE = 2;

export const EVENT_CHAT_MESSAGE_CREATE = 0;
export const EVENT_CHAT_MESSAGE_DELETE = 1;
export const EVENT_CHAT_MESSAGE_EDIT   = 2;

export const EVENT_SUBTITLE_ATTACH = 0;
export const EVENT_SUBTITLE_DELETE = 1;
export const EVENT_SUBTITLE_UPDATE = 2;
export const EVENT_SUBTITLE_SHIFT  = 3;
*/

export const EVENT_PLAYER_PLAY  = 0;
export const EVENT_PLAYER_PAUSE = 1;
export const EVENT_PLAYER_SEEK  = 2;
export const EVENT_PLAYER_SET   = 3;

export const EVENT_CHAT_SEND   = 4;
export const EVENT_CHAT_EDIT   = 5;
export const EVENT_CHAT_DELETE = 6;

export const EVENT_PLAYLIST_ADD     = 7;
export const EVENT_PLAYLIST_PLAY    = 8;
export const EVENT_PLAYLIST_MOVE    = 9;
export const EVENT_PLAYLIST_CLEAR   = 10;
export const EVENT_PLAYLIST_DELETE  = 11;
export const EVENT_PLAYLIST_UPDATE  = 12;
export const EVENT_PLAYLIST_SHUFFLE = 13;

let websocket = null;
let token = null;

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

        return await response.json();
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
    console.info("INFO: Requesting server version.");
    return await httpGet("/watch/api/version");
}

export async function uptime() {
    console.info("INFO: Requesting server uptime.");
    return await httpGet("/watch/api/uptime");
}

export async function uploadMedia(file, filename) {
    console.info("INFO: Uploading a file to the server.");
    let fileUrl = await httpPostFile("/watch/api/uploadmedia", file, filename);
    return fileUrl;
}

export async function uploadMediaWithProgress(file, onprogress) {
    console.info("INFO: Uploading a file to the server (with progress callback).");
    let fileUrl = await httpPostFileWithProgress("/watch/api/uploadmedia", file, onprogress);
    return fileUrl;
}

export async function userCreate() {
    let data = await httpGet("/watch/api/user/create");
    console.info("INFO: Received data from createuser request to the server: ", data);
    return data;
}

export async function userGetAll() {
    let data = await httpGet("/watch/api/user/getall");
    console.info("INFO: Received data from user getall request to the server: ", data);
    return data;
}

export async function userVerify(token) {
    let postVerify = httpPost("/watch/api/user/verify", token);
    return await postVerify;
}

export async function userUpdateName(username) {
    console.info("INFO: Sending update username request.");
    let result = httpPost("/watch/api/user/updatename", username);
	return result
}

export async function userUpdateAvatar(file) {
    console.info("INFO: Uploading avatar file to the server.");
    let avatarUrl = await httpPostFile("/watch/api/user/updateavatar", file);
    return avatarUrl;
}

export async function userDelete(token) {
    console.info("INFO: Requesting user deletion.");
    let result = await httpPost("/watch/api/user/delete", token);
    return result;
}

export async function playerGet() {
    let data = await httpGet("/watch/api/player/get");
    console.info("INFO: Received data from player get request to the server: ", data);
    return data;
}

export async function playerSet(requestEntry) {
    const payload = {
        request_entry: requestEntry,
    };

    console.info("INFO: Sending player set request for a entry");
    return httpPost("/watch/api/player/set", payload);
}

export async function playerNext(currentEntryId) {
    const payload = {
        entry_id: currentEntryId,
    };

    console.info("INFO: Sending player next request with:", payload);
    return await httpPost("/watch/api/player/next", payload);
}

export async function playerPlay(timestamp) {
    const payload = {
        timestamp: timestamp,
    };

    console.info("INFO: Sending player play request to the server.");
    httpPost("/watch/api/player/play", payload);
}

export async function playerPause(timestamp) {
    const payload = {
        timestamp: timestamp,
    };

    console.info("INFO: Sending player pause request to the server.");
    httpPost("/watch/api/player/pause", payload);
}

export async function playerSeek(timestamp) {
    const payload = {
        timestamp: timestamp,
    };

    console.info("INFO: Sending player seek request to the server.");
    httpPost("/watch/api/player/seek", payload);
}

export async function playerAutoplay(state) {
    console.info("INFO: Sending player autoplay request.");
    httpPost("/watch/api/player/autoplay", state);
}

export async function playerLooping(state) {
    console.info("INFO: Sending player autoplay request.");
    httpPost("/watch/api/player/looping", state);
}

export async function playerUpdateTitle(title) {
    console.info("INFO: Sending player title update request.");
    httpPost("/watch/api/player/updatetitle", title);
}

export async function subtitleDelete(subtitleId) {
    console.info("INFO: Sending player subtitle delete request for subtitle", subtitleId);
    httpPost("/watch/api/subtitle/delete", subtitleId);
}

export async function subtitleUpdate(id, name) {
    let data = {
        id:    id,
        name:  name,
    };

    console.info("INFO: Sending player subtitle update request for subtitle", id);
    httpPost("/watch/api/subtitle/update", data);
}

export async function subtitleAttach(subtitle) {
    console.info("INFO: Sending player subtitle attach request for subtitle", subtitle.id);
    httpPost("/watch/api/subtitle/attach", subtitle);
}

export async function subtitleShift(id, shift) {
    console.info("INFO: Sending player subtitle shift request.");
    let data = {
        id:    id,
        shift: shift,
    };
    httpPost("/watch/api/subtitle/shift", data);
}

export async function subtitleSearch(search) {
    console.info("INFO: Requesting server to search for a subtitle.");
    return httpPost("/watch/api/subtitle/search", search);
}

export async function subtitleUpload(file, filename) {
    console.info("INFO: Uploading a subtitle file to the server.");
    let subtitle = await httpPostFile("/watch/api/subtitle/upload", file, filename);
    return subtitle;
}

export async function subtitleDownload(url, name, referer) {
    let data = {
        url:  url,
        name: name,
        referer: referer
    };

    console.info("INFO: Sending subtitle download for url", url);
    return await httpPost("/watch/api/subtitle/download", data);
}

export async function playlistGet() {
    console.info("INFO: Sending playlist get request.");
    return await httpGet("/watch/api/playlist/get");
}

export async function playlistPlay(entryId) {
    const payload = {
        entry_id: entryId,
    };

    console.info("INFO: Sending playlist play request.");
    return await httpPost("/watch/api/playlist/play", payload);
}

export async function playlistAdd(requestEntry) {
    const payload = {
        request_entry: requestEntry,
    };

    console.info("INFO: Sending playlist add request for entry: ", payload);
    return await httpPost("/watch/api/playlist/add", payload);
}

export async function playlistClear() {
    console.info("INFO: Sending playlist clear request.");
    return await httpPost("/watch/api/playlist/clear");
}

export async function playlistDelete(entryId) {
    const payload = {
        entry_id: entryId,
    };

    console.info("INFO: Sending playlist delete request.");
    return await httpPost("/watch/api/playlist/delete", payload);
}

export async function playlistShuffle() {
    console.info("INFO: Sending playlist shuffle request.");
    return await httpPost("/watch/api/playlist/shuffle", null);
}

export async function playlistMove(entryId, dest) {
    const payload = {
        entry_id:   entryId,
        dest_index: dest,
    };

    console.info("INFO: Sending playlist move request with: ", payload);
    return await httpPost("/watch/api/playlist/move", payload);
}

export async function playlistUpdate(entry) {
    const payload = {
        entry: entry,
    };

    console.info("INFO: Sending playlist update request for entry id:", entry);
    return await httpPost("/watch/api/playlist/update", payload);
}

export async function historyGet() {
    console.info("INFO: Sending history get request.");
    return await httpGet("/watch/api/history/get");
}

export async function historyClear() {
    console.info("INFO: Sending history clear request.");
    return await httpPost("/watch/api/history/clear", null);
}

export async function historyPlay(entryId) {
    console.info("INFO: Sending history play request for entry id =", entryId);
    return await httpPost("/watch/api/history/play", entryId);
}

export async function historyDelete(entryId) {
    console.info("INFO: Sending history delete request for entry id =", entryId);
    return await httpPost("/watch/api/history/delete", entryId);
}

// CHAT requests

export async function chatSend(message) {
    const payload = {
        message: message
    };

    console.info("INFO: Sending new chat to the server.");
    httpPost("/watch/api/chat/send", payload);
}

export async function chatEdit(message, messageId) {
    const payload = {
        editedMessage: message,
        id: messageId
    };

    console.info("INFO: Sending a chat edit to the server.");
    httpPost("/watch/api/chat/edit", payload);
}

export async function chatGet(count, backwardOffset) {
    let data = {
        count: count,
        backwardOffset: backwardOffset,
    };
    console.info("INFO: Fetching chat from server.");
    return await httpPost("/watch/api/chat/get", data);
}

export async function chatDelete(messageId) {
    let data = {
        id: messageId
    };
    console.info("INFO: Deleting chat message.");
    return await httpPost("/watch/api/chat/delete", data);
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

    const event = {
        type: type,
        data: data,
    };

    let json = JSON.stringify(event);
    websocket.send(json);
}

export function wsPlayerSet(entry) {
    const data = { request_entry: entry };
    wsSendEvent(EVENT_PLAYER_SET, data);
}

export function wsPlayerPlay(timestamp) {
    const data = { timestamp: timestamp };
    wsSendEvent(EVENT_PLAYER_PLAY, data);
}

export function wsPlayerPause(timestamp) {
    const data = { timestamp: timestamp };
    wsSendEvent(EVENT_PLAYER_PAUSE, data);
}

export function wsPlayerSeek(timestamp) {
    const data = { timestamp: timestamp };
    wsSendEvent(EVENT_PLAYER_SEEK, data);
}

export function wsChatSend(messageText) {
    const data = { message: messageText };
    wsSendEvent(EVENT_CHAT_SEND, data);
}

export function wsChatEdit(messageId, messageContent) {
    const data = {
        id: messageId,
        editedMessage: messageContent,
    };

    wsSendEvent(EVENT_CHAT_EDIT, data);
}

export function wsChatDelete(messageId) {
    const data = { id: messageId };
    wsSendEvent(EVENT_CHAT_DELETE, data);
}

export function wsPlaylistAdd(requestEntry) {
    const data = { request_entry: requestEntry };
    wsSendEvent(EVENT_PLAYLIST_ADD, data);
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
    const data = { entry_id: entryId };
    wsSendEvent(EVENT_PLAYLIST_PLAY, data);
}

export function wsPlaylistShuffle() {
    wsSendEvent(EVENT_PLAYLIST_SHUFFLE, null);
}

export function wsPlaylistDelete(entryId) {
    const data = { entry_id: entryId };
    wsSendEvent(EVENT_PLAYLIST_DELETE, data);
}

export function wsPlaylistUpdate(entry) {
    const data = { entry: entry };
    wsSendEvent(EVENT_PLAYLIST_UPDATE, data);
}
