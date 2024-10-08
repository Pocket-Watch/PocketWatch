import { Player } from "./player.js"

function blockingHttpGet(endpoint) {
    let request = new XMLHttpRequest();
    request.open("GET", endpoint, false);
    request.send(null);
    return request.responseText;
}

// TODO(kihau): Make this non blocking?
function getServerState() {
    let response = blockingHttpGet("/watch/api/get");
    let state = JSON.parse(response);
    return state;
}

function subscribeToServerEvents(player) {
    let eventSource = new EventSource("/watch/api/events");

    eventSource.addEventListener("play", function(_event) {
        let server = JSON.parse(event.data);
        if (player.isDesynced(server.timestamp)) {
            player.seek(server.timestamp)
        }

        player.play();
    });

    eventSource.addEventListener("pause", function(_event) {
        let server = JSON.parse(event.data);
        if (player.isDesynced(server.timestamp)) {
            player.seek(server.timestamp)
        }

        player.pause();
    });

    eventSource.addEventListener("seek", function(event) { 
        let server = JSON.parse(event.data);
        if (player.isDesynced(server.timestamp)) {
            player.seek(server.timestamp);
        }
    });

    eventSource.addEventListener("seturl", function(event) {
        let server = JSON.parse(event.data)
        console.info("INFO: URL received from the server: ", server.url)
        player.setUrl(server.url);
    });
}

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

function getRandomId() {
    const min = 1;
    const max = 999999999999999;
    const number = Math.floor(Math.random() * (max - min) + min);
    return number.toString();
}

function setUrlButton() {
    var input_url = document.getElementById("input_url");

    let uuid = getRandomId();
    let url = input_url.value;
    input_url.value = ''

    const payload = {
        uuid: uuid,
        url: url,
    };

    httpPostAsync("/watch/api/seturl", payload);
}

function attachHtmlCallbacks() {
    window.setUrlButton = setUrlButton;
}

function main() {
    attachHtmlCallbacks();

    let player = new Player();
    const test_url = "https://download.blender.org/peach/bigbuckbunny_movies/big_buck_bunny_1080p_h264.mov"
    player.createPlayer(test_url)

    // NOTE(kihau): This is blocking and might cause a page lag!
    // let state = getServerState();
    // player.setUrl(state.url);

    // subscribeToServerEvents(player);
}

main();
