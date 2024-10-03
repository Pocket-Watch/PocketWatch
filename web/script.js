const DELTA = 1.5;

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



var serverPlaying     = false // Updates on welcome-message and event-message
var programmaticPlay  = false // Updates before programmatic play() and in .onplay
var programmaticPause = false // Updates before programmatic pause() and in .onpause
var programmaticSeek  = false // Updates before programmatic currentTime assignment and in .onseeked

var ignoreNextRequest  = false // Updates before sending a sync request and on hasty events

function readEventMaybeResync(type, event) {
    if (ignoreNextRequest) {
        // The next request will always be outdated so we can safely ignore it
        ignoreNextRequest = false;
        return;
    }
    let jsonData = JSON.parse(event.data)
    let timestamp = jsonData["timestamp"]
    let priority = jsonData["priority"]
    let origin = jsonData["origin"]
    console.log(priority, type, "from", origin, "at", timestamp);

    let deSync = timestamp - video.currentTime
    console.log("Your deSync: ", deSync)
    if (type === "seek") {
        programmaticSeek = true;
        player.skipTo(timestamp)
        return
    }

    if (DELTA < Math.abs(deSync)) {
        console.log("EXCEEDED DELTA=", DELTA, " resyncing!")
        programmaticSeek = true;
        player.skipTo(timestamp)
    }
}

function subscribeToServerEvents() {
    let eventSource = new EventSource("/watch/api/events");
    
    // Allow user to de-sync themselves freely and watch at their own pace
    eventSource.addEventListener("play", function (event) {
        readEventMaybeResync("play", event)

        serverPlaying = true
        if (!isVideoPlaying()) {
            programmaticPlay = true
            // this will merely append 'onplay' to the synchronous JS event queue
            // so there's no guarantee that it will be executed next, same logic follows for 'onpause'
            player.play()
        }
    })

    eventSource.addEventListener("pause", function (event) {
        readEventMaybeResync("pause", event)

        serverPlaying = false;
        if (isVideoPlaying()) {
            programmaticPause = true
            player.pause()
        }
    })

    eventSource.addEventListener("seek", function (event) {
        readEventMaybeResync("seek", event)
    });

    eventSource.addEventListener("seturl", function (event) {
        let jsonData = JSON.parse(event.data)
        let url = jsonData["url"]
        console.log("Media url received from the server: ", url)

        // NOTE(kihau): Destroying the player might cause a bug when other functions try to access it.
        player.destroy();
        createPlayer(url);

        let state = loadPlayerState();
        serverPlaying = state.is_playing;
        if (serverPlaying) {
            programmaticPlay = true
            player.play();
        } else {
            programmaticPause = true
            player.pause();
        }
    })
}

function subscribeToPlayerEvents(new_player) {
    new_player.on('seeked', function() {
        if (programmaticSeek) {
            programmaticSeek = false;
            return
        }
        console.log("You seeked to", video.currentTime);
        let request = httpPost("/watch/api/seek")
        ignoreNextRequest = true
        sendSyncEventAsync(request).then(function() {
            console.log("Sending seek!");
        });
    });

    new_player.on('play', function() {
        if (programmaticPlay) {
            programmaticPlay = false;
            return
        }
        // if it's even possible for a user to initiate a play, despite the last server state being a play (event)
        if (serverPlaying) {
            console.log("WARNING: USER TRIGGERED PLAY WHILE SERVER WAS PLAYING!")
            return
        }
        let request = httpPost("/watch/api/play")
        ignoreNextRequest = true
        sendSyncEventAsync(request).then(function() {
            console.log("Sending play!");
        });
        // We cannot make assumptions about the next state of the server because our request will not have any priority
    });

    new_player.on('pause', function() {
        if (programmaticPause) {
            programmaticPause = false;
            return
        }
        // again - in case it's possible for a user to initiate a pause, despite the last server state being a pause
        if (!serverPlaying) {
            console.log("WARNING: USER TRIGGERED PAUSE WHILE SERVER WAS PAUSED AS WELL!")
            return
        }

        let request = httpPost("/watch/api/pause")
        ignoreNextRequest = true
        sendSyncEventAsync(request).then(function() {
            console.log("Sending pause!");
        });
        // This request might not even come through
    });
}

function main() {
    let state = loadPlayerState();
    serverPlaying = state.is_playing;

    if (state.url === "") {
        createPlayer("dummy.mp4");
    } else {
        createPlayer(state.url);
    }

    subscribeToServerEvents();
}

main();
