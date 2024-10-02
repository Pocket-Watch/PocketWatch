const DELTA = 1.5;

var server_playing = true;
var server_source_timestamp = false;

var player;
var video;
var vidSource;

var input_url  = document.getElementById("input_url");
var name_field = document.getElementById("user_name");

function getUrlMediaType(url) {
    if (url.endsWith(".m3u8")) {
        return "application/x-mpegURL";
    }

    if (url.endsWith(".mp4")) {
        return "video/mp4";
    } 

    if (url.endsWith(".mpeg")) {
        return "video/mpeg";
    }

    if (url.endsWith(".webm")) {
        return "video/webm";
    }

    if (url.endsWith(".ogv")) {
        return "video/ogg";
    }

    return ""
}

function createPlayer(url) {
    let container = document.getElementById('player_container');
    let new_video = document.createElement('video');
    new_video.width = window.innerWidth;
    // new_video.height = window.innerHeight;
    new_video.id = "player";

    let new_source = document.createElement('source');
    new_source.src = url;
    new_source.type = getUrlMediaType(url);
    new_video.appendChild(new_source);

    container.appendChild(new_video);

    let new_player = fluidPlayer('player', {
        hls: {
            overrideNative: true
        },
        layoutControls: {
            title: "TITLE PLACEHOLDER",
            doubleclickFullscreen: true,
            subtitlesEnabled: true,
            autoPlay: true,
            controlBar: {
                autoHide: true,
                autoHideTimeout: 2.5,
                animated: true,
                playbackRates: ['x2', 'x1.5', 'x1', 'x0.5']
            },
            miniPlayer: {
                enabled: false,
                width: 400,
                height: 225
            }
        }
    });

    player    = new_player;
    video     = new_video;
    vidSource = new_source;

    subscribeToPlayerEvents(new_player);
}

function httpPost(endpoint) {
    if (!endpoint.startsWith("/")) {
        endpoint = "/" + endpoint
    }
    let req = new XMLHttpRequest();
    req.open("POST", endpoint, true);
    req.setRequestHeader('Content-Type', 'application/json');
    return req;
}

function blockingHttpGet(endpoint) {
    if (!endpoint.startsWith("/")) {
        endpoint = "/" + endpoint
    }
    let request = new XMLHttpRequest();
    request.open("GET", endpoint, false);
    request.send(null);
    return request.responseText;
}

async function sendSyncEventAsync(request) {
    request.send(JSON.stringify({
        uuid: "4613443434343",
        timestamp: video.currentTime,
        username: name_field.value
    }));
}

async function sendSetAsync(request, url) {
    request.send(JSON.stringify({
        uuid: "4613443434343",
        url: url
    }));
}

function setUrlButton() {
    let request = httpPost("/watch/api/seturl")
    let url = input_url.value;

    console.log("Current video source url: ", url)
    sendSetAsync(request, url).then(function(res) {
        console.log("Sending seturl for a new url");
    });
}

function isVideoPlaying() {
    return video.currentTime > 0 && !video.paused && !video.ended
}

function loadPlayerState() {
    let response = blockingHttpGet("/watch/api/get");
    let jsonData = JSON.parse(response);
    let state = {}
    state.url = jsonData["url"];
    state.timestamp = jsonData["timestamp"];
    state.is_playing = jsonData["is_playing"];

    console.log("Received get request from the server. The state is:");
    console.log(state);

    return state;
}

function subscribeToServerEvents() {
    let eventSource = new EventSource("/watch/api/events");

    // Allow user to de-sync themselves freely and watch at their own pace
    eventSource.addEventListener("start", function (event) {
        let jsonData = JSON.parse(event.data)
        let timestamp = jsonData["timestamp"]
        console.log("EVENT: PLAYING, " +
            jsonData["priority"],
            "from", jsonData["origin"],
            "at", timestamp,
        );

        let deSync = timestamp - video.currentTime
        console.log("Your deSync: ", deSync)
        if (DELTA < Math.abs(deSync)) {
            console.log("EXCEEDED DELTA=", DELTA, " resyncing!")
            player.skipTo(timestamp)
        }

        server_playing = true;
        if (!isVideoPlaying()) {
            player.play()
        }
    })

    eventSource.addEventListener("pause", function (event) {
        let jsonData = JSON.parse(event.data)
        let timestamp = jsonData["timestamp"]
        console.log("EVENT: PAUSED, " +
            jsonData["priority"],
            "from", jsonData["origin"],
            "at", timestamp,
        );

        let deSync = timestamp - video.currentTime
        console.log("Your deSync: ", deSync)
        if (DELTA < Math.abs(deSync)) {
            console.log("EXCEEDED DELTA=", DELTA, " resyncing!")
            player.skipTo(timestamp)
        }

        server_playing = false;
        if (isVideoPlaying()) {
            player.pause()
        }
    })

    eventSource.addEventListener("seturl", function (event) {
        let jsonData = JSON.parse(event.data)
        let url = jsonData["url"]
        console.log("Media url received from the server: ", url)

        // NOTE(kihau): Destroying the player might cause a bug when other functions try to access it.
        player.destroy();
        createPlayer(url);

        let state = loadPlayerState();
        server_playing = state.is_playing;
        if (server_playing) {
            player.play();
        } else {
            player.pause();
        }
    })
}

function subscribeToPlayerEvents(new_player) {
    new_player.on('seeked', function() {
        console.log("seeked, currentTime", video.currentTime);
        let request = httpPost("/watch/api/seek")
        sendSyncEventAsync(request).then(function(res) {
            console.log("Sending seek ", res);
        });
    });

    new_player.on('play', function() {
        if (!server_playing) {
            let request = httpPost("/watch/api/start")
            sendSyncEventAsync(request).then(function(res) {
                console.log("Sending start");
            });
            server_playing = true;
        }
    });

    new_player.on('pause', function() {
        if (server_playing) {
            let request = httpPost("/watch/api/pause")
            sendSyncEventAsync(request).then(function(res) {
                console.log("Sending pause");
            });
            server_playing = false;
        }
    });
}

function main() {
    let state = loadPlayerState();
    server_playing = state.is_playing;

    if (state.url === "") {
        createPlayer("dummy.mp4");
    } else {
        createPlayer(state.url);
    }

    subscribeToServerEvents();
}

main();
