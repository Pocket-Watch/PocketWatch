import { token, connectionId } from "./main_old.js"

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

export async function userVerify() {
    let data = await httpPost("/watch/api/user/verify");
    console.info("INFO: Received data from user verify request to the server: ", data);
    return data;
}

export async function userUpdateName(username) {
    console.info("INFO: Sending update username request.");
    httpPost("/watch/api/user/updatename", username);
}

export async function playerGet() {
    let data = await httpGet("/watch/api/player/get");
    console.info("INFO: Received data from player get request to the server: ", data);
    return data;
}

export async function playerSet(entry) {
    const payload = {
        connection_id: connectionId,
        entry: entry,
    };

    console.info("INFO: Sending player set request for a entry");
    httpPost("/watch/api/player/set", payload);
}

export async function playerNext(currentEntryId) {
    const payload = {
        connection_id: connectionId,
        entry_id: currentEntryId,
    };

    console.info("INFO: Sending player next request.");
    httpPost("/watch/api/player/next", payload);
}

export async function playerPlay(timestamp) {
    const payload = {
        connection_id: connectionId,
        timestamp: timestamp,
    };

    console.info("INFO: Sending player play request to the server.");
    httpPost("/watch/api/player/play", payload);
}

export async function playerPause(timestamp) {
    const payload = {
        connection_id: connectionId,
        timestamp: timestamp,
    };

    console.info("INFO: Sending player pause request to the server.");
    httpPost("/watch/api/player/pause", payload);
}

export async function playerSeek(timestamp) {
    const payload = {
        connection_id: connectionId,
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

export async function playlistGet() {
    console.info("INFO: Sending playlist get request.");
    return await httpGet("/watch/api/playlist/get");
}

export async function playlistAdd(entry) {
    const payload = {
        connection_id: connectionId,
        entry: entry,
    };

    console.info("INFO: Sending playlist add request for entry: ", payload);
    httpPost("/watch/api/playlist/add", payload);
}

export async function playlistClear() {
    console.info("INFO: Sending playlist clear request.");
    httpPost("/watch/api/playlist/clear", connectionId);
}

export async function playlistRemove(entryId, index) {
    const payload = {
        connection_id: connectionId,
        entry_id: entryId,
        index: index,
    };

    console.info("INFO: Sending playlist remove request.");
    httpPost("/watch/api/playlist/remove", payload);
}

export async function playlistShuffle() {
    console.info("INFO: Sending playlist shuffle request.");
    httpPost("/watch/api/playlist/shuffle", null);
}

export async function playlistMove(entryId, source, dest) {
    const payload = {
        connection_id: connectionId, 
        entry_id: entryId,
        source_index: source,
        dest_index: dest,
    }

    console.info("INFO: Sending playlist move request with: ", payload);
    httpPost("/watch/api/playlist/move", payload);
}

export async function historyGet() {
    console.info("INFO: Sending history get request.");
    return await httpGet("/watch/api/history/get");
}

export async function historyClear() {
    console.info("INFO: Sending history clear request.");
    httpPost("/watch/api/history/clear", null);
}
