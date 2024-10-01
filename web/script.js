const DELTA = 1.5;

var server_playing = true;
var server_source_timestamp = false;

var player;
var video;
var vidSource;

var input_hls_url = document.getElementById("input_hls_url");
var input_mp4_url = document.getElementById("input_mp4_url");
var name_field    = document.getElementById("user_name");

// function destroyPlayer() {
//     var container = document.getElementById('player_container');
//     while (container.hasChildNodes()) {
//         container.removeChild(container.lastChild);
//     }
// }

const MEDIA_HLS = "application/x-mpegURL";
const MEDIA_MP4 = "video/mp4";

function createPlayer(url, media_type) {
    let container = document.getElementById('player_container');
    let new_video = document.createElement('video');
    new_video.width = window.innerWidth;
    // new_video.height = window.innerHeight;
    new_video.id = "player";

    let new_source = document.createElement('source');
    new_source.src = url;
    new_source.type = media_type;
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

// endpoint should be prefixed with slash
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

function setHlsButton() {
    let request = httpPost("/watch/api/set/hls")
    console.log("Current video source url: ", input_hls_url.value)
    sendSetAsync(request, input_hls_url.value).then(function(res) {
        console.log("Sending set for this hls file: ", res);
    });
}

function setMp4Button() {
    let request = httpPost("/watch/api/set/mp4")
    console.log("Current video source url: ", input_mp4_url.value)
    sendSetAsync(request, input_mp4_url.value).then(function(res) {
        console.log("Sending set for this mp4 file: ", res);
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
    state.is_hls = jsonData["is_hls"];
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

    eventSource.addEventListener("set/hls", function (event) {
        let jsonData = JSON.parse(event.data)
        let url = jsonData["url"]
        console.log("Hls url received from the server: ", url)

        // NOTE(kihau): Destroying the player might cause a bug when other functions try to access it.
        // destroyPlayer();
        player.destroy();
        createPlayer(url, MEDIA_HLS);

        let state = loadPlayerState();
        server_playing = state.is_playing;
        if (server_playing) {
            player.play();
        } else {
            player.pause();
        }
    })

    eventSource.addEventListener("set/mp4", function (event) {
        let jsonData = JSON.parse(event.data)
        let url = jsonData["url"]
        console.log("Mp4 url received from the server: ", url)

        // NOTE(kihau): Destroying the player might cause a bug when other functions try to access it.
        // destroyPlayer();
        player.destroy();
        createPlayer(url, MEDIA_MP4);

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
        createPlayer("dummy.mp4", MEDIA_MP4);
    } else if (state.is_hls) {
        createPlayer(state.url, MEDIA_HLS);
    } else {
        createPlayer(state.url, MEDIA_MP4);
    }  

    subscribeToServerEvents();
}

main();
