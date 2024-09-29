var player = fluidPlayer('player', {
    hls: {
        overrideNative: true
    },
    layoutControls: {
        title: "TITLE PLACEHOLDER",
        doubleclickFullscreen: false,
        subtitlesEnabled: true,
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

video = document.getElementById("player");
vidSource = document.querySelector("source");

input = document.getElementById("user_url");

function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

// endpoint should be prefixed with slash
function newPost(endpoint) {
    if (!endpoint.startsWith("/")) {
        endpoint = "/" + endpoint
    }
    let req = new XMLHttpRequest();
    req.open("POST", endpoint, true);
    req.setRequestHeader('Content-Type', 'application/json');
    return req;
}

async function sendSyncEventAsync(request) {
    request.send(JSON.stringify({
        uuid: "4613443434343",
        timestamp: video.currentTime
    }));
}

async function sendSetAsync(request, url) {
    request.send(JSON.stringify({
        uuid: "4613443434343",
        url: url
    }));
}

function stopButton(fromHtml) {
    let request= newPost("/watch/pause")
    sendSyncEventAsync(request).then(function(res) {
        console.log("Sending stop ", res);
    });
    if (fromHtml) {
        player.pause();
    }
}

function startButton(fromHtml) {
    let request= newPost("/watch/start")
    sendSyncEventAsync(request).then(function(res) {
        console.log("Sending start ", res);
    });
    if (fromHtml) {
        player.play();
    }
}

function setButton() {
    let request= newPost("/watch/set")
    console.log("CURRENT VALUE: ", input.value)
    sendSetAsync(request, input.value).then(function(res) {
        console.log("Sending set ", res);
    });
}

class AtomicBoolean {
    constructor(bool) {
        this.byte = new Int8Array(new SharedArrayBuffer(4));
        Atomics.store(this.byte, 0, bool ? 1 : 0);
    }

    setBoolean(flag) {
        Atomics.store(this.byte, 0, flag ? 1 : 0);
    }

    getBoolean() {
        return Atomics.load(this.byte, 0) === 1;
    }
}

function main() {
    let eventSource = new EventSource("/watch/events");

    let DELTA = 1.5;
    eventSource.addEventListener("start", function (event) {
        let jsonData = JSON.parse(event.data)
        let timestamp = jsonData["timestamp"]
        console.log("Video state: PLAYING, PRIORITY:", jsonData["priority"], "Timestamp:", timestamp);
        let deSync = timestamp - video.currentTime
        console.log("Your deSync: ", deSync)
        if (DELTA < Math.abs(deSync)) {
            console.log("EXCEEDED DELTA=", DELTA, " resyncing!")
            player.skipTo(timestamp)
        }
        player.play()
    })
    eventSource.addEventListener("pause", function (event) {
        let jsonData = JSON.parse(event.data)
        let timestamp = jsonData["timestamp"]
        console.log("Video state: PAUSED, PRIORITY:", jsonData["priority"], "Timestamp:", timestamp);
        let deSync = timestamp - video.currentTime
        console.log("Your deSync: ", deSync)
        if (DELTA < Math.abs(deSync)) {
            console.log("EXCEEDED DELTA=", DELTA, " resyncing!")
            player.skipTo(timestamp)
        }
        player.pause()
    })

    eventSource.addEventListener("set", function (event) {
        let jsonData = JSON.parse(event.data)
        let url = jsonData["url"]
        console.log("RECEIVED SET CHANGING URL:", url)

        player.pause();
        vidSource.src = url;
        let hls = player.hlsInstance()
        if (hls == null) {
            player.instance.hlsPlayer = new Hls();
            hls = player.hlsInstance()
        }
        hls.loadSource(url);
    })

    player.on('play', function() {
        startButton(false)
    });

    player.on('pause', function() {
        stopButton(false)
    });

    player.on('seeked', function(){
        console.log("seeked, currentTime", video.currentTime);
        let request= newPost("/watch/seek")
        sendSyncEventAsync(request).then(function(res) {
            console.log("Sending seek ", res);
        });
    });

    eventSource.onmessage = function(event) {
        console.log("event.data: ", event.lastEventId);
        console.log("event.data: ", event.data);
    };

}

main();
